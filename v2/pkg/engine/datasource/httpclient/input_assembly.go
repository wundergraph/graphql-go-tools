package httpclient

import (
	"bytes"

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
