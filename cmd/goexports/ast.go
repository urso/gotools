package main

import (
	"go/ast"
	"go/token"
)

func makeExportsVisitor(cb func(id *ast.Ident, n ast.Node)) ast.Visitor {
	return fileVisitor(visitFn(func(n ast.Node) ast.Visitor {
		switch v := n.(type) {
		case *ast.FuncDecl:
			if !v.Name.IsExported() {
				return nil
			}

			// handle functions
			if v.Recv == nil {
				cb(v.Name, v)
				return nil
			}

			// check receiver type is exported
			return visitFn(func(n ast.Node) ast.Visitor {
				if l, ok := n.(*ast.FieldList); ok && l.NumFields() == 1 {
					ast.Walk(
						ignoreStars(exportedIdentVisitor(func(_ *ast.Ident) {
							cb(v.Name, v)
						}).Visit),
						l.List[0].Type)
				}

				return nil
			})

		case *ast.GenDecl:
			if v.Tok != token.IMPORT {
				return visitFn(func(n ast.Node) ast.Visitor {
					switch v := n.(type) {
					case *ast.TypeSpec:
						if v.Name != nil && v.Name.IsExported() {
							cb(v.Name, v)
						}
						visitTypeExports(cb, v.Type)

					case *ast.ValueSpec:
						return exportedIdentVisitor(func(id *ast.Ident) {
							cb(id, v.Type)
						})
					}

					return nil
				})
			}
		}

		return nil
	}))
}

func visitTypeExports(cb func(*ast.Ident, ast.Node), t ast.Expr) {
	switch v := t.(type) {
	case *ast.ArrayType:
		visitTypeExports(cb, v.Elt)

	case *ast.ChanType:
		visitTypeExports(cb, v.Value)

	case *ast.MapType:
		visitTypeExports(cb, v.Value)
		visitTypeExports(cb, v.Key)

	case *ast.StructType:
		if v.Fields == nil || v.Fields.NumFields() == 0 {
			break
		}

		for _, field := range v.Fields.List {
			exported := len(field.Names) == 0
			for _, name := range field.Names {
				if name.IsExported() {
					exported = true
					cb(name, field)
				}
			}

			if exported {
				visitTypeExports(cb, field.Type)
			}
		}

	case *ast.InterfaceType:
		if v.Methods == nil || v.Methods.NumFields() == 0 {
			break
		}
		for _, field := range v.Methods.List {
			for _, name := range field.Names {
				if name.IsExported() {
					cb(name, field)
				}
			}
		}
	}
}

func fileVisitor(cont ast.Visitor) ast.Visitor {
	return visitFn(func(n ast.Node) ast.Visitor {
		if _, isFile := n.(*ast.File); !isFile {
			return nil
		}

		return cont
	})
}

func identVisitor(fn func(*ast.Ident)) ast.Visitor {
	return visitFn(func(n ast.Node) ast.Visitor {
		if id, isIdent := n.(*ast.Ident); isIdent {
			fn(id)
		}
		return nil
	})
}

func exportedIdentVisitor(fn func(*ast.Ident)) ast.Visitor {
	return identVisitor(func(id *ast.Ident) {
		if id.IsExported() {
			fn(id)
		}
	})
}

func ignoreStars(fn func(ast.Node) ast.Visitor) ast.Visitor {
	return visitFn(func(n ast.Node) ast.Visitor {
		if _, isStar := n.(*ast.StarExpr); isStar {
			return ignoreStars(fn)
		}
		return fn(n)
	})
}

type visitFn func(ast.Node) ast.Visitor

func (f visitFn) Visit(n ast.Node) ast.Visitor { return f(n) }

func isFieldType(f *ast.Field, t *ast.TypeSpec) bool {
	if t.Name.Obj == nil {
		return f.Type == t.Type
	}

	matches := false
	ast.Walk(ignoreStars(func(n ast.Node) ast.Visitor {
		if id, isIdent := n.(*ast.Ident); isIdent && id.Obj != nil {
			matches = t.Name.Obj == id.Obj
		}
		return nil
	}), f.Type)
	return matches || f.Type == t.Type
}
