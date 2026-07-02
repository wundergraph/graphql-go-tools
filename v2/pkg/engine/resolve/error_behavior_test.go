package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapErrorBehavior(t *testing.T) {
	cases := []struct {
		in   string
		want ErrorBehavior
		ok   bool
	}{
		{"", ErrorBehaviorPropagate, true}, // empty => default
		{"PROPAGATE", ErrorBehaviorPropagate, true},
		{"NULL", ErrorBehaviorNull, true},
		{"HALT", ErrorBehaviorHalt, true},
		{"null", "", false}, // case-sensitive per spec
		{"BOGUS", "", false},
	}
	for _, c := range cases {
		got, ok := MapErrorBehavior(c.in)
		assert.Equal(t, c.ok, ok, "ok for %q", c.in)
		assert.Equal(t, c.want, got, "value for %q", c.in)
	}
}

func TestErrorBehavior_OperatorDefaultApplied(t *testing.T) {
	// simulate: no request onError, operator default = NULL => router sets NULL.
	effective, ok := MapErrorBehavior("") // request omitted
	assert.True(t, ok)
	operatorDefault := ErrorBehaviorNull
	if effective == ErrorBehaviorPropagate { // request omitted -> apply operator default
		effective = operatorDefault
	}
	assert.Equal(t, ErrorBehaviorNull, effective)
}
