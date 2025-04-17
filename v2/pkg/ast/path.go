package ast

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

type PathKind int

const (
	InlineFragmentPathPrefix     = "$"
	InlineFragmentPathPrefixRune = '$'
)

const (
	UnknownPathKind PathKind = iota
	ArrayIndex
	FieldName
	InlineFragmentName
)

type PathItem struct {
	Kind        PathKind
	ArrayIndex  int
	FieldName   ByteSlice
	FragmentRef int // only used for InlineFragmentName, allows to distinguish between multiple inline fragments on the same type
}

type Path []PathItem

func (p Path) Equals(another Path) bool {
	if len(p) != len(another) {
		return false
	}
	for i := len(p) - 1; i >= 0; i-- {
		if p[i].Kind != another[i].Kind {
			return false
		}
		if p[i].Kind == ArrayIndex && p[i].ArrayIndex != another[i].ArrayIndex {
			return false
		}
		if !bytes.Equal(p[i].FieldName, another[i].FieldName) {
			return false
		}
		if p[i].FragmentRef != another[i].FragmentRef {
			return false
		}
	}
	return true
}

func (p Path) Overlaps(other Path) bool {
	for i, el := range p {
		switch {
		case i >= len(other):
			return true
		case el.Kind != other[i].Kind:
			return false
		case el.FragmentRef != other[i].FragmentRef:
			return false
		case el.Kind == ArrayIndex && el.ArrayIndex != other[i].ArrayIndex:
			return false
		case !bytes.Equal(el.FieldName, other[i].FieldName):
			return false
		}
	}
	return true
}

func (p Path) EndsWithFragment() bool {
	if len(p) == 0 {
		return false
	}
	return p[len(p)-1].Kind == InlineFragmentName
}

func (p Path) WithoutInlineFragmentNames() Path {
	count := 0
	for i := range p {
		if p[i].Kind != InlineFragmentName {
			count++
		}
	}
	out := make(Path, 0, count)
	for i := range p {
		if p[i].Kind != InlineFragmentName {
			out = append(out, p[i])
		}
	}
	return out
}

func (p Path) StringSlice() []string {
	ret := make([]string, len(p))
	for i, item := range p {
		ret[i] = item.String()
	}
	return ret
}

func (p Path) String() string {
	return "[" + strings.Join(p.StringSlice(), ",") + "]"
}

func (p PathItem) String() string {
	switch p.Kind {
	case ArrayIndex:
		return strconv.Itoa(p.ArrayIndex)
	case FieldName:
		out := "query"
		if len(p.FieldName) != 0 {
			out = unsafebytes.BytesToString(p.FieldName)
		}
		return out
	case InlineFragmentName:
		out := InlineFragmentPathPrefix
		out += strconv.Itoa(p.FragmentRef)
		out += unsafebytes.BytesToString(p.FieldName)
		return out
	}
	return ""
}

func (p Path) DotDelimitedString() string {
	builder := strings.Builder{}

	toGrow := 0
	for i := range p {
		switch p[i].Kind {
		case ArrayIndex:
			toGrow += 1
		case InlineFragmentName:
			toGrow += len(p[i].FieldName) + 1 + 4 // 1 for the prefix $, 4 for the fragment ref
		case FieldName:
			toGrow += len(p[i].FieldName)
		}
	}
	builder.Grow(toGrow + 5 + len(p) - 1) // 5 is for the query prefix, len(p) - 1 for each dot

	builder.WriteString("")
	for i := range p {
		if i != 0 {
			builder.WriteString(".")
		}
		switch p[i].Kind {
		case ArrayIndex:
			builder.WriteString(strconv.Itoa(p[i].ArrayIndex))
		case FieldName:
			if len(p[i].FieldName) == 0 {
				builder.WriteString("query")
			} else {
				builder.WriteString(unsafebytes.BytesToString(p[i].FieldName))
			}
		case InlineFragmentName:
			builder.WriteString(InlineFragmentPathPrefix)
			builder.WriteString(strconv.Itoa(p[i].FragmentRef))
			builder.WriteString(unsafebytes.BytesToString(p[i].FieldName))
		}
	}

	return builder.String()
}

func (p *PathItem) UnmarshalJSON(data []byte) error {
	if data == nil {
		return fmt.Errorf("data must not be nil")
	}
	if data[0] == '"' && data[len(data)-1] == '"' {
		p.FieldName = data[1 : len(data)-1]
		if p.FieldName[0] == InlineFragmentPathPrefixRune {
			p.Kind = InlineFragmentName
		} else {
			p.Kind = FieldName
		}
		return nil
	}
	out, err := strconv.ParseInt(*(*string)(unsafe.Pointer(&data)), 10, 32)
	if err != nil {
		return err
	}
	p.Kind = ArrayIndex
	p.ArrayIndex = int(out)
	return nil
}

func (p *PathItem) MarshalJSON() ([]byte, error) {
	switch p.Kind {
	case ArrayIndex:
		return strconv.AppendInt(nil, int64(p.ArrayIndex), 10), nil
	case FieldName, InlineFragmentName:
		return append([]byte("\""), append(p.FieldName, []byte("\"")...)...), nil
	default:
		return nil, fmt.Errorf("cannot marshal unknown PathKind")
	}
}
