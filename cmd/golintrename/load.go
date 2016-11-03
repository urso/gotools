package main

import (
	"fmt"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"strings"

	"golang.org/x/tools/go/loader"
)

func loadProgram(
	fset *token.FileSet,
	ctx *build.Context,
	packages map[string]bool,
) (*loader.Program, error) {
	// import all packages
	conf := &loader.Config{
		Fset:        fset,
		Build:       ctx,
		ParserMode:  parser.ParseComments,
		AllowErrors: false,
		TypeCheckFuncBodies: func(path string) bool {
			return packages[path] || packages[strings.TrimSuffix(path, "_test")]
		},
	}

	for pkg := range packages {
		if verbose {
			log.Println("load package: ", pkg)
		}
		conf.ImportWithTests(pkg)
	}

	if verbose {
		log.Println("Do Load and check")
	}
	conf.AllowErrors = true
	return doLoadProgram(conf)
}

func doLoadProgram(conf *loader.Config) (*loader.Program, error) {
	allowErrors := conf.AllowErrors
	defer func() {
		conf.AllowErrors = allowErrors
	}()

	// Ideally we would just return conf.Load() here, but go/types
	// reports certain "soft" errors that gc does not (Go issue 14596).
	// As a workaround, we set AllowErrors=true and then duplicate
	// the loader's error checking but allow soft errors.
	// It would be nice if the loader API permitted "AllowErrors: soft".
	conf.AllowErrors = true
	prog, err := conf.Load()
	if err != nil {
		return nil, err
	}

	var errpkgs []string
	// Report hard errors in indirectly imported packages.
	for _, info := range prog.AllPackages {
		if containsHardErrors(info.Errors) {
			errpkgs = append(errpkgs, info.Pkg.Path())
		}
	}

	if errpkgs != nil {
		var more string
		if len(errpkgs) > 3 {
			more = fmt.Sprintf(" and %d more", len(errpkgs)-3)
			errpkgs = errpkgs[:3]
		}
		err := fmt.Errorf("couldn't load packages due to errors: %s%s",
			strings.Join(errpkgs, ", "), more)
		return nil, err
	}

	return prog, nil
}

func containsHardErrors(errors []error) bool {
	for _, err := range errors {
		if err, ok := err.(types.Error); ok && err.Soft {
			continue
		}
		return true
	}
	return false
}
