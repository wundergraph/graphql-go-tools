package graphqlerror

import (
	"fmt"
	"strconv"
	"unsafe"
)

type ExternalError struct {
	Message   string     `json:"message"`
	Path      []Path     `json:"path"`
	Locations []Location `json:"locations"`
}

type Location struct {
	Line   uint32 `json:"line"`
	Column uint32 `json:"column"`
}

type PathKind int

const (
	UnknownPathKind PathKind = iota
	ArrayIndex
	FieldName
)

type Path struct {
	Kind       PathKind
	ArrayIndex int
	FieldName  string
}

func (p *Path) UnmarshalJSON(data []byte) error {
	if data == nil {
		return fmt.Errorf("data must not be nil")
	}
	if data[0] == '"' && data[len(data)-1] == '"' {
		p.Kind = FieldName
		p.FieldName = string(data[1 : len(data)-1])
		return nil
	}
	out, err := strconv.ParseInt(*(*string)(unsafe.Pointer(&data)), 10, 64)
	if err != nil {
		return err
	}
	p.Kind = ArrayIndex
	p.ArrayIndex = int(out)
	return nil
}

func (p Path) MarshalJSON() ([]byte, error) {
	switch p.Kind {
	case ArrayIndex:
		return strconv.AppendInt(nil, int64(p.ArrayIndex), 10), nil
	case FieldName:
		return append([]byte("\""), append([]byte(p.FieldName), []byte("\"")...)...), nil
	default:
		return nil, fmt.Errorf("cannot marshal unknown PathKind")
	}
}
