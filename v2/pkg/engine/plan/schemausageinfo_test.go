package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGetSchemaUsageInfo(t *testing.T) {
	source := resolve.TypeFieldSource{
		IDs: []string{"https://swapi.dev/api"},
	}
	res := &resolve.GraphQLResponse{
		Info: &resolve.GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		},
		Data: &resolve.Object{
			Nullable: false,
			Fields: []*resolve.Field{
				{
					Name: []byte("searchResults"),
					Info: &resolve.FieldInfo{
						Name:            "searchResults",
						NamedType:       "SearchResults",
						ParentTypeNames: []string{"Query"},
						Source:          source,
					},
					Value: &resolve.Array{
						Path:                []string{"searchResults"},
						Nullable:            true,
						ResolveAsynchronous: false,
						Item: &resolve.Object{
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("__typename"),
									Value: &resolve.String{
										Path:     []string{"__typename"},
										Nullable: false,
									},
									Info: &resolve.FieldInfo{
										Name:            "__typename",
										NamedType:       "String",
										ParentTypeNames: []string{"Human", "Droid"},
										Source:          source,
									},
								},
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path:     []string{"name"},
										Nullable: false,
									},
									OnTypeNames: [][]byte{[]byte("Human"), []byte("Droid")},
									Info: &resolve.FieldInfo{
										Name:            "name",
										NamedType:       "String",
										ParentTypeNames: []string{"Human", "Droid"},
										Source:          source,
									},
								},
								{
									Name: []byte("length"),
									Value: &resolve.Float{
										Path:     []string{"length"},
										Nullable: false,
									},
									OnTypeNames: [][]byte{[]byte("Starship")},
									Info: &resolve.FieldInfo{
										Name:            "length",
										NamedType:       "String",
										ParentTypeNames: []string{"Starship"},
										Source:          source,
									},
								},
								{
									Name: []byte("user"),
									Info: &resolve.FieldInfo{
										Name:            "user",
										NamedType:       "User",
										ParentTypeNames: []string{"SearchResults"},
										Source:          source,
									},
									Value: &resolve.Object{
										Path:     []string{"user"},
										Nullable: true,
										Fields: []*resolve.Field{
											{
												Name: []byte("account"),
												Info: &resolve.FieldInfo{
													Name:            "account",
													NamedType:       "Account",
													ParentTypeNames: []string{"User"},
													Source:          source,
												},
												Value: &resolve.Object{
													Path:     []string{"account"},
													Nullable: true,
													Fields: []*resolve.Field{
														{
															Name: []byte("name"),
															Info: &resolve.FieldInfo{
																Name:            "name",
																NamedType:       "String",
																ParentTypeNames: []string{"Account"},
																Source:          source,
															},
															Value: &resolve.String{
																Path: []string{"name"},
															},
														},
														{
															Name: []byte("shippingInfo"),
															Info: &resolve.FieldInfo{
																Name:            "shippingInfo",
																NamedType:       "ShippingInfo",
																ParentTypeNames: []string{"Account"},
																Source:          source,
															},
															Value: &resolve.Object{
																Path:     []string{"ShippingInfo"},
																Nullable: true,
																Fields: []*resolve.Field{
																	{
																		Name: []byte("zip"),
																		Info: &resolve.FieldInfo{
																			Name:            "zip",
																			NamedType:       "String",
																			ParentTypeNames: []string{"ShippingInfo"},
																			Source:          source,
																		},
																		Value: &resolve.String{
																			Path: []string{"zip"},
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
	syncUsage := GetSchemaUsageInfo(&SynchronousResponsePlan{
		Response: res,
	})
	subscriptionUsage := GetSchemaUsageInfo(&SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Response: res,
		},
	})
	expected := SchemaUsageInfo{
		OperationType: ast.OperationTypeQuery,
		TypeFields: []TypeFieldUsageInfo{
			{
				FieldName: "searchResults",
				TypeNames: []string{"Query"},
				Path:      []string{"searchResults"},
				NamedType: "SearchResults",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "__typename"},
				TypeNames: []string{"Human", "Droid"},
				FieldName: "__typename",
				NamedType: "String",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "name"},
				TypeNames: []string{"Human", "Droid"},
				FieldName: "name",
				NamedType: "String",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "length"},
				TypeNames: []string{"Starship"},
				NamedType: "String",
				FieldName: "length",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "user"},
				NamedType: "User",
				TypeNames: []string{"SearchResults"},
				FieldName: "user",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "user", "account"},
				TypeNames: []string{"User"},
				NamedType: "Account",
				FieldName: "account",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "user", "account", "name"},
				TypeNames: []string{"Account"},
				NamedType: "String",
				FieldName: "name",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "user", "account", "shippingInfo"},
				NamedType: "ShippingInfo",
				TypeNames: []string{"Account"},
				FieldName: "shippingInfo",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "user", "account", "shippingInfo", "zip"},
				TypeNames: []string{"ShippingInfo"},
				NamedType: "String",
				FieldName: "zip",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
		},
	}
	assert.Equal(t, expected, syncUsage)
	assert.Equal(t, expected, subscriptionUsage)
}
