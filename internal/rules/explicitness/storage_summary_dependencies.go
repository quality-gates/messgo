package explicitness

import (
	"go/ast"

	"github.com/quality-gates/messgo/internal/model"
)

func addStorageFields(d *storageDependencies, fields *ast.FieldList) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		for _, name := range field.Names {
			if name.Obj != nil {
				d.objects[name.Obj] = true
			}
		}
	}
}

func collectStorageIdentifier(fn *model.Function, ident *ast.Ident, d *storageDependencies) {
	if ident.Obj != nil && !storageLocalObject(fn, ident.Obj) {
		d.objects[ident.Obj] = true
	}
	// Function values can be bound locally before they are called.
	if ident.Obj == nil || ident.Obj.Kind == ast.Fun {
		collectStorageDependencies(model.ResolveCall(fn, &ast.CallExpr{Fun: ident}), d)
	}
}

func storageLocalObject(fn *model.Function, object *ast.Object) bool {
	decl, ok := object.Decl.(ast.Node)
	if !ok {
		return false
	}
	return storageNodeInside(fn.Body, decl) || storageNodeInside(fn.Decl.Type, decl) ||
		(fn.Decl.Recv != nil && storageNodeInside(fn.Decl.Recv, decl))
}

func storageNodeInside(container, node ast.Node) bool {
	return node.Pos() >= container.Pos() && node.End() <= container.End()
}
