package resolve

import "strings"

// ErrorBehavior controls how errors are handled during GraphQL resolution.
// This implements the proposed GraphQL spec change from PR #1163.
type ErrorBehavior int

const (
	// ErrorBehaviorPropagate is the default behavior (traditional null bubbling).
	// When a non-nullable field returns null due to an error, the null value
	// propagates up to the nearest nullable parent.
	ErrorBehaviorPropagate ErrorBehavior = iota

	// ErrorBehaviorNull stops null propagation at the error site.
	// Even non-nullable fields return null without bubbling up.
	// Errors are still recorded but don't cause parent nullification.
	ErrorBehaviorNull

	// ErrorBehaviorHalt stops execution on the first error.
	// The entire data field becomes null, and only the first error is returned.
	ErrorBehaviorHalt
)

// String returns the string representation of the ErrorBehavior.
func (e ErrorBehavior) String() string {
	switch e {
	case ErrorBehaviorPropagate:
		return "PROPAGATE"
	case ErrorBehaviorNull:
		return "NULL"
	case ErrorBehaviorHalt:
		return "HALT"
	default:
		return "PROPAGATE"
	}
}

// ParseErrorBehavior parses a string into an ErrorBehavior.
// Returns the parsed value and true if valid, or ErrorBehaviorPropagate and false if invalid.
// The parsing is case-insensitive.
func ParseErrorBehavior(s string) (ErrorBehavior, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "PROPAGATE":
		return ErrorBehaviorPropagate, true
	case "NULL":
		return ErrorBehaviorNull, true
	case "HALT":
		return ErrorBehaviorHalt, true
	default:
		return ErrorBehaviorPropagate, false
	}
}
