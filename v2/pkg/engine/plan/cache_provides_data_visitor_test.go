package plan

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestCacheProvidesDataVisitor(t *testing.T) {
	tests := []struct {
		name string
		op   string
		want func(t *testing.T, pd map[*resolve.FetchInfo]*resolve.Object) map[*resolve.FetchInfo]*resolve.Object
	}{
		{
			name: "single key entity selection",
			op: `
				query {
					user(id: "1") {
						id
						username
					}
				}`,
			want: func(t *testing.T, pd map[*resolve.FetchInfo]*resolve.Object) map[*resolve.FetchInfo]*resolve.Object {
				accountInfo := requireProvidesDataInfo(t, pd, "accountDS")
				profileInfo := requireProvidesDataInfo(t, pd, "profileDS")
				return map[*resolve.FetchInfo]*resolve.Object{
					accountInfo: {
						Fields: []*resolve.Field{
							{
								Name: []byte("user"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"user"},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Nullable: false,
												Path:     []string{"id"},
											},
										},
									},
								},
							},
						},
					},
					profileInfo: {
						Fields: []*resolve.Field{
							{
								Name: []byte("username"),
								Value: &resolve.Scalar{
									Nullable: true,
									Path:     []string{"username"},
								},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					},
				}
			},
		},
		{
			name: "multi field entity with nested object",
			op: `
				query {
					user(id: "1") {
						id
						username
						profile {
							displayName
						}
					}
				}`,
			want: func(t *testing.T, pd map[*resolve.FetchInfo]*resolve.Object) map[*resolve.FetchInfo]*resolve.Object {
				accountInfo := requireProvidesDataInfo(t, pd, "accountDS")
				profileInfo := requireProvidesDataInfo(t, pd, "profileDS")
				return map[*resolve.FetchInfo]*resolve.Object{
					accountInfo: {
						Fields: []*resolve.Field{
							{
								Name: []byte("user"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"user"},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Nullable: false,
												Path:     []string{"id"},
											},
										},
									},
								},
							},
						},
					},
					profileInfo: {
						Fields: []*resolve.Field{
							{
								Name: []byte("username"),
								Value: &resolve.Scalar{
									Nullable: true,
									Path:     []string{"username"},
								},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name: []byte("profile"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"profile"},
									Fields: []*resolve.Field{
										{
											Name: []byte("displayName"),
											Value: &resolve.Scalar{
												Nullable: true,
												Path:     []string{"displayName"},
											},
										},
									},
								},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					},
				}
			},
		},
		{
			name: "aliased field stores original name",
			op: `
				query {
					user(id: "1") {
						id
						handle: username
					}
				}`,
			want: func(t *testing.T, pd map[*resolve.FetchInfo]*resolve.Object) map[*resolve.FetchInfo]*resolve.Object {
				accountInfo := requireProvidesDataInfo(t, pd, "accountDS")
				profileInfo := requireProvidesDataInfo(t, pd, "profileDS")
				return map[*resolve.FetchInfo]*resolve.Object{
					accountInfo: {
						Fields: []*resolve.Field{
							{
								Name: []byte("user"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"user"},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Nullable: false,
												Path:     []string{"id"},
											},
										},
									},
								},
							},
						},
					},
					profileInfo: {
						Fields: []*resolve.Field{
							{
								Name:         []byte("handle"),
								OriginalName: []byte("username"),
								Value: &resolve.Scalar{
									Nullable: true,
									Path:     []string{"handle"},
								},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					},
				}
			},
		},
		{
			name: "variable arguments are captured except on root operation fields",
			op: `
				query($id: ID!, $first: Int, $after: String) {
					user(id: $id) {
						id
						friends(first: $first, after: $after) {
							username
						}
					}
				}`,
			want: func(t *testing.T, pd map[*resolve.FetchInfo]*resolve.Object) map[*resolve.FetchInfo]*resolve.Object {
				accountInfo := requireProvidesDataInfo(t, pd, "accountDS")
				profileInfo := requireProvidesDataInfo(t, pd, "profileDS")
				return map[*resolve.FetchInfo]*resolve.Object{
					accountInfo: {
						Fields: []*resolve.Field{
							{
								Name: []byte("user"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"user"},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Nullable: false,
												Path:     []string{"id"},
											},
										},
									},
								},
							},
						},
					},
					profileInfo: {
						Fields: []*resolve.Field{
							{
								Name: []byte("friends"),
								Value: &resolve.Array{
									Nullable: true,
									Path:     []string{"friends"},
									Item: &resolve.Object{
										Nullable: true,
										Fields: []*resolve.Field{
											{
												Name: []byte("username"),
												Value: &resolve.Scalar{
													Nullable: true,
													Path:     []string{"username"},
												},
											},
										},
									},
								},
								OnTypeNames: [][]byte{[]byte("User")},
								CacheArgs: []resolve.CacheFieldArg{
									{Name: "after", VariableName: "after"},
									{Name: "first", VariableName: "first"},
								},
							},
						},
					},
				}
			},
		},
		{
			name: "typename is deduplicated in one frame",
			op: `
				query {
					user(id: "1") {
						id
						__typename
						__typename
						username
					}
				}`,
			want: func(t *testing.T, pd map[*resolve.FetchInfo]*resolve.Object) map[*resolve.FetchInfo]*resolve.Object {
				accountInfo := requireProvidesDataInfo(t, pd, "accountDS")
				profileInfo := requireProvidesDataInfo(t, pd, "profileDS")
				return map[*resolve.FetchInfo]*resolve.Object{
					accountInfo: {
						Fields: []*resolve.Field{
							{
								Name: []byte("user"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"user"},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Nullable: false,
												Path:     []string{"id"},
											},
										},
										{
											Name: []byte("__typename"),
											Value: &resolve.Scalar{
												Nullable: false,
												Path:     []string{"__typename"},
											},
										},
									},
								},
							},
						},
					},
					profileInfo: {
						Fields: []*resolve.Field{
							{
								Name: []byte("__typename"),
								Value: &resolve.Scalar{
									Nullable: false,
									Path:     []string{"__typename"},
								},
								OnTypeNames: [][]byte{[]byte("User")},
							},
							{
								Name: []byte("username"),
								Value: &resolve.Scalar{
									Nullable: true,
									Path:     []string{"username"},
								},
								OnTypeNames: [][]byte{[]byte("User")},
							},
						},
					},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := planCacheProvidesData(t, tt.op)

			assert.Equal(t, tt.want(t, pd), pd)
		})
	}
}

func planCacheProvidesData(t *testing.T, operation string) map[*resolve.FetchInfo]*resolve.Object {
	t.Helper()

	definition := unsafeparser.ParseGraphqlDocumentString(cacheProvidesDataDefinition)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definition))

	op := unsafeparser.ParseGraphqlDocumentString(operation)
	var report operationreport.Report
	norm := astnormalization.NewNormalizer(true, true)
	norm.NormalizeOperation(&op, &definition, &report)
	require.False(t, report.HasErrors(), report.Error())

	valid := astvalidation.DefaultOperationValidator()
	valid.Validate(&op, &definition, &report)
	require.False(t, report.HasErrors(), report.Error())

	accountInfo := &resolve.FetchInfo{DataSourceID: "accountDS"}
	profileInfo := &resolve.FetchInfo{DataSourceID: "profileDS"}
	planners := []PlannerConfiguration{
		&plannerConfiguration[any]{
			objectFetchConfiguration: &objectFetchConfiguration{
				fetchItem: &resolve.FetchItem{
					Fetch: &resolve.SingleFetch{Info: accountInfo},
				},
			},
		},
		&plannerConfiguration[any]{
			objectFetchConfiguration: &objectFetchConfiguration{
				fetchItem: &resolve.FetchItem{
					ResponsePath: "user",
					Fetch:        &resolve.SingleFetch{Info: profileInfo},
				},
			},
		},
	}

	walker := astvisitor.NewWalkerWithID(48, "CacheProvidesDataVisitorTest")
	visitor := &cacheProvidesDataVisitor{
		Walker:        &walker,
		operation:     &op,
		definition:    &definition,
		planners:      planners,
		fieldPlanners: cacheProvidesDataFieldPlanners(&op, &definition),
	}
	visitor.reset()
	walker.RegisterEnterFieldVisitor(visitor)
	walker.RegisterLeaveFieldVisitor(visitor)
	walker.Walk(&op, &definition, &report)
	require.False(t, report.HasErrors(), report.Error())

	pre := &SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{},
	}
	visitor.attachTo(pre)
	return pre.Response.CacheProvidesData()
}

func requireProvidesDataInfo(t *testing.T, pd map[*resolve.FetchInfo]*resolve.Object, dataSourceID string) *resolve.FetchInfo {
	t.Helper()

	for info := range pd {
		if info != nil && info.DataSourceID == dataSourceID {
			return info
		}
	}
	require.Failf(t, "missing provides data info", "data source %q in %#v", dataSourceID, pd)
	return nil
}

func cacheProvidesDataFieldPlanners(operation, definition *ast.Document) map[int][]int {
	fieldPlanners := make(map[int][]int)
	walker := astvisitor.NewWalkerWithID(48, "CacheProvidesDataFieldPlannerCollector")
	collector := &cacheProvidesDataFieldPlannerCollector{
		walker:        &walker,
		operation:     operation,
		fieldPlanners: fieldPlanners,
	}
	walker.RegisterEnterFieldVisitor(collector)
	var report operationreport.Report
	walker.Walk(operation, definition, &report)
	return fieldPlanners
}

const cacheProvidesDataDefinition = `
	type Query {
		user(id: ID!): User
	}
	type User {
		id: ID!
		username: String
		profile: Profile
		friends(first: Int, after: String): [User]
	}
	type Profile {
		displayName: String
	}
`

type cacheProvidesDataFieldPlannerCollector struct {
	walker        *astvisitor.Walker
	operation     *ast.Document
	fieldPlanners map[int][]int
}

func (c *cacheProvidesDataFieldPlannerCollector) EnterField(ref int) {
	path := c.walker.Path.DotDelimitedString() + "." + c.operation.FieldAliasOrNameString(ref)
	switch {
	case path == "query.user":
		c.fieldPlanners[ref] = []int{0, 1}
	case path == "query.user.__typename":
		c.fieldPlanners[ref] = []int{0, 1}
	case path == "query.user.id":
		c.fieldPlanners[ref] = []int{0}
	case strings.HasPrefix(path, "query.user."):
		c.fieldPlanners[ref] = []int{1}
	}
}
