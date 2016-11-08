package main

import (
	"fmt"
	"go/ast"
	"strings"

	"github.com/urso/gotools/ana"
	"github.com/urso/gotools/filespec"
	"golang.org/x/tools/go/loader"
)

func collectExports(
	prog *loader.Program,
	files []filespec.FileInfo,
) map[*loader.PackageInfo][]exports {
	pkgs := map[*loader.PackageInfo][]exports{}
	for _, file := range files {
		results := collectFileExports(prog, file)
		if len(results) == 0 {
			continue
		}

		pkgs[file.Package] = append(pkgs[file.Package], results...)
	}
	return pkgs
}

func collectFileExports(
	prog *loader.Program,
	file filespec.FileInfo,
) []exports {
	var es []exports

	isTest := strings.HasSuffix(file.Path, "_test.go")

	ast.Walk(makeExportsVisitor(func(id *ast.Ident, n ast.Node) {
		fn, isFunc := n.(*ast.FuncDecl)
		if isTest && isTestName(id.Name) && isFunc && fn.Recv == nil {
			return
		}

		objs, err := ana.CollectIdentObjects(prog, file.Package, id)
		if err == nil {
			es = append(es, exports{file: file, ident: id, objs: objs, scope: n})
		}
	}), file.File)
	return es
}

func allImporters(
	prog *loader.Program,
	pkg *loader.PackageInfo,
) (importers []*loader.PackageInfo) {
	for _, other := range prog.Imported {
		if importsPackage(other, pkg) {
			importers = append(importers, other)
		}
	}
	return
}

func importsPackage(pkg, imports *loader.PackageInfo) bool {
	for _, i := range pkg.Pkg.Imports() {
		if i == imports.Pkg {
			return true
		}
	}
	return false
}

func usesExport(info *loader.PackageInfo, e exports) bool {
	for _, obj := range e.objs {
		for id, other := range info.Uses {
			if other == nil || other.Pkg() == nil {
				continue
			}

			if obj == other {
				if verbose {
					fmt.Printf("(object check) package %v uses %v(=%v) (package %v)\n",
						info.Pkg.Name(),
						e.ident.Name,
						id.Name,
						e.file.Package.Pkg.Name(),
					)
				}

				return true
			}

			samePackage := obj.Pkg().Path() == other.Pkg().Path()
			sameName := obj.Name() == other.Name()
			if samePackage && sameName {
				if verbose {
					fmt.Printf("(name check) package %v uses %v(=%v) (package %v)\n",
						info.Pkg.Name(),
						e.ident.Name,
						id.Name,
						e.file.Package.Pkg.Name(),
					)
				}
				return true
			}
		}
	}

	return false
}

// Filter unused type names if types are indirectly exported but not used by name
func filterIndirectExports(used, unused []exports) []exports {
	res := unused[:0]
	for _, u := range unused {
		// only type might be indirectly exported
		t, isType := u.scope.(*ast.TypeSpec)
		if !isType {
			res = append(res, u)
			continue
		}

		// check type is indirectly exported
		isUsed := false
		for _, other := range used {
			switch v := other.scope.(type) {
			case *ast.FuncDecl:
				// check function parameters and results

				if v.Type.Params != nil {
					for _, f := range v.Type.Params.List {
						if isFieldType(f, t) {
							isUsed = true
							break
						}
					}
				}
				if !isUsed && v.Type.Results != nil {
					for _, f := range v.Type.Results.List {
						if isFieldType(f, t) {
							isUsed = true
							break
						}
					}
				}
			case *ast.Field:
				// check used and exported structure fields
				isUsed = isFieldType(v, t)
			case *ast.TypeSpec:
				// TODO: we need to check type definitions?
			case ast.Expr:
				// TODO: we need to check explicit type expressions?
			}

			if isUsed {
				break
			}
		}

		if !isUsed {
			res = append(res, u)
		}
	}

	return res
}

func isTestName(t string) bool {
	for _, prefix := range []string{"Example", "Test", "Benchmark"} {
		if strings.HasPrefix(t, prefix) {
			return true
		}
	}
	return false
}
