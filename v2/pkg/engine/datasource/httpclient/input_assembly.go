package httpclient

import (
	"bytes"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

// AssembleGraphQLRequestInput builds the upstream GraphQL request input with
// an explicit, code-defined key order:
//
//	{"method":M,"url":U,"header":H,"body":{"query":Q,"variables":V}}
//
// Empty components are omitted (a "null" header counts as empty); when every
// component is empty the result is nil. Each component passes through
// wrapQuotesIfString exactly like the sjson-based Set* helpers it replaces.
//
// The order is constructed directly rather than via sjson because sjson's
// placement of NEW keys is version-dependent (v1.0.4 prepends, v1.2.5
// appends): assembling through sjson made the byte layout — and the split
// invariant of AssembleGraphQLRequestInputSplitVariables — depend on which
// sjson version a consumer build resolves.
func AssembleGraphQLRequestInput(variables, query, header []byte, url, method string) []byte {
	includeHeader := len(header) != 0 && !bytes.Equal(header, literal.NULL)
	includeBody := len(query) != 0 || len(variables) != 0
	if method == "" && url == "" && !includeHeader && !includeBody {
		return nil
	}

	var buf bytes.Buffer
	buf.WriteByte('{')
	writeKey := func(key string) {
		if buf.Len() > 1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('"')
		buf.WriteString(key)
		buf.WriteString(`":`)
	}
	if method != "" {
		writeKey(METHOD)
		buf.Write(wrapQuotesIfString([]byte(method)))
	}
	if url != "" {
		writeKey(URL)
		buf.Write(wrapQuotesIfString([]byte(url)))
	}
	if includeHeader {
		writeKey(HEADER)
		buf.Write(wrapQuotesIfString(header))
	}
	if includeBody {
		writeKey(BODY)
		buf.WriteByte('{')
		if len(query) != 0 {
			buf.WriteString(`"query":`)
			buf.Write(wrapQuotesIfString(query))
		}
		if len(variables) != 0 {
			if len(query) != 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(`"variables":`)
			buf.Write(wrapQuotesIfString(variables))
		}
		buf.WriteByte('}')
	}
	buf.WriteByte('}')
	return buf.Bytes()
}

// emptyVariablesTail is the exact byte tail AssembleGraphQLRequestInput
// produces when body.variables is written as an empty object: `"variables":{`
// plus the three closing braces of variables, body and the root document.
const emptyVariablesTail = `"variables":{}}}`

// AssembleGraphQLRequestInputSplitVariables assembles the same envelope as
// AssembleGraphQLRequestInput but returns it split around the (empty) body
// variables object: prefix ends with `"variables":{`, suffix begins with `}`.
// Callers that render the variables object themselves (the merged
// MultiEntityFetch input template) write their content between the two halves,
// which reconstructs a byte-identical envelope by construction.
//
// The split point is knowable without scanning: AssembleGraphQLRequestInput
// writes body as the root object's last member and variables as body's last
// member by explicit construction, so the assembled document always ENDS with
// `"variables":{}}}` — see TestAssembleGraphQLRequestInput, which pins the
// full key order. Splitting off that fixed-length tail cannot be confused by
// a query or header that happens to contain the same text (a query is a JSON
// string, so its quotes are escaped, and any raw header occurrence is
// necessarily earlier in the document). A document that does not end with the
// tail means the assembly changed; that is reported as an error rather than
// guessed at.
func AssembleGraphQLRequestInputSplitVariables(query, header []byte, url, method string) (prefix, suffix string, err error) {
	assembled := AssembleGraphQLRequestInput([]byte("{}"), query, header, url, method)
	if !bytes.HasSuffix(assembled, []byte(emptyVariablesTail)) {
		return "", "", fmt.Errorf("assembled GraphQL request input does not end with %s: %s", emptyVariablesTail, assembled)
	}
	// Keep the opening `{` of the variables object in the prefix; the three
	// closing braces form the suffix.
	boundary := len(assembled) - len("}}}")
	return string(assembled[:boundary]), string(assembled[boundary:]), nil
}
