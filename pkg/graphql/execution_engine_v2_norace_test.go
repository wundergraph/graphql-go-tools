//go:build !race

package graphql

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting"
	"github.com/wundergraph/graphql-go-tools/pkg/testing/flags"
)

// This tests produces data races in the generated gql code. Disable it when the race
// detector is enabled.
func TestExecutionEngineV2_FederationAndSubscription_IntegrationTest(t *testing.T) {
	if flags.IsWindows {
		t.Skip("skip on windows - test is timing dependendent")
	}

	runIntegration := func(t *testing.T, enableDataLoader bool, secondRun bool) {
		t.Helper()
		ctx, cancelFn := context.WithCancel(context.Background())
		setup := newFederationSetup()
		t.Cleanup(func() {
			cancelFn()
			setup.accountsUpstreamServer.Close()
			setup.productsUpstreamServer.Close()
			setup.reviewsUpstreamServer.Close()
			setup.pollingUpstreamServer.Close()
		})

		engine, schema, err := newFederationEngine(ctx, setup, enableDataLoader)
		require.NoError(t, err)

		t.Run("should successfully execute a federation operation", func(t *testing.T) {
			gqlRequest := &Request{
				OperationName: "",
				Variables:     nil,
				Query:         federationtesting.QueryReviewsOfMe,
			}

			validationResult, err := gqlRequest.ValidateForSchema(schema)
			require.NoError(t, err)
			require.True(t, validationResult.Valid)

			execCtx, execCtxCancelFn := context.WithCancel(context.Background())
			defer execCtxCancelFn()

			resultWriter := NewEngineResultWriter()
			err = engine.Execute(execCtx, gqlRequest, &resultWriter)
			if assert.NoError(t, err) {
				assert.Equal(t,
					`{"data":{"me":{"reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby","price":11}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora","price":22}}]}}}`,
					resultWriter.String(),
				)
			}
		})

		t.Run("should successfully execute a federation subscription", func(t *testing.T) {
			query := `
subscription UpdatedPrice {
  updatedPrice {
    name
    price
	reviews {
      body
      author {
		id
		username
      }
    }
  }
}`

			gqlRequest := &Request{
				OperationName: "",
				Variables:     nil,
				Query:         query,
			}

			validationResult, err := gqlRequest.ValidateForSchema(schema)
			require.NoError(t, err)
			require.True(t, validationResult.Valid)

			execCtx, execCtxCancelFn := context.WithCancel(context.Background())
			defer execCtxCancelFn()

			message := make(chan string)
			resultWriter := NewEngineResultWriter()
			resultWriter.SetFlushCallback(func(data []byte) {
				message <- string(data)
			})

			go func() {
				err := engine.Execute(execCtx, gqlRequest, &resultWriter)
				assert.NoError(t, err)
			}()

			if assert.NoError(t, err) {
				assert.Eventuallyf(t, func() bool {
					msg := `{"data":{"updatedPrice":{"name":"Trilby","price":%d,"reviews":[{"body":"A highly effective form of birth control.","author":{"id":"1234","username":"Me"}}]}}}`
					price := 10
					if secondRun {
						price += 2
					}

					firstMessage := <-message
					expectedFirstMessage := fmt.Sprintf(msg, price)
					assert.Equal(t, expectedFirstMessage, firstMessage)

					secondMessage := <-message
					expectedSecondMessage := fmt.Sprintf(msg, price+1)
					assert.Equal(t, expectedSecondMessage, secondMessage)
					return true
				}, time.Second, 10*time.Millisecond, "did not receive expected messages")
			}
		})

		/* Uncomment when polling subscriptions are ready:

		t.Run("should successfully subscribe to rest data source", func(t *testing.T) {
			gqlRequest := &Request{
				OperationName: "",
				Variables:     nil,
				Query:         "subscription Counter { counter }",
			}

			validationResult, err := gqlRequest.ValidateForSchema(setup.schema)
			require.NoError(t, err)
			require.True(t, validationResult.Valid)

			execCtx, execCtxCancelFn := context.WithCancel(context.Background())
			defer execCtxCancelFn()

			message := make(chan string)
			resultWriter := NewEngineResultWriter()
			resultWriter.SetFlushCallback(func(data []byte) {
				fmt.Println(string(data))
				message <- string(data)
			})

			err = setup.engine.Execute(execCtx, gqlRequest, &resultWriter)
			assert.NoError(t, err)

			if assert.NoError(t, err) {
				assert.Eventuallyf(t, func() bool {
					firstMessage := <-message
					assert.Equal(t, `{"data":{"counter":1}}`, firstMessage)
					secondMessage := <-message
					assert.Equal(t, `{"data":{"counter":2}}`, secondMessage)
					return true
				}, time.Second, 10*time.Millisecond, "did not receive expected messages")
			}
		})
		*/

	}

	t.Run("federation", func(t *testing.T) {
		runIntegration(t, false, false)
	})

	t.Run("federation with data loader enabled", func(t *testing.T) {
		runIntegration(t, true, true)
	})
}
