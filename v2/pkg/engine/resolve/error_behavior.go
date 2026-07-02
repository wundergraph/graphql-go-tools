package resolve

// ErrorBehavior selects how execution errors interact with non-null positions
// in the response, per the GraphQL onError proposal (graphql-spec#1163).
type ErrorBehavior string

const (
	// ErrorBehaviorPropagate is the spec default and current behavior: an
	// execution error in a non-null position propagates to the nearest nullable
	// ancestor (or data: null at the root).
	ErrorBehaviorPropagate ErrorBehavior = "PROPAGATE"
	// ErrorBehaviorNull sets the errored position to null in place (even if
	// non-nullable) without propagating; the error is still recorded.
	ErrorBehaviorNull ErrorBehavior = "NULL"
	// ErrorBehaviorHalt aborts response assembly on any error: data is null and
	// a single error is returned.
	ErrorBehaviorHalt ErrorBehavior = "HALT"
)

// MapErrorBehavior validates and normalizes an incoming onError value.
// The empty string maps to the default (PROPAGATE). Any other unknown value
// returns ok=false so the caller can raise a request error.
func MapErrorBehavior(s string) (ErrorBehavior, bool) {
	switch ErrorBehavior(s) {
	case "":
		return ErrorBehaviorPropagate, true
	case ErrorBehaviorPropagate, ErrorBehaviorNull, ErrorBehaviorHalt:
		return ErrorBehavior(s), true
	default:
		return "", false
	}
}
