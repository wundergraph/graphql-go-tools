package statementv3

import (
	"context"
)

// resolve() -> client
type SingleStatement struct {
	DataSourceDefinitions []DataSourceDefinition
	Template              Node
}

// for {resolve() -> client }
type StreamingStatement struct {
}

// for { trigger -> resolve() -> client }
type SubscriptionStatement struct {
}

type Variable interface {
	VariableKind() VariableKind
}

type VariableKind int

const (
	ContextVariableKind VariableKind = iota + 1
	ParentDataSourceVariableKind
)

type ParentDataSourceVariable struct {
	Path string
}

func (_ ParentDataSourceVariable) VariableKind() VariableKind {
	return ParentDataSourceVariableKind
}

type ContextVariable struct {
	Path string
}

func (_ ContextVariable) VariableKind() VariableKind {
	return ContextVariableKind
}

type Resolver interface {
	Resolve(ctx context.Context, config, input []byte) (output []byte, err error)
}

type DataSourceDefinition interface {
	DataSourceDefinitionKind() DataSourceDefinitionKind
}

type DataSourceDefinitionKind int

const (
	ResolveOneDataSourceDefinitionKind DataSourceDefinitionKind = iota
	ResolveManyDataSourceDefinitionKind
)

type ResolveOne struct {
	Input     string
	Resolvers []ResolverDefinition
	Variables []Variable
	Children  []DataSourceDefinition
}

func (_ ResolveOne) DataSourceDefinitionKind() DataSourceDefinitionKind {
	return ResolveOneDataSourceDefinitionKind
}

type ResolveMany struct {
	ArrayPath string
	Input     string
	Resolvers []ResolverDefinition
	Variables []Variable
	Children  []DataSourceDefinition
}

func (_ ResolveMany) DataSourceDefinitionKind() DataSourceDefinitionKind {
	return ResolveManyDataSourceDefinitionKind
}

type ResolverDefinition struct {
	Name   string
	Config string
}

type Node interface {
	NodeKind() NodeKind
}

type NodeKind int

const (
	ObjectNodeKind NodeKind = iota + 1
	FieldNodeKind
	ArrayNodeKind
	ResultValueNodeKind
	StaticValueNodeKind
)

type Object struct {
	ResultSetSelector []int
	FieldSets         []FieldSet
}

func (_ Object) NodeKind() NodeKind {
	return ObjectNodeKind
}

type FieldSet struct {
	TypeNameRestriction string
	Fields              []Field
}

type Field struct {
	Name  string
	Value Node
}

func (_ Field) NodeKind() NodeKind {
	return FieldNodeKind
}

type ArrayNode struct {
	ResultSetSelector []int
	Path              string
	Item              Node
}

func (_ ArrayNode) NodeKind() NodeKind {
	return ArrayNodeKind
}

type ResultValue string

func (_ ResultValue) NodeKind() NodeKind {
	return ResultValueNodeKind
}

type StaticValue string

func (_ StaticValue) NodeKind() NodeKind {
	return StaticValueNodeKind
}

var nestedRestAPIs = SingleStatement{
	DataSourceDefinitions: []DataSourceDefinition{
		ResolveOne{
			Input: `{"host":"example.com","url":"/post/$$1","method":"GET"}`,
			Resolvers: []ResolverDefinition{
				{
					Name:   "HttpJSON",
					Config: `{"defaultTimeout":"5s"}`,
				},
				{
					Name: "defaultReturnNull",
				},
			},
			Variables: []Variable{
				ContextVariable{
					Path: "id",
				},
			},
			Children: []DataSourceDefinition{
				ResolveOne{
					Input: `{"host":"example.com","url":"/post/$$1/comments","method":"GET"}`,
					Resolvers: []ResolverDefinition{
						{
							Name:   "HttpJSON",
							Config: `{"defaultTimeout":"5s"}`,
						},
						{
							Name: "defaultReturnNull",
						},
					},
					Variables: []Variable{
						ParentDataSourceVariable{
							Path: "id",
						},
					},
				},
			},
		},
	},
	Template: Object{
		FieldSets: []FieldSet{
			{
				Fields: []Field{
					{
						Name: "data",
						Value: Object{
							FieldSets: []FieldSet{
								{
									Fields: []Field{
										{
											Name: "post",
											Value: Object{
												ResultSetSelector: []int{0},
												FieldSets: []FieldSet{
													{
														Fields: []Field{
															{
																Name:  "__typename",
																Value: StaticValue("Post"),
															},
															{
																Name:  "id",
																Value: ResultValue("id"),
															},
															{
																Name:  "title",
																Value: ResultValue("title"),
															},
															{
																Name: "comments",
																Value: Object{
																	ResultSetSelector: []int{0},
																	FieldSets: []FieldSet{
																		{
																			Fields: []Field{
																				{
																					Name:  "__typename",
																					Value: ResultValue("__typename"),
																				},
																				{
																					Name:  "name",
																					Value: ResultValue("name"),
																				},
																			},
																		},
																		{
																			TypeNameRestriction: "Dog",
																			Fields: []Field{
																				{
																					Name:  "woof",
																					Value: ResultValue("woof"),
																				},
																			},
																		},
																		{
																			TypeNameRestriction: "Cat",
																			Fields: []Field{
																				{
																					Name:  "meow",
																					Value: ResultValue("meow"),
																				},
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},
}

var unions = SingleStatement{
	DataSourceDefinitions: []DataSourceDefinition{
		ResolveOne{
			Input: `{"host":"example.com","url":"/user/$$1","method":"GET"}`,
			Resolvers: []ResolverDefinition{
				{
					Name:   "HttpJSON",
					Config: `{"defaultTimeout":"5s"}`,
				},
				{
					Name: "defaultReturnNull",
				},
			},
			Variables: []Variable{
				ContextVariable{
					Path: "id",
				},
			},
			Children: []DataSourceDefinition{
				ResolveOne{
					Input: `{"host":"example.com","url":"/user/$$1/friends","method":"GET"}`,
					Resolvers: []ResolverDefinition{
						{
							Name:   "HttpJSON",
							Config: `{"defaultTimeout":"5s"}`,
						},
						{
							Name: "defaultReturnNull",
						},
					},
					Variables: []Variable{
						ParentDataSourceVariable{
							Path: "id",
						},
					},
					Children: []DataSourceDefinition{
						ResolveMany{
							ArrayPath: "",
							Input:     `{"host":"example.com","url":"/user/$$1/pets","method":"GET"}`,
							Resolvers: []ResolverDefinition{
								{
									Name:   "HttpJSON",
									Config: `{"defaultTimeout":"5s"}`,
								},
								{
									Name: "defaultReturnNull",
								},
							},
							Variables: []Variable{
								ParentDataSourceVariable{
									Path: "id",
								},
							},
						},
					},
				},
			},
		},
	},
	Template: Object{
	},
}
