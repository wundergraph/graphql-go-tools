package resolve

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// TestInboundSingleFlight_ConcurrentFollowerTimeout exercises the scenario where
// multiple followers time out concurrently. Before the fix, each follower wrote
// its context error to the shared request.Err field without synchronization,
// causing a data race. After the fix, followers return ctx.Err() directly
// without mutating shared state. Run with -race to verify.
func TestInboundSingleFlight_ConcurrentFollowerTimeout(t *testing.T) {
	sf := NewRequestSingleFlight(1)
	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		},
	}

	// Leader creates the inflight request
	leaderCtx := NewContext(context.Background())
	leaderCtx.Request.ID = 1
	inflight, err := sf.GetOrCreate(leaderCtx, response)
	if err != nil {
		t.Fatalf("leader GetOrCreate: %v", err)
	}
	if inflight == nil {
		t.Fatal("expected inflight request from leader")
	}

	const numFollowers = 10
	var wg sync.WaitGroup
	wg.Add(numFollowers)

	for i := 0; i < numFollowers; i++ {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			followerCtx := NewContext(ctx)
			followerCtx.Request.ID = 1

			// Cancel immediately so the follower's context is done
			cancel()

			_, followerErr := sf.GetOrCreate(followerCtx, response)
			if followerErr == nil {
				t.Error("expected error from timed-out follower")
			}
		}()
	}

	wg.Wait()

	// Clean up: finish the leader request
	sf.FinishOk(inflight, []byte("ok"))
}

func TestInboundSingleFlight_FollowerReceivesLeaderError(t *testing.T) {
	sf := NewRequestSingleFlight(1)
	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		},
	}

	leaderCtx := NewContext(context.Background())
	leaderCtx.Request.ID = 2
	inflight, err := sf.GetOrCreate(leaderCtx, response)
	if err != nil {
		t.Fatalf("leader GetOrCreate: %v", err)
	}

	// The follower calls GetOrCreate which blocks on inflight.Done.
	// We wait for followerCount to confirm it has entered before calling FinishErr.
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		followerCtx := NewContext(context.Background())
		followerCtx.Request.ID = 2

		_, followerErr := sf.GetOrCreate(followerCtx, response)
		if followerErr == nil {
			t.Error("expected error from follower after leader FinishErr")
		}
	}()

	// Poll until the follower has actually registered inside GetOrCreate.
	deadline := time.After(3 * time.Second)
	for !inflight.HasFollowers() {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for follower to enter singleflight")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	sf.FinishErr(inflight, context.DeadlineExceeded)
	wg.Wait()
}
