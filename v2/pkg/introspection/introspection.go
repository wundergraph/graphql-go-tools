//go:generate go-enum -f=$GOFILE --noprefix --marshal

// Package introspection takes a GraphQL Schema and provides the introspection JSON to fulfill introspection queries.
package introspection

import (
	"bytes"
)

type Data struct {
	Schema Schema `json:"__schema"`
}

type Schema struct {
	QueryType        FullType    `json:"queryType"`
	MutationType     *FullType   `json:"mutationType"`
	SubscriptionType *FullType   `json:"subscriptionType"`
	Types            []*FullType `json:"types"`
	Directives       []Directive `json:"directives"`
	TypeName         string      `json:"__typename"`
	fullTypeMap      map[string]*FullType
}

func (s *Schema) AddType(t *FullType) {
	s.Types = append(s.Types, t)
	s.fullTypeMap[t.Name] = t
}

func (s *Schema) TypeByName(name string) *FullType {
	fullType, _ := s.fullTypeMap[name]
	return fullType
}

func (s *Schema) TypeNames() (query, mutation, subscription string) {
	query = s.QueryType.Name

	if s.MutationType != nil {
		mutation = s.MutationType.Name
	}
	if s.SubscriptionType != nil {
		subscription = s.SubscriptionType.Name
	}
	return
}

func NewSchema() Schema {
	return Schema{
		Types:       make([]*FullType, 0),
		Directives:  make([]Directive, 0),
		TypeName:    "__Schema",
		fullTypeMap: make(map[string]*FullType),
	}
}

type FullType struct {
	Kind        __TypeKind `json:"kind"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	// not empty for __TypeKind OBJECT and INTERFACE only
	Fields []Field `json:"fields,omitempty"`
	// not empty for __TypeKind INPUT_OBJECT only
	InputFields []InputValue `json:"inputFields"`
	// not empty for __TypeKind OBJECT only
	Interfaces []TypeRef `json:"interfaces"`
	// not empty for __TypeKind ENUM only
	EnumValues []EnumValue `json:"enumValues,omitempty"`
	// not empty for __TypeKind INTERFACE and UNION only
	PossibleTypes []TypeRef `json:"possibleTypes"`
	TypeName      string    `json:"__typename"`
}

func NewFullType() *FullType {
	return &FullType{
		Fields:        make([]Field, 0),
		InputFields:   make([]InputValue, 0),
		Interfaces:    make([]TypeRef, 0),
		EnumValues:    make([]EnumValue, 0),
		PossibleTypes: make([]TypeRef, 0),
		TypeName:      "__Type",
	}
}

/*
ENUM(
SCALAR
LIST
NON_NULL
OBJECT
ENUM
INTERFACE
UNION
INPUT_OBJECT
)
*/
type __TypeKind int

func (x __TypeKind) MarshalJSON() ([]byte, error) {

	text, err := x.MarshalText()
	if err != nil {
		return nil, err
	}

	var buff bytes.Buffer
	_, err = buff.WriteRune('"')
	if err != nil {
		return nil, err
	}
	_, err = buff.Write(text)
	if err != nil {
		return nil, err
	}
	_, err = buff.WriteRune('"')

	return buff.Bytes(), err
}

type TypeRef struct {
	Kind     __TypeKind `json:"kind"`
	Name     *string    `json:"name"`
	OfType   *TypeRef   `json:"ofType"`
	TypeName string     `json:"__typename"`
}

type Field struct {
	Name              string       `json:"name"`
	Description       string       `json:"description"`
	Args              []InputValue `json:"args"`
	Type              TypeRef      `json:"type"`
	IsDeprecated      bool         `json:"isDeprecated"`
	DeprecationReason *string      `json:"deprecationReason"`
	TypeName          string       `json:"__typename"`
}

func NewField() Field {
	return Field{
		Args:     make([]InputValue, 0),
		TypeName: "__Field",
	}
}

type EnumValue struct {
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	IsDeprecated      bool    `json:"isDeprecated"`
	DeprecationReason *string `json:"deprecationReason"`
	TypeName          string  `json:"__typename"`
}

type InputValue struct {
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	Type         TypeRef `json:"type"`
	DefaultValue *string `json:"defaultValue"`
	TypeName     string  `json:"__typename"`
}

type Directive struct {
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	Locations    []string     `json:"locations"`
	Args         []InputValue `json:"args"`
	IsRepeatable bool         `json:"isRepeatable"`
	TypeName     string       `json:"__typename"`
}

func NewDirective() Directive {
	return Directive{
		Locations: make([]string, 0),
		Args:      make([]InputValue, 0),
		TypeName:  "__Directive",
	}
}
