package pubsub_datasource

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type testPubsub struct {
}

func (t *testPubsub) Subscribe(_ context.Context, _ NatsSubscriptionEventConfiguration, _ resolve.SubscriptionUpdater) error {
	return errors.New("not implemented")
}
func (t *testPubsub) Publish(_ context.Context, _ NatsPublishAndRequestEventConfiguration) error {
	return errors.New("not implemented")
}

func (t *testPubsub) Request(_ context.Context, _ NatsPublishAndRequestEventConfiguration, _ io.Writer) error {
	return errors.New("not implemented")
}

func TestPubSub(t *testing.T) {
	factory := &Factory[Configuration]{
		natsPubSubByProviderID: map[string]NatsPubSub{"default": &testPubsub{}},
	}

	const schema = `
	type Query {
		helloQuery(userKey: UserKey!): User! @edfs__natsRequest(subject: "tenants.{{ args.userKey.tenantId }}.users.{{ args.userKey.id }}")
	}

	type Mutation {
		helloMutation(userKey: UserKey!): edfs__PublishResult! @edfs__natsPublish(subject: "tenants.{{ args.userKey.tenantId }}.users.{{ args.userKey.id }}")
	}

	type Subscription {
		helloSubscription(userKey: UserKey!): User! @edfs__natsSubscribe(subjects: ["tenants.{{ args.userKey.tenantId }}.users.{{ args.userKey.id }}"])
		subscriptionWithMultipleSubjects(userKeyOne: UserKey!, userKeyTwo: UserKey!): User! @edfs__natsSubscribe(subjects: ["tenantsOne.{{ args.userKeyOne.tenantId }}.users.{{ args.userKeyOne.id }}", "tenantsTwo.{{ args.userKeyTwo.tenantId }}.users.{{ args.userKeyTwo.id }}"])
		subscriptionWithStaticValues: User! @edfs__natsSubscribe(subjects: ["tenants.1.users.1"])
		subscriptionWithArgTemplateAndStaticValue(nestedUserKey: NestedUserKey!): User! @edfs__natsSubscribe(subjects: ["tenants.1.users.{{ args.nestedUserKey.user.id }}"])
	}
	
	type User @key(fields: "id tenant { id }") {
		id: Int! @external
		tenant: Tenant! @external
	}

	type Tenant {
		id: Int! @external
	}

	input UserKey {
		id: Int!
		tenantId: Int!
	}

	input NestedUserKey {
		user: UserInput!
		tenant: TenantInput!
	}
	
	input UserInput {
		id: Int!
	}
	
	input TenantInput {
		id: Int!
	}

	type edfs__PublishResult {
		success: Boolean!	
	}

	input edfs__NatsStreamConfiguration {
		consumerName: String!
		streamName: String!
	}
	`

	dataSourceCustomConfig := Configuration{
		Events: []EventConfiguration{
			{
				Metadata: &EventMetadata{
					FieldName:  "helloQuery",
					ProviderID: "default",
					Type:       EventTypeRequest,
					TypeName:   "Query",
				},
				Configuration: &NatsEventConfiguration{
					Subjects: []string{"tenants.{{ args.userKey.tenantId }}.users.{{ args.userKey.id }}"},
				},
			},
			{
				Metadata: &EventMetadata{
					FieldName:  "helloMutation",
					ProviderID: "default",
					Type:       EventTypePublish,
					TypeName:   "Mutation",
				},
				Configuration: &NatsEventConfiguration{
					Subjects: []string{"tenants.{{ args.userKey.tenantId }}.users.{{ args.userKey.id }}"},
				},
			},
			{
				Metadata: &EventMetadata{
					FieldName:  "helloSubscription",
					ProviderID: "default",
					Type:       EventTypeSubscribe,
					TypeName:   "Subscription",
				},
				Configuration: &NatsEventConfiguration{
					Subjects: []string{"tenants.{{ args.userKey.tenantId }}.users.{{ args.userKey.id }}"},
				},
			},
			{
				Metadata: &EventMetadata{
					FieldName:  "subscriptionWithMultipleSubjects",
					ProviderID: "default",
					Type:       EventTypeSubscribe,
					TypeName:   "Subscription",
				},
				Configuration: &NatsEventConfiguration{
					Subjects: []string{"tenantsOne.{{ args.userKeyOne.tenantId }}.users.{{ args.userKeyOne.id }}", "tenantsTwo.{{ args.userKeyTwo.tenantId }}.users.{{ args.userKeyTwo.id }}"},
				},
			},
			{
				Metadata: &EventMetadata{
					FieldName:  "subscriptionWithStaticValues",
					ProviderID: "default",
					Type:       EventTypeSubscribe,
					TypeName:   "Subscription",
				},
				Configuration: &NatsEventConfiguration{
					Subjects: []string{"tenants.1.users.1"},
				},
			},
			{
				Metadata: &EventMetadata{
					FieldName:  "subscriptionWithArgTemplateAndStaticValue",
					ProviderID: "default",
					Type:       EventTypeSubscribe,
					TypeName:   "Subscription",
				},
				Configuration: &NatsEventConfiguration{
					Subjects: []string{"tenants.1.users.{{ args.nestedUserKey.user.id }}"},
				},
			},
		},
	}

	dataSourceConfiguration, err := plan.NewDataSourceConfiguration[Configuration](
		"test",
		factory,
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"helloQuery"},
				},
				{
					TypeName:   "Mutation",
					FieldNames: []string{"helloMutation"},
				},
				{
					TypeName:   "Subscription",
					FieldNames: []string{"helloSubscription"},
				},
				{
					TypeName:   "Subscription",
					FieldNames: []string{"subscriptionWithMultipleSubjects"},
				},
				{
					TypeName:   "Subscription",
					FieldNames: []string{"subscriptionWithStaticValues"},
				},
				{
					TypeName:   "Subscription",
					FieldNames: []string{"subscriptionWithArgTemplateAndStaticValue"},
				},
			},
			ChildNodes: []plan.TypeField{
				// Entities are child nodes in pubsub datasources
				{
					TypeName:   "User",
					FieldNames: []string{"id", "tenant"},
				},
				{
					TypeName:   "Tenant",
					FieldNames: []string{"id"},
				},
				{
					TypeName:   "edfs__PublishResult",
					FieldNames: []string{"success"},
				},
			},
		},
		dataSourceCustomConfig,
	)
	require.NoError(t, err)

	planConfig := plan.Configuration{
		DataSources: []plan.DataSource{
			dataSourceConfiguration,
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "helloQuery",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "userKey",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Mutation",
				FieldName: "helloMutation",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "userKey",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Subscription",
				FieldName: "helloSubscription",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "userKey",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Subscription",
				FieldName: "subscriptionWithMultipleSubjects",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "userKeyOne",
						SourceType: plan.FieldArgumentSource,
					},
					{
						Name:       "userKeyTwo",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Subscription",
				FieldName: "subscriptionWithArgTemplateAndStaticValue",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "nestedUserKey",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		DisableResolveFieldPositions: true,
	}

	t.Run("query", func(t *testing.T) {
		const operation = "query HelloQuery { helloQuery(userKey:{id:42,tenantId:3}) { id } }"
		const operationName = `HelloQuery`
		expect := &plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("helloQuery"),
							Value: &resolve.Object{
								Path:     []string{"helloQuery"},
								Nullable: false,
								Fields: []*resolve.Field{
									{
										Name: []byte("id"),
										Value: &resolve.Integer{
											Path:     []string{"id"},
											Nullable: false,
										},
									},
								},
							},
						},
					},
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input: `{"subject":"tenants.$$0$$.users.$$1$$", "data": {"userKey":$$2$$}, "providerId":"default"}`,
							Variables: resolve.Variables{
								&resolve.ContextVariable{
									Path:     []string{"a", "tenantId"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["integer"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"a", "id"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["integer"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["object"],"properties":{"id":{"type":["integer"]},"tenantId":{"type":["integer"]}},"required":["id","tenantId"],"additionalProperties":false}`),
								},
							},
							DataSource: &NatsRequestDataSource{
								pubSub: &testPubsub{},
							},
							PostProcessing: resolve.PostProcessingConfiguration{
								MergePath: []string{"helloQuery"},
							},
						},
						DataSourceIdentifier: []byte("pubsub_datasource.NatsRequestDataSource"),
					},
				},
			},
		}
		datasourcetesting.RunTest(schema, operation, operationName, expect, planConfig)(t)
	})

	t.Run("mutation", func(t *testing.T) {
		const operation = "mutation HelloMutation { helloMutation(userKey:{id:42,tenantId:3}) { success } }"
		const operationName = `HelloMutation`
		expect := &plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("helloMutation"),
							Value: &resolve.Object{
								Path:     []string{"helloMutation"},
								Nullable: false,
								Fields: []*resolve.Field{
									{
										Name: []byte("success"),
										Value: &resolve.Boolean{
											Path:     []string{"success"},
											Nullable: false,
										},
									},
								},
							},
						},
					},
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input: `{"subject":"tenants.$$0$$.users.$$1$$", "data": {"userKey":$$2$$}, "providerId":"default"}`,
							Variables: resolve.Variables{
								&resolve.ContextVariable{
									Path:     []string{"a", "tenantId"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["integer"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"a", "id"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["integer"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["object"],"properties":{"id":{"type":["integer"]},"tenantId":{"type":["integer"]}},"required":["id","tenantId"],"additionalProperties":false}`),
								},
							},
							DataSource: &NatsPublishDataSource{
								pubSub: &testPubsub{},
							},
							PostProcessing: resolve.PostProcessingConfiguration{
								MergePath: []string{"helloMutation"},
							},
						},
						DataSourceIdentifier: []byte("pubsub_datasource.NatsPublishDataSource"),
					},
				},
			},
		}
		datasourcetesting.RunTest(schema, operation, operationName, expect, planConfig)(t)
	})

	t.Run("subscription", func(t *testing.T) {
		const operation = "subscription HelloSubscription { helloSubscription(userKey:{id:42,tenantId:3}) { id } }"
		const operationName = `HelloSubscription`
		expect := &plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte(`{"providerId":"default","subjects":["tenants.$$0$$.users.$$1$$"]}`),
					Variables: resolve.Variables{
						&resolve.ContextVariable{
							Path:     []string{"a", "tenantId"},
							Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["integer"]}`),
						},
						&resolve.ContextVariable{
							Path:     []string{"a", "id"},
							Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["integer"]}`),
						},
					},
					Source: &NatsSubscriptionSource{
						pubSub: &testPubsub{},
					},
					PostProcessing: resolve.PostProcessingConfiguration{
						MergePath: []string{"helloSubscription"},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("helloSubscription"),
								Value: &resolve.Object{
									Path:     []string{"helloSubscription"},
									Nullable: false,
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Integer{
												Path:     []string{"id"},
												Nullable: false,
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
		datasourcetesting.RunTest(schema, operation, operationName, expect, planConfig)(t)
	})

	t.Run("subscription with multiple subjects", func(t *testing.T) {
		const operation = "subscription SubscriptionWithMultipleSubjects { subscriptionWithMultipleSubjects(userKeyOne:{id:42,tenantId:3},userKeyTwo:{id:24,tenantId:99}) { id } }"
		const operationName = `SubscriptionWithMultipleSubjects`
		expect := &plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte(`{"providerId":"default","subjects":["tenantsOne.$$0$$.users.$$1$$","tenantsTwo.$$2$$.users.$$3$$"]}`),
					Variables: resolve.Variables{
						&resolve.ContextVariable{
							Path:     []string{"a", "tenantId"},
							Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["integer"]}`),
						},
						&resolve.ContextVariable{
							Path:     []string{"a", "id"},
							Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["integer"]}`),
						},
						&resolve.ContextVariable{
							Path:     []string{"b", "tenantId"},
							Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["integer"]}`),
						},
						&resolve.ContextVariable{
							Path:     []string{"b", "id"},
							Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["integer"]}`),
						},
					},
					Source: &NatsSubscriptionSource{
						pubSub: &testPubsub{},
					},
					PostProcessing: resolve.PostProcessingConfiguration{
						MergePath: []string{"subscriptionWithMultipleSubjects"},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("subscriptionWithMultipleSubjects"),
								Value: &resolve.Object{
									Path:     []string{"subscriptionWithMultipleSubjects"},
									Nullable: false,
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Integer{
												Path:     []string{"id"},
												Nullable: false,
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
		datasourcetesting.RunTest(schema, operation, operationName, expect, planConfig)(t)
	})

	t.Run("subscription with only static values", func(t *testing.T) {
		const operation = "subscription SubscriptionWithStaticValues { subscriptionWithStaticValues { id } }"
		const operationName = `SubscriptionWithStaticValues`
		expect := &plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte(`{"providerId":"default","subjects":["tenants.1.users.1"]}`),
					Source: &NatsSubscriptionSource{
						pubSub: &testPubsub{},
					},
					PostProcessing: resolve.PostProcessingConfiguration{
						MergePath: []string{"subscriptionWithStaticValues"},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("subscriptionWithStaticValues"),
								Value: &resolve.Object{
									Path:     []string{"subscriptionWithStaticValues"},
									Nullable: false,
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Integer{
												Path:     []string{"id"},
												Nullable: false,
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
		datasourcetesting.RunTest(schema, operation, operationName, expect, planConfig)(t)
	})

	t.Run("subscription with deeply nested argument and static value", func(t *testing.T) {
		const operation = "subscription SubscriptionWithArgTemplateAndStaticValue { subscriptionWithArgTemplateAndStaticValue(nestedUserKey: { user: { id: 44, tenantId: 2 } }) { id } }"
		const operationName = `SubscriptionWithArgTemplateAndStaticValue`
		expect := &plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte(`{"providerId":"default","subjects":["tenants.1.users.$$0$$"]}`),
					Variables: resolve.Variables{
						&resolve.ContextVariable{
							Path:     []string{"a", "user", "id"},
							Renderer: resolve.NewPlainVariableRendererWithValidation(`{"type":["integer"]}`),
						},
					},
					Source: &NatsSubscriptionSource{
						pubSub: &testPubsub{},
					},
					PostProcessing: resolve.PostProcessingConfiguration{
						MergePath: []string{"subscriptionWithArgTemplateAndStaticValue"},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("subscriptionWithArgTemplateAndStaticValue"),
								Value: &resolve.Object{
									Path:     []string{"subscriptionWithArgTemplateAndStaticValue"},
									Nullable: false,
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Integer{
												Path:     []string{"id"},
												Nullable: false,
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
		datasourcetesting.RunTest(schema, operation, operationName, expect, planConfig)(t)
	})
}
