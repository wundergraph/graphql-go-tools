package httpjsondatasource

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasourcetesting"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

const (
	schema = `
		type Query {
			friend: Friend
		}

		type Friend {
			name: String
		}
	`

	simpleOperation = `
		query {
			friend {
				name
			}
		}
	`
)

func TestHttpJsonDataSourcePlanning(t *testing.T) {
	t.Run("get request", datasourcetesting.RunTest(schema, simpleOperation, "",
		&plan.SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						Input:    []byte(`{"method":"GET","url":"https://example.com/$$0$$"}`),
						DataSource: &Source{
							client: NewPlanner(nil).getClient(),
						},
						Variables: resolve.NewVariables(
							&resolve.ObjectVariable{
								Path: []string{"id"},
							},
						),
					},
					FieldSets: []resolve.FieldSet{
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("friend"),
									Value: &resolve.Object{
										FieldSets: []resolve.FieldSet{
											{
												Fields: []resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
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
		plan.Configuration{
			FieldConfigurations: []plan.FieldConfiguration{
				{
					TypeName:   "Query",
					FieldNames: []string{"friend"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "url",
							Value: []byte("https://example.com/{{ .object.id }}"),
						},
						{
							Key:   "method",
							Value: []byte("GET"),
						},
					},
					DataSourcePlanner: &Planner{},
					FieldMappings: []plan.FieldMapping{
						{
							FieldName:             "friend",
							DisableDefaultMapping: true,
						},
					},
				},
			},
		},
	))
	t.Run("post request with body", datasourcetesting.RunTest(schema, simpleOperation, "",
		&plan.SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						Input:    []byte(`{"body":{"foo":"bar"},"method":"POST","url":"https://example.com"}`),
						DataSource: &Source{
							client: NewPlanner(nil).getClient(),
						},
						Variables: resolve.Variables{},
					},
					FieldSets: []resolve.FieldSet{
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("friend"),
									Value: &resolve.Object{
										FieldSets: []resolve.FieldSet{
											{
												Fields: []resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
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
		plan.Configuration{
			FieldConfigurations: []plan.FieldConfiguration{
				{
					TypeName:   "Query",
					FieldNames: []string{"friend"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "url",
							Value: []byte("https://example.com"),
						},
						{
							Key:   "method",
							Value: []byte("POST"),
						},
						{
							Key:   "body",
							Value: []byte(`{"foo":"bar"}`),
						},
					},
					DataSourcePlanner: &Planner{},
					FieldMappings: []plan.FieldMapping{
						{
							FieldName:             "friend",
							DisableDefaultMapping: true,
						},
					},
				},
			},
		},
	))
	t.Run("get request with headers", datasourcetesting.RunTest(schema, simpleOperation, "",
		&plan.SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						Input:    []byte(`{"headers":{"Authorization":"Bearer 123","X-API-Key":"456"},"method":"GET","url":"https://example.com"}`),
						DataSource: &Source{
							client: NewPlanner(nil).getClient(),
						},
						Variables: resolve.Variables{},
					},
					FieldSets: []resolve.FieldSet{
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("friend"),
									Value: &resolve.Object{
										FieldSets: []resolve.FieldSet{
											{
												Fields: []resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
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
		plan.Configuration{
			FieldConfigurations: []plan.FieldConfiguration{
				{
					TypeName:   "Query",
					FieldNames: []string{"friend"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "url",
							Value: []byte("https://example.com"),
						},
						{
							Key:   "method",
							Value: []byte("GET"),
						},
						{
							Key:   "headers",
							Value: []byte(`{"Authorization":"Bearer 123","X-API-Key":"456"}`),
						},
					},
					DataSourcePlanner: &Planner{},
					FieldMappings: []plan.FieldMapping{
						{
							FieldName:             "friend",
							DisableDefaultMapping: true,
						},
					},
				},
			},
		},
	))
}

func TestHttpJsonDataSource_Load(t *testing.T) {

	source := &Source{
		client: &http.Client{},
	}

	t.Run("simple get", func(t *testing.T) {

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, r.Method, http.MethodGet)
			w.Write([]byte(`ok`))
		}))

		defer server.Close()

		input := []byte(fmt.Sprintf(`{"method":"GET","url":"%s"}`, server.URL))
		pair := resolve.NewBufPair()
		err := source.Load(context.Background(), input, pair)
		assert.NoError(t, err)
		assert.Equal(t, `ok`, pair.Data.String())
	})
	t.Run("get with headers", func(t *testing.T) {

		authorization := "Bearer 123"
		xApiKey := "456"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, r.Method, http.MethodGet)
			assert.Equal(t, authorization, r.Header.Get("Authorization"))
			assert.Equal(t, xApiKey, r.Header.Get("X-API-KEY"))
			w.Write([]byte(`ok`))
		}))

		defer server.Close()

		input := []byte(fmt.Sprintf(`{"method":"GET","url":"%s","headers":{"Authorization":"%s","X-API-KEY":"%s"}}`, server.URL, authorization, xApiKey))
		pair := resolve.NewBufPair()
		err := source.Load(context.Background(), input, pair)
		assert.NoError(t, err)
		assert.Equal(t, `ok`, pair.Data.String())
	})
	t.Run("post with body", func(t *testing.T) {

		body := `{"foo":"bar"}`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			actualBody, err := ioutil.ReadAll(r.Body)
			assert.NoError(t, err)
			assert.Equal(t, string(actualBody), body)
			w.Write([]byte(`ok`))
		}))

		defer server.Close()

		input := []byte(fmt.Sprintf(`{"method":"POST","url":"%s","body":%s}`, server.URL, body))
		pair := resolve.NewBufPair()
		err := source.Load(context.Background(), input, pair)
		assert.NoError(t, err)
		assert.Equal(t, `ok`, pair.Data.String())
	})
}
