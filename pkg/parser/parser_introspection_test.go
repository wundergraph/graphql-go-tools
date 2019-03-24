package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/introspection"
	"testing"
)

func TestParser_ParseIntrospectionResponse(t *testing.T) {

	type check func(p *Parser)

	run := func(response *introspection.Response, checks ...check) {
		p := NewParser()
		err := p.ParseIntrospectionResponse(response)
		if err != nil {
			panic(err)
		}

		for i := range checks {
			checks[i](p)
		}
	}

	mustHaveSchema := func(wantQuery, wantMutation, wantSubscription string) check {
		return func(p *Parser) {
			if wantQuery != "" {
				gotQuery := string(p.ByteSlice(p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Query))
				if wantQuery != gotQuery {
					panic(fmt.Errorf("mustHaveSchema: want(query): %s, got: %s", wantQuery, gotQuery))
				}
			}
			if wantMutation != "" {
				gotMutation := string(p.ByteSlice(p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Mutation))
				if wantMutation != gotMutation {
					panic(fmt.Errorf("mustHaveSchema: want(mutation): %s, got: %s", wantMutation, gotMutation))
				}
			}
			if wantSubscription != "" {
				gotSubscription := string(p.ByteSlice(p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Subscription))
				if wantSubscription != gotSubscription {
					panic(fmt.Errorf("mustHaveSchema: want(subscription): %s, got: %s", wantSubscription, gotSubscription))
				}
			}
		}
	}

	t.Run("simple schema", func(t *testing.T) {
		run(&introspection.Response{
			Data: introspection.Data{
				Schema: introspection.Schema{
					QueryType: &introspection.TypeName{
						Name: "Query",
					},
					MutationType: &introspection.TypeName{
						Name: "Mutation",
					},
					SubscriptionType: &introspection.TypeName{
						Name: "Subscription",
					},
				},
			},
		}, mustHaveSchema("Query", "Mutation", "Subscription"))
	})
}
