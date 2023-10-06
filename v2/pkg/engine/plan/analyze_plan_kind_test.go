package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
)

type expectation func(t *testing.T, subscription, streaming bool, err error)

func mustNotErr() expectation {
	return func(t *testing.T, subscription, streaming bool, err error) {
		assert.NoError(t, err)
	}
}

func mustSubscription(expectSubscription bool) expectation {
	return func(t *testing.T, subscription, streaming bool, err error) {
		assert.Equal(t, expectSubscription, subscription)
	}
}

func mustStreaming(expectStreaming bool) expectation {
	return func(t *testing.T, subscription, streaming bool, err error) {
		assert.Equal(t, expectStreaming, streaming)
	}
}

func TestAnalyzePlanKind(t *testing.T) {
	run := func(definition, operation, operationName string, expectations ...expectation) func(t *testing.T) {
		return func(t *testing.T) {
			def := unsafeparser.ParseGraphqlDocumentString(definition)
			op := unsafeparser.ParseGraphqlDocumentString(operation)
			err := asttransform.MergeDefinitionWithBaseSchema(&def)
			if err != nil {
				t.Fatal(err)
			}
			subscription, streaming, err := AnalyzePlanKind(&op, &def, operationName)
			for i := range expectations {
				expectations[i](t, subscription, streaming, err)
			}
		}
	}

	t.Run("query", run(testDefinition, `
		query MyQuery($id: ID!) {
			droid(id: $id){
				name
				friends {
					name
				}
				friends {
					name
				}
				primaryFunction
				favoriteEpisode
			}
		}`,
		"MyQuery",
		mustNotErr(),
		mustStreaming(false),
		mustSubscription(false),
	))
	t.Run("query stream", run(testDefinition, `
		query MyQuery($id: ID!) {
			droid(id: $id){
				name
				friends @stream {
					name
				}
				friends {
					name
				}
				primaryFunction
				favoriteEpisode
			}
		}`,
		"MyQuery",
		mustNotErr(),
		mustStreaming(true),
		mustSubscription(false),
	))
	t.Run("query defer", run(testDefinition, `
		query MyQuery($id: ID!) {
			droid(id: $id){
				name
				friends {
					name
				}
				friends {
					name
				}
				primaryFunction
				favoriteEpisode @defer
			}
		}`,
		"MyQuery",
		mustNotErr(),
		mustStreaming(true),
		mustSubscription(false),
	))
	t.Run("query defer", run(testDefinition, `
		query MyQuery($id: ID!) {
			droid(id: $id){
				name
				friends {
					name
				}
				friends {
					name
				}
				primaryFunction
				favoriteEpisode
			}
		}
		query OtherDeferredQuery {
			droid(id: $id){
				name
				friends @stream {
					name
				}
			}
		}`,
		"MyQuery",
		mustNotErr(),
		mustStreaming(false),
		mustSubscription(false),
	))
	t.Run("query defer different name", run(testDefinition, `
		query MyQuery($id: ID!) {
			droid(id: $id){
				name
				friends {
					name
				}
				friends {
					name
				}
				primaryFunction
				favoriteEpisode @defer
			}
		}`,
		"OperationNameNotExists",
		mustNotErr(),
		mustStreaming(false),
		mustSubscription(false),
	))
	t.Run("subscription", run(testDefinition, `
		subscription RemainingJedis {
			remainingJedis
		}`,
		"RemainingJedis",
		mustNotErr(),
		mustStreaming(false),
		mustSubscription(true),
	))
	t.Run("subscription with streaming", run(testDefinition, `
		subscription NewReviews {
			newReviews {
				id
				stars @defer
			}
		}`,
		"NewReviews",
		mustNotErr(),
		mustStreaming(true),
		mustSubscription(true),
	))
	t.Run("subscription name not exists", run(testDefinition, `
		subscription RemainingJedis {
			remainingJedis
		}`,
		"OperationNameNotExists",
		mustNotErr(),
		mustStreaming(false),
		mustSubscription(false),
	))
}
