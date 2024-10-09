package ast

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
)

type Alias struct {
	IsDefined bool
	Name      ByteSliceReference // optional, e.g. renamedField
	Colon     position.Position  // :
}

func (d *Document) CopyAlias(alias Alias) Alias {
	return Alias{
		IsDefined: alias.IsDefined,
		Name:      d.copyByteSliceReference(alias.Name),
	}
}

func (d *Document) FieldAliasOrNameBytes(ref int) ByteSlice {
	if d.FieldAliasIsDefined(ref) {
		return d.FieldAliasBytes(ref)
	}
	return d.FieldNameBytes(ref)
}

func (d *Document) FieldAliasOrNameString(ref int) string {
	return unsafebytes.BytesToString(d.FieldAliasOrNameBytes(ref))
}

func (d *Document) FieldPath(ref int, path Path) string {
	if d.Fields[ref].Path == "" {
		name := string(d.FieldAliasOrNameBytes(ref))
		if len(path) == 0 {
			d.Fields[ref].Path = name
		} else {
			d.Fields[ref].Path = fmt.Sprintf("%s.%s", path.DotDelimitedString(), name)
		}
	}
	p := d.Fields[ref].Path
	return p
}

func (d *Document) FieldParentPath(ref int, path Path) string {
	if d.Fields[ref].ParentPath == "" {
		if len(path) == 0 {
			d.Fields[ref].ParentPath = ""
		} else {
			d.Fields[ref].ParentPath = path.DotDelimitedString()
		}
	}
	return d.Fields[ref].ParentPath
}

func (d *Document) FieldGrandParentPath(ref int, path Path) string {
	if d.Fields[ref].GrandParentPath == "" {
		if len(path) < 2 {
			d.Fields[ref].GrandParentPath = ""
		} else {
			d.Fields[ref].GrandParentPath = path[:len(path)-1].DotDelimitedString()
		}
	}
	return d.Fields[ref].GrandParentPath
}

func (d *Document) FieldAliasBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.Fields[ref].Alias.Name)
}

func (d *Document) FieldAliasString(ref int) string {
	return unsafebytes.BytesToString(d.Input.ByteSlice(d.Fields[ref].Alias.Name))
}

func (d *Document) FieldAliasIsDefined(ref int) bool {
	return d.Fields[ref].Alias.IsDefined
}

func (d *Document) RemoveFieldAlias(ref int) {
	d.Fields[ref].Alias.IsDefined = false
	d.Fields[ref].Alias.Name.Start = 0
	d.Fields[ref].Alias.Name.End = 0
}
