package resolve

import (
	"bytes"
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

func TestResolver_GroupedSubscriptions(t *testing.T) {
	// This is just test setup. Actual tests are within subtests down below.
	type subscription struct {
		ctx      *Context
		id       SubscriptionIdentifier
		plan     *GraphQLSubscription
		recorder *SubscriptionRecorder
	}

	newSubscription := func(ctx context.Context, connID, subID int64, plan *GraphQLSubscription) subscription {
		id := SubscriptionIdentifier{
			ConnectionID:   ConnectionID(connID),
			SubscriptionID: subID,
		}

		recorder := &SubscriptionRecorder{buf: &bytes.Buffer{}, messages: []string{}}

		return subscription{
			ctx:      NewContext(ctx),
			id:       id,
			plan:     plan,
			recorder: recorder,
		}
	}

	subscribe := func(resolver *Resolver, sub *subscription) error {
		return resolver.AsyncResolveGraphQLSubscription(sub.ctx, sub.plan, sub.recorder, sub.id)
	}

	newDataSource := func() *countingDataSource {
		return &countingDataSource{
			started: make(chan struct{}),
			data:    []byte(`{"f1":1,"f2":2}`),
		}
	}

	// newPlanFactory returns a plan factory bound to the given datasource.
	newPlanFactory := func(ds *countingDataSource) func(fields ...string) *GraphQLSubscription {
		trigger := GraphQLSubscriptionTrigger{
			Source: ds,
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(`{"method":"POST","url":"http://localhost:4000","body":{"query":"subscription { f1 f2 }"}}`),
					},
				},
			},
		}

		return func(fields ...string) *GraphQLSubscription {
			responseFields := make([]*Field, 0, len(fields))
			for _, name := range fields {
				responseFields = append(responseFields, &Field{
					Name: []byte(name),
					Value: &Integer{
						Path:     []string{name},
						Nullable: false,
					},
				})
			}

			return &GraphQLSubscription{
				Trigger: trigger,
				Response: &GraphQLResponse{
					Data: &Object{
						Fields: responseFields,
					},
					Fetches: SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: ds,
						},
						Info: &FetchInfo{},
					}, ""),
					Info: &GraphQLResponseInfo{},
				},
			}
		}
	}

	resolverOptions := ResolverOptions{
		AsyncErrorWriter:             &TestErrorWriter{},
		ResolveSubscriptionsInGroups: true, // this is what the tests are about
	}

	t.Run("2 out of 3 subscribers are deduplicated", func(t *testing.T) {
		resolver := New(t.Context(), resolverOptions)

		ds := newDataSource()
		newPlan := newPlanFactory(ds)
		plan1 := newPlan("f1")
		plan2 := newPlan("f1", "f2")

		// create and register subscribers
		sub1 := newSubscription(t.Context(), 1, 1, plan1)
		sub2 := newSubscription(t.Context(), 2, 2, plan1)
		sub3 := newSubscription(t.Context(), 3, 3, plan2)
		require.NoError(t, subscribe(resolver, &sub1))
		require.NoError(t, subscribe(resolver, &sub2))
		require.NoError(t, subscribe(resolver, &sub3))

		// wait until resolver starts datasource
		<-ds.started

		// we expect the datasource to be called once because we only have one trigger
		require.Equal(t, int32(1), ds.startCount.Load())

		// send an event into the updater
		ds.updater.Update([]byte(`{"data":{}}`))

		// await the event on all subscribers
		sub1.recorder.AwaitMessages(t, 1, time.Second*5)
		sub2.recorder.AwaitMessages(t, 1, time.Second*5)
		sub3.recorder.AwaitMessages(t, 1, time.Second*5)

		// verify sub1 and sub2 got the correct and identical message
		assert.Equal(t, `{"data":{"f1":1}}`, sub1.recorder.Messages()[0])
		assert.Equal(t, sub1.recorder.Messages()[0], sub2.recorder.Messages()[0])

		// verify sub3 got it's own resolved message
		assert.Equal(t, `{"data":{"f1":1,"f2":2}}`, sub3.recorder.Messages()[0])

		// Now we know all 3 subscribers got their data.
		// sub1 and sub3 should share one fetch, since they share the same query.
		// sub2 needs a dedicated fetch since it's query is different.
		// Have we been able to achieve this with just two fetches (aka ds loads)?
		assert.Equal(t, int32(2), ds.loadCount.Load())
	})

	t.Run("subscriber with query plan enabled is not grouped with identical subscribers", func(t *testing.T) {
		resolver := New(t.Context(), resolverOptions)

		ds := newDataSource()
		newPlan := newPlanFactory(ds)
		plan := newPlan("f1", "f2")

		// create and register subscribers, all using the same plan/query,
		// but sub3 includes the query plan in the response --> not groupable
		sub1 := newSubscription(t.Context(), 1, 1, plan)
		sub2 := newSubscription(t.Context(), 2, 2, plan)
		sub3 := newSubscription(t.Context(), 3, 3, plan)
		sub3.ctx.ExecutionOptions.IncludeQueryPlanInResponse = true
		require.NoError(t, subscribe(resolver, &sub1))
		require.NoError(t, subscribe(resolver, &sub2))
		require.NoError(t, subscribe(resolver, &sub3))

		// wait until resolver starts datasource
		<-ds.started

		// we expect the datasource to be called once because we only have one trigger
		require.Equal(t, int32(1), ds.startCount.Load())

		// send an event into the updater
		ds.updater.Update([]byte(`{"data":{}}`))

		// await the event on all subscribers
		sub1.recorder.AwaitMessages(t, 1, time.Second*5)
		sub2.recorder.AwaitMessages(t, 1, time.Second*5)
		sub3.recorder.AwaitMessages(t, 1, time.Second*5)

		// verify sub1 and sub2 got the identical message
		assert.Equal(t, `{"data":{"f1":1,"f2":2}}`, sub1.recorder.Messages()[0])
		assert.Equal(t, sub1.recorder.Messages()[0], sub2.recorder.Messages()[0])

		// verify only sub3 has the queryplan extension in the response
		assert.Contains(t, sub3.recorder.Messages()[0], `"extensions":{"queryPlan":`)
		assert.NotContains(t, sub2.recorder.Messages()[0], `"extensions":{"queryPlan":`)
		assert.NotContains(t, sub1.recorder.Messages()[0], `"extensions":{"queryPlan":`)

		// sub1 and sub2 should share one fetch, since they have the same query and both
		// have query plans disabled. sub3 has the same query, but needs a dedicated fetch
		// since it requires a query plan in the response.
		assert.Equal(t, int32(2), ds.loadCount.Load())
	})

}

type countingDataSource struct {
	data       []byte
	started    chan struct{}
	updater    SubscriptionUpdater
	startCount atomic.Int32
	loadCount  atomic.Int32
}

func (ds *countingDataSource) Start(ctx *Context, headers http.Header, input []byte, updater SubscriptionUpdater) error {
	ds.updater = updater
	ds.startCount.Add(1)
	close(ds.started)
	return nil
}

func (ds *countingDataSource) HashTriggerInput(input []byte, xxh *xxhash.Digest) error {
	_, err := xxh.Write(input)
	return err
}

func (ds *countingDataSource) Load(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
	ds.loadCount.Add(1)
	return ds.data, nil
}

func (ds *countingDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) ([]byte, error) {
	return ds.Load(ctx, headers, input)
}
