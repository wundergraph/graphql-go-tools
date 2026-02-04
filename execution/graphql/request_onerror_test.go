package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestRequest_GetOnErrorBehavior(t *testing.T) {
	tests := []struct {
		name       string
		extensions string
		expected   resolve.ErrorBehavior
		ok         bool
	}{
		{
			name:       "NULL mode",
			extensions: `{"onError":"NULL"}`,
			expected:   resolve.ErrorBehaviorNull,
			ok:         true,
		},
		{
			name:       "PROPAGATE mode",
			extensions: `{"onError":"PROPAGATE"}`,
			expected:   resolve.ErrorBehaviorPropagate,
			ok:         true,
		},
		{
			name:       "HALT mode",
			extensions: `{"onError":"HALT"}`,
			expected:   resolve.ErrorBehaviorHalt,
			ok:         true,
		},
		{
			name:       "lowercase null",
			extensions: `{"onError":"null"}`,
			expected:   resolve.ErrorBehaviorNull,
			ok:         true,
		},
		{
			name:       "mixed case",
			extensions: `{"onError":"Halt"}`,
			expected:   resolve.ErrorBehaviorHalt,
			ok:         true,
		},
		{
			name:       "empty extensions",
			extensions: ``,
			expected:   resolve.ErrorBehaviorPropagate,
			ok:         false,
		},
		{
			name:       "no onError field",
			extensions: `{"other":"value"}`,
			expected:   resolve.ErrorBehaviorPropagate,
			ok:         false,
		},
		{
			name:       "empty onError value",
			extensions: `{"onError":""}`,
			expected:   resolve.ErrorBehaviorPropagate,
			ok:         false,
		},
		{
			name:       "invalid onError value",
			extensions: `{"onError":"INVALID"}`,
			expected:   resolve.ErrorBehaviorPropagate,
			ok:         false,
		},
		{
			name:       "invalid JSON",
			extensions: `{invalid}`,
			expected:   resolve.ErrorBehaviorPropagate,
			ok:         false,
		},
		{
			name:       "extensions with other fields",
			extensions: `{"tracing":true,"onError":"NULL","persistedQuery":{"hash":"abc"}}`,
			expected:   resolve.ErrorBehaviorNull,
			ok:         true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &Request{
				Extensions: []byte(tc.extensions),
			}
			result, ok := req.GetOnErrorBehavior()
			assert.Equal(t, tc.expected, result)
			assert.Equal(t, tc.ok, ok)
		})
	}
}

func TestRequest_GetOnErrorBehavior_WithNilExtensions(t *testing.T) {
	req := &Request{
		Query: "{ hello }",
	}
	result, ok := req.GetOnErrorBehavior()
	assert.Equal(t, resolve.ErrorBehaviorPropagate, result)
	assert.False(t, ok)
}
