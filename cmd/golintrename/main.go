package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/token"
	"log"
	"os"
	"strings"

	"github.com/urso/gotools/ana"
	"github.com/urso/gotools/filespec"
	"github.com/urso/gotools/names"
	"github.com/urso/gotools/renamer"
	"github.com/urso/gotools/write"

	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/refactor/importgraph"
)

type fileInfo struct {
	pkg  *loader.PackageInfo
	path string
	file *ast.File
}

type correction struct {
	file   filespec.FileInfo
	ident  *ast.Ident
	should string
	thing  string
	pos    token.Position
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

func doMain() int {
	diff := flag.Bool("d", false, "Display diff instead of rewriting")
	diffCmd := flag.String("diff", "diff", "Diff command")
	initials := flag.String("i", "", "additional initialisms")
	verboseLogging := flag.Bool("v", false, "verbose")

	flag.Usage = usage
	flag.Parse()

	verbose = *verboseLogging

	writer, err := write.CreateWriter(*diff, *diffCmd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

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

	// analyze all given files for naming errors
	initialisms := names.NewInitials(*initials)
	files := spec.CollectFiles(prog)
	names := analyzeAllNames(fset, files, initialisms)

	// check for exports and reload program + names if necessary
	if requiresGlobal(names) {
		if verbose {
			log.Print("Potentially global renaming; scanning workspace...")
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

		// Enumerate the set of potentially affected packages.
		affectedPackages := map[string]bool{}
		for pkg := range spec.Packages {
			for path := range rev.Search(pkg) {
				affectedPackages[path] = true
			}
		}

		// reload the larger program
		prog, err = loadProgram(fset, ctx, affectedPackages)
		if err != nil {
			log.Println(err)
			return 1
		}

		// re-analyze renamings symbols from larger corpus
		files = spec.CollectFiles(prog)
		names = analyzeAllNames(fset, files, initialisms)
	}

	// print all found symbols for testing
	if verbose {
		for pkg, files := range names {
			log.Println("package: ", pkg)
			for filename, cs := range files {
				log.Println("  file:", filename)
				for _, c := range cs {
					exported := "exported"
					if !c.ident.IsExported() {
						exported = ""
					}
					log.Printf("    %v should rename %v %v %v to %v\n",
						c.pos, exported, c.thing, c.ident.Name, c.should)
					log.Println(c.ident.Obj)
				}
			}
		}
	}

	packages := make([]*loader.PackageInfo, 0, len(prog.Imported)+len(prog.Created))
	for _, info := range prog.Imported {
		packages = append(packages, info)
	}
	for _, info := range prog.Created {
		packages = append(packages, info)
	}

	// start renaming symbols
	updatedFiles := map[*token.File]bool{}
	for pkg, files := range names {
		if verbose {
			log.Println("process package: ", pkg)
		}
		for filename, cs := range files {
			if verbose {
				log.Println("  process file:", filename)
			}
			for _, c := range cs {
				if verbose {
					log.Printf("process %v -> %v\n", c.ident.Name, c.should)
				}

				objs, err := ana.CollectIdentObjects(prog, c.file.Package, c.ident)
				if err != nil {
					fmt.Println(err)
					return 1
				}

				r := renamer.New(prog, c.should)
				r.AddAllPackages(packages...)

				files, err := r.Update(objs...)
				if err != nil {
					fmt.Fprintln(os.Stderr, "renaming failed with: ", err)
					return 1
				}
				for file := range files {
					if verbose {
						log.Println("updated: ", file.Name())
					}
					updatedFiles[file] = true
				}
			}
		}
	}

	// serialize changes for all files changed into buffers
	changed := map[string][]byte{}
	// write changed files to stdout
	for _, info := range packages {
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

	// write changed files
	for file, buf := range changed {
		if verbose {
			log.Println("update file: ", file)
		}
		writer.Write(file, buf)
	}

	return 0
}

func requiresGlobal(names map[string]map[string][]correction) bool {
	for _, files := range names {
		for _, cs := range files {
			for _, c := range cs {
				if c.ident.IsExported() {
					return true
				}
			}
		}
	}
	return false
}

func analyzeAllNames(
	fset *token.FileSet,
	files []filespec.FileInfo,
	initialisms *names.Initials,
) map[string]map[string][]correction {
	pkgs := map[string]map[string][]correction{}
	for _, file := range files {
		results := analyzeNames(fset, file, initialisms)
		if len(results) == 0 {
			continue
		}

		pkgName := file.Package.Pkg.Path()
		M := pkgs[pkgName]
		if M == nil {
			M = map[string][]correction{}
			pkgs[pkgName] = M
		}
		M[file.Path] = results
	}

	return pkgs
}

func analyzeNames(
	fset *token.FileSet,
	file filespec.FileInfo,
	initialisms *names.Initials,
) []correction {
	path := file.Path
	isTest := strings.HasSuffix(path, "_test.go")

	corrections := []correction{}
	iterNameDecls(isTest, file.File, func(id *ast.Ident, thing string) {
		name := id.Name
		should := lintName(name, initialisms)
		if name != should {
			corrections = append(corrections, correction{
				file:   file,
				ident:  id,
				should: should,
				thing:  thing,
				pos:    fset.Position(id.NamePos),
			})
		}
	})
	return corrections
}
