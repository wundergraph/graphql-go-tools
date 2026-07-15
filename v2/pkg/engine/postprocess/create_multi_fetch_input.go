package postprocess

import "strings"

const (
	repoQueryAnchor   = `"body":{"query":"`
	repoVarsMarker    = `,"variables":`
	appendVarsPrefix  = `{"body":{"variables":`
	appendQueryMarker = `,"query":"`
)

// fetchInputSplit locates the body.query string value and the body.variables
// object value inside a fetch input, supporting both sjson key orders:
//
//	repo shape   {"method":...,["header":...,]"body":{"query":"...","variables":{...}}}
//	append shape {"body":{"variables":{...},"query":"..."},...}
//
// ok is false when the input matches neither; such groups are not merged.
type fetchInputSplit struct {
	queryStart, queryEnd         int // query string value content range (between the quotes)
	variablesStart, variablesEnd int // variables object value range including braces
}

// splitEntityFetchInput locates the query and variables ranges. The raw,
// unescaped query is never scanned through: both shapes bound it by its
// neighbors, and ambiguous inputs fail safe (ok=false).
func splitEntityFetchInput(input string) (s fetchInputSplit, ok bool) {
	switch {
	case strings.HasPrefix(input, appendVarsPrefix):
		return splitAppendShape(input)
	case !strings.HasPrefix(input, `{"body":`):
		return splitRepoShape(input)
	default:
		return fetchInputSplit{}, false
	}
}

func splitRepoShape(input string) (fetchInputSplit, bool) {
	anchor := strings.LastIndex(input, repoQueryAnchor)
	if anchor == -1 {
		return fetchInputSplit{}, false
	}
	queryStart := anchor + len(repoQueryAnchor)
	if len(input) < 3 || input[len(input)-3:] != "}}}" {
		return fetchInputSplit{}, false
	}
	// The variables object is the last balanced object before the trailing
	// "}}}"; scan backward, quote-aware, until brace depth returns to zero.
	variablesStart := -1
	depth := 0
	inString := false
	for i := len(input) - 3; i >= queryStart; i-- {
		c := input[i]
		if c == '"' && !isEscapedAt(input, i) {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch c {
		case '}':
			depth++
		case '{':
			depth--
			if depth == 0 {
				variablesStart = i
			}
		}
		if variablesStart != -1 {
			break
		}
	}
	if variablesStart == -1 {
		return fetchInputSplit{}, false
	}
	variablesEnd := len(input) - 2
	markerStart := variablesStart - len(repoVarsMarker)
	if markerStart < queryStart || input[markerStart:variablesStart] != repoVarsMarker {
		return fetchInputSplit{}, false
	}
	queryEnd := markerStart - 1
	if queryEnd < queryStart || input[queryEnd] != '"' {
		return fetchInputSplit{}, false
	}
	return fetchInputSplit{queryStart: queryStart, queryEnd: queryEnd, variablesStart: variablesStart, variablesEnd: variablesEnd}, true
}

func splitAppendShape(input string) (fetchInputSplit, bool) {
	variablesStart := len(appendVarsPrefix)
	if variablesStart >= len(input) || input[variablesStart] != '{' {
		return fetchInputSplit{}, false
	}
	variablesEnd := -1
	depth := 0
	inString := false
	for i := variablesStart; i < len(input); i++ {
		c := input[i]
		if c == '"' && !isEscapedAt(input, i) {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				variablesEnd = i + 1
			}
		}
		if variablesEnd != -1 {
			break
		}
	}
	if variablesEnd == -1 {
		return fetchInputSplit{}, false
	}
	if !strings.HasPrefix(input[variablesEnd:], appendQueryMarker) {
		return fetchInputSplit{}, false
	}
	queryStart := variablesEnd + len(appendQueryMarker)
	// A printed operation ends with a selection-set brace, so the query closes
	// with `}"},` (closing brace, closing quote, body brace, top-level comma).
	// Requiring a unique such match fails safe on header values that end in a
	// raw '}'.
	queryEnd := -1
	matches := 0
	for q := queryStart; q+2 < len(input); q++ {
		if input[q] == '"' && input[q+1] == '}' && input[q+2] == ',' && q > 0 && input[q-1] == '}' {
			queryEnd = q
			matches++
		}
	}
	if matches != 1 {
		return fetchInputSplit{}, false
	}
	return fetchInputSplit{queryStart: queryStart, queryEnd: queryEnd, variablesStart: variablesStart, variablesEnd: variablesEnd}, true
}

// isEscapedAt reports whether the byte at i is escaped, i.e. preceded by an
// odd-length run of backslashes.
func isEscapedAt(input string, i int) bool {
	backslashes := 0
	for j := i - 1; j >= 0 && input[j] == '\\'; j-- {
		backslashes++
	}
	return backslashes%2 == 1
}
