package statement

import (
	"context"
)

type Statement struct {
	RootTemplate string
	Templates    []Template
}

type TemplateKind int

const (
	ObjectTemplateKind TemplateKind = iota + 1
	ArrayTemplateKind
	ConditionalTemplateKind
)

type Template interface {
	Kind() TemplateKind
	Name() string
}

type ResolverPipeline struct {
	Input     string
	Resolvers []ResolverConfig
}

type ResolverConfig struct {
	Name   string
	Config string
}

type ObjectTemplate struct {
	TemplateName     string
	ResolverPipeline ResolverPipeline
	Variables        []Variable
	ObjectDefinition string
}

func (o ObjectTemplate) Name() string {
	return o.TemplateName
}

func (_ ObjectTemplate) Kind() TemplateKind {
	return ObjectTemplateKind
}

type ArrayTemplate struct {
	TemplateName     string
	ResolverPipeline ResolverPipeline
	Variables        []Variable
	ItemPath         string
	ItemDefinition   string
}

func (a ArrayTemplate) Name() string {
	return a.TemplateName
}

func (_ ArrayTemplate) Kind() TemplateKind {
	return ArrayTemplateKind
}

type Variable interface {
	Kind() VariableKind
}

type VariableKind int

const (
	TemplateVariableKind VariableKind = iota + 1
	DataSourceFieldValueKind
	DataSourceResponseValueKind
	ContextVariableKind
	ParentObjectFieldValueKind
	TemplateInputFieldValueKind
)

type TemplateInputFieldValue struct {
	Path string
}

func (_ TemplateInputFieldValue) Kind() VariableKind {
	return TemplateInputFieldValueKind
}

type ParentObjectFieldValue struct {
	Path string
}

func (_ ParentObjectFieldValue) Kind() VariableKind {
	return ParentObjectFieldValueKind
}

type ContextVariable struct {
	Name string
}

func (_ ContextVariable) Kind() VariableKind {
	return ContextVariableKind
}

type DataSourceArrayValue struct {
	Path string
}

func (_ DataSourceArrayValue) Kind() VariableKind {
	return DataSourceResponseValueKind
}

type DataSourceFieldValue struct {
	Path string
}

func (_ DataSourceFieldValue) Kind() VariableKind {
	return DataSourceFieldValueKind
}

type TemplateVariable struct {
	TemplateName string
	Input        string
}

func (_ TemplateVariable) Kind() VariableKind {
	return TemplateVariableKind
}

/*
{"id":1,"title":"GraphQL is confusing!?"}
*/

/*
{"__typename":"Post","id":1,"title":"GraphQL is confusing!?"}
*/

type Resolver interface {
	Resolve(ctx context.Context, input []byte) (output []byte, err error)
}

/*
TODO:
	1. field validation
*/

var stmt = Statement{
	RootTemplate: "root",
	Templates: []Template{
		ObjectTemplate{
			TemplateName:     "root",
			ObjectDefinition: `{"data":{"post":$$0},"errors":$$1}`,
			Variables: []Variable{
				TemplateVariable{
					TemplateName: "queryPost",
				},
				TemplateVariable{
					TemplateName: "errors",
				},
			},
		},
		ObjectTemplate{
			TemplateName:     "queryPost",
			ObjectDefinition: `{"id":$$0,"title":$$1,"comments":$$2}`,
			ResolverPipeline: ResolverPipeline{
				Input: `{"host":"example.com","url":"/posts/$$3","method":"GET"}`,
				Resolvers: []ResolverConfig{
					{
						Name:   "HTTPJson",
						Config: "url...",
					},
					{
						Name:   "defaultValue",
						Config: "bla",
					},
				},
			},
			Variables: []Variable{
				DataSourceFieldValue{
					Path: "id",
				},
				DataSourceFieldValue{
					Path: "title",
				},
				TemplateVariable{
					TemplateName: "Post_comments",
				},
				ContextVariable{
					Name: "id",
				},
			},
		},
		ArrayTemplate{
			TemplateName:   "Post_comments",
			ItemPath:       "sub.comments",
			ItemDefinition: `{"id":$$1,"title":$$2}`,
			ResolverPipeline: ResolverPipeline{
				Input: `{"host":"example.com","url":"/comments/$$0"}`,
				Resolvers: []ResolverConfig{
					{
						Name:   "HTTPJson",
						Config: "url...",
					},
				},
			},
			Variables: []Variable{
				ParentObjectFieldValue{
					Path: "id",
				},
				DataSourceFieldValue{
					Path: "id",
				},
				DataSourceFieldValue{
					Path: "title",
				},
			},
		},
		ObjectTemplate{
			TemplateName:     "errors",
			ObjectDefinition: "[renderErrors()]",
		},
	},
}