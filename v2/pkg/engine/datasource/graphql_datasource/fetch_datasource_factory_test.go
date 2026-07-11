package graphql_datasource

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	grpcdatasource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type recordingFetchDataSourceFactory struct {
	calls         int
	onFetch       func(fetch PlannedFetch)
	newDataSource func(fetch PlannedFetch) (resolve.DataSource, error)
}

type borrowedPlannedFetchPayload struct {
	Operation      string          `json:"operation"`
	FetchMode      FetchMode       `json:"fetchMode"`
	PostProcessing json.RawMessage `json:"postProcessing"`
}

type copyingBorrowedPlannedFetchFactory struct{}

func (copyingBorrowedPlannedFetchFactory) NewDataSource(fetch PlannedFetch) (resolve.DataSource, error) {
	operation, err := astprinter.PrintString(fetch.Operation)
	if err != nil {
		return nil, err
	}
	postProcessing, err := json.Marshal(fetch.PostProcessing)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(borrowedPlannedFetchPayload{
		Operation:      operation,
		FetchMode:      fetch.FetchMode,
		PostProcessing: postProcessing,
	})
	if err != nil {
		return nil, err
	}
	return &copiedPlannedFetchDataSource{payload: payload}, nil
}

type copiedPlannedFetchDataSource struct {
	payload []byte
}

func (d *copiedPlannedFetchDataSource) Load(context.Context, http.Header, []byte) ([]byte, error) {
	return append([]byte(nil), d.payload...), nil
}

func (d *copiedPlannedFetchDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, _ []*httpclient.FileUpload) ([]byte, error) {
	return d.Load(ctx, headers, input)
}

func (f *recordingFetchDataSourceFactory) NewDataSource(fetch PlannedFetch) (resolve.DataSource, error) {
	f.calls++
	if f.onFetch != nil {
		f.onFetch(fetch)
	}
	if f.newDataSource != nil {
		return f.newDataSource(fetch)
	}
	return &Source{}, nil
}

func planFactoryOperationWithReport(t *testing.T, definition, operation, operationName string, configuration plan.Configuration, options ...plan.Opts) (plan.Plan, *operationreport.Report) {
	t.Helper()

	definitionDocument := unsafeparser.ParseGraphqlDocumentString(definition)
	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definitionDocument))

	normalizer := astnormalization.NewWithOpts(
		astnormalization.WithExtractVariables(),
		astnormalization.WithInlineFragmentSpreads(),
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveUnusedVariables(),
	)
	report := &operationreport.Report{}
	normalizer.NormalizeOperation(&operationDocument, &definitionDocument, report)
	require.False(t, report.HasErrors(), report.Error())

	validator := astvalidation.DefaultOperationValidator()
	validator.Validate(&operationDocument, &definitionDocument, report)
	require.False(t, report.HasErrors(), report.Error())

	planner, err := plan.NewPlanner(configuration)
	require.NoError(t, err)
	planned := planner.Plan(&operationDocument, &definitionDocument, operationName, report, options...)

	return planned, report
}

func planFactoryOperation(t *testing.T, definition, operation, operationName string, configuration plan.Configuration, options ...plan.Opts) plan.Plan {
	t.Helper()

	planned, report := planFactoryOperationWithReport(t, definition, operation, operationName, configuration, options...)
	require.False(t, report.HasErrors(), report.Error())
	return planned
}

func entityFactoryPlanConfiguration(t *testing.T, factory FetchDataSourceFactory) (string, plan.Configuration) {
	return entityFactoryPlanConfigurationWithRootFactory(t, nil, factory)
}

func entityFactoryPlanConfigurationWithRootFactory(t *testing.T, rootFactory, entityFactory FetchDataSourceFactory) (string, plan.Configuration) {
	t.Helper()

	definition := `
		type Query {
			user: User
			users: [User!]!
		}

		type User {
			id: ID!
			name: String!
		}
	`
	usersSDL := `
		type Query {
			user: User
			users: [User!]!
		}

		type User @key(fields: "id") {
			id: ID!
		}
	`
	namesSDL := `
		type User @key(fields: "id") {
			id: ID!
			name: String!
		}
	`

	return definition, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(t, "users", &plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "Query", FieldNames: []string{"user", "users"}},
					{TypeName: "User", FieldNames: []string{"id"}},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{TypeName: "User", SelectionSet: "id"},
					},
				},
			}, mustCustomConfiguration(t, ConfigurationInput{
				Fetch:                  &FetchConfiguration{URL: "https://users.example/graphql"},
				FetchDataSourceFactory: rootFactory,
				SchemaConfiguration: mustSchema(t, &FederationConfiguration{
					Enabled:    true,
					ServiceSDL: usersSDL,
				}, usersSDL),
			})),
			mustDataSourceConfiguration(t, "names", &plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "User", FieldNames: []string{"id", "name"}},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{TypeName: "User", SelectionSet: "id"},
					},
				},
			}, mustCustomConfiguration(t, ConfigurationInput{
				FetchDataSourceFactory: entityFactory,
				SchemaConfiguration: mustSchema(t, &FederationConfiguration{
					Enabled:    true,
					ServiceSDL: namesSDL,
				}, namesSDL),
			})),
		},
		DisableResolveFieldPositions: true,
	}
}

func TestFetchModeValues(t *testing.T) {
	require.Equal(t, FetchMode(0), FetchModeSingle)
	require.Equal(t, FetchMode(1), FetchModeEntity)
	require.Equal(t, FetchMode(2), FetchModeEntityBatch)
}

func TestConfigureFetch_FactoryReceivesNormalizedRootOperation(t *testing.T) {
	definition := `
		type Query {
			hello(arg: String): String
		}
	`

	factory := &recordingFetchDataSourceFactory{
		onFetch: func(fetch PlannedFetch) {
			printedOperation, err := astprinter.PrintString(fetch.Operation)
			require.NoError(t, err)
			require.Equal(t, "query($a: String){hello(arg: $a)}", printedOperation)
			require.NotContains(t, printedOperation, "world")
			require.Len(t, fetch.Variables, 1)
			contextVariable, ok := fetch.Variables[0].(*resolve.ContextVariable)
			require.True(t, ok)
			require.Equal(t, []string{"a"}, contextVariable.Path)
			require.Equal(t, FetchModeSingle, fetch.FetchMode)
		},
	}

	RunTest(definition, `
		query Root {
			...Greeting
		}

		fragment Greeting on Query {
			hello(arg: "world")
		}
	`, "Root", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Fetches: resolve.Sequence(
				resolve.Single(&resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						Input:      `{"body":{"query":"query($a: String){hello(arg: $a)}","variables":{"a":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"a"},
								Renderer: resolve.NewJSONVariableRenderer(),
							},
						),
						PostProcessing: DefaultPostProcessingConfiguration,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}),
			),
			Data: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("hello"),
						Value: &resolve.String{
							Nullable: true,
							Path:     []string{"hello"},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"hello"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					FetchDataSourceFactory: factory,
					SchemaConfiguration:    mustSchema(t, nil, definition),
				}),
			),
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "hello",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "arg",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		DisableResolveFieldPositions: true,
	}, WithDefaultPostProcessor())(t)

	require.Equal(t, 1, factory.calls)
}

func TestConfigureFetch_BorrowedPlannedFetchDoesNotEscape(t *testing.T) {
	dataSource := func() resolve.DataSource {
		definition := `type Query { hello: String }`
		configuration := plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(t, "root", &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{{TypeName: "Query", FieldNames: []string{"hello"}}},
				}, mustCustomConfiguration(t, ConfigurationInput{
					FetchDataSourceFactory: copyingBorrowedPlannedFetchFactory{},
					SchemaConfiguration:    mustSchema(t, nil, definition),
				})),
			},
			DisableResolveFieldPositions: true,
		}

		planned := planFactoryOperation(t, definition, `query Borrowed { hello }`, "Borrowed", configuration)
		return firstPlannedSingleFetch(t, planned).DataSource
	}()

	data, err := dataSource.Load(context.Background(), nil, nil)
	require.NoError(t, err)

	var payload borrowedPlannedFetchPayload
	require.NoError(t, json.Unmarshal(data, &payload))
	require.Equal(t, "{hello}", payload.Operation)
	require.Equal(t, FetchModeSingle, payload.FetchMode)
	expectedPostProcessing, err := json.Marshal(DefaultPostProcessingConfiguration)
	require.NoError(t, err)
	require.JSONEq(t, string(expectedPostProcessing), string(payload.PostProcessing))
}

func TestConfigureFetch_FactoryReceivesFetchModes(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		definition := `type Query { hello: String }`
		factory := &recordingFetchDataSourceFactory{
			onFetch: func(fetch PlannedFetch) {
				require.Equal(t, FetchModeSingle, fetch.FetchMode)
				require.Equal(t, DefaultPostProcessingConfiguration, fetch.PostProcessing)
				require.Empty(t, fetch.Variables)
				require.Empty(t, fetch.RequiredFields)
			},
		}

		planFactoryOperation(t, definition, `{ hello }`, "", plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(t, "root", &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{{TypeName: "Query", FieldNames: []string{"hello"}}},
				}, mustCustomConfiguration(t, ConfigurationInput{
					FetchDataSourceFactory: factory,
					SchemaConfiguration:    mustSchema(t, nil, definition),
				})),
			},
			DisableResolveFieldPositions: true,
		})
		require.Equal(t, 1, factory.calls)
	})

	t.Run("single entity", func(t *testing.T) {
		factory := &recordingFetchDataSourceFactory{
			onFetch: func(fetch PlannedFetch) {
				require.Equal(t, FetchModeEntity, fetch.FetchMode)
				require.Equal(t, SingleEntityPostProcessingConfiguration, fetch.PostProcessing)
				require.Len(t, fetch.RequiredFields, 1)
				require.Equal(t, "User", fetch.RequiredFields[0].TypeName)
				require.Equal(t, "id", fetch.RequiredFields[0].SelectionSet)
				require.Len(t, fetch.Variables, 1)
				require.IsType(t, &resolve.ResolvableObjectVariable{}, fetch.Variables[0])
				printedOperation, err := astprinter.PrintString(fetch.Operation)
				require.NoError(t, err)
				require.Contains(t, printedOperation, "_entities")
			},
		}
		definition, configuration := entityFactoryPlanConfiguration(t, factory)

		planFactoryOperation(t, definition, `{ user { name } }`, "", configuration)
		require.Equal(t, 1, factory.calls)
	})

	t.Run("batch entity", func(t *testing.T) {
		factory := &recordingFetchDataSourceFactory{
			onFetch: func(fetch PlannedFetch) {
				require.Equal(t, FetchModeEntityBatch, fetch.FetchMode)
				require.Equal(t, EntitiesPostProcessingConfiguration, fetch.PostProcessing)
				require.Len(t, fetch.RequiredFields, 1)
				require.Equal(t, "User", fetch.RequiredFields[0].TypeName)
				require.Equal(t, "id", fetch.RequiredFields[0].SelectionSet)
				require.Len(t, fetch.Variables, 1)
				require.IsType(t, &resolve.ResolvableObjectVariable{}, fetch.Variables[0])
				printedOperation, err := astprinter.PrintString(fetch.Operation)
				require.NoError(t, err)
				require.Contains(t, printedOperation, "_entities")
			},
		}
		definition, configuration := entityFactoryPlanConfiguration(t, factory)

		planFactoryOperation(t, definition, `{ users { name } }`, "", configuration)
		require.Equal(t, 1, factory.calls)
	})
}

func TestConfigureFetch_FactoryReceivesQueryPlan(t *testing.T) {
	definition := `type Query { hello: String }`
	configuration := func(t *testing.T, factory FetchDataSourceFactory) plan.Configuration {
		t.Helper()
		return plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(t, "root", &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{{TypeName: "Query", FieldNames: []string{"hello"}}},
				}, mustCustomConfiguration(t, ConfigurationInput{
					FetchDataSourceFactory: factory,
					SchemaConfiguration:    mustSchema(t, nil, definition),
				})),
			},
			DisableResolveFieldPositions: true,
		}
	}

	t.Run("disabled", func(t *testing.T) {
		factory := &recordingFetchDataSourceFactory{
			onFetch: func(fetch PlannedFetch) {
				require.Nil(t, fetch.QueryPlan)
			},
		}

		planFactoryOperation(t, definition, `{ hello }`, "", configuration(t, factory))
		require.Equal(t, 1, factory.calls)
	})

	t.Run("enabled", func(t *testing.T) {
		factory := &recordingFetchDataSourceFactory{
			onFetch: func(fetch PlannedFetch) {
				require.NotNil(t, fetch.QueryPlan)
				require.Contains(t, fetch.QueryPlan.Query, "hello")
			},
		}

		planFactoryOperation(t, definition, `{ hello }`, "", configuration(t, factory), plan.IncludeQueryPlanInResponse())
		require.Equal(t, 1, factory.calls)
	})
}

func TestConfigureFetch_FactorySeesRepeatedAliasedOccurrences(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		definition := `type Query { hello: String }`
		factory := &recordingFetchDataSourceFactory{
			onFetch: func(fetch PlannedFetch) {
				printedOperation, err := astprinter.PrintString(fetch.Operation)
				require.NoError(t, err)
				require.Contains(t, printedOperation, "first: hello")
				require.Contains(t, printedOperation, "second: hello")
				require.Equal(t, 2, strings.Count(printedOperation, "hello"))
			},
		}
		configuration := plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(t, "root", &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{{TypeName: "Query", FieldNames: []string{"hello"}}},
				}, mustCustomConfiguration(t, ConfigurationInput{
					FetchDataSourceFactory: factory,
					SchemaConfiguration:    mustSchema(t, nil, definition),
				})),
			},
			DisableResolveFieldPositions: true,
		}

		planFactoryOperation(t, definition, `
			query Root {
				...Greeting
			}

			fragment Greeting on Query {
				first: hello
				second: hello
			}
		`, "Root", configuration)
		require.Equal(t, 1, factory.calls)
	})

	t.Run("entity", func(t *testing.T) {
		factory := &recordingFetchDataSourceFactory{
			onFetch: func(fetch PlannedFetch) {
				printedOperation, err := astprinter.PrintString(fetch.Operation)
				require.NoError(t, err)
				require.Contains(t, printedOperation, "first: name")
				require.Contains(t, printedOperation, "second: name")
			},
		}
		definition, configuration := entityFactoryPlanConfiguration(t, factory)

		planFactoryOperation(t, definition, `
			query Root {
				user {
					...Names
				}
			}

			fragment Names on User {
				first: name
				second: name
			}
		`, "Root", configuration)
		require.Equal(t, 1, factory.calls)
	})
}

func firstPlannedSingleFetch(t *testing.T, planned plan.Plan) *resolve.SingleFetch {
	t.Helper()

	synchronousPlan, ok := planned.(*plan.SynchronousResponsePlan)
	require.True(t, ok)
	require.Len(t, synchronousPlan.Response.RawFetches, 1)
	singleFetch, ok := synchronousPlan.Response.RawFetches[0].Fetch.(*resolve.SingleFetch)
	require.True(t, ok)
	return singleFetch
}

func TestConfigureFetch_CustomFactoryPreservesFetchConfiguration(t *testing.T) {
	definition := `type Query { hello: String }`
	fetchConfiguration := &FetchConfiguration{
		URL:    "https://example.com/graphql",
		Method: http.MethodPatch,
		Header: http.Header{"X-Test": []string{"value"}},
	}
	configuration := func(t *testing.T, factory FetchDataSourceFactory) plan.Configuration {
		t.Helper()
		return plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(t, "root", &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{{TypeName: "Query", FieldNames: []string{"hello"}}},
				}, mustCustomConfiguration(t, ConfigurationInput{
					Fetch:                  fetchConfiguration,
					FetchDataSourceFactory: factory,
					SchemaConfiguration:    mustSchema(t, nil, definition),
				})),
			},
			DisableResolveFieldPositions:   true,
			EnableOperationNamePropagation: true,
		}
	}

	operation := `query ClientOperation { hello }`
	httpPlan := planFactoryOperation(t, definition, operation, "ClientOperation", configuration(t, nil))
	factory := &recordingFetchDataSourceFactory{}
	factoryPlan := planFactoryOperation(t, definition, operation, "ClientOperation", configuration(t, factory))

	httpFetch := firstPlannedSingleFetch(t, httpPlan)
	factoryFetch := firstPlannedSingleFetch(t, factoryPlan)
	require.Equal(t, httpFetch.Input, factoryFetch.Input)
	require.Equal(t, httpFetch.Variables, factoryFetch.Variables)
	require.Equal(t, httpFetch.RequiresEntityFetch, factoryFetch.RequiresEntityFetch)
	require.Equal(t, httpFetch.RequiresEntityBatchFetch, factoryFetch.RequiresEntityBatchFetch)
	require.Equal(t, httpFetch.SetTemplateOutputToNullOnVariableNull, factoryFetch.SetTemplateOutputToNullOnVariableNull)
	require.Equal(t, httpFetch.PostProcessing, factoryFetch.PostProcessing)
	require.Equal(t, httpFetch.QueryPlan, factoryFetch.QueryPlan)
	require.Equal(t, httpFetch.OperationName, factoryFetch.OperationName)
	require.NotEmpty(t, factoryFetch.OperationName)
	require.Contains(t, factoryFetch.Input, `"method":"PATCH"`)
	require.Contains(t, factoryFetch.Input, `"url":"https://example.com/graphql"`)
	require.Contains(t, factoryFetch.Input, `"X-Test":["value"]`)
	require.Equal(t, 1, factory.calls)
}

func TestConfigureFetch_FactoryErrors(t *testing.T) {
	definition := `type Query { hello: String }`
	tests := []struct {
		name          string
		newDataSource func(fetch PlannedFetch) (resolve.DataSource, error)
		errorContains string
	}{
		{
			name: "factory error",
			newDataSource: func(PlannedFetch) (resolve.DataSource, error) {
				return nil, errors.New("factory failed")
			},
			errorContains: "ConfigureFetch: failed to create fetch data source: factory failed",
		},
		{
			name: "nil data source",
			newDataSource: func(PlannedFetch) (resolve.DataSource, error) {
				return nil, nil
			},
			errorContains: "ConfigureFetch: fetch data source factory returned a nil data source",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory := &recordingFetchDataSourceFactory{newDataSource: test.newDataSource}
			configuration := plan.Configuration{
				DataSources: []plan.DataSource{
					mustDataSourceConfiguration(t, "root", &plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{{TypeName: "Query", FieldNames: []string{"hello"}}},
					}, mustCustomConfiguration(t, ConfigurationInput{
						Fetch:                  &FetchConfiguration{URL: "https://example.com/graphql"},
						FetchDataSourceFactory: factory,
						SchemaConfiguration:    mustSchema(t, nil, definition),
					})),
				},
				DisableResolveFieldPositions: true,
			}

			planned, report := planFactoryOperationWithReport(t, definition, `{ hello }`, "", configuration)
			require.True(t, report.HasErrors())
			require.Contains(t, report.Error(), test.errorContains)
			require.Nil(t, planned)
			require.Equal(t, 1, factory.calls)
		})
	}
}

func TestConfigureFetch_FactoryOverridesTransport(t *testing.T) {
	definition := `type Query { hello: String }`
	factoryDataSource := &Source{httpClient: &http.Client{}}
	factory := &recordingFetchDataSourceFactory{
		newDataSource: func(PlannedFetch) (resolve.DataSource, error) {
			return factoryDataSource, nil
		},
	}
	configuration := plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(t, "root", &plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{{TypeName: "Query", FieldNames: []string{"hello"}}},
			}, mustCustomConfiguration(t, ConfigurationInput{
				Fetch:                  &FetchConfiguration{URL: "https://example.com/graphql"},
				FetchDataSourceFactory: factory,
				GRPC:                   &grpcdatasource.GRPCConfiguration{},
				SchemaConfiguration:    mustSchema(t, nil, definition),
			})),
		},
		DisableResolveFieldPositions: true,
	}

	planned := planFactoryOperation(t, definition, `{ hello }`, "", configuration)
	require.Same(t, factoryDataSource, firstPlannedSingleFetch(t, planned).DataSource)
	require.Equal(t, 1, factory.calls)
}

func TestConfigureSubscription_IgnoresFetchFactory(t *testing.T) {
	definition := `type Subscription { hello: String }`
	factory := &recordingFetchDataSourceFactory{}
	configuration := plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(t, "subscription", &plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{{TypeName: "Subscription", FieldNames: []string{"hello"}}},
			}, mustCustomConfiguration(t, ConfigurationInput{
				FetchDataSourceFactory: factory,
				Subscription:           &SubscriptionConfiguration{URL: "wss://example.com/graphql"},
				SchemaConfiguration:    mustSchema(t, nil, definition),
			})),
		},
		DisableResolveFieldPositions: true,
	}

	planFactoryOperation(t, definition, `subscription { hello }`, "", configuration)
	require.Zero(t, factory.calls)
}
