package main

import (
	"fmt"
	"go/build"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func exists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func isDir(filename string) bool {
	fi, err := os.Stat(filename)
	return err == nil && fi.IsDir()
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

func isPackageLevel(obj types.Object) bool {
	return obj.Pkg().Scope().Lookup(obj.Name()) == obj
}

// isLocal reports whether obj is local to some function.
// Precondition: not a struct field or interface method.
func isLocal(obj types.Object) bool {
	// [... 5=stmt 4=func 3=file 2=pkg 1=universe]
	var depth int
	for scope := obj.Parent(); scope != nil; scope = scope.Parent() {
		depth++
	}
	return depth >= 4
}

func objectKind(obj types.Object) string {
	switch obj := obj.(type) {
	case *types.PkgName:
		return "imported package name"
	case *types.TypeName:
		return "type"
	case *types.Var:
		if obj.IsField() {
			return "field"
		}
	case *types.Func:
		if obj.Type().(*types.Signature).Recv() != nil {
			return "method"
		}
	}
	// label, func, var, const
	return strings.ToLower(strings.TrimPrefix(reflect.TypeOf(obj).String(), "*types."))
}
