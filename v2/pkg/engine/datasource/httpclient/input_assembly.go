package httpclient

import (
	"bytes"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

// AssembleGraphQLRequestInput builds the upstream GraphQL request input
// envelope in the exact order the planner assembles it today: body.variables,
// body.query, header (only when non-empty and not JSON null), url, method.
//
// It uses the same SetInputBodyWithPath / SetInputHeader / SetInputURL /
// SetInputMethod setters in the same sequence, so its output is byte-identical
// to the inline assembly in createInputForQuery/ConfigureFetch. variables and
// query are the raw bytes written at body.variables and body.query
// respectively; header is the marshalled header JSON (nil or empty to omit);
// url and method are the fetch endpoint values ("" to omit).
func AssembleGraphQLRequestInput(variables, query, header []byte, url, method string) ([]byte, error) {
	var input []byte
	input = SetInputBodyWithPath(input, variables, "variables")
	input = SetInputBodyWithPath(input, query, "query")
	if len(header) != 0 && !bytes.Equal(header, literal.NULL) {
		input = SetInputHeader(input, header)
	}
	input = SetInputURL(input, []byte(url))
	input = SetInputMethod(input, []byte(method))
	return input, nil
}

// emptyVariablesTail is the exact byte tail AssembleGraphQLRequestInput produces
// when body.variables is written as an empty object: `"variables":{` + the three
// closing braces of variables, body and the root document.
const emptyVariablesTail = `"variables":{}}}`

// AssembleGraphQLRequestInputSplitVariables assembles the same envelope as
// AssembleGraphQLRequestInput but returns it split around the (empty) body
// variables object: prefix ends with `"variables":{`, suffix begins with `}`.
// Callers that render the variables object themselves (the merged
// MultiEntityFetch input template) write their content between the two halves,
// which reconstructs a byte-identical envelope by construction.
//
// Invariant (why the split point is knowable without scanning): body.variables
// is the FIRST key written into an empty document and sjson prepends every
// subsequent key, so body is the root object's last member and variables is
// body's last member. The assembled document therefore always ENDS with
// `"variables":{}}}` — see TestAssembleGraphQLRequestInput, which pins the full
// key order. Splitting off that fixed-length tail cannot be confused by a query
// or header that happens to contain the same text (a query is a JSON string, so
// its quotes are escaped, and any raw header occurrence is necessarily earlier
// in the document). A document that does not end with the tail means the
// assembly order changed; that is reported as an error rather than guessed at.
func AssembleGraphQLRequestInputSplitVariables(query, header []byte, url, method string) (prefix, suffix string, err error) {
	assembled, err := AssembleGraphQLRequestInput([]byte("{}"), query, header, url, method)
	if err != nil {
		return "", "", err
	}
	if !bytes.HasSuffix(assembled, []byte(emptyVariablesTail)) {
		return "", "", fmt.Errorf("assembled GraphQL request input does not end with %s: %s", emptyVariablesTail, assembled)
	}
	// Keep the opening `{` of the variables object in the prefix; the three
	// closing braces form the suffix.
	boundary := len(assembled) - len("}}}")
	return string(assembled[:boundary]), string(assembled[boundary:]), nil
}
