package graphql_datasource

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestRequestScopedWidening_ViewerSubgraphPlanning(t *testing.T) {
	t.Parallel()

	t.Run("without requestScoped the root fetch stays narrow and the child fetch stays wide", func(t *testing.T) {
		t.Parallel()

		actual := planViewerScenario(t, requestScopedScenario{
			enableRequestScoped: false,
			operationSDL: `
				query Widening {
					currentViewer {
						id
						name
					}
					article {
						id
						title
						currentViewer {
							id
							name
							email
						}
					}
				}
			`,
		})

		expected := expectedViewerScenario(
			resolve.Sequence(
				rootFetch(0, "http://viewer.service", `
					{
						currentViewer {
							id
							name
						}
					}
				`),
				rootFetch(1, "http://articles.service", `
					{
						article {
							id
							title
							__typename
						}
					}
				`),
				entityFetch(2, 1, "article", "http://viewer.service", `
					query($representations: [_Any!]!) {
						_entities(representations: $representations) {
							... on Article {
								__typename
								currentViewer {
									id
									name
									email
								}
							}
						}
					}
				`),
			),
			rootObject(
				field("currentViewer", viewerObject(
					scalarField("id"),
					stringField("name"),
				)),
				field("article", articleObject(
					scalarField("id"),
					stringField("title"),
					field("currentViewer", viewerObject(
						scalarField("id"),
						stringField("name"),
						stringField("email"),
					)),
				)),
			),
			nil,
			nil,
		)

		assert.Equal(t, expected, actual)
	})

	t.Run("with requestScoped the root fetch widens and both fetches share the same loader mapping", func(t *testing.T) {
		t.Parallel()

		actual := planViewerScenario(t, requestScopedScenario{
			enableRequestScoped: true,
			operationSDL: `
				query Widening {
					currentViewer {
						id
						name
					}
					article {
						id
						title
						currentViewer {
							id
							name
							email
						}
					}
				}
			`,
		})

		expectedProvides := viewerProvides(
			providesScalarField("id"),
			providesScalarField("name"),
			providesScalarField("email"),
		)
		expected := expectedViewerScenario(
			resolve.Sequence(
				rootFetch(0, "http://viewer.service", `
					{
						currentViewer {
							id
							name
							email
						}
					}
				`, requestScopedField("currentViewer", expectedProvides)),
				rootFetch(1, "http://articles.service", `
					{
						article {
							id
							title
							__typename
						}
					}
				`),
				entityFetch(2, 1, "article", "http://viewer.service", `
					query($representations: [_Any!]!) {
						_entities(representations: $representations) {
							... on Article {
								__typename
								currentViewer {
									id
									name
									email
								}
							}
						}
					}
				`, requestScopedField("currentViewer", expectedProvides)),
			),
			rootObject(
				field("currentViewer", viewerObject(
					scalarField("id"),
					stringField("name"),
				)),
				field("article", articleObject(
					scalarField("id"),
					stringField("title"),
					field("currentViewer", viewerObject(
						scalarField("id"),
						stringField("name"),
						stringField("email"),
					)),
				)),
			),
			[]plannedRequestScopedContract{
				contract(0, "currentViewer", "viewer.currentViewer", "id", "id"),
				contract(0, "currentViewer", "viewer.currentViewer", "name", "name"),
				contract(0, "currentViewer", "viewer.currentViewer", "email", "email"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "id", "id"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "name", "name"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "email", "email"),
			},
			[]plannedResponseBinding{
				binding("currentViewer.id", "viewer.currentViewer", "id"),
				binding("currentViewer.name", "viewer.currentViewer", "name"),
				binding("article.currentViewer.id", "viewer.currentViewer", "id"),
				binding("article.currentViewer.name", "viewer.currentViewer", "name"),
				binding("article.currentViewer.email", "viewer.currentViewer", "email"),
			},
		)

		assert.Equal(t, expected, actual)
	})

	t.Run("field conflicts use synthetic aliases in the subgraph fetches while the response tree stays user-shaped", func(t *testing.T) {
		t.Parallel()

		actual := planViewerScenario(t, requestScopedScenario{
			enableRequestScoped: true,
			operationSDL: `
				query Widening {
					currentViewer {
						id
						name
					}
					article {
						id
						title
						currentViewer {
							id
							name: email
						}
					}
				}
			`,
		})

		expected := expectedViewerScenario(
			resolve.Sequence(
				rootFetch(0, "http://viewer.service", `
					{
						currentViewer {
							id
							__request_scoped__name_1: name
							__request_scoped__name_0: email
						}
					}
				`,
					requestScopedField("currentViewer", viewerProvides(
						providesScalarField("id"),
						providesAliasedScalarField("__request_scoped__name_1", "name"),
						providesAliasedScalarField("__request_scoped__name_0", "email"),
					)),
				),
				rootFetch(1, "http://articles.service", `
					{
						article {
							id
							title
							__typename
						}
					}
				`),
				entityFetch(2, 1, "article", "http://viewer.service", `
					query($representations: [_Any!]!) {
						_entities(representations: $representations) {
							... on Article {
								__typename
								currentViewer {
									id
									__request_scoped__name_0: email
									__request_scoped__name_1: name
								}
							}
						}
					}
				`,
					requestScopedField("currentViewer", viewerProvides(
						providesScalarField("id"),
						providesAliasedScalarField("__request_scoped__name_0", "email"),
						providesAliasedScalarField("__request_scoped__name_1", "name"),
					)),
				),
			),
			rootObject(
				field("currentViewer", viewerObject(
					scalarField("id"),
					stringFieldAt("name", "__request_scoped__name_1"),
				)),
				field("article", articleObject(
					scalarField("id"),
					stringField("title"),
					field("currentViewer", viewerObject(
						scalarField("id"),
						aliasedStringFieldAt("name", "email", "__request_scoped__name_0"),
					)),
				)),
			),
			[]plannedRequestScopedContract{
				contract(0, "currentViewer", "viewer.currentViewer", "id", "id"),
				contract(0, "currentViewer", "viewer.currentViewer", "__request_scoped__name_1", "name"),
				contract(0, "currentViewer", "viewer.currentViewer", "__request_scoped__name_0", "email"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "id", "id"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "__request_scoped__name_0", "email"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "__request_scoped__name_1", "name"),
			},
			[]plannedResponseBinding{
				binding("currentViewer.id", "viewer.currentViewer", "id"),
				binding("currentViewer.name", "viewer.currentViewer", "__request_scoped__name_1"),
				binding("article.currentViewer.id", "viewer.currentViewer", "id"),
				binding("article.currentViewer.name", "viewer.currentViewer", "__request_scoped__name_0"),
			},
		)

		assert.Equal(t, expected, actual)
	})

	t.Run("argument conflicts use synthetic aliases in fetches and cache-arg mappings in requestScoped provides data", func(t *testing.T) {
		t.Parallel()

		actual := planViewerScenario(t, requestScopedScenario{
			enableRequestScoped: true,
			operationSDL: `
				query Widening {
					currentViewer {
						id
						posts(first: 1) {
							id
						}
					}
					article {
						id
						title
						currentViewer {
							id
							posts(first: 2) {
								id
								title
							}
						}
					}
				}
			`,
		})

		expected := expectedViewerScenario(
			resolve.Sequence(
				rootFetch(0, "http://viewer.service", `
					query($a: Int!, $b: Int!) {
						currentViewer {
							id
							__request_scoped__posts_0: posts(first: $a) {
								id
							}
							__request_scoped__posts_1: posts(first: $b) {
								id
								title
							}
						}
					}
				`,
					requestScopedField("currentViewer", viewerProvides(
						providesScalarField("id"),
						providesArrayField("__request_scoped__posts_0", "posts", "a",
							postItemProvides(
								providesScalarField("id"),
							),
						),
						providesArrayField("__request_scoped__posts_1", "posts", "b",
							postItemProvides(
								providesScalarField("id"),
								providesScalarField("title"),
							),
						),
					)),
				),
				rootFetch(1, "http://articles.service", `
					{
						article {
							id
							title
							__typename
						}
					}
				`),
				entityFetch(2, 1, "article", "http://viewer.service", `
					query($representations: [_Any!]!, $b: Int!, $a: Int!) {
						_entities(representations: $representations) {
							... on Article {
								__typename
								currentViewer {
									id
									__request_scoped__posts_1: posts(first: $b) {
										id
										title
									}
									__request_scoped__posts_0: posts(first: $a) {
										id
									}
								}
							}
						}
					}
				`,
					requestScopedField("currentViewer", viewerProvides(
						providesScalarField("id"),
						providesArrayField("__request_scoped__posts_1", "posts", "b",
							postItemProvides(
								providesScalarField("id"),
								providesScalarField("title"),
							),
						),
						providesArrayField("__request_scoped__posts_0", "posts", "a",
							postItemProvides(
								providesScalarField("id"),
							),
						),
					)),
				),
			),
			rootObject(
				field("currentViewer", viewerObject(
					scalarField("id"),
					postsDataFieldAt("__request_scoped__posts_0",
						postItem(
							scalarField("id"),
						),
					),
				)),
				field("article", articleObject(
					scalarField("id"),
					stringField("title"),
					field("currentViewer", viewerObject(
						scalarField("id"),
						postsDataFieldAt("__request_scoped__posts_1",
							postItem(
								scalarField("id"),
								stringField("title"),
							),
						),
					)),
				)),
			),
			[]plannedRequestScopedContract{
				contract(0, "currentViewer", "viewer.currentViewer", "id", "id"),
				contract(0, "currentViewer", "viewer.currentViewer", "__request_scoped__posts_0", "posts", "first:a"),
				contract(0, "currentViewer", "viewer.currentViewer", "__request_scoped__posts_1", "posts", "first:b"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "id", "id"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "__request_scoped__posts_1", "posts", "first:b"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "__request_scoped__posts_0", "posts", "first:a"),
			},
			[]plannedResponseBinding{
				binding("currentViewer.id", "viewer.currentViewer", "id"),
				binding("currentViewer.posts", "viewer.currentViewer", "__request_scoped__posts_0"),
				binding("article.currentViewer.id", "viewer.currentViewer", "id"),
				binding("article.currentViewer.posts", "viewer.currentViewer", "__request_scoped__posts_1"),
			},
		)

		assert.Equal(t, expected, actual)
	})

	t.Run("requires-decorated fields widen through an aliased dependency without changing the user response", func(t *testing.T) {
		t.Parallel()

		// The root participant exposes name through a user alias, while a downstream
		// handle field on another subgraph requires the schema field name `name`.
		// Widening must preserve the user alias at the root while still planning the
		// hidden dependency fields needed for the later entity fetch.
		actual := planRequestScopedRequiresChainViewerScenario(t, true, `
			query Widening {
				currentViewer {
					viewerName: name
				}
				article {
					id
					title
					currentViewer {
						handle
					}
				}
			}
		`)
		rootExpectedProvides := viewerProvides(
			providesAliasedScalarField("viewerName", "name"),
			providesScalarField("__typename"),
			providesScalarField("id"),
		)
		entityExpectedProvides := viewerProvides(
			providesScalarField("name"),
			providesScalarField("__typename"),
			providesScalarField("id"),
		)
		expected := expectedViewerScenario(
			// The root viewer fetch is widened with the hidden fields that the later
			// handle entity fetch will need, but the response object still keeps only
			// the user-visible alias at the root.
			resolve.Sequence(
				rootFetch(0, "http://viewer.service", `
					{
						currentViewer {
							viewerName: name
							__typename
							id
						}
					}
				`, requestScopedField("currentViewer", rootExpectedProvides)),
				rootFetch(1, "http://articles.service", `
					{
						article {
							id
							title
							__typename
						}
					}
				`),
				entityFetch(2, 1, "article", "http://viewer.service", `
					query($representations: [_Any!]!) {
						_entities(representations: $representations) {
							... on Article {
								__typename
								currentViewer {
									name
									__typename
									id
								}
							}
						}
					}
				`, requestScopedField("currentViewer", entityExpectedProvides)),
				entityFetch(3, 2, "article.currentViewer", "http://handles.service", `
					query($representations: [_Any!]!) {
						_entities(representations: $representations) {
							... on Viewer {
								__typename
								handle
							}
						}
					}
				`),
			),
			rootObject(
				field("currentViewer", viewerObject(
					stringFieldAt("viewerName", "viewerName"),
				)),
				field("article", articleObject(
					scalarField("id"),
					stringField("title"),
					field("currentViewer", viewerObject(
						stringField("handle"),
					)),
				)),
			),
			[]plannedRequestScopedContract{
				contract(0, "currentViewer", "viewer.currentViewer", "__typename", "__typename"),
				contract(0, "currentViewer", "viewer.currentViewer", "viewerName", "name"),
				contract(0, "currentViewer", "viewer.currentViewer", "id", "id"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "__typename", "__typename"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "id", "id"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "name", "name"),
			},
			[]plannedResponseBinding{
				binding("currentViewer.viewerName", "viewer.currentViewer", "viewerName"),
				binding("article.currentViewer.handle", "viewer.currentViewer", "handle"),
			},
		)

		assert.Equal(t, expected, actual)
	})

	t.Run("requires-decorated field rewrites the first participant to include the hidden dependency", func(t *testing.T) {
		t.Parallel()

		// The first participant only asks for id, but the second participant asks for
		// handle on another subgraph, which requires `name` as an external field.
		// Widening therefore has to rewrite the first fetch to include the hidden
		// dependency field `name` even though the user did not ask for it there.
		actual := planRequestScopedRequiresChainViewerScenario(t, true, `
			query Widening {
				currentViewer {
					id
				}
				article {
					id
					title
					currentViewer {
						handle
					}
				}
			}
		`)

		rootExpectedProvides := viewerProvides(
			providesScalarField("id"),
			providesScalarField("__typename"),
			providesScalarField("name"),
		)
		entityExpectedProvides := viewerProvides(
			providesScalarField("name"),
			providesScalarField("__typename"),
			providesScalarField("id"),
		)
		expected := expectedViewerScenario(
			// The widened root fetch now carries id, __typename, and the hidden name
			// dependency so the later handles subgraph can be fed without a viewer hop.
			resolve.Sequence(
				rootFetch(0, "http://viewer.service", `
					{
						currentViewer {
							id
							__typename
							name
						}
					}
				`, requestScopedField("currentViewer", rootExpectedProvides)),
				rootFetch(1, "http://articles.service", `
					{
						article {
							id
							title
							__typename
						}
					}
				`),
				entityFetch(2, 1, "article", "http://viewer.service", `
					query($representations: [_Any!]!) {
						_entities(representations: $representations) {
							... on Article {
								__typename
								currentViewer {
									name
									__typename
									id
								}
							}
						}
					}
				`, requestScopedField("currentViewer", entityExpectedProvides)),
				entityFetch(3, 2, "article.currentViewer", "http://handles.service", `
					query($representations: [_Any!]!) {
						_entities(representations: $representations) {
							... on Viewer {
								__typename
								handle
							}
						}
					}
				`),
			),
			rootObject(
				field("currentViewer", viewerObject(
					scalarField("id"),
				)),
				field("article", articleObject(
					scalarField("id"),
					stringField("title"),
					field("currentViewer", viewerObject(
						stringField("handle"),
					)),
				)),
			),
			[]plannedRequestScopedContract{
				contract(0, "currentViewer", "viewer.currentViewer", "__typename", "__typename"),
				contract(0, "currentViewer", "viewer.currentViewer", "id", "id"),
				contract(0, "currentViewer", "viewer.currentViewer", "name", "name"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "__typename", "__typename"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "id", "id"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "name", "name"),
			},
			[]plannedResponseBinding{
				binding("currentViewer.id", "viewer.currentViewer", "id"),
				binding("article.currentViewer.handle", "viewer.currentViewer", "handle"),
			},
		)

		assert.Equal(t, expected, actual)
	})

	t.Run("three requestScoped participants widen to a common superset while keeping the user response unchanged", func(t *testing.T) {
		t.Parallel()

		actual := planViewerScenario(t, requestScopedScenario{
			enableRequestScoped: true,
			operationSDL: `
				query Widening {
					currentViewer {
						id
					}
					article {
						id
						title
						currentViewer {
							id
							name
						}
					}
					review {
						id
						body
						currentViewer {
							id
							name
							email
						}
					}
				}
			`,
		})

		expectedProvides := viewerProvides(
			providesScalarField("id"),
			providesScalarField("email"),
			providesScalarField("name"),
		)
		expected := expectedViewerScenario(
			resolve.Sequence(
				rootFetch(0, "http://viewer.service", `
					{
						currentViewer {
							id
							email
							name
						}
					}
				`, requestScopedField("currentViewer", expectedProvides)),
				rootFetch(1, "http://articles.service", `
					{
						article {
							id
							title
							__typename
						}
					}
				`),
				rootFetch(3, "http://reviews.service", `
					{
						review {
							id
							body
							__typename
						}
					}
				`),
				entityFetch(2, 1, "article", "http://viewer.service", `
					query($representations: [_Any!]!) {
						_entities(representations: $representations) {
							... on Article {
								__typename
								currentViewer {
									id
									name
									email
								}
							}
						}
					}
				`, requestScopedField("currentViewer", viewerProvides(
					providesScalarField("id"),
					providesScalarField("name"),
					providesScalarField("email"),
				))),
				entityFetch(4, 3, "review", "http://viewer.service", `
					query($representations: [_Any!]!) {
						_entities(representations: $representations) {
							... on Review {
								__typename
								currentViewer {
									id
									name
									email
								}
							}
						}
					}
				`, requestScopedField("currentViewer", viewerProvides(
					providesScalarField("id"),
					providesScalarField("name"),
					providesScalarField("email"),
				))),
			),
			rootObject(
				field("currentViewer", viewerObject(
					scalarField("id"),
				)),
				field("article", articleObject(
					scalarField("id"),
					stringField("title"),
					field("currentViewer", viewerObject(
						scalarField("id"),
						stringField("name"),
					)),
				)),
				field("review", reviewObject(
					scalarField("id"),
					stringField("body"),
					field("currentViewer", viewerObject(
						scalarField("id"),
						stringField("name"),
						stringField("email"),
					)),
				)),
			),
			[]plannedRequestScopedContract{
				contract(0, "currentViewer", "viewer.currentViewer", "id", "id"),
				contract(0, "currentViewer", "viewer.currentViewer", "email", "email"),
				contract(0, "currentViewer", "viewer.currentViewer", "name", "name"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "id", "id"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "name", "name"),
				contract(2, "article.currentViewer", "viewer.currentViewer", "email", "email"),
				contract(4, "review.currentViewer", "viewer.currentViewer", "id", "id"),
				contract(4, "review.currentViewer", "viewer.currentViewer", "name", "name"),
				contract(4, "review.currentViewer", "viewer.currentViewer", "email", "email"),
			},
			[]plannedResponseBinding{
				binding("currentViewer.id", "viewer.currentViewer", "id"),
				binding("article.currentViewer.id", "viewer.currentViewer", "id"),
				binding("article.currentViewer.name", "viewer.currentViewer", "name"),
				binding("review.currentViewer.id", "viewer.currentViewer", "id"),
				binding("review.currentViewer.name", "viewer.currentViewer", "name"),
				binding("review.currentViewer.email", "viewer.currentViewer", "email"),
			},
		)

		assert.Equal(t, expected, actual)
	})
}

type requestScopedScenario struct {
	enableRequestScoped bool
	operationSDL        string
}

type plannedViewerScenario struct {
	Plan             *plan.SynchronousResponsePlan
	RequestScoped    []plannedRequestScopedContract
	ResponseBindings []plannedResponseBinding
}

type plannedRequestScopedContract struct {
	FetchID          int
	ResponsePath     string
	L1Key            string
	RequestScopedKey string
	SchemaField      string
	CacheArgs        []string
}

type plannedResponseBinding struct {
	ResponsePath string
	L1Key        string
	CacheKey     string
}

func planViewerScenario(t *testing.T, scenario requestScopedScenario) plannedViewerScenario {
	t.Helper()

	planned := planRequestScopedWideningScenario(t, scenario.enableRequestScoped, scenario.operationSDL)
	return postprocessViewerScenario(t, planned)
}

func planRequestScopedRequiresChainViewerScenario(t *testing.T, enableRequestScoped bool, operationSDL string) plannedViewerScenario {
	t.Helper()

	planned := planRequestScopedRequiresChainScenario(t, enableRequestScoped, operationSDL)
	return postprocessViewerScenario(t, planned)
}

func postprocessViewerScenario(t *testing.T, planned plan.Plan) plannedViewerScenario {
	t.Helper()

	processor := postprocess.NewProcessor(
		postprocess.DisableResolveInputTemplates(),
		postprocess.DisableCreateConcreteSingleFetchTypes(),
		postprocess.DisableCreateParallelNodes(),
		postprocess.DisableMergeFields(),
	)
	processor.Process(planned)

	syncPlan, ok := planned.(*plan.SynchronousResponsePlan)
	require.True(t, ok)
	require.NotNil(t, syncPlan.Response)
	require.NotNil(t, syncPlan.Response.Fetches)
	require.NotNil(t, syncPlan.Response.Data)

	return projectViewerScenario(t, syncPlan)
}

func expectedViewerPlan(fetches *resolve.FetchTreeNode, data *resolve.Object) *plan.SynchronousResponsePlan {
	return &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Fetches: fetches,
			Data:    data,
		},
	}
}

func expectedViewerScenario(fetches *resolve.FetchTreeNode, data *resolve.Object, requestScoped []plannedRequestScopedContract, responseBindings []plannedResponseBinding) plannedViewerScenario {
	sortRequestScopedContracts(requestScoped)
	sortResponseBindings(responseBindings)
	return plannedViewerScenario{
		Plan:             expectedViewerPlan(fetches, data),
		RequestScoped:    requestScoped,
		ResponseBindings: responseBindings,
	}
}

func projectViewerScenario(t *testing.T, syncPlan *plan.SynchronousResponsePlan) plannedViewerScenario {
	t.Helper()

	plan := &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Fetches: normalizeFetchTree(t, syncPlan.Response.Fetches),
			Data:    normalizeObject(syncPlan.Response.Data),
		},
	}
	requestScoped := collectRequestScopedContracts(plan.Response.Fetches)
	responseBindings := collectResponseBindings(plan.Response.Fetches, plan.Response.Data)
	sortRequestScopedContracts(requestScoped)
	sortResponseBindings(responseBindings)
	return plannedViewerScenario{
		Plan:             plan,
		RequestScoped:    requestScoped,
		ResponseBindings: responseBindings,
	}
}

func objectAtPath(obj *resolve.Object, path []string) *resolve.Object {
	current := obj
	for _, segment := range path {
		if current == nil {
			return nil
		}

		var next resolve.Node
		for _, field := range current.Fields {
			if string(field.Name) == segment {
				next = field.Value
				break
			}
		}
		if next == nil {
			return nil
		}

		switch typed := next.(type) {
		case *resolve.Object:
			current = typed
		default:
			return nil
		}
	}
	return current
}

func collectRequestScopedContracts(fetchTree *resolve.FetchTreeNode) []plannedRequestScopedContract {
	var out []plannedRequestScopedContract
	walkFetchTree(fetchTree, func(fetch *resolve.SingleFetch, responsePath string) {
		for _, field := range fetch.Caching.RequestScopedFields {
			objectPath := joinPath(responsePath, strings.Join(field.FieldPath, "."))
			for _, providedField := range field.ProvidesData.Fields {
				out = append(out, plannedRequestScopedContract{
					FetchID:          fetch.FetchID,
					ResponsePath:     objectPath,
					L1Key:            field.L1Key,
					RequestScopedKey: string(providedField.Name),
					SchemaField:      providedField.SchemaFieldName(),
					CacheArgs:        cacheArgsStrings(providedField.CacheArgs),
				})
			}
		}
	})
	return out
}

func collectResponseBindings(fetchTree *resolve.FetchTreeNode, data *resolve.Object) []plannedResponseBinding {
	var out []plannedResponseBinding
	walkFetchTree(fetchTree, func(fetch *resolve.SingleFetch, responsePath string) {
		for _, field := range fetch.Caching.RequestScopedFields {
			objectPath := joinPath(responsePath, strings.Join(field.FieldPath, "."))
			responseObj := objectAtPath(data, strings.Split(objectPath, "."))
			if responseObj == nil {
				continue
			}
			for _, responseField := range responseObj.Fields {
				nodePath := responseField.Value.NodePath()
				if len(nodePath) == 0 {
					continue
				}
				out = append(out, plannedResponseBinding{
					ResponsePath: joinPath(objectPath, string(responseField.Name)),
					L1Key:        field.L1Key,
					CacheKey:     nodePath[0],
				})
			}
		}
	})
	return out
}

func walkFetchTree(node *resolve.FetchTreeNode, visit func(fetch *resolve.SingleFetch, responsePath string)) {
	if node == nil {
		return
	}
	if node.Item != nil {
		if fetch, ok := node.Item.Fetch.(*resolve.SingleFetch); ok {
			visit(fetch, node.Item.ResponsePath)
		}
	}
	for _, child := range node.ChildNodes {
		walkFetchTree(child, visit)
	}
}

func joinPath(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, ".")
}

func cacheArgsStrings(args []resolve.CacheFieldArg) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for _, arg := range args {
		out = append(out, fmt.Sprintf("%s:%s", arg.ArgName, arg.VariableName))
	}
	return out
}

func sortRequestScopedContracts(contracts []plannedRequestScopedContract) {
	sort.Slice(contracts, func(i, j int) bool {
		if contracts[i].FetchID != contracts[j].FetchID {
			return contracts[i].FetchID < contracts[j].FetchID
		}
		if contracts[i].ResponsePath != contracts[j].ResponsePath {
			return contracts[i].ResponsePath < contracts[j].ResponsePath
		}
		return contracts[i].RequestScopedKey < contracts[j].RequestScopedKey
	})
}

func sortResponseBindings(bindings []plannedResponseBinding) {
	sort.Slice(bindings, func(i, j int) bool {
		return bindings[i].ResponsePath < bindings[j].ResponsePath
	})
}

func normalizeFetchTree(t *testing.T, node *resolve.FetchTreeNode) *resolve.FetchTreeNode {
	t.Helper()

	if node == nil {
		return nil
	}

	out := &resolve.FetchTreeNode{
		Kind: node.Kind,
	}
	if node.Item != nil {
		singleFetch, ok := node.Item.Fetch.(*resolve.SingleFetch)
		require.True(t, ok, "expected *resolve.SingleFetch, got %T", node.Item.Fetch)
		item := &resolve.FetchItem{
			Fetch:                normalizeSingleFetch(t, singleFetch),
			ResponsePath:         node.Item.ResponsePath,
			ResponsePathElements: append([]string(nil), node.Item.ResponsePathElements...),
		}
		if len(node.Item.FetchPath) > 0 {
			item.FetchPath = append([]resolve.FetchItemPathElement(nil), node.Item.FetchPath...)
		}
		out.Item = item
	}
	if len(node.ChildNodes) > 0 {
		out.ChildNodes = make([]*resolve.FetchTreeNode, 0, len(node.ChildNodes))
		for _, child := range node.ChildNodes {
			out.ChildNodes = append(out.ChildNodes, normalizeFetchTree(t, child))
		}
	}
	return out
}

func normalizeSingleFetch(t *testing.T, fetch *resolve.SingleFetch) *resolve.SingleFetch {
	t.Helper()

	return &resolve.SingleFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           fetch.FetchID,
			DependsOnFetchIDs: append([]int(nil), fetch.DependsOnFetchIDs...),
		},
		DataSourceIdentifier: append([]byte(nil), fetch.DataSourceIdentifier...),
		FetchConfiguration: resolve.FetchConfiguration{
			Input:                                 normalizeFetchInput(t, fetch.Input),
			DataSource:                            &Source{},
			RequiresEntityFetch:                   fetch.RequiresEntityFetch,
			RequiresEntityBatchFetch:              fetch.RequiresEntityBatchFetch,
			PostProcessing:                        fetch.PostProcessing,
			SetTemplateOutputToNullOnVariableNull: fetch.SetTemplateOutputToNullOnVariableNull,
			Caching: resolve.FetchCacheConfiguration{
				RequestScopedFields: normalizeRequestScopedFields(fetch.Caching.RequestScopedFields),
			},
		},
	}
}

func normalizeRequestScopedFields(fields []resolve.RequestScopedField) []resolve.RequestScopedField {
	if len(fields) == 0 {
		return nil
	}
	out := make([]resolve.RequestScopedField, 0, len(fields))
	for _, field := range fields {
		out = append(out, resolve.RequestScopedField{
			FieldName:    field.FieldName,
			FieldPath:    append([]string(nil), field.FieldPath...),
			L1Key:        field.L1Key,
			ProvidesData: normalizeObject(field.ProvidesData),
		})
	}
	return out
}

func normalizeObject(obj *resolve.Object) *resolve.Object {
	if obj == nil {
		return nil
	}
	fields := make([]*resolve.Field, 0, len(obj.Fields))
	for _, field := range obj.Fields {
		fields = append(fields, normalizeField(field))
	}
	return &resolve.Object{
		Nullable:   obj.Nullable,
		Path:       append([]string(nil), obj.Path...),
		Fields:     fields,
		HasAliases: obj.HasAliases,
	}
}

func normalizeField(field *resolve.Field) *resolve.Field {
	if field == nil {
		return nil
	}
	out := &resolve.Field{
		Name:      append([]byte(nil), field.Name...),
		Value:     normalizeNode(field.Value),
		CacheArgs: append([]resolve.CacheFieldArg(nil), field.CacheArgs...),
	}
	if field.OriginalName != nil {
		out.OriginalName = append([]byte(nil), field.OriginalName...)
	}
	return out
}

func normalizeNode(node resolve.Node) resolve.Node {
	switch n := node.(type) {
	case *resolve.Object:
		return normalizeObject(n)
	case *resolve.Array:
		return &resolve.Array{
			Path:     append([]string(nil), n.Path...),
			Nullable: n.Nullable,
			Item:     normalizeNode(n.Item),
		}
	case *resolve.String:
		return &resolve.String{
			Path:     append([]string(nil), n.Path...),
			Nullable: n.Nullable,
		}
	case *resolve.Scalar:
		return &resolve.Scalar{
			Path:     append([]string(nil), n.Path...),
			Nullable: n.Nullable,
		}
	case *resolve.Integer:
		return &resolve.Integer{
			Path:     append([]string(nil), n.Path...),
			Nullable: n.Nullable,
		}
	case *resolve.Float:
		return &resolve.Float{
			Path:     append([]string(nil), n.Path...),
			Nullable: n.Nullable,
		}
	case *resolve.Boolean:
		return &resolve.Boolean{
			Path:     append([]string(nil), n.Path...),
			Nullable: n.Nullable,
		}
	case *resolve.BigInt:
		return &resolve.BigInt{
			Path:     append([]string(nil), n.Path...),
			Nullable: n.Nullable,
		}
	case *resolve.StaticString:
		return &resolve.StaticString{
			Path: n.Path,
		}
	default:
		panic(fmt.Sprintf("unsupported resolve node type %T", node))
	}
}

func normalizeFetchInput(t *testing.T, input string) string {
	t.Helper()

	url := extractFetchInputField(t, input, "url")
	query := extractQueryFromFetchInput(t, input)

	return graphqlInput(url, query)
}

func extractFetchInputField(t *testing.T, input, key string) string {
	t.Helper()

	match := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `":"((?:\\.|[^"])*)"`).FindStringSubmatch(input)
	require.Len(t, match, 2, input)

	value, err := strconv.Unquote(`"` + match[1] + `"`)
	require.NoError(t, err)

	return value
}

func extractQueryFromFetchInput(t *testing.T, input string) string {
	t.Helper()

	match := regexp.MustCompile(`"query":"((?:\\.|[^"])*)"`).FindStringSubmatch(input)
	require.Len(t, match, 2, input)

	query, err := strconv.Unquote(`"` + match[1] + `"`)
	require.NoError(t, err)
	require.NotEmpty(t, query)

	return query
}

func graphqlInput(url, query string) string {
	return fmt.Sprintf(
		`{"method":"POST","url":%s,"body":{"query":%s}}`,
		strconv.Quote(url),
		strconv.Quote(unsafeprinter.Prettify(query)),
	)
}

func rootFetch(fetchID int, url, query string, requestScopedFields ...resolve.RequestScopedField) *resolve.FetchTreeNode {
	return resolve.Single(&resolve.SingleFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID: fetchID,
		},
		DataSourceIdentifier: []byte("graphql_datasource.Source"),
		FetchConfiguration: resolve.FetchConfiguration{
			Input:          graphqlInput(url, query),
			DataSource:     &Source{},
			PostProcessing: DefaultPostProcessingConfiguration,
			Caching: resolve.FetchCacheConfiguration{
				RequestScopedFields: requestScopedFields,
			},
		},
	})
}

func entityFetch(fetchID int, dependsOnFetchID int, responsePath, url, query string, requestScopedFields ...resolve.RequestScopedField) *resolve.FetchTreeNode {
	return resolve.SingleWithPath(&resolve.SingleFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           fetchID,
			DependsOnFetchIDs: []int{dependsOnFetchID},
		},
		DataSourceIdentifier: []byte("graphql_datasource.Source"),
		FetchConfiguration: resolve.FetchConfiguration{
			Input:                                 graphqlInput(url, query),
			DataSource:                            &Source{},
			RequiresEntityFetch:                   true,
			PostProcessing:                        SingleEntityPostProcessingConfiguration,
			SetTemplateOutputToNullOnVariableNull: true,
			Caching: resolve.FetchCacheConfiguration{
				RequestScopedFields: requestScopedFields,
			},
		},
	}, responsePath, entityFetchPath(responsePath)...)
}

func entityFetchPath(responsePath string) []resolve.FetchItemPathElement {
	if responsePath == "" {
		return nil
	}

	segments := strings.Split(responsePath, ".")
	path := make([]resolve.FetchItemPathElement, 0, len(segments))
	for _, segment := range segments {
		path = append(path, resolve.ObjectPath(segment))
	}
	return path
}

func requestScopedField(fieldName string, providesData *resolve.Object) resolve.RequestScopedField {
	return resolve.RequestScopedField{
		FieldName:    fieldName,
		FieldPath:    []string{fieldName},
		L1Key:        "viewer.currentViewer",
		ProvidesData: providesData,
	}
}

func rootObject(fields ...*resolve.Field) *resolve.Object {
	return &resolve.Object{
		Fields: fields,
	}
}

func viewerObject(fields ...*resolve.Field) *resolve.Object {
	return &resolve.Object{
		Nullable: true,
		Path:     []string{"currentViewer"},
		Fields:   fields,
	}
}

func articleObject(fields ...*resolve.Field) *resolve.Object {
	return &resolve.Object{
		Path:   []string{"article"},
		Fields: fields,
	}
}

func reviewObject(fields ...*resolve.Field) *resolve.Object {
	return &resolve.Object{
		Path:   []string{"review"},
		Fields: fields,
	}
}

func field(name string, value resolve.Node) *resolve.Field {
	return &resolve.Field{
		Name:  []byte(name),
		Value: value,
	}
}

func scalarField(name string) *resolve.Field {
	return scalarFieldAt(name, name)
}

func scalarFieldAt(name, path string) *resolve.Field {
	return &resolve.Field{
		Name: []byte(name),
		Value: &resolve.Scalar{
			Path: []string{path},
		},
	}
}

func stringField(name string) *resolve.Field {
	return stringFieldAt(name, name)
}

func stringFieldAt(name, path string) *resolve.Field {
	return &resolve.Field{
		Name: []byte(name),
		Value: &resolve.String{
			Path: []string{path},
		},
	}
}

func aliasedStringFieldAt(name, originalName, path string) *resolve.Field {
	return &resolve.Field{
		Name:         []byte(name),
		OriginalName: []byte(originalName),
		Value: &resolve.String{
			Path: []string{path},
		},
	}
}

func postsDataField(item *resolve.Object) *resolve.Field {
	return postsDataFieldAt("posts", item)
}

func postsDataFieldAt(path string, item *resolve.Object) *resolve.Field {
	return &resolve.Field{
		Name: []byte("posts"),
		Value: &resolve.Array{
			Path: []string{path},
			Item: item,
		},
	}
}

func postItem(fields ...*resolve.Field) *resolve.Object {
	return &resolve.Object{
		Fields: fields,
	}
}

func viewerProvides(fields ...*resolve.Field) *resolve.Object {
	obj := &resolve.Object{
		Nullable: true,
		Path:     []string{"currentViewer"},
		Fields:   fields,
	}
	resolve.ComputeHasAliases(obj)
	return obj
}

func postItemProvides(fields ...*resolve.Field) *resolve.Object {
	obj := &resolve.Object{
		Fields: fields,
	}
	resolve.ComputeHasAliases(obj)
	return obj
}

func providesScalarField(name string) *resolve.Field {
	return &resolve.Field{
		Name: []byte(name),
		Value: &resolve.Scalar{
			Path: []string{name},
		},
	}
}

func providesAliasedScalarField(name, originalName string) *resolve.Field {
	return &resolve.Field{
		Name:         []byte(name),
		OriginalName: []byte(originalName),
		Value: &resolve.Scalar{
			Path: []string{name},
		},
	}
}

func providesArrayField(name, originalName, variableName string, item *resolve.Object) *resolve.Field {
	field := &resolve.Field{
		Name: []byte(name),
		Value: &resolve.Array{
			Path: []string{name},
			Item: item,
		},
		CacheArgs: []resolve.CacheFieldArg{
			{
				ArgName:      "first",
				VariableName: variableName,
			},
		},
	}
	if originalName != "" {
		field.OriginalName = []byte(originalName)
	}
	return field
}

func contract(fetchID int, responsePath, l1Key, requestScopedKey, schemaField string, cacheArgs ...string) plannedRequestScopedContract {
	return plannedRequestScopedContract{
		FetchID:          fetchID,
		ResponsePath:     responsePath,
		L1Key:            l1Key,
		RequestScopedKey: requestScopedKey,
		SchemaField:      schemaField,
		CacheArgs:        cacheArgs,
	}
}

func binding(responsePath, l1Key, cacheKey string) plannedResponseBinding {
	return plannedResponseBinding{
		ResponsePath: responsePath,
		L1Key:        l1Key,
		CacheKey:     cacheKey,
	}
}

func planRequestScopedWideningScenario(t *testing.T, enableRequestScoped bool, operationSDL string) plan.Plan {
	t.Helper()

	const definitionSDL = `
		directive @tag(label: String!) on FIELD

		schema { query: Query }

		type Query {
			currentViewer: Viewer
			article: Article!
			review: Review!
		}

		type Viewer {
			id: ID!
			name: String!
			email: String!
			handle: String!
			posts(first: Int!): [Post!]!
		}

		type Post {
			id: ID!
			title: String!
		}

		type Article {
			id: ID!
			title: String!
			currentViewer: Viewer
		}

		type Review {
			id: ID!
			body: String!
			currentViewer: Viewer
		}
	`

	def := unsafeparser.ParseGraphqlDocumentString(definitionSDL)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&def))

	op := unsafeparser.ParseGraphqlDocumentString(operationSDL)
	report := &operationreport.Report{}

	normalizer := astnormalization.NewWithOpts(
		astnormalization.WithExtractVariables(),
		astnormalization.WithInlineFragmentSpreads(),
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveUnusedVariables(),
	)
	normalizer.NormalizeOperation(&op, &def, report)
	require.False(t, report.HasErrors(), report.Error())

	validator := astvalidation.DefaultOperationValidator()
	validator.Validate(&op, &def, report)
	require.False(t, report.HasErrors(), report.Error())

	plannerInstance, err := plan.NewPlanner(plan.Configuration{
		DataSources:                  buildRequestScopedWideningDataSources(t, enableRequestScoped),
		DisableResolveFieldPositions: true,
		DisableEntityCaching:         true,
		Fields: plan.FieldConfigurations{
			{
				TypeName:  "Viewer",
				FieldName: "posts",
				Arguments: plan.ArgumentsConfigurations{
					{
						Name:       "first",
						SourceType: plan.FieldArgumentSource,
						SourcePath: []string{"first"},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	result := plannerInstance.Plan(&op, &def, "Widening", report)
	require.False(t, report.HasErrors(), report.Error())

	return result
}

func buildRequestScopedWideningDataSources(t *testing.T, enableRequestScoped bool) []plan.DataSource {
	t.Helper()

	const viewerSDL = `
		directive @tag(label: String!) on FIELD

		type Query {
			currentViewer: Viewer
		}

		type Article @key(fields: "id") {
			id: ID!
			currentViewer: Viewer
		}

		type Review @key(fields: "id") {
			id: ID!
			currentViewer: Viewer
		}

		type Viewer @key(fields: "id") {
			id: ID!
			name: String!
			email: String!
			handle: String!
			posts(first: Int!): [Post!]!
		}

		type Post {
			id: ID!
			title: String!
		}
	`

	const articlesSDL = `
		type Query {
			article: Article!
		}

		type Article @key(fields: "id") {
			id: ID!
			title: String!
		}
	`

	const reviewsSDL = `
		type Query {
			review: Review!
		}

		type Review @key(fields: "id") {
			id: ID!
			body: String!
		}
	`

	viewerMetadata := &plan.DataSourceMetadata{
		RootNodes: []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"currentViewer"}},
			{TypeName: "Article", FieldNames: []string{"id", "currentViewer"}},
			{TypeName: "Review", FieldNames: []string{"id", "currentViewer"}},
		},
		ChildNodes: []plan.TypeField{
			{TypeName: "Viewer", FieldNames: []string{"id", "name", "email", "handle", "posts"}},
			{TypeName: "Post", FieldNames: []string{"id", "title"}},
		},
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{TypeName: "Viewer", SelectionSet: "id"},
				{TypeName: "Article", SelectionSet: "id"},
				{TypeName: "Review", SelectionSet: "id"},
			},
		},
	}
	if enableRequestScoped {
		viewerMetadata.FederationMetaData.RequestScopedFields = []plan.RequestScopedField{
			{TypeName: "Query", FieldName: "currentViewer", L1Key: "viewer.currentViewer"},
			{TypeName: "Article", FieldName: "currentViewer", L1Key: "viewer.currentViewer"},
			{TypeName: "Review", FieldName: "currentViewer", L1Key: "viewer.currentViewer"},
		}
	}

	articlesMetadata := &plan.DataSourceMetadata{
		RootNodes: []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"article"}},
		},
		ChildNodes: []plan.TypeField{
			{TypeName: "Article", FieldNames: []string{"id", "title"}},
		},
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{TypeName: "Article", SelectionSet: "id"},
			},
		},
	}

	reviewsMetadata := &plan.DataSourceMetadata{
		RootNodes: []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"review"}},
		},
		ChildNodes: []plan.TypeField{
			{TypeName: "Review", FieldNames: []string{"id", "body"}},
		},
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{TypeName: "Review", SelectionSet: "id"},
			},
		},
	}

	viewerConfiguration := mustCustomConfiguration(t, ConfigurationInput{
		Fetch: &FetchConfiguration{
			URL: "http://viewer.service",
		},
		SchemaConfiguration: mustSchema(t, &FederationConfiguration{
			Enabled:    true,
			ServiceSDL: viewerSDL,
		}, viewerSDL),
	})

	articlesConfiguration := mustCustomConfiguration(t, ConfigurationInput{
		Fetch: &FetchConfiguration{
			URL: "http://articles.service",
		},
		SchemaConfiguration: mustSchema(t, &FederationConfiguration{
			Enabled:    true,
			ServiceSDL: articlesSDL,
		}, articlesSDL),
	})

	reviewsConfiguration := mustCustomConfiguration(t, ConfigurationInput{
		Fetch: &FetchConfiguration{
			URL: "http://reviews.service",
		},
		SchemaConfiguration: mustSchema(t, &FederationConfiguration{
			Enabled:    true,
			ServiceSDL: reviewsSDL,
		}, reviewsSDL),
	})

	return []plan.DataSource{
		mustDataSourceConfiguration(t, "viewer", viewerMetadata, viewerConfiguration),
		mustDataSourceConfiguration(t, "articles", articlesMetadata, articlesConfiguration),
		mustDataSourceConfiguration(t, "reviews", reviewsMetadata, reviewsConfiguration),
	}
}

func planRequestScopedRequiresChainScenario(t *testing.T, enableRequestScoped bool, operationSDL string) plan.Plan {
	t.Helper()

	const definitionSDL = `
		directive @tag(label: String!) on FIELD

		schema { query: Query }

		type Query {
			currentViewer: Viewer
			article: Article!
		}

		type Viewer {
			id: ID!
			name: String!
			handle: String!
		}

		type Article {
			id: ID!
			title: String!
			currentViewer: Viewer
		}
	`

	def := unsafeparser.ParseGraphqlDocumentString(definitionSDL)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&def))

	op := unsafeparser.ParseGraphqlDocumentString(operationSDL)
	report := &operationreport.Report{}

	normalizer := astnormalization.NewWithOpts(
		astnormalization.WithExtractVariables(),
		astnormalization.WithInlineFragmentSpreads(),
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveUnusedVariables(),
	)
	normalizer.NormalizeOperation(&op, &def, report)
	require.False(t, report.HasErrors(), report.Error())

	validator := astvalidation.DefaultOperationValidator()
	validator.Validate(&op, &def, report)
	require.False(t, report.HasErrors(), report.Error())

	plannerInstance, err := plan.NewPlanner(plan.Configuration{
		DataSources:                  buildRequestScopedRequiresChainDataSources(t, enableRequestScoped),
		DisableResolveFieldPositions: true,
		DisableEntityCaching:         true,
	})
	require.NoError(t, err)

	result := plannerInstance.Plan(&op, &def, "Widening", report)
	require.False(t, report.HasErrors(), report.Error())

	return result
}

func buildRequestScopedRequiresChainDataSources(t *testing.T, enableRequestScoped bool) []plan.DataSource {
	t.Helper()

	const viewerSDL = `
		type Query {
			currentViewer: Viewer
		}

		type Article @key(fields: "id") {
			id: ID!
			currentViewer: Viewer
		}

		type Viewer @key(fields: "id") {
			id: ID!
			name: String!
		}
	`

	const articlesSDL = `
		type Query {
			article: Article!
		}

		type Article @key(fields: "id") {
			id: ID!
			title: String!
		}
	`

	const handlesSDL = `
		directive @external on FIELD_DEFINITION
		directive @requires(fields: String!) on FIELD_DEFINITION

		type Viewer @key(fields: "id") {
			id: ID! @external
			name: String! @external
			handle: String! @requires(fields: "name")
		}
	`

	viewerMetadata := &plan.DataSourceMetadata{
		RootNodes: []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"currentViewer"}},
			{TypeName: "Article", FieldNames: []string{"id", "currentViewer"}},
			{TypeName: "Viewer", FieldNames: []string{"id", "name"}},
		},
		ChildNodes: []plan.TypeField{
			{TypeName: "Viewer", FieldNames: []string{"id", "name"}},
		},
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{TypeName: "Viewer", SelectionSet: "id"},
				{TypeName: "Article", SelectionSet: "id"},
			},
		},
	}
	if enableRequestScoped {
		viewerMetadata.FederationMetaData.RequestScopedFields = []plan.RequestScopedField{
			{TypeName: "Query", FieldName: "currentViewer", L1Key: "viewer.currentViewer"},
			{TypeName: "Article", FieldName: "currentViewer", L1Key: "viewer.currentViewer"},
		}
	}

	articlesMetadata := &plan.DataSourceMetadata{
		RootNodes: []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"article"}},
		},
		ChildNodes: []plan.TypeField{
			{TypeName: "Article", FieldNames: []string{"id", "title"}},
		},
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{TypeName: "Article", SelectionSet: "id"},
			},
		},
	}

	handlesMetadata := &plan.DataSourceMetadata{
		RootNodes: []plan.TypeField{
			{TypeName: "Viewer", FieldNames: []string{"id", "handle"}, ExternalFieldNames: []string{"name"}},
		},
		ChildNodes: []plan.TypeField{
			{TypeName: "Viewer", FieldNames: []string{"id", "handle"}, ExternalFieldNames: []string{"name"}},
		},
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{TypeName: "Viewer", SelectionSet: "id"},
			},
			Requires: plan.FederationFieldConfigurations{
				{TypeName: "Viewer", FieldName: "handle", SelectionSet: "name"},
			},
		},
	}

	viewerConfiguration := mustCustomConfiguration(t, ConfigurationInput{
		Fetch: &FetchConfiguration{URL: "http://viewer.service"},
		SchemaConfiguration: mustSchema(t, &FederationConfiguration{
			Enabled:    true,
			ServiceSDL: viewerSDL,
		}, viewerSDL),
	})

	articlesConfiguration := mustCustomConfiguration(t, ConfigurationInput{
		Fetch: &FetchConfiguration{URL: "http://articles.service"},
		SchemaConfiguration: mustSchema(t, &FederationConfiguration{
			Enabled:    true,
			ServiceSDL: articlesSDL,
		}, articlesSDL),
	})

	handlesConfiguration := mustCustomConfiguration(t, ConfigurationInput{
		Fetch: &FetchConfiguration{URL: "http://handles.service"},
		SchemaConfiguration: mustSchema(t, &FederationConfiguration{
			Enabled:    true,
			ServiceSDL: handlesSDL,
		}, handlesSDL),
	})

	return []plan.DataSource{
		mustDataSourceConfiguration(t, "viewer", viewerMetadata, viewerConfiguration),
		mustDataSourceConfiguration(t, "articles", articlesMetadata, articlesConfiguration),
		mustDataSourceConfiguration(t, "handles", handlesMetadata, handlesConfiguration),
	}
}
