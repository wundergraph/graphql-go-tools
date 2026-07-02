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

// The FIDELITY GATE rows: ported 1:1 from the first-pass/OLD ProvidesData
// builder test set, asserting the FULL per-fetch tree map. Planner 0 is the
// root fetch (accountDS), planner 1 the nested entity fetch at "user"
// (profileDS).
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
		{
			name: "inline fragment fields carry the concrete type condition",
			op: `
				query {
					user(id: "1") {
						id
						pet {
							name
							... on Dog {
								barks
							}
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
								Name: []byte("pet"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"pet"},
									Fields: []*resolve.Field{
										{
											Name: []byte("name"),
											Value: &resolve.Scalar{
												Nullable: true,
												Path:     []string{"name"},
											},
										},
										{
											Name: []byte("barks"),
											Value: &resolve.Scalar{
												Nullable: true,
												Path:     []string{"barks"},
											},
											OnTypeNames: [][]byte{[]byte("Dog")},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := planCacheProvidesData(t, tt.op)

			assert.Equal(t, tt.want(t, pd), pd)
		})
	}
}

// TestCacheProvidesDataVisitorAdversarial goes beyond the OLD test set (a
// verbatim port would carry latent OLD bugs): per-fetch attribution with an
// irrelevant provider sharing the consumer query, partial overlap of two
// entity fetches on the same field, and a planner with no attributed fields.
func TestCacheProvidesDataVisitorAdversarial(t *testing.T) {
	t.Run("irrelevant provider does not leak into sibling trees", func(t *testing.T) {
		planners := defaultProvidesDataPlanners()
		planners = append(planners, newTestProvidesDataPlanner("", &resolve.FetchInfo{DataSourceID: "statsDS"}))
		routes := func(path string, ref int, out map[int][]int) {
			switch {
			case path == "query.stats":
				out[ref] = []int{2}
			case path == "query.user":
				out[ref] = []int{0, 1}
			case path == "query.user.id":
				out[ref] = []int{0}
			case strings.HasPrefix(path, "query.user."):
				out[ref] = []int{1}
			}
		}
		pd := planCacheProvidesDataWith(t, `
			query {
				user(id: "1") { id username }
				stats
			}`, planners, routes)

		accountInfo := requireProvidesDataInfo(t, pd, "accountDS")
		profileInfo := requireProvidesDataInfo(t, pd, "profileDS")
		gotStatsInfo := requireProvidesDataInfo(t, pd, "statsDS")
		assert.Equal(t, map[*resolve.FetchInfo]*resolve.Object{
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
			gotStatsInfo: {
				Fields: []*resolve.Field{
					{
						Name: []byte("stats"),
						Value: &resolve.Scalar{
							Nullable: true,
							Path:     []string{"stats"},
						},
					},
				},
			},
		}, pd)
	})

	t.Run("partial overlap: two entity fetches sharing one field get their own trees", func(t *testing.T) {
		planners := defaultProvidesDataPlanners()
		planners = append(planners, newTestProvidesDataPlanner("user", &resolve.FetchInfo{DataSourceID: "secondEntityDS"}))
		routes := func(path string, ref int, out map[int][]int) {
			switch {
			case path == "query.user":
				out[ref] = []int{0, 1, 2}
			case path == "query.user.id":
				out[ref] = []int{0}
			case path == "query.user.username":
				out[ref] = []int{1, 2}
			case strings.HasPrefix(path, "query.user."):
				out[ref] = []int{1}
			}
		}
		pd := planCacheProvidesDataWith(t, `
			query {
				user(id: "1") { id username profile { displayName } }
			}`, planners, routes)

		accountInfo := requireProvidesDataInfo(t, pd, "accountDS")
		profileInfo := requireProvidesDataInfo(t, pd, "profileDS")
		gotSecondInfo := requireProvidesDataInfo(t, pd, "secondEntityDS")
		assert.Equal(t, map[*resolve.FetchInfo]*resolve.Object{
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
			gotSecondInfo: {
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
		}, pd)
	})

	t.Run("empty selections: unrouted planner absent, boundary-only entity planner empty", func(t *testing.T) {
		planners := defaultProvidesDataPlanners()
		planners = append(planners, newTestProvidesDataPlanner("", &resolve.FetchInfo{DataSourceID: "unusedDS"}))
		pd := planCacheProvidesDataWith(t, `
			query {
				user(id: "1") { id }
			}`, planners, defaultProvidesDataRoutes)

		accountInfo := requireProvidesDataInfo(t, pd, "accountDS")
		// profileDS is attributed the boundary field but nothing below it, so
		// its tree exists and is EMPTY (zero coverage); unusedDS was never
		// attributed any field and is absent entirely.
		profileInfo := requireProvidesDataInfo(t, pd, "profileDS")
		assert.Equal(t, map[*resolve.FetchInfo]*resolve.Object{
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
				Fields: []*resolve.Field{},
			},
		}, pd)
	})
}

// TestCacheProvidesDataVisitorDeterminism plans the same operation twice and
// asserts identical side-tables (compared by datasource, since *FetchInfo keys
// differ per run).
func TestCacheProvidesDataVisitorDeterminism(t *testing.T) {
	op := `
		query {
			user(id: "1") {
				id
				handle: username
				profile { displayName }
			}
		}`
	first := rekeyProvidesDataByDataSource(t, planCacheProvidesData(t, op))
	second := rekeyProvidesDataByDataSource(t, planCacheProvidesData(t, op))
	assert.Equal(t, first, second)
}

func rekeyProvidesDataByDataSource(t *testing.T, pd map[*resolve.FetchInfo]*resolve.Object) map[string]*resolve.Object {
	t.Helper()
	out := make(map[string]*resolve.Object, len(pd))
	for info, obj := range pd {
		require.NotContains(t, out, info.DataSourceID)
		out[info.DataSourceID] = obj
	}
	return out
}

func defaultProvidesDataPlanners() []PlannerConfiguration {
	return []PlannerConfiguration{
		newTestProvidesDataPlanner("", &resolve.FetchInfo{DataSourceID: "accountDS"}),
		newTestProvidesDataPlanner("user", &resolve.FetchInfo{DataSourceID: "profileDS"}),
	}
}

// newTestProvidesDataPlanner builds the minimal planner configuration the
// visitor dereferences: a fetch item (with the entity-boundary response path),
// an empty datasource configuration (for the federation lookup), and an empty
// paths configuration (for HasPathWithFieldRef).
func newTestProvidesDataPlanner(responsePath string, info *resolve.FetchInfo) PlannerConfiguration {
	return &plannerConfiguration[any]{
		plannerPathsConfiguration: &plannerPathsConfiguration{},
		dataSourceConfiguration: &dataSourceConfiguration[any]{
			DataSourceMetadata: &DataSourceMetadata{},
		},
		objectFetchConfiguration: &objectFetchConfiguration{
			fetchItem: &resolve.FetchItem{
				ResponsePath: responsePath,
				Fetch:        &resolve.SingleFetch{Info: info},
			},
		},
	}
}

// defaultProvidesDataRoutes emulates the main walk's field→planner attribution
// for the two-planner fixture: the root fetch resolves user+id, the entity
// fetch at "user" everything below it.
func defaultProvidesDataRoutes(path string, ref int, out map[int][]int) {
	switch {
	case path == "query.user":
		out[ref] = []int{0, 1}
	case path == "query.user.__typename":
		out[ref] = []int{0, 1}
	case path == "query.user.id":
		out[ref] = []int{0}
	case strings.HasPrefix(path, "query.user."):
		out[ref] = []int{1}
	}
}

func planCacheProvidesData(t *testing.T, operation string) map[*resolve.FetchInfo]*resolve.Object {
	t.Helper()
	return planCacheProvidesDataWith(t, operation, defaultProvidesDataPlanners(), defaultProvidesDataRoutes)
}

func planCacheProvidesDataWith(t *testing.T, operation string, planners []PlannerConfiguration, routes func(path string, ref int, out map[int][]int)) map[*resolve.FetchInfo]*resolve.Object {
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

	walker := astvisitor.NewWalkerWithID(48, "CacheProvidesDataVisitorTest")
	visitor := &cacheProvidesDataVisitor{
		Walker:        &walker,
		operation:     &op,
		definition:    &definition,
		planners:      planners,
		fieldPlanners: collectFieldPlanners(&op, &definition, routes),
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

// collectFieldPlanners emulates the main walk's fieldPlanners output using the
// row's path-based routing.
func collectFieldPlanners(operation, definition *ast.Document, routes func(path string, ref int, out map[int][]int)) map[int][]int {
	fieldPlanners := make(map[int][]int)
	walker := astvisitor.NewWalkerWithID(48, "CacheProvidesDataFieldPlannerCollector")
	collector := &cacheProvidesDataFieldPlannerCollector{
		walker:        &walker,
		operation:     operation,
		fieldPlanners: fieldPlanners,
		routes:        routes,
	}
	walker.RegisterEnterFieldVisitor(collector)
	var report operationreport.Report
	walker.Walk(operation, definition, &report)
	return fieldPlanners
}

const cacheProvidesDataDefinition = `
	type Query {
		user(id: ID!): User
		stats: Int
	}
	type User {
		id: ID!
		username: String
		profile: Profile
		friends(first: Int, after: String): [User]
		pet: Pet
	}
	type Profile {
		displayName: String
	}
	interface Pet {
		name: String
	}
	type Dog implements Pet {
		name: String
		barks: Boolean
	}
	type Cat implements Pet {
		name: String
		meows: Boolean
	}
`

type cacheProvidesDataFieldPlannerCollector struct {
	walker        *astvisitor.Walker
	operation     *ast.Document
	fieldPlanners map[int][]int
	routes        func(path string, ref int, out map[int][]int)
}

func (c *cacheProvidesDataFieldPlannerCollector) EnterField(ref int) {
	path := c.walker.Path.DotDelimitedString() + "." + c.operation.FieldAliasOrNameString(ref)
	c.routes(cacheProvidesDataFragmentMarkerRegex.ReplaceAllString(path, ""), ref, c.fieldPlanners)
}
