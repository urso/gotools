package main

import (
	"go/ast"
	"go/token"
)

// walker adapts a function to satisfy the ast.Visitor interface.
// The function return whether the walk should proceed into the node's children.
type walker func(ast.Node) bool

func (w walker) Visit(node ast.Node) ast.Visitor {
	if w(node) {
		return w
	}
	return nil
}

func walkAll(fn func(ast.Node)) walker {
	return walker(func(n ast.Node) bool {
		fn(n)
		return true
	})
}

func walkAST(f *ast.File, v ast.Visitor) {
	ast.Walk(v, f)
}

func iterNameDecls(isTest bool, f *ast.File, check func(id *ast.Ident, thing string)) {
	checkList := func(l *ast.FieldList, thing string) {
		if l == nil {
			return
		}
		for _, f := range l.List {
			for _, id := range f.Names {
				check(id, thing)
			}
		}
	}

	walkAST(f, walkAll(func(node ast.Node) {
		switch v := node.(type) {
		case *ast.AssignStmt:
			if v.Tok != token.ASSIGN {
				for _, exp := range v.Lhs {
					if id, ok := exp.(*ast.Ident); ok {
						check(id, "var")
					}
				}
			}

		case *ast.FuncDecl:
			name := v.Name.Name
			if isTest && isTestName(name) {
				return
			}

			thing := "func"
			if v.Recv != nil {
				thing = "method"
			}
			check(v.Name, thing)

			checkList(v.Type.Params, thing+" parameter")
			checkList(v.Type.Results, thing+" result")

		case *ast.GenDecl:
			if v.Tok != token.IMPORT {
				var thing string
				switch v.Tok {
				case token.CONST:
					thing = "const"
				case token.TYPE:
					thing = "type"
				case token.VAR:
					thing = "var"
				}

				for _, spec := range v.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						check(s.Name, thing)
					case *ast.ValueSpec:
						for _, id := range s.Names {
							check(id, thing)
						}
					}
				}
			}

		case *ast.InterfaceType:
			// Do not check interface method names.
			// They are often constrainted by the method names of concrete types.
			for _, x := range v.Methods.List {
				ft, ok := x.Type.(*ast.FuncType)
				if !ok { // might be an embedded interface name
					continue
				}
				checkList(ft.Params, "interface method parameter")
				checkList(ft.Results, "interface method result")
			}

		case *ast.RangeStmt:
			if v.Tok != token.ASSIGN {
				if id, ok := v.Key.(*ast.Ident); ok {
					check(id, "range var")
				}
				if id, ok := v.Value.(*ast.Ident); ok {
					check(id, "range var")
				}
			}

		case *ast.StructType:
			for _, f := range v.Fields.List {
				for _, id := range f.Names {
					check(id, "struct field")
				}
			}
		}
	}))
}
