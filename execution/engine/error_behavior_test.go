package engine

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/service_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestErrorBehavior_EndToEnd tests the onError request parameter behavior
// as specified in GraphQL spec PR #1163.
//
// Error Behavior Modes:
// - PROPAGATE (default): Null bubbles up to nearest nullable ancestor
// - NULL: Error yields null at site, no bubbling, errors are collected
// - HALT: First error stops execution, data becomes null
func TestErrorBehavior_EndToEnd(t *testing.T) {
	// Set up a mock subgraph that returns data with null in non-nullable fields
	setupErrorScenario := func(t *testing.T, subgraphResponse string) (*ExecutionEngine, *graphql.Schema) {
		t.Helper()

		// Create a mock server that returns the subgraph response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(subgraphResponse))
		}))
		t.Cleanup(server.Close)

		// Schema with non-nullable fields that can trigger errors
		schemaSDL := `
			type Query {
				user: User
				users: [User!]!
			}

			type User {
				id: ID!
				name: String!
				email: String
				profile: Profile
				posts: [Post!]!
			}

			type Profile {
				bio: String!
				avatar: String
			}

			type Post {
				id: ID!
				title: String!
				content: String
			}
		`

		schema, err := graphql.NewSchemaFromString(schemaSDL)
		require.NoError(t, err)

		httpClient := http.DefaultClient
		subscriptionClient := graphql_datasource.NewGraphQLSubscriptionClient(httpClient, httpClient, context.Background())

		factory, err := graphql_datasource.NewFactory(context.Background(), httpClient, subscriptionClient)
		require.NoError(t, err)

		schemaConfig, err := graphql_datasource.NewSchemaConfiguration(schemaSDL, nil)
		require.NoError(t, err)

		customConfig, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{
				URL:    server.URL,
				Method: "POST",
			},
			SchemaConfiguration: schemaConfig,
		})
		require.NoError(t, err)

		dsConfig, err := plan.NewDataSourceConfiguration[graphql_datasource.Configuration](
			"graphql_datasource",
			factory,
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "Query", FieldNames: []string{"user", "users"}},
				},
				ChildNodes: []plan.TypeField{
					{TypeName: "User", FieldNames: []string{"id", "name", "email", "profile", "posts"}},
					{TypeName: "Profile", FieldNames: []string{"bio", "avatar"}},
					{TypeName: "Post", FieldNames: []string{"id", "title", "content"}},
				},
			},
			customConfig,
		)
		require.NoError(t, err)

		engineConfig := NewConfiguration(schema)
		engineConfig.SetDataSources([]plan.DataSource{dsConfig})
		engineConfig.SetFieldConfigurations(plan.FieldConfigurations{
			{TypeName: "Query", FieldName: "user"},
			{TypeName: "Query", FieldName: "users"},
		})

		eng, err := NewExecutionEngine(context.Background(), abstractlogger.NoopLogger, engineConfig, resolve.ResolverOptions{
			MaxConcurrency: 1,
		})
		require.NoError(t, err)

		return eng, schema
	}

	t.Run("PROPAGATE mode - null bubbles up to nearest nullable ancestor", func(t *testing.T) {
		// Subgraph returns null for non-nullable `name` field
		// In PROPAGATE mode, the null should bubble up to the nullable `user` field
		subgraphResponse := `{"data":{"user":{"id":"1","name":null,"email":"test@example.com"}}}`

		eng, _ := setupErrorScenario(t, subgraphResponse)

		query := `query { user { id name email } }`
		req := &graphql.Request{
			Query: query,
		}

		ctx := context.Background()
		buf := new(bytes.Buffer)
		resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

		err := eng.Execute(ctx, req, &resultWriter, WithErrorBehavior(resolve.ErrorBehaviorPropagate))
		require.NoError(t, err)

		expected := `{"errors":[{"message":"Cannot return null for non-nullable field 'User.name'.","path":["user","name"]}],"data":{"user":null}}`
		assert.JSONEq(t, expected, buf.String())
	})

	t.Run("NULL mode - error at site, no bubbling, errors collected", func(t *testing.T) {
		// Subgraph returns null for non-nullable `name` field
		// In NULL mode, the null should stay at `name`, not bubble up
		subgraphResponse := `{"data":{"user":{"id":"1","name":null,"email":"test@example.com"}}}`

		eng, _ := setupErrorScenario(t, subgraphResponse)

		query := `query { user { id name email } }`
		req := &graphql.Request{
			Query: query,
		}

		ctx := context.Background()
		buf := new(bytes.Buffer)
		resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

		err := eng.Execute(ctx, req, &resultWriter, WithErrorBehavior(resolve.ErrorBehaviorNull))
		require.NoError(t, err)

		// In NULL mode: error at site, no bubbling - user object preserved with name=null
		// Error included so client can distinguish error null from intentional null
		expected := `{"errors":[{"message":"Cannot return null for non-nullable field 'User.name'.","path":["user","name"]}],"data":{"user":{"id":"1","name":null,"email":"test@example.com"}}}`
		assert.JSONEq(t, expected, buf.String())
	})

	t.Run("HALT mode - first error stops execution, data becomes null", func(t *testing.T) {
		// Subgraph returns null for non-nullable `name` field
		// In HALT mode, the entire data should become null on first error
		subgraphResponse := `{"data":{"user":{"id":"1","name":null,"email":"test@example.com"}}}`

		eng, _ := setupErrorScenario(t, subgraphResponse)

		query := `query { user { id name email } }`
		req := &graphql.Request{
			Query: query,
		}

		ctx := context.Background()
		buf := new(bytes.Buffer)
		resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

		err := eng.Execute(ctx, req, &resultWriter, WithErrorBehavior(resolve.ErrorBehaviorHalt))
		require.NoError(t, err)

		// In HALT mode: execution stops, data becomes null
		expected := `{"errors":[{"message":"Cannot return null for non-nullable field 'User.name'.","path":["user","name"]}],"data":null}`
		assert.JSONEq(t, expected, buf.String())
	})

	t.Run("NULL mode with multiple errors - all errors collected", func(t *testing.T) {
		// Subgraph returns multiple null values for non-nullable fields
		subgraphResponse := `{"data":{"user":{"id":"1","name":null,"email":"test@example.com","profile":{"bio":null,"avatar":"pic.jpg"}}}}`

		eng, _ := setupErrorScenario(t, subgraphResponse)

		query := `query { user { id name email profile { bio avatar } } }`
		req := &graphql.Request{
			Query: query,
		}

		ctx := context.Background()
		buf := new(bytes.Buffer)
		resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

		err := eng.Execute(ctx, req, &resultWriter, WithErrorBehavior(resolve.ErrorBehaviorNull))
		require.NoError(t, err)

		// In NULL mode: both errors collected, objects preserved
		expected := `{"errors":[{"message":"Cannot return null for non-nullable field 'User.name'.","path":["user","name"]},{"message":"Cannot return null for non-nullable field 'Profile.bio'.","path":["user","profile","bio"]}],"data":{"user":{"id":"1","name":null,"email":"test@example.com","profile":{"bio":null,"avatar":"pic.jpg"}}}}`
		assert.JSONEq(t, expected, buf.String())
	})

	t.Run("PROPAGATE mode with nested non-nullable - bubble to correct level", func(t *testing.T) {
		// Profile has non-nullable bio, profile itself is nullable
		// Null bio should bubble up to profile becoming null
		subgraphResponse := `{"data":{"user":{"id":"1","name":"Test","email":"test@example.com","profile":{"bio":null,"avatar":"pic.jpg"}}}}`

		eng, _ := setupErrorScenario(t, subgraphResponse)

		query := `query { user { id name email profile { bio avatar } } }`
		req := &graphql.Request{
			Query: query,
		}

		ctx := context.Background()
		buf := new(bytes.Buffer)
		resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

		err := eng.Execute(ctx, req, &resultWriter, WithErrorBehavior(resolve.ErrorBehaviorPropagate))
		require.NoError(t, err)

		// In PROPAGATE mode: null bio bubbles up to nullable profile
		expected := `{"errors":[{"message":"Cannot return null for non-nullable field 'Profile.bio'.","path":["user","profile","bio"]}],"data":{"user":{"id":"1","name":"Test","email":"test@example.com","profile":null}}}`
		assert.JSONEq(t, expected, buf.String())
	})

	t.Run("NULL mode with array containing errors", func(t *testing.T) {
		// Array of users where one has null non-nullable field
		subgraphResponse := `{"data":{"users":[{"id":"1","name":"Alice","email":"alice@example.com","profile":null,"posts":[]},{"id":"2","name":null,"email":"bob@example.com","profile":null,"posts":[]}]}}`

		eng, _ := setupErrorScenario(t, subgraphResponse)

		query := `query { users { id name email } }`
		req := &graphql.Request{
			Query: query,
		}

		ctx := context.Background()
		buf := new(bytes.Buffer)
		resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

		err := eng.Execute(ctx, req, &resultWriter, WithErrorBehavior(resolve.ErrorBehaviorNull))
		require.NoError(t, err)

		// In NULL mode: array preserved, second user has null name with error
		expected := `{"errors":[{"message":"Cannot return null for non-nullable field 'User.name'.","path":["users",1,"name"]}],"data":{"users":[{"id":"1","name":"Alice","email":"alice@example.com"},{"id":"2","name":null,"email":"bob@example.com"}]}}`
		assert.JSONEq(t, expected, buf.String())
	})

	t.Run("default behavior without explicit mode is PROPAGATE", func(t *testing.T) {
		subgraphResponse := `{"data":{"user":{"id":"1","name":null,"email":"test@example.com"}}}`

		eng, _ := setupErrorScenario(t, subgraphResponse)

		query := `query { user { id name email } }`
		req := &graphql.Request{
			Query: query,
		}

		ctx := context.Background()
		buf := new(bytes.Buffer)
		resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

		// Execute WITHOUT specifying error behavior - should default to PROPAGATE
		err := eng.Execute(ctx, req, &resultWriter)
		require.NoError(t, err)

		// Default behavior is PROPAGATE: null bubbles up
		expected := `{"errors":[{"message":"Cannot return null for non-nullable field 'User.name'.","path":["user","name"]}],"data":{"user":null}}`
		assert.JSONEq(t, expected, buf.String())
	})

	t.Run("successful query - no difference between modes", func(t *testing.T) {
		// No errors in the response
		subgraphResponse := `{"data":{"user":{"id":"1","name":"Test User","email":"test@example.com"}}}`

		eng, _ := setupErrorScenario(t, subgraphResponse)

		query := `query { user { id name email } }`
		expected := `{"data":{"user":{"id":"1","name":"Test User","email":"test@example.com"}}}`

		for _, mode := range []resolve.ErrorBehavior{
			resolve.ErrorBehaviorPropagate,
			resolve.ErrorBehaviorNull,
			resolve.ErrorBehaviorHalt,
		} {
			t.Run(mode.String(), func(t *testing.T) {
				req := &graphql.Request{
					Query: query,
				}

				ctx := context.Background()
				buf := new(bytes.Buffer)
				resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

				err := eng.Execute(ctx, req, &resultWriter, WithErrorBehavior(mode))
				require.NoError(t, err)

				// All modes should return the same successful result
				assert.JSONEq(t, expected, buf.String())
			})
		}
	})
}

// TestErrorBehavior_RequestExtensions tests that error behavior can be set via request extensions
func TestErrorBehavior_RequestExtensions(t *testing.T) {
	t.Run("parse NULL from extensions", func(t *testing.T) {
		req := &graphql.Request{
			Query:      `query { user { id name } }`,
			Extensions: []byte(`{"onError":"NULL"}`),
		}

		behavior, ok := req.GetOnErrorBehavior()
		assert.True(t, ok)
		assert.Equal(t, resolve.ErrorBehaviorNull, behavior)
	})

	t.Run("parse PROPAGATE from extensions", func(t *testing.T) {
		req := &graphql.Request{
			Query:      `query { user { id name } }`,
			Extensions: []byte(`{"onError":"PROPAGATE"}`),
		}

		behavior, ok := req.GetOnErrorBehavior()
		assert.True(t, ok)
		assert.Equal(t, resolve.ErrorBehaviorPropagate, behavior)
	})

	t.Run("parse HALT from extensions", func(t *testing.T) {
		req := &graphql.Request{
			Query:      `query { user { id name } }`,
			Extensions: []byte(`{"onError":"HALT"}`),
		}

		behavior, ok := req.GetOnErrorBehavior()
		assert.True(t, ok)
		assert.Equal(t, resolve.ErrorBehaviorHalt, behavior)
	})

	t.Run("invalid onError value returns false", func(t *testing.T) {
		req := &graphql.Request{
			Query:      `query { user { id name } }`,
			Extensions: []byte(`{"onError":"INVALID"}`),
		}

		behavior, ok := req.GetOnErrorBehavior()
		assert.False(t, ok)
		assert.Equal(t, resolve.ErrorBehaviorPropagate, behavior) // Default fallback
	})

	t.Run("missing onError returns false", func(t *testing.T) {
		req := &graphql.Request{
			Query:      `query { user { id name } }`,
			Extensions: []byte(`{"persistedQuery":{"hash":"abc123"}}`),
		}

		behavior, ok := req.GetOnErrorBehavior()
		assert.False(t, ok)
		assert.Equal(t, resolve.ErrorBehaviorPropagate, behavior) // Default fallback
	})

	t.Run("empty extensions returns false", func(t *testing.T) {
		req := &graphql.Request{
			Query: `query { user { id name } }`,
		}

		behavior, ok := req.GetOnErrorBehavior()
		assert.False(t, ok)
		assert.Equal(t, resolve.ErrorBehaviorPropagate, behavior) // Default fallback
	})
}

// TestErrorBehavior_ServiceCapabilityIntrospection tests the __service query for onError capability discovery
func TestErrorBehavior_ServiceCapabilityIntrospection(t *testing.T) {
	// Schema that includes the _Service type for introspection
	schemaSDL := `
		type Query {
			__service: _Service!
			user: User
		}

		type _Service {
			capabilities: [_Capability!]!
		}

		type _Capability {
			identifier: String!
			value: String
			description: String
		}

		type User {
			id: ID!
			name: String!
		}
	`

	setupServiceIntrospection := func(t *testing.T, defaultBehavior string) *ExecutionEngine {
		t.Helper()

		schema, err := graphql.NewSchemaFromString(schemaSDL)
		require.NoError(t, err)

		// Create service datasource configuration
		serviceFactory := service_datasource.NewServiceConfigFactory(service_datasource.ServiceOptions{
			DefaultErrorBehavior: defaultBehavior,
		})

		engineConfig := NewConfiguration(schema)

		// Add service datasource
		dataSources := serviceFactory.BuildDataSourceConfigurations()
		engineConfig.SetDataSources(dataSources)

		fieldConfigs := serviceFactory.BuildFieldConfigurations()
		engineConfig.SetFieldConfigurations(fieldConfigs)

		eng, err := NewExecutionEngine(context.Background(), abstractlogger.NoopLogger, engineConfig, resolve.ResolverOptions{
			MaxConcurrency: 1,
		})
		require.NoError(t, err)

		return eng
	}

	t.Run("introspect onError capability with PROPAGATE default", func(t *testing.T) {
		eng := setupServiceIntrospection(t, "PROPAGATE")

		query := `query { __service { capabilities { identifier value description } } }`
		req := &graphql.Request{
			Query: query,
		}

		ctx := context.Background()
		buf := new(bytes.Buffer)
		resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

		err := eng.Execute(ctx, req, &resultWriter)
		require.NoError(t, err)

		expected := `{
			"data": {
				"__service": {
					"capabilities": [
						{
							"identifier": "graphql.onError",
							"value": null,
							"description": "Supports the onError request extension for controlling error propagation behavior"
						},
						{
							"identifier": "graphql.defaultErrorBehavior",
							"value": "PROPAGATE",
							"description": "The default error behavior when onError is not specified in the request"
						}
					]
				}
			}
		}`
		assert.JSONEq(t, expected, buf.String())
	})

	t.Run("introspect onError capability with NULL default", func(t *testing.T) {
		eng := setupServiceIntrospection(t, "NULL")

		query := `query { __service { capabilities { identifier value description } } }`
		req := &graphql.Request{
			Query: query,
		}

		ctx := context.Background()
		buf := new(bytes.Buffer)
		resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

		err := eng.Execute(ctx, req, &resultWriter)
		require.NoError(t, err)

		expected := `{
			"data": {
				"__service": {
					"capabilities": [
						{
							"identifier": "graphql.onError",
							"value": null,
							"description": "Supports the onError request extension for controlling error propagation behavior"
						},
						{
							"identifier": "graphql.defaultErrorBehavior",
							"value": "NULL",
							"description": "The default error behavior when onError is not specified in the request"
						}
					]
				}
			}
		}`
		assert.JSONEq(t, expected, buf.String())
	})

	t.Run("introspect onError capability with HALT default", func(t *testing.T) {
		eng := setupServiceIntrospection(t, "HALT")

		query := `query { __service { capabilities { identifier value description } } }`
		req := &graphql.Request{
			Query: query,
		}

		ctx := context.Background()
		buf := new(bytes.Buffer)
		resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

		err := eng.Execute(ctx, req, &resultWriter)
		require.NoError(t, err)

		expected := `{
			"data": {
				"__service": {
					"capabilities": [
						{
							"identifier": "graphql.onError",
							"value": null,
							"description": "Supports the onError request extension for controlling error propagation behavior"
						},
						{
							"identifier": "graphql.defaultErrorBehavior",
							"value": "HALT",
							"description": "The default error behavior when onError is not specified in the request"
						}
					]
				}
			}
		}`
		assert.JSONEq(t, expected, buf.String())
	})

	t.Run("introspect without default behavior configured", func(t *testing.T) {
		eng := setupServiceIntrospection(t, "")

		query := `query { __service { capabilities { identifier value description } } }`
		req := &graphql.Request{
			Query: query,
		}

		ctx := context.Background()
		buf := new(bytes.Buffer)
		resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

		err := eng.Execute(ctx, req, &resultWriter)
		require.NoError(t, err)

		// Without default behavior configured, only onError capability is returned
		expected := `{
			"data": {
				"__service": {
					"capabilities": [
						{
							"identifier": "graphql.onError",
							"value": null,
							"description": "Supports the onError request extension for controlling error propagation behavior"
						}
					]
				}
			}
		}`
		assert.JSONEq(t, expected, buf.String())
	})

	t.Run("introspect only identifiers", func(t *testing.T) {
		eng := setupServiceIntrospection(t, "PROPAGATE")

		// Client can query only the fields they need
		query := `query { __service { capabilities { identifier } } }`
		req := &graphql.Request{
			Query: query,
		}

		ctx := context.Background()
		buf := new(bytes.Buffer)
		resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

		err := eng.Execute(ctx, req, &resultWriter)
		require.NoError(t, err)

		expected := `{
			"data": {
				"__service": {
					"capabilities": [
						{"identifier": "graphql.onError"},
						{"identifier": "graphql.defaultErrorBehavior"}
					]
				}
			}
		}`
		assert.JSONEq(t, expected, buf.String())
	})
}

// TestServiceCapability_CosmoRouterIntegration tests the schema extension API
// that Cosmo router uses to add service capability types to a schema.
//
// This mimics the Cosmo router integration pattern:
// 1. Parse a user schema (no service types)
// 2. Merge with base schema (adds introspection types)
// 3. Extend with service types via NewServiceConfigFactoryWithSchema
// 4. Verify introspection shows _Service and _Capability types
// 5. Verify __service query works
func TestServiceCapability_CosmoRouterIntegration(t *testing.T) {
	t.Run("schema extension and introspection", func(t *testing.T) {
		// User's schema - does NOT include _Service, _Capability, or __service
		userSchemaSDL := `
			type Query {
				user(id: ID!): User
			}
			type User {
				id: ID!
				name: String!
			}
		`

		// Create schema and extend with service types using the new API
		schema, err := graphql.NewSchemaFromString(userSchemaSDL)
		require.NoError(t, err)

		// Use NewServiceConfigFactoryWithSchema to extend schema AND create factory
		serviceFactory, err := service_datasource.NewServiceConfigFactoryWithSchema(
			schema.Document(),
			service_datasource.ServiceOptions{
				DefaultErrorBehavior: "PROPAGATE",
			},
		)
		require.NoError(t, err)

		// Build engine configuration
		// NOTE: NewExecutionEngine automatically adds introspection datasources,
		// so we don't need to add them manually here
		engineConfig := NewConfiguration(schema)

		// Add service capabilities datasource
		for _, ds := range serviceFactory.BuildDataSourceConfigurations() {
			engineConfig.AddDataSource(ds)
		}
		for _, fc := range serviceFactory.BuildFieldConfigurations() {
			engineConfig.AddFieldConfiguration(fc)
		}

		eng, err := NewExecutionEngine(context.Background(), abstractlogger.NoopLogger, engineConfig, resolve.ResolverOptions{
			MaxConcurrency: 1,
		})
		require.NoError(t, err)

		// Test __service query works
		t.Run("__service query returns capabilities", func(t *testing.T) {
			query := `{ __service { capabilities { identifier value description } } }`
			req := &graphql.Request{Query: query}

			buf := new(bytes.Buffer)
			resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

			err := eng.Execute(context.Background(), req, &resultWriter)
			require.NoError(t, err)

			expected := `{
				"data": {
					"__service": {
						"capabilities": [
							{
								"identifier": "graphql.onError",
								"value": null,
								"description": "Supports the onError request extension for controlling error propagation behavior"
							},
							{
								"identifier": "graphql.defaultErrorBehavior",
								"value": "PROPAGATE",
								"description": "The default error behavior when onError is not specified in the request"
							}
						]
					}
				}
			}`
			assert.JSONEq(t, expected, buf.String())
		})

		// Test introspection shows _Service type
		t.Run("introspection returns _Service type", func(t *testing.T) {
			query := `{
				__type(name: "_Service") {
					name
					kind
					fields { name }
				}
			}`
			req := &graphql.Request{Query: query}

			buf := new(bytes.Buffer)
			resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

			err := eng.Execute(context.Background(), req, &resultWriter)
			require.NoError(t, err)

			expected := `{
				"data": {
					"__type": {
						"name": "_Service",
						"kind": "OBJECT",
						"fields": [
							{"name": "capabilities"}
						]
					}
				}
			}`
			assert.JSONEq(t, expected, buf.String())
		})

		// Test introspection shows _Capability type
		t.Run("introspection returns _Capability type", func(t *testing.T) {
			query := `{
				__type(name: "_Capability") {
					name
					kind
					fields { name }
				}
			}`
			req := &graphql.Request{Query: query}

			buf := new(bytes.Buffer)
			resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

			err := eng.Execute(context.Background(), req, &resultWriter)
			require.NoError(t, err)

			expected := `{
				"data": {
					"__type": {
						"name": "_Capability",
						"kind": "OBJECT",
						"fields": [
							{"name": "identifier"},
							{"name": "value"},
							{"name": "description"}
						]
					}
				}
			}`
			assert.JSONEq(t, expected, buf.String())
		})

		// Test __schema introspection shows user fields (but not __ prefixed fields per GraphQL spec)
		// NOTE: Per GraphQL spec and standard behavior, fields starting with __ are not
		// included in introspection results (like __schema, __type, and now __service).
		// This is intentional - the query works, it's just hidden from field listings.
		t.Run("schema introspection shows user-defined fields", func(t *testing.T) {
			query := `{
				__schema {
					queryType {
						fields {
							name
						}
					}
				}
			}`
			req := &graphql.Request{Query: query}

			buf := new(bytes.Buffer)
			resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

			err := eng.Execute(context.Background(), req, &resultWriter)
			require.NoError(t, err)

			// Verify user-defined fields are present
			result := buf.String()
			assert.Contains(t, result, `"name":"user"`)

			// NOTE: __service is NOT in the fields list (per GraphQL spec - __ prefixed fields
			// are hidden from introspection). This matches __schema and __type behavior.
			// The query still works (tested above), it's just hidden from field listings.
		})
	})

	t.Run("works with NULL default error behavior", func(t *testing.T) {
		userSchemaSDL := `
			type Query {
				hello: String
			}
		`

		schema, err := graphql.NewSchemaFromString(userSchemaSDL)
		require.NoError(t, err)

		serviceFactory, err := service_datasource.NewServiceConfigFactoryWithSchema(
			schema.Document(),
			service_datasource.ServiceOptions{
				DefaultErrorBehavior: "NULL",
			},
		)
		require.NoError(t, err)

		engineConfig := NewConfiguration(schema)
		for _, ds := range serviceFactory.BuildDataSourceConfigurations() {
			engineConfig.AddDataSource(ds)
		}
		for _, fc := range serviceFactory.BuildFieldConfigurations() {
			engineConfig.AddFieldConfiguration(fc)
		}

		eng, err := NewExecutionEngine(context.Background(), abstractlogger.NoopLogger, engineConfig, resolve.ResolverOptions{
			MaxConcurrency: 1,
		})
		require.NoError(t, err)

		query := `{ __service { capabilities { identifier value } } }`
		req := &graphql.Request{Query: query}

		buf := new(bytes.Buffer)
		resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)

		err = eng.Execute(context.Background(), req, &resultWriter)
		require.NoError(t, err)

		// Verify NULL default is returned
		result := buf.String()
		assert.Contains(t, result, `"identifier":"graphql.defaultErrorBehavior"`)
		assert.Contains(t, result, `"value":"NULL"`)
	})
}
