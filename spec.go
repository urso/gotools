package main

import (
	"fmt"
	"go/build"
	"path/filepath"
	"strings"
)

type spec struct {
	files    map[string][]string
	packages map[string]bool
}

func createLoadSpecs(ctx *build.Context, args []string) (*spec, error) {
	files := map[string]map[string]bool{}
	packages := map[string]bool{}

	// collect files and packages to and type check from list of unnamed args
	for _, arg := range args {
		switch {
		case strings.HasSuffix(arg, "./...") && isDir(arg[:len(arg)-4]):
			for _, dirname := range allPackagesInFS(arg) {
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
			for _, rel := range importPaths(arg) {
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

	return &spec{filtered, packages}, nil
}
