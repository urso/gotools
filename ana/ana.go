package ana

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/loader"
)

func CollectIdentObjects(
	prog *loader.Program,
	info *loader.PackageInfo,
	id *ast.Ident,
) ([]types.Object, error) {
	obj := info.Uses[id]
	if obj == nil {
		obj = info.Defs[id]
		if obj == nil {
			pos := id.Pos()

			// Ident without Object.

			// Package clause?
			_, path, _ := prog.PathEnclosingInterval(pos, pos)
			if len(path) == 2 { // [Ident File]
				// TODO(adonovan): support this case.
				return nil, fmt.Errorf("cannot rename %q: renaming package clauses is not yet supported",
					id)
			}

			// Implicit y in "switch y := x.(type) {"?
			if obj := typeSwitchVar(&info.Info, path); obj != nil {
				return []types.Object{obj}, nil
			}

			// Probably a type error.
			return nil, fmt.Errorf("cannot find object for %q", id.Name)
		}
	}

	if obj.Pkg() == nil {
		return nil, fmt.Errorf("cannot rename predeclared identifiers (%s)", obj)
	}
	return []types.Object{obj}, nil
}

func typeSwitchVar(info *types.Info, path []ast.Node) types.Object {
	if len(path) > 3 {
		// [Ident AssignStmt TypeSwitchStmt...]
		if sw, ok := path[2].(*ast.TypeSwitchStmt); ok {
			// choose the first case.
			if len(sw.Body.List) > 0 {
				obj := info.Implicits[sw.Body.List[0].(*ast.CaseClause)]
				if obj != nil {
					return obj
				}
			}
		}
	}
	return nil
}
