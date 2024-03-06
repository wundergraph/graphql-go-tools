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
	Kind       PathKind
	ArrayIndex int
	FieldName  ByteSlice
}

type Path []PathItem

func (p Path) Equals(another Path) bool {
	if len(p) != len(another) {
		return false
	}
	for i := range p {
		if p[i].Kind != another[i].Kind {
			return false
		}
		if p[i].Kind == ArrayIndex && p[i].ArrayIndex != another[i].ArrayIndex {
			return false
		} else if !bytes.Equal(p[i].FieldName, another[i].FieldName) {
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

func (p Path) String() string {
	out := "["
	for i := range p {
		if i != 0 {
			out += ","
		}
		switch p[i].Kind {
		case ArrayIndex:
			out += strconv.Itoa(p[i].ArrayIndex)
		case FieldName:
			if len(p[i].FieldName) == 0 {
				out += "query"
			} else {
				out += unsafebytes.BytesToString(p[i].FieldName)
			}
		case InlineFragmentName:
			out += InlineFragmentPathPrefix
			out += unsafebytes.BytesToString(p[i].FieldName)
		}
	}
	out += "]"
	return out
}

func (p Path) DotDelimitedString() string {
	builder := strings.Builder{}

	toGrow := 0
	for i := range p {
		switch p[i].Kind {
		case ArrayIndex:
			toGrow += 1
		case InlineFragmentName:
			toGrow += len(p[i].FieldName) + 1 // 1 for the prefix $
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

func (p PathItem) MarshalJSON() ([]byte, error) {
	switch p.Kind {
	case ArrayIndex:
		return strconv.AppendInt(nil, int64(p.ArrayIndex), 10), nil
	case FieldName, InlineFragmentName:
		return append([]byte("\""), append(p.FieldName, []byte("\"")...)...), nil
	default:
		return nil, fmt.Errorf("cannot marshal unknown PathKind")
	}
}
