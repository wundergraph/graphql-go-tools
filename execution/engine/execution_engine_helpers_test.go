package engine

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
)

// fetchSequencerCtxKey is the context key under which a *fetchSequencer is
// injected into the per-execution context. The conditional round tripper reads
// it to deterministically order concurrent subgraph fetches without relying on
// response latencies.
type fetchSequencerCtxKeyType struct{}

var fetchSequencerCtxKey = fetchSequencerCtxKeyType{}

// fetchSequencer deterministically orders concurrent subgraph fetches for
// order-dependent defer tests, replacing brittle per-response latencies.
//
// Frame ordering in the engine is decided by which goroutine acquires the
// shared DataBuffer lock first after its fetch returns (render+flush run under
// that lock). Instead of racing on latency, a gated fetch blocks in the round
// tripper until enough streamed frames have been flushed.
//
// served counts streamed frames (incremented from the streaming writer's flush
// callback, including the initial frame). A request whose body has a gate G
// blocks until served >= G, i.e. until all frames that must precede it have
// been written. gates maps an exact request body to its required served count.
//
// One sequencer is created per execution and injected via context, so the
// shared round tripper used across parallel subtests never collides.
type fetchSequencer struct {
	mu     sync.Mutex
	cond   *sync.Cond
	served int
	gates  map[string]int
}

func newFetchSequencer(gates map[string]int) *fetchSequencer {
	s := &fetchSequencer{gates: gates}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// advance records that one streamed frame has been flushed and wakes any gated
// fetches whose threshold is now met. Called from the streaming flush callback.
func (s *fetchSequencer) advance() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.served++
	s.cond.Broadcast()
	s.mu.Unlock()
}

// waitForBody blocks until the frames that must precede the given request body
// have been flushed. Bodies without a gate return immediately.
func (s *fetchSequencer) waitForBody(body string) {
	if s == nil || len(s.gates) == 0 {
		return
	}
	gate, ok := s.gates[body]
	if !ok {
		return
	}
	s.mu.Lock()
	for s.served < gate {
		s.cond.Wait()
	}
	s.mu.Unlock()
}

type testRoundTripper func(req *http.Request) *http.Response

func (t testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t(req), nil
}

type roundTripperTestCase struct {
	expectedHost     string
	expectedPath     string
	expectedBody     string
	sendStatusCode   int
	sendResponseBody string
}

func createTestRoundTripper(t *testing.T, testCase roundTripperTestCase) testRoundTripper {
	t.Helper()

	return func(req *http.Request) *http.Response {
		t.Helper()

		assert.Equal(t, testCase.expectedHost, req.URL.Host)
		assert.Equal(t, testCase.expectedPath, req.URL.Path)

		if len(testCase.expectedBody) > 0 {
			var receivedBodyBytes []byte
			if req.Body != nil {
				var err error
				receivedBodyBytes, err = io.ReadAll(req.Body)
				require.NoError(t, err)
			}
			require.Equal(t, testCase.expectedBody, string(receivedBodyBytes), "roundTripperTestCase received unexpected body")
		}

		body := bytes.NewBuffer([]byte(testCase.sendResponseBody))
		return &http.Response{StatusCode: testCase.sendStatusCode, Body: io.NopCloser(body)}
	}
}

type conditionalTestCase struct {
	expectedHost string
	expectedPath string

	// responses map an expected body to the output that should be sent
	responses map[string]sendResponse

	reportUnused bool
	reportUsed   bool
}

type sendResponse struct {
	statusCode int
	body       string
}

func createConditionalTestRoundTripper(t *testing.T, testCase conditionalTestCase) testRoundTripper {
	t.Helper()

	require.True(t, len(testCase.responses) > 0, "no responses defined")

	used := make(map[string]bool)
	usedMu := &sync.RWMutex{}
	if testCase.reportUnused {
		t.Cleanup(func() {
			for key := range testCase.responses {
				if !used[key] {
					t.Logf("UNUSED MOCK [%s]: %s", testCase.expectedHost, key)
				}
			}
		})
	}

	return func(req *http.Request) *http.Response {
		t.Helper()

		assert.Equal(t, testCase.expectedHost, req.URL.Host)
		assert.Equal(t, testCase.expectedPath, req.URL.Path)

		require.NotNil(t, req.Body, "roundTripperTestCase received nil body")

		gotBody, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		defer req.Body.Close()

		if testCase.reportUsed {
			t.Logf("Requested MOCK [%s]: %s", testCase.expectedHost, string(gotBody))
		}

		if !assert.Containsf(t, testCase.responses, string(gotBody), "received unexpected body: %v", string(gotBody)) {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBuffer([]byte("received unexpected body"))),
			}
		}

		response := testCase.responses[string(gotBody)]

		if testCase.reportUnused {
			usedMu.Lock()
			used[string(gotBody)] = true
			usedMu.Unlock()
		}

		if testCase.reportUsed {
			t.Logf("Send MOCK Response:\n %s", response.body)
		}

		// Deterministically order concurrent fetches: block until the frames that
		// must precede this fetch have been streamed (see fetchSequencer).
		if seq, ok := req.Context().Value(fetchSequencerCtxKey).(*fetchSequencer); ok {
			seq.waitForBody(string(gotBody))
		}

		return &http.Response{
			StatusCode: response.statusCode,
			Body:       io.NopCloser(bytes.NewBuffer([]byte(response.body))),
		}
	}
}

func stringify(any any) []byte {
	out, _ := json.Marshal(any)
	return out
}

func heroWithArgumentSchema(t *testing.T) *graphql.Schema {
	schemaString := `
		type Query {
			hero(name: String): String
			heroDefault(name: String = "Any"): String
			heroDefaultRequired(name: String! = "AnyRequired"): String
			heroes(names: [String!]!): [String!]
		}`

	schema, err := graphql.NewSchemaFromString(schemaString)
	require.NoError(t, err)
	return schema
}
