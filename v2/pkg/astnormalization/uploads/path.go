package uploads

import (
	"bytes"
	"fmt"
)

type Path struct {
	path []pathItem
}

func (v *Path) hasPath() bool {
	return len(v.path) > 0
}

func (v *Path) render() string {
	out := &bytes.Buffer{}
	for i, item := range v.path {
		if i > 0 {
			out.WriteString(".")
		}
		out.Write(item.name)
		if item.kind == pathItemKindArray {
			out.WriteString(fmt.Sprintf("%d", item.arrayIndex))
		}
	}
	return out.String()
}

func (v *Path) reset() {
	if v.path != nil {
		v.path = v.path[:0]
	}
}

type pathItemKind int

const (
	pathItemKindObject pathItemKind = iota
	pathItemKindArray
)

type pathItem struct {
	kind       pathItemKind
	name       []byte
	arrayIndex int
}

func (v *Path) pushObjectPath(name []byte) {
	v.path = append(v.path, pathItem{
		kind: pathItemKindObject,
		name: name,
	})
}

func (v *Path) pushArrayPath(index int) {
	v.path = append(v.path, pathItem{
		kind:       pathItemKindArray,
		arrayIndex: index,
	})
}

func (v *Path) popPath() {
	v.path = v.path[:len(v.path)-1]
}
