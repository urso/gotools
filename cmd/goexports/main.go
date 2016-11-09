package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/token"
	"go/types"
	"log"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/refactor/importgraph"

	"github.com/urso/gotools/filespec"
	"github.com/urso/gotools/names"
	"github.com/urso/gotools/renamer"
	"github.com/urso/gotools/write"
)

type exports struct {
	file  filespec.FileInfo
	ident *ast.Ident
	scope ast.Node
	objs  []types.Object
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\t  [flags] # runs on package in current directory\n")
	fmt.Fprintf(os.Stderr, "\t  [flags] package\n")
	fmt.Fprintf(os.Stderr, "\t  [flags] directory\n")
	fmt.Fprintf(os.Stderr, "\t  [flags] files... # must be a single package\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	flag.PrintDefaults()
}

func main() {
	rc := doMain()
	os.Exit(rc)
}

var verbose = false

func doMain() (rc int) {
	diff := flag.Bool("d", false, "Display diff instead of rewriting")
	diffCmd := flag.String("diff", "diff", "Diff command")
	lintOnly := flag.Bool("l", false, "Lint mode")
	ignoreConflicts := flag.Bool("c", false, "ignore conflicts (do not rename)")
	verboseLogging := flag.Bool("v", false, "verbose")
	initials := flag.String("initials", "", "Name Initialisms")
	filter := registerFilterFlag("i", "e", " names regular expression")

	flag.Usage = usage
	flag.Parse()

	verbose = *verboseLogging
	args := flag.Args()
	if len(args) == 0 {
		args = []string{"."}
	}

	fset := token.NewFileSet()
	ctx := &build.Default
	spec, err := filespec.New(ctx, args)
	if err != nil {
		log.Println(err)
		return 1
	}

	prog, err := loadProgram(fset, ctx, spec.Packages)
	if err != nil {
		log.Println(err)
		return 1
	}

	// Scan the workspace and build the import graph.
	_, rev, errors := importgraph.Build(ctx)
	if len(errors) > 0 {
		// With a large GOPATH tree, errors are inevitable.
		// Report them but proceed.
		fmt.Fprintf(os.Stderr, "While scanning Go workspace:\n")
		for path, err := range errors {
			fmt.Fprintf(os.Stderr, "Package %q: %s.\n", path, err)
		}
	}

	// Enumerate the set of packages potentially using exported symbols
	packages := map[string]bool{}
	for pkg := range spec.Packages {
		for path := range rev.Search(pkg) {
			packages[path] = true
		}
	}

	// reload the larger program
	prog, err = loadProgram(fset, ctx, packages)

	// collect exported symbols
	files := spec.CollectFiles(prog)
	allExported := collectExports(prog, files, filter.report)
	if len(allExported) == 0 {
		fmt.Println("no exports found")
		return
	}

	// filter out all unused exported symbols
	unusedExports := map[*loader.PackageInfo][]exports{}
	if verbose {
		log.Println("filter exported symbols")
	}
	for pkg, es := range allExported {
		if verbose {
			log.Println("process package: ", pkg.Pkg.Name())
		}

		// for every package importing pkg check if any exported symbols are used
		importers := allImporters(prog, pkg)
		if len(importers) == 0 {
			unusedExports[pkg] = es
			continue
		}

		used := make([]bool, len(es))
		count := 0
		for _, importer := range importers {
			if verbose {
				fmt.Println("check importer using symbols: ", importer.Pkg.Path())
			}

			for i, e := range es {
				if used[i] {
					continue
				}

				uses := usesExport(importer, e)
				used[i] = uses
				if uses {
					count++
				}
			}
		}

		if count == 0 {
			unusedExports[pkg] = es
			continue
		}
		if count == len(es) {
			// all symbols being used
			continue
		}

		// if subset of exported symbols is not used,
		// check if symbols are indirectly used due to type inference.
		// e.g. an exported function should not return a unexported symbol
		pkgUsed := make([]exports, 0, len(es)-count)
		pkgUnused := make([]exports, 0, count)
		for i, u := range used {
			if !u {
				pkgUnused = append(pkgUnused, es[i])
			} else {
				pkgUsed = append(pkgUsed, es[i])
			}
		}
		unusedExports[pkg] = filterIndirectExports(pkgUsed, pkgUnused)
	}

	// Print results if verbose or lint mode is enabled
	// If lint mode is enabled, stop processing here
	if verbose || *lintOnly {
		if len(unusedExports) > 0 {
			fmt.Println("Unused exports")
		}

		for pkg, es := range unusedExports {
			fmt.Println("package: ", pkg.Pkg.Name())
			for _, e := range es {
				position := fset.Position(e.ident.Pos())
				fmt.Printf("    unused export at %v: %v\n", position, e.ident.String())
			}
		}

		if *lintOnly {
			if len(unusedExports) > 0 {
				return 1
			}
			return 0
		}
	}

	// start renaming symbols in memory
	if verbose {
		fmt.Println("try renaming unused exports")
	}

	initialisms := names.NewInitials(*initials)
	updatedFiles := map[*token.File]bool{}
	for pkg, es := range unusedExports {
		if verbose {
			fmt.Println("process package: ", pkg.Pkg.Name())
		}

		for _, e := range es {
			r := renamer.New(prog, unexportedName(e.ident.Name, initialisms))
			r.AddAllPackages(prog.InitialPackages()...)

			files, err := r.Update(e.objs...)
			if err != nil {
				fmt.Fprintln(os.Stderr, "renaming failed with: ", err)
				if !(*ignoreConflicts) {
					return 1
				}
				fmt.Fprintln(os.Stderr, "ignore renaming conflict")
			}

			for file := range files {

				if verbose {
					log.Println("updated: ", file.Name())
				}
				updatedFiles[file] = true
			}
		}
	}

	// serialize changes for all files changed into buffers
	changed := map[string][]byte{}
	// write changed files to stdout
	for _, info := range prog.InitialPackages() {
		for _, f := range info.Files {
			tokenFile := prog.Fset.File(f.Pos())
			if !updatedFiles[tokenFile] {
				continue
			}

			var buf bytes.Buffer
			err := format.Node(&buf, prog.Fset, f)
			if err != nil {
				log.Printf("failed to pretty-print syntax tree: %v", err)
				return 1
			}

			changed[tokenFile.Name()] = buf.Bytes()
		}
	}

	// update files
	writer, err := write.CreateWriter(*diff, *diffCmd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for file, buf := range changed {
		if verbose {
			log.Println("update file: ", file)
		}
		writer.Write(file, buf)
	}

	return
}

func unexportedName(n string, initialisms *names.Initials) string {
	if n == "" {
		return ""
	}

	if i := initialisms.StartsWith(n); i != "" {
		return strings.ToLower(i) + n[len(i):]
	}

	r, _ := utf8.DecodeRuneInString(n)
	return string(unicode.ToLower(r)) + n[1:]
}
