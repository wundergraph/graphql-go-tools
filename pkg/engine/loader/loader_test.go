package loader

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/rest_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/staticdatasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
)

func TestLoader_GraphQL(t *testing.T) {
	factory := &graphql_datasource.Factory{
		Client: httpclient.NewFastHttpClient(httpclient.DefaultFastHttpClient),
	}
	expected := plan.Configuration{
		DefaultFlushInterval: 10,
		Schema:               "scalar String scalar Int scalar ID schema { query: Query } type Product { upc: String! name: String! price: Int! reviews: [Review] } type Query { me: User topProducts(first: Int = 5): [Product] } type Review { body: String! author: User! product: Product! } type User { id: ID! username: String! reviews: [Review] }",
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"me"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "username"},
					},
				},
				Custom: uglifyJSON(graphql_datasource.ConfigJson(graphql_datasource.Configuration{
					Federation: graphql_datasource.FederationConfiguration{
						Enabled:    true,
						ServiceSDL: "extend type Query {me: User} type User @key(fields: \"id\"){ id: ID! username: String!}",
					},
					Fetch: graphql_datasource.FetchConfiguration{
						URL: "http://user.service",
					},
				})),
				Factory: factory,
			},
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"topProducts"},
					},
					{
						TypeName:   "Subscription",
						FieldNames: []string{"updatedPrice"},
					},
					{
						TypeName:   "Product",
						FieldNames: []string{"upc", "name", "price"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Product",
						FieldNames: []string{"upc", "name", "price"},
					},
				},
				Custom: uglifyJSON(graphql_datasource.ConfigJson(graphql_datasource.Configuration{
					Fetch: graphql_datasource.FetchConfiguration{
						URL: "http://product.service",
					},
					Subscription: graphql_datasource.SubscriptionConfiguration{
						URL: "ws://product.service",
					},
					Federation: graphql_datasource.FederationConfiguration{
						Enabled:    true,
						ServiceSDL: "extend type Query {topProducts(first: Int = 5): [Product]} type Product @key(fields: \"upc\") {upc: String! name: String! price: Int!}",
					},
				})),
				Factory: factory,
			},
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"reviews"},
					},
					{
						TypeName:   "Product",
						FieldNames: []string{"reviews"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Review",
						FieldNames: []string{"body", "author", "product"},
					},
					{
						TypeName:   "User",
						FieldNames: []string{"id", "username"},
					},
					{
						TypeName:   "Product",
						FieldNames: []string{"upc"},
					},
				},
				Custom: uglifyJSON(graphql_datasource.ConfigJson(graphql_datasource.Configuration{
					Fetch: graphql_datasource.FetchConfiguration{
						URL: "http://review.service",
					},
					Federation: graphql_datasource.FederationConfiguration{
						Enabled:    true,
						ServiceSDL: "type Review { body: String! author: User! @provides(fields: \"username\") product: Product! } extend type User @key(fields: \"id\") { id: ID! @external reviews: [Review] } extend type Product @key(fields: \"upc\") { upc: String! @external reviews: [Review] }",
					},
				})),
				Factory: factory,
			},
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "topProducts",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "first",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:       "User",
				FieldName:      "reviews",
				RequiresFields: []string{"id"},
			},
			{
				TypeName:       "Product",
				FieldName:      "name",
				RequiresFields: []string{"upc"},
			},
			{
				TypeName:       "Product",
				FieldName:      "price",
				RequiresFields: []string{"upc"},
			},
			{
				TypeName:       "Product",
				FieldName:      "reviews",
				RequiresFields: []string{"upc"},
			},
		},
	}

	data, err := ioutil.ReadFile("./testdata/graphql.json")
	assert.NoError(t, err)

	client := httpclient.NewFastHttpClient(httpclient.DefaultFastHttpClient)
	loader := New(NewDefaultFactoryResolver(client))

	actual, err := loader.Load(data)
	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestLoader_REST(t *testing.T) {
	factory := &rest_datasource.Factory{
		Client: httpclient.NewFastHttpClient(httpclient.DefaultFastHttpClient),
	}
	expected := plan.Configuration{
		Schema: "type Query { friend: Friend withArgument(id: String!, name: String, optional: String): Friend withArrayArguments(names: [String]): Friend } type Subscription { friend: Friend withArgument(id: String!, name: String, optional: String): Friend withArrayArguments(names: [String]): Friend } type Friend { name: String pet: Pet } type Pet { id: String name: String }",
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"withArgument"},
					},
				},
				Custom: uglifyJSON(rest_datasource.ConfigJSON(rest_datasource.Configuration{
					Fetch: rest_datasource.FetchConfiguration{
						URL:    "https://example.com/friend",
						Method: "GET",
						Query: []rest_datasource.QueryConfiguration{
							{
								Name:  "static",
								Value: "staticValue",
							},
							{
								Name:  "static",
								Value: "secondStaticValue",
							},
							{
								Name:  "name",
								Value: "{{ .arguments.name }}",
							},
							{
								Name:  "id",
								Value: "{{ .arguments.id }}",
							},
							{
								Name:  "optional",
								Value: "{{ .arguments.optional }}",
							},
						},
					},
				})),
				Factory: factory,
			},
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:              "Query",
				FieldName:             "withArgument",
				DisableDefaultMapping: true,
			},
		},
	}

	data, err := ioutil.ReadFile("./testdata/rest.json")
	assert.NoError(t, err)

	client := httpclient.NewFastHttpClient(httpclient.DefaultFastHttpClient)
	loader := New(NewDefaultFactoryResolver(client))

	actual, err := loader.Load(data)
	assert.NoError(t, err)
	assert.Equal(t, expected, actual)

}

func TestLoader_Static(t *testing.T) {
	factory := &staticdatasource.Factory{}
	expected := plan.Configuration{
		Schema: "type Query { hello: String }",
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"hello"},
					},
				},
				Custom: staticdatasource.ConfigJSON(staticdatasource.Configuration{
					Data: "world",
				}),
				Factory: factory,
			},
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:              "Query",
				FieldName:             "hello",
				DisableDefaultMapping: true,
			},
		},
	}

	data, err := ioutil.ReadFile("./testdata/static.json")
	assert.NoError(t, err)

	client := httpclient.NewFastHttpClient(httpclient.DefaultFastHttpClient)
	loader := New(NewDefaultFactoryResolver(client))

	actual, err := loader.Load(data)
	assert.NoError(t, err)
	assert.Equal(t, expected, actual)

}
