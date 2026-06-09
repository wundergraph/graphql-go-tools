package grpcdatasource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc/metadata"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1/productv1connect"
)

// setupTestConnectServer starts an httptest server backed by the
// MockServiceConnect adapter (the gRPC MockService wrapped onto the
// ConnectRPC handler interface). The server speaks Connect, gRPC, and
// gRPC-Web on the same H2C endpoint, but for these tests we drive it via
// the Connect transport.
//
// Returns a base URL that can be passed to NewConnectTransport.
func setupTestConnectServer(t testing.TB) (baseURL string, cleanup func()) {
	t.Helper()

	mock := &grpctest.MockService{}
	connectImpl := grpctest.NewMockServiceConnect(mock)

	mux := http.NewServeMux()
	mux.Handle(productv1connect.NewProductServiceHandler(connectImpl))

	srv := httptest.NewUnstartedServer(h2c.NewHandler(mux, &http2.Server{}))
	srv.EnableHTTP2 = true
	srv.Start()

	cleanup = srv.Close
	return srv.URL, cleanup
}

// Test_DataSource_Load_WithMockServiceConnect mirrors the gRPC end-to-end
// happy path (Test_DataSource_Load_WithMockService) but routes the call
// through the Connect transport instead of the gRPC client connection.
// It proves that the data source pipeline (compiler -> JSON builder ->
// transport -> response unmarshal) works for the Connect protocol against
// the same MockService implementation. Runs the same query under both
// the protobuf and JSON wire formats so the two encoders are exercised
// from the very first happy path.
func Test_DataSource_Load_WithMockServiceConnect(t *testing.T) {
	baseURL, cleanup := setupTestConnectServer(t)
	t.Cleanup(cleanup)

	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	vars := `{"variables":{"filter":{"filter":{"name":"Test Product","filterField1":"filterField1","filterField2":"filterField2"}}}}`

	type response struct {
		Data struct {
			ComplexFilterType []struct {
				Id   string `json:"id"`
				Name string `json:"name"`
			} `json:"complexFilterType"`
		} `json:"data"`
	}

	for _, encoding := range []ConnectEncoding{ConnectEncodingProtobuf, ConnectEncodingJSON} {
		t.Run(string(encoding), func(t *testing.T) {
			output := loadConnectQuery(t, connectE2E{
				BaseURL:  baseURL,
				Encoding: encoding,
				Query:    query,
				Vars:     vars,
			})

			var resp response
			require.NoError(t, json.Unmarshal(output, &resp))
			require.NotEmpty(t, resp.Data.ComplexFilterType, "response should contain at least one item; empty slice would otherwise panic on index below")
			require.Equal(t, "test-id-123", resp.Data.ComplexFilterType[0].Id)
			require.Equal(t, "Test Product", resp.Data.ComplexFilterType[0].Name)
		})
	}
}

// connectE2E bundles the per-test inputs for the table-driven Connect e2e
// tests below. The zero value of Ctx falls back to context.Background(),
// of Encoding to ConnectEncodingProtobuf, and Headers/FederationConfigs
// to nil; callers only set the knobs that matter for the case at hand.
type connectE2E struct {
	BaseURL           string
	Query             string
	Vars              string
	Ctx               context.Context
	Headers           http.Header
	Encoding          ConnectEncoding
	FederationConfigs plan.FederationFieldConfigurations
}

// loadConnectQuery runs a GraphQL query through a DataSource that dials a
// ConnectRPC server backed by the MockService. The helper exists so the
// table-driven Connect tests below can focus on query/mapping/validation
// without re-stating the planner/transport scaffolding.
func loadConnectQuery(t *testing.T, opts connectE2E) []byte {
	t.Helper()

	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	if opts.Encoding == "" {
		opts.Encoding = ConnectEncodingProtobuf
	}

	schemaDoc := grpctest.MustGraphQLSchema(t)
	queryDoc, report := astparser.ParseGraphqlDocumentString(opts.Query)
	require.False(t, report.HasErrors(), "failed to parse query: %s", report.Error())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	transport := NewConnectTransport(ConnectTransportConfig{
		BaseURL:  opts.BaseURL,
		Encoding: opts.Encoding,
	})

	ds, err := NewDataSource(transport, DataSourceConfig{
		Operation:         &queryDoc,
		Definition:        &schemaDoc,
		SubgraphName:      "Products",
		Mapping:           testMapping(),
		Compiler:          compiler,
		FederationConfigs: opts.FederationConfigs,
	})
	require.NoError(t, err)

	input := fmt.Sprintf(`{"query":%q,"body":%s}`, opts.Query, opts.Vars)
	output, err := ds.Load(opts.Ctx, opts.Headers, []byte(input))
	require.NoError(t, err)
	return output
}

// Test_DataSource_Load_WithMockServiceConnect_UnionTypes mirrors the
// inline-fragment / union cases covered by Test_Datasource_Load_WithUnionTypes
// in grpc_datasource_test.go, but exercises them over the Connect transport.
// The planner emits one Connect RPC for the parent selection and folds the
// inline-fragment branches into the response shape; this test pins that
// folding still produces a typed __typename + branch fields when the wire
// is Connect rather than native gRPC.
func Test_DataSource_Load_WithMockServiceConnect_UnionTypes(t *testing.T) {
	baseURL, cleanup := setupTestConnectServer(t)
	t.Cleanup(cleanup)

	type graphqlResponse struct {
		Data   map[string]any `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}

	testCases := []struct {
		name     string
		query    string
		vars     string
		validate func(t *testing.T, data map[string]any)
	}{
		{
			name:  "random search result (single union value)",
			query: `query { randomSearchResult { __typename ... on Product { id name price } ... on User { id name } ... on Category { id name kind } } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]any) {
				searchResult, ok := data["randomSearchResult"].(map[string]any)
				require.True(t, ok, "randomSearchResult should be an object")
				require.NotEmpty(t, searchResult)
				require.Contains(t, searchResult, "__typename")
				switch typeName := searchResult["__typename"].(string); typeName {
				case "Product":
					require.Equal(t, "product-random-1", searchResult["id"])
					require.Equal(t, "Random Product", searchResult["name"])
					require.Equal(t, 29.99, searchResult["price"])
				case "User":
					require.Equal(t, "user-random-1", searchResult["id"])
					require.Equal(t, "Random User", searchResult["name"])
				case "Category":
					require.Equal(t, "category-random-1", searchResult["id"])
					require.Equal(t, "Random Category", searchResult["name"])
					require.Equal(t, "ELECTRONICS", searchResult["kind"])
				default:
					t.Fatalf("Unexpected __typename: %s", typeName)
				}
			},
		},
		{
			name:  "search returns mixed union values",
			query: `query($input: SearchInput!) { search(input: $input) { __typename ... on Product { id name price } ... on User { id name } ... on Category { id name kind } } }`,
			vars:  `{"variables":{"input":{"query":"test","limit":6}}}`,
			validate: func(t *testing.T, data map[string]any) {
				results, ok := data["search"].([]any)
				require.True(t, ok, "search should be an array")
				require.Len(t, results, 6)

				seen := map[string]int{}
				for _, item := range results {
					obj, ok := item.(map[string]any)
					require.True(t, ok)
					require.Contains(t, obj, "__typename")
					seen[obj["__typename"].(string)]++
				}
				// The mock seeds at least one of each union member when limit ≥ 3,
				// so we assert breadth instead of pinning every position.
				require.NotZero(t, seen["Product"], "expected at least one Product")
				require.NotZero(t, seen["User"], "expected at least one User")
				require.NotZero(t, seen["Category"], "expected at least one Category")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output := loadConnectQuery(t, connectE2E{BaseURL: baseURL, Query: tc.query, Vars: tc.vars})

			var resp graphqlResponse
			require.NoError(t, json.Unmarshal(output, &resp))
			require.Empty(t, resp.Errors, "Response should not contain errors: %s", string(output))
			require.NotEmpty(t, resp.Data)
			tc.validate(t, resp.Data)
		})
	}
}

// Test_DataSource_Load_WithMockServiceConnect_FieldResolvers exercises
// fields whose resolution requires a separate RPC after the parent loads
// (popularityScore, categoryMetrics) over the Connect transport. The
// planner fans the request into multiple Connect calls; this test pins
// that the data source still merges them into a single GraphQL response,
// matching the gRPC behaviour covered in grpc_datasource_test.go.
func Test_DataSource_Load_WithMockServiceConnect_FieldResolvers(t *testing.T) {
	baseURL, cleanup := setupTestConnectServer(t)
	t.Cleanup(cleanup)

	query := `query CategoryQuery($id: ID!, $threshold: Int, $metricType: String!) {
		category(id: $id) {
			id
			name
			popularityScore(threshold: $threshold)
			categoryMetrics(metricType: $metricType) {
				id
			}
		}
	}`
	vars := `{"variables":{"id":"cat-1","threshold":10,"metricType":"views"}}`

	output := loadConnectQuery(t, connectE2E{BaseURL: baseURL, Query: query, Vars: vars})

	type response struct {
		Data struct {
			Category *struct {
				Id              string  `json:"id"`
				Name            string  `json:"name"`
				PopularityScore float64 `json:"popularityScore"`
				CategoryMetrics *struct {
					Id string `json:"id"`
				} `json:"categoryMetrics"`
			} `json:"category"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	var resp response
	require.NoError(t, json.Unmarshal(output, &resp))
	require.Empty(t, resp.Errors, "Response should not contain errors: %s", string(output))
	require.NotNil(t, resp.Data.Category, "category should be present")
	require.Equal(t, "cat-1", resp.Data.Category.Id)
	require.NotEmpty(t, resp.Data.Category.Name)
	require.NotZero(t, resp.Data.Category.PopularityScore, "popularityScore resolver must have been invoked over Connect")
	require.NotNil(t, resp.Data.Category.CategoryMetrics, "categoryMetrics resolver must have been invoked over Connect")
	require.NotEmpty(t, resp.Data.Category.CategoryMetrics.Id)
}

// Test_DataSource_Load_WithMockServiceConnect_Entities exercises the
// Apollo Federation `_entities` lookup path over the Connect transport.
// The data source receives a batch of `{__typename, key}` representations
// and dispatches one Lookup<Type>By<Key> RPC per type. This test pins
// that the batched lookup completes over Connect and the response shape
// matches the gRPC variant in grpc_datasource_federation_test.go.
func Test_DataSource_Load_WithMockServiceConnect_Entities(t *testing.T) {
	baseURL, cleanup := setupTestConnectServer(t)
	t.Cleanup(cleanup)

	query := `query($representations: [_Any!]!) {
		_entities(representations: $representations) {
			... on Product { id name }
			... on Storage { id name }
		}
	}`
	vars := `{"variables":{"representations":[
		{"__typename":"Product","id":"1"},
		{"__typename":"Storage","id":"3"},
		{"__typename":"Product","id":"2"},
		{"__typename":"Storage","id":"4"}
	]}}`
	federationConfigs := plan.FederationFieldConfigurations{
		{TypeName: "Product", SelectionSet: "id"},
		{TypeName: "Storage", SelectionSet: "id"},
	}

	output := loadConnectQuery(t, connectE2E{BaseURL: baseURL, Query: query, Vars: vars, FederationConfigs: federationConfigs})

	// Use gjson here so we sidestep the per-entity __typename narrowing the
	// data source does — the test only cares that each representation came
	// back populated, regardless of the concrete union type the JSON encoder
	// picked for the response wrapper.
	require.True(t, gjson.ValidBytes(output), "Response must be valid JSON: %s", string(output))
	require.Empty(t, gjson.GetBytes(output, "errors").Array(), "Response should not contain errors: %s", string(output))

	entities := gjson.GetBytes(output, "data._entities").Array()
	require.Len(t, entities, 4, "expected 4 entities back, one per representation")

	require.Equal(t, "1", entities[0].Get("id").String())
	require.Equal(t, "Product 1", entities[0].Get("name").String())
	require.Equal(t, "3", entities[1].Get("id").String())
	require.Equal(t, "Storage 3", entities[1].Get("name").String())
	require.Equal(t, "2", entities[2].Get("id").String())
	require.Equal(t, "Product 2", entities[2].Get("name").String())
	require.Equal(t, "4", entities[3].Get("id").String())
	require.Equal(t, "Storage 4", entities[3].Get("name").String())
}

// Test_DataSource_Load_WithMockServiceConnect_AnimalInterface exercises
// inline fragments against an interface type (Cat/Dog implement the
// Animal interface), which the proto schema models as a oneof. The
// oneof wire shape is the same whether the encoder is the proto binary
// marshaller or protojson, but protojson's discriminator handling
// differs from binary — the JSON case has to flow through here, not
// just through proto, to pin both code paths.
func Test_DataSource_Load_WithMockServiceConnect_AnimalInterface(t *testing.T) {
	baseURL, cleanup := setupTestConnectServer(t)
	t.Cleanup(cleanup)

	encodings := []ConnectEncoding{ConnectEncodingProtobuf, ConnectEncodingJSON}
	for _, encoding := range encodings {
		t.Run(string(encoding), func(t *testing.T) {
			output := loadConnectQuery(t, connectE2E{
				BaseURL:  baseURL,
				Encoding: encoding,
				Query: `query RandomPetQuery {
					randomPet {
						__typename
						id
						name
						kind
						... on Cat { meowVolume }
						... on Dog { barkVolume }
					}
				}`,
				Vars: "{}",
			})

			type graphqlResponse struct {
				Data   map[string]any `json:"data"`
				Errors []any          `json:"errors,omitempty"`
			}
			var resp graphqlResponse
			require.NoError(t, json.Unmarshal(output, &resp))
			require.Empty(t, resp.Errors, "Response should not contain errors: %s", string(output))

			pet, ok := resp.Data["randomPet"].(map[string]any)
			require.True(t, ok, "randomPet should be an object")
			require.NotEmpty(t, pet)

			require.Contains(t, pet, "__typename")
			typename := pet["__typename"].(string)
			require.Contains(t, []string{"Cat", "Dog"}, typename)

			// The interface branch must yield the matching fragment-only field
			// and must *not* include the other branch's field — the same
			// __typename narrowing the gRPC test pins.
			switch typename {
			case "Cat":
				require.Contains(t, pet, "meowVolume")
				require.NotContains(t, pet, "barkVolume")
			case "Dog":
				require.Contains(t, pet, "barkVolume")
				require.NotContains(t, pet, "meowVolume")
			}
		})
	}
}

// Test_DataSource_Load_WithMockServiceConnect_NullableFields covers the
// nullable-fields proto wrappers (StringValue, Int32Value, etc.). proto
// binary and protojson serialise these very differently (wrapper message
// vs bare value or JSON null), so the test runs both encodings to pin
// down that the data source still emits a coherent GraphQL response in
// either case.
func Test_DataSource_Load_WithMockServiceConnect_NullableFields(t *testing.T) {
	baseURL, cleanup := setupTestConnectServer(t)
	t.Cleanup(cleanup)

	encodings := []ConnectEncoding{ConnectEncodingProtobuf, ConnectEncodingJSON}
	for _, encoding := range encodings {
		t.Run(string(encoding), func(t *testing.T) {
			output := loadConnectQuery(t, connectE2E{
				BaseURL:  baseURL,
				Encoding: encoding,
				Query: `query NullableFields {
					nullableFieldsType {
						id
						name
						requiredString
						requiredInt
						optionalString
						optionalInt
						optionalFloat
						optionalBoolean
					}
				}`,
				Vars: "{}",
			})

			type graphqlResponse struct {
				Data   map[string]any `json:"data"`
				Errors []any          `json:"errors,omitempty"`
			}
			var resp graphqlResponse
			require.NoError(t, json.Unmarshal(output, &resp))
			require.Empty(t, resp.Errors, "Response should not contain errors: %s", string(output))

			nft, ok := resp.Data["nullableFieldsType"].(map[string]any)
			require.True(t, ok, "nullableFieldsType should be an object")

			// Required fields must be present and non-empty regardless of encoding.
			for _, k := range []string{"id", "name", "requiredString", "requiredInt"} {
				require.Contains(t, nft, k, "required field %q must be present", k)
				require.NotEmpty(t, nft[k], "required field %q must be non-empty", k)
			}
			// Optional fields are present (under both encodings) but may be null.
			// The important thing is the shape decoded into the response — the
			// JSON encoding tends to drop unset wrappers, which the planner
			// has to re-inject as nulls; pin that here.
			for _, k := range []string{"optionalString", "optionalInt", "optionalFloat", "optionalBoolean"} {
				require.Contains(t, nft, k, "optional field %q should be present (may be null)", k)
			}
		})
	}
}

// Test_DataSource_Load_WithMockServiceConnect_Headers covers the
// metadata-to-HTTP-header bridge specific to the Connect transport.
// ds.Load takes an http.Header and translates it onto the outgoing
// context via grpc/metadata; the Connect transport then has to put
// those headers on the wire. The mock service echoes selected header
// values back through the response so we can assert the headers
// actually reached the server.
func Test_DataSource_Load_WithMockServiceConnect_Headers(t *testing.T) {
	baseURL, cleanup := setupTestConnectServer(t)
	t.Cleanup(cleanup)

	type graphqlResponse struct {
		Data   map[string]any `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}

	t.Run("header overrides query variable", func(t *testing.T) {
		h := make(http.Header)
		h.Set("X-User-ID", "header-user-42")
		output := loadConnectQuery(t, connectE2E{
			BaseURL: baseURL,
			Query:   `query UserQuery($id: ID!) { user(id: $id) { id name } }`,
			Vars:    `{"variables":{"id":"original-user-123"}}`,
			Headers: h,
		})

		var resp graphqlResponse
		require.NoError(t, json.Unmarshal(output, &resp))
		require.Empty(t, resp.Errors)
		user := resp.Data["user"].(map[string]any)
		require.Equal(t, "header-user-42", user["id"], "header value must reach the subgraph over Connect")
		require.Equal(t, "User header-user-42", user["name"])
	})

	t.Run("custom prefix header is applied", func(t *testing.T) {
		h := make(http.Header)
		h.Set("X-User-Prefix", "Admin")
		output := loadConnectQuery(t, connectE2E{
			BaseURL: baseURL,
			Query:   `query UsersQuery { users { id name } }`,
			Vars:    `{"variables":{}}`,
			Headers: h,
		})

		var resp graphqlResponse
		require.NoError(t, json.Unmarshal(output, &resp))
		require.Empty(t, resp.Errors)
		users := resp.Data["users"].([]any)
		require.NotEmpty(t, users)
		for _, u := range users {
			require.Contains(t, u.(map[string]any)["name"].(string), "Admin")
		}
	})

	t.Run("no headers yields baseline behaviour", func(t *testing.T) {
		output := loadConnectQuery(t, connectE2E{
			BaseURL: baseURL,
			Query:   `query UserQuery($id: ID!) { user(id: $id) { id name } }`,
			Vars:    `{"variables":{"id":"baseline-user-99"}}`,
		})

		var resp graphqlResponse
		require.NoError(t, json.Unmarshal(output, &resp))
		require.Empty(t, resp.Errors)
		user := resp.Data["user"].(map[string]any)
		require.Equal(t, "baseline-user-99", user["id"])
	})
}

// Test_DataSource_Load_WithMockServiceConnect_PreservesContextMetadata
// pins the metadata-stacking contract: when the caller hands in a
// context that already carries grpc/metadata, the data source must
// thread *both* that metadata and any HTTP headers down to the Connect
// transport. The mock service echoes the existing metadata value into
// the response so we can verify it made the round trip.
func Test_DataSource_Load_WithMockServiceConnect_PreservesContextMetadata(t *testing.T) {
	baseURL, cleanup := setupTestConnectServer(t)
	t.Cleanup(cleanup)

	ctx := metadata.NewOutgoingContext(
		context.Background(),
		metadata.Pairs("x-existing-key", "existing-value"),
	)
	headers := make(http.Header)
	headers.Set("X-User-ID", "header-user-456")

	output := loadConnectQuery(t, connectE2E{
		BaseURL: baseURL,
		Query:   `query UserQuery($id: ID!) { user(id: $id) { id name } }`,
		Vars:    `{"variables":{"id":"test-user-123"}}`,
		Ctx:     ctx,
		Headers: headers,
	})

	type graphqlResponse struct {
		Data   map[string]any `json:"data"`
		Errors []any          `json:"errors,omitempty"`
	}
	var resp graphqlResponse
	require.NoError(t, json.Unmarshal(output, &resp))
	require.Empty(t, resp.Errors, "Response should not contain errors: %s", string(output))

	user := resp.Data["user"].(map[string]any)
	require.Equal(t, "header-user-456", user["id"], "user ID should come from HTTP header")
	require.Equal(t, "User header-user-456 (existing: existing-value)", user["name"],
		"name should include both header-derived ID and existing context metadata; if either is missing, the data source dropped one of the two metadata streams between Load and the Connect transport")
}

// Test_DataSource_Load_WithMockServiceConnect_Error pins the error
// passthrough: the Connect transport returns a *connect.Error for
// non-2xx HTTP responses, the data source wraps the failed Load into a
// GraphQL-shaped errors array, and the user-facing payload includes
// the upstream error message. This is the gRPC-error mirror — Connect
// has a different error model (HTTP status + JSON body vs gRPC status
// trailer), so the test verifies the data source still produces a
// valid GraphQL response in either case.
func Test_DataSource_Load_WithMockServiceConnect_Error(t *testing.T) {
	baseURL, cleanup := setupTestConnectServer(t)
	t.Cleanup(cleanup)

	output := loadConnectQuery(t, connectE2E{
		BaseURL: baseURL,
		Query:   `query UserQuery($id: ID!) { user(id: $id) { id name } }`,
		Vars:    `{"variables":{"id":"error-user"}}`,
	})

	// Even when the upstream RPC errors out, Load itself must return a
	// well-formed GraphQL response with an errors array; surface the raw
	// payload on failure to make debugging the error path easier.
	responseJSON := string(output)
	require.Contains(t, responseJSON, "errors", "response should have a top-level errors array: %s", responseJSON)
	require.Contains(t, responseJSON, "user not found: error-user", "upstream error message should be carried into the GraphQL response: %s", responseJSON)

	var resp struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(output, &resp))
	require.NotEmpty(t, resp.Errors, "errors array should contain at least one entry")
	require.Contains(t, resp.Errors[0].Message, "user not found: error-user")
}

// Test_DataSource_Load_WithMockServiceConnect_NestedLists exercises the
// list/array encoding paths in both wire formats. proto binary uses
// repeated fields; protojson maps them onto JSON arrays. The blog post
// query mixes required/optional outer lists with required/optional
// item nullability, so the test pins that the data source decodes
// "null entries inside non-null array" the same way for both wires.
func Test_DataSource_Load_WithMockServiceConnect_NestedLists(t *testing.T) {
	baseURL, cleanup := setupTestConnectServer(t)
	t.Cleanup(cleanup)

	query := `query NestedLists {
		blogPost {
			id
			title
			content
			tags
			optionalTags
			categories
			keywords
		}
	}`

	encodings := []ConnectEncoding{ConnectEncodingProtobuf, ConnectEncodingJSON}
	for _, encoding := range encodings {
		t.Run(string(encoding), func(t *testing.T) {
			output := loadConnectQuery(t, connectE2E{
				BaseURL:  baseURL,
				Encoding: encoding,
				Query:    query,
				Vars:     "{}",
			})

			type graphqlResponse struct {
				Data   map[string]any `json:"data"`
				Errors []any          `json:"errors,omitempty"`
			}
			var resp graphqlResponse
			require.NoError(t, json.Unmarshal(output, &resp))
			require.Empty(t, resp.Errors, "Response should not contain errors: %s", string(output))

			post, ok := resp.Data["blogPost"].(map[string]any)
			require.True(t, ok, "blogPost should be an object")

			// Required scalar fields must be present.
			require.NotEmpty(t, post["id"])
			require.NotEmpty(t, post["title"])
			require.NotEmpty(t, post["content"])

			// Required-list-of-required-items: must be a non-empty array.
			tags, ok := post["tags"].([]any)
			require.True(t, ok, "tags should be an array")
			require.NotEmpty(t, tags)

			// Required-list-of-optional-items: must be an array (may contain nulls).
			_, ok = post["categories"].([]any)
			require.True(t, ok, "categories should be an array")

			// Optional outer lists: either null or array. The interesting case
			// is that protojson tends to omit unset repeated fields entirely,
			// and the data source has to surface them as either null or an
			// empty array — the planner can pick either, but the response key
			// itself must exist so callers can detect "field selected but
			// upstream gave nothing".
			require.Contains(t, post, "optionalTags")
			require.Contains(t, post, "keywords")
		})
	}
}

