package filespec

import (
	"fmt"
	"go/ast"
	"go/build"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/loader"
)

type Spec struct {
	Files    map[string][]string
	Packages map[string]bool
}

func New(ctx *build.Context, args []string) (*Spec, error) {
	files := map[string]map[string]bool{}
	packages := map[string]bool{}

	// collect files and packages to and type check from list of unnamed args
	for _, arg := range args {
		switch {
		case strings.HasSuffix(arg, "./...") && isDir(arg[:len(arg)-4]):
			for _, dirname := range allPackagesInFS(ctx, arg) {
				pkgname, err := dirPkgName(ctx, dirname)
				if err != nil {
					return nil, err
				}
				packages[pkgname] = true
			}

		case isDir(arg):
			pkgname, err := dirPkgName(ctx, arg)
			if err != nil {
				return nil, err
			}
			packages[pkgname] = true

		case exists(arg):
			path, err := filepath.Abs(arg)
			if err != nil {
				return nil, err
			}

			dir := filepath.Dir(path)
			pkgname, err := dirPkgName(ctx, dir)
			if err != nil {
				return nil, err
			}

			M := files[pkgname]
			if M == nil {
				M = map[string]bool{}
				files[pkgname] = M
			}
			M[path] = true

		default:
			for _, rel := range importPaths(ctx, arg) {
				abs, err := filepath.Abs(rel)
				if err != nil {
					return nil, err
				}

				GOSRC := ctx.GOPATH + "/src/"
				if !strings.HasPrefix(abs, GOSRC) {
					return nil, fmt.Errorf("package '%v' no in $GOPATH", rel)
				}

				pkgname := abs[len(GOSRC):]
				packages[pkgname] = true
			}
		}
	}

	for pkg := range files {
		packages[pkg] = true
	}

	filtered := map[string][]string{}
	for pkg, fs := range files {
		if len(fs) == 0 {
			continue
		}

		files := make([]string, 0, len(fs))
		for name := range fs {
			files = append(files, name)
		}
		filtered[pkg] = files
	}

	return &Spec{filtered, packages}, nil
}

func (s *Spec) IterFiles(
	prog *loader.Program,
	fn func(*loader.PackageInfo, *ast.File) error,
) error {
	fset := prog.Fset
	for pkg, info := range prog.AllPackages {
		name := pkg.Path()
		if !s.Packages[name] {
			continue
		}

		filter := createFilter(s.Files[name])
		for _, file := range info.Files {
			path := fset.File(file.Name.NamePos).Name()
			if !filter(path) {
				continue
			}

			if err := fn(info, file); err != nil {
				return err
			}
		}
	}
	return nil
}

func createFilter(names []string) func(string) bool {
	if len(names) == 0 {
		return func(_ string) bool {
			return true
		}
	}
	return func(path string) bool {
		for _, other := range names {
			if path == other {
				return true
			}
		}
		return false
	}
}

func isDir(filename string) bool {
	fi, err := os.Stat(filename)
	return err == nil && fi.IsDir()
}

func exists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func dirPkgName(ctx *build.Context, path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	GOSRC := ctx.GOPATH + "/src/"
	if !strings.HasPrefix(abs, GOSRC) {
		return "", fmt.Errorf("package '%v' no in $GOPATH", path)
	}

	pkgname := abs[len(GOSRC):]
	return pkgname, nil
}
