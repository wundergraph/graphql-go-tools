package resolve

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func BenchmarkUpdateSubscription(b *testing.B) {
	for _, n := range []int{1, 10, 100, 1000} {
		b.Run(func() string {
			switch n {
			case 1:
				return "1_subs"
			case 10:
				return "10_subs"
			case 100:
				return "100_subs"
			default:
				return "1000_subs"
			}
		}(), func(b *testing.B) {
			ctx, cancel := context.WithCancel(context.Background())
			b.Cleanup(cancel)

			streamDone := make(chan struct{})
			b.Cleanup(func() { close(streamDone) })

			updaters := make([]func([]byte), n)
			var setupWg sync.WaitGroup
			setupWg.Add(n)

			// Each subscription gets its own fakeStream so that its
			// subscriptionOnStartFn captures the correct slot index i.
			// executeStartupHooks uses add.resolve.Trigger.Source (the
			// subscription's own plan source), so the hook fires on the
			// right stream regardless of goroutine scheduling order.
			// All subscriptions share the same static input, so they all
			// land on a single trigger whose subscriptionIdentifiers map
			// grows to N entries — the map we want to exercise.
			makePlan := func(i int) *GraphQLSubscription {
				stream := createFakeStream(
					func(counter int) (message string, done bool) {
						<-streamDone
						return "", true
					},
					0,
					nil,
					func(hookCtx StartupHookContext, _ []byte) error {
						updaters[i] = hookCtx.Updater
						setupWg.Done()
						return nil
					},
				)

				fetches := Sequence()
				fetches.Trigger = &FetchTreeNode{
					Kind: FetchTreeNodeKindTrigger,
					Item: &FetchItem{
						Fetch: &SingleFetch{
							FetchDependencies: FetchDependencies{
								FetchID: 0,
							},
							Info: &FetchInfo{
								DataSourceID:   "0",
								DataSourceName: "counter",
								QueryPlan: &QueryPlan{
									Query: "subscription {\n    counter\n}",
								},
							},
						},
						ResponsePath: "counter",
					},
				}

				return &GraphQLSubscription{
					Trigger: GraphQLSubscriptionTrigger{
						Source: stream,
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { counter }"}}`),
								},
							},
						},
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath:   []string{"data"},
							SelectResponseErrorsPath: []string{"errors"},
						},
					},
					Response: &GraphQLResponse{
						Data: &Object{
							Fields: []*Field{
								{
									Name: []byte("counter"),
									Value: &Integer{
										Path: []string{"counter"},
									},
									Info: &FieldInfo{
										Name:                "counter",
										ExactParentTypeName: "Subscription",
										Source: TypeFieldSource{
											IDs:   []string{"0"},
											Names: []string{"counter"},
										},
										FetchID: 0,
									},
								},
							},
						},
						Fetches: fetches,
					},
				}
			}

			resolver := newResolver(ctx)

			recorders := make([]*SubscriptionRecorder, n)
			for i := 0; i < n; i++ {
				recorders[i] = &SubscriptionRecorder{
					buf:      &bytes.Buffer{},
					messages: []string{},
					complete: atomic.Bool{},
				}
				subCtx := NewContext(context.Background())
				id := SubscriptionIdentifier{
					ConnectionID:   1,
					SubscriptionID: int64(i + 1),
				}
				err := resolver.AsyncResolveGraphQLSubscription(subCtx, makePlan(i), recorders[i], id)
				require.NoError(b, err)
			}

			// Block until all N startup hooks have fired, guaranteeing all
			// entries are in subscriptionIdentifiers before timing starts.
			setupWg.Wait()

			b.ResetTimer()
			b.ReportAllocs()

			data := []byte(`{"data":{"counter":1}}`)
			for i := 0; i < b.N; i++ {
				// Update every subscription on the trigger sequentially.
				// Each call does an O(1) map lookup in subscriptionIdentifiers
				// then delivers to that subscription's worker.
				for j := 0; j < n; j++ {
					updaters[j](data)
				}
			}

			// Every recorder must have received exactly b.N messages.
			for i := 0; i < n; i++ {
				recorders[i].AwaitMessages(b, b.N, 30*time.Second)
			}
		})
	}
}
