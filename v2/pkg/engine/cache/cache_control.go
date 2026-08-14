package cache

import (
	"fmt"
	"math"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// FieldNames is a map of field names.
type FieldNames struct {
	// lockFields is a flag to prevent adding any fields.
	// This is set when an empty field list is parsed.
	lockFields bool

	// Present indicates whether the directive is present in the response.
	Present bool
	// Fields is a list of field names.
	// Do not add any fields to this map directly. Use the add method instead
	Fields map[string]struct{}
}

func NewFieldNames() *FieldNames {
	return &FieldNames{
		// By default, when a FieldNames is created, it is present.
		Present: true,
	}
}

func (f *FieldNames) Add(name string) {
	if f.Fields == nil {
		f.Fields = make(map[string]struct{})
	}

	if f.lockFields {
		return
	}

	// Stored verbatim: these may be GraphQL field names, which are case-sensitive.
	f.Fields[name] = struct{}{}
}

func (f *FieldNames) has(name string) bool {
	if f == nil || !f.Present {
		return false
	}

	_, ok := f.Fields[name]
	return ok
}

func (f *FieldNames) ClearAndLock() {
	clear(f.Fields)
	f.lockFields = true
}

// DeltaSeconds is a signed 32-bit integer representing the number of seconds.
// It is defined in https://datatracker.ietf.org/doc/html/rfc7234#section-1.2.1
//
// The delta-seconds rule specifies a non-negative integer, representing
// time in seconds.
//
// A recipient parsing a delta-seconds value and converting it to binary
// form ought to use an arithmetic type of at least 31 bits of
// non-negative integer range.
//
// If a cache receives a delta-seconds
// value greater than the greatest integer it can represent, or if any
// of its subsequent calculations overflows, the cache MUST consider the
// value to be either 2147483648 (2^31) or the greatest positive integer
// it can conveniently represent.
type DeltaSeconds int32

// parseDeltaSeconds parses a string into a DeltaSeconds.
func parseDeltaSeconds(s string) (DeltaSeconds, error) {
	num, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return DeltaSeconds(-1), err
	}

	// We expect a non-negative integer.
	// If the integer is negative, we return -1.
	if num < 0 {
		return DeltaSeconds(-1), nil
	}

	if num > math.MaxInt32 {
		return DeltaSeconds(math.MaxInt32), nil
	}

	return DeltaSeconds(int32(num)), nil
}

// ToGoSeconds converts a DeltaSeconds to a time.Duration.
func (d DeltaSeconds) ToGoSeconds() time.Duration {
	return time.Duration(d) * time.Second
}

type CacheControlResponse struct {
	// MaxAge is the maximum age of the response in seconds.
	//
	// Defined in https://datatracker.ietf.org/doc/html/rfc7234#section-5.2.2.8
	//
	// The "max-age" response directive indicates that the response is to be
	// considered stale after its age is greater than the specified number of seconds.
	MaxAge *DeltaSeconds

	// SMaxAge is the maximum age of the response in seconds for shared caches.
	//
	// Defined in https://datatracker.ietf.org/doc/html/rfc7234#section-5.2.2.9
	//
	// The "s-maxage" response directive indicates that, in shared caches,
	// the maximum age specified by this directive overrides the maximum age
	// specified by either the max-age directive or the Expires header
	// field.
	//
	// The s-maxage directive also implies the semantics of the
	// proxy-revalidate response directive.
	SMaxAge *DeltaSeconds

	// NoStore indicates that a cache MUST NOT
	// store any part of either the immediate request or response.
	//
	// Defined in https://datatracker.ietf.org/doc/html/rfc7234#section-5.2.2.3
	//
	// This directive applies to both private and shared caches.
	// "MUST NOT store" in this context means that the cache MUST NOT intentionally
	// store the information in non-volatile storage, and MUST make a
	// best-effort attempt to remove the information from volatile storage
	// as promptly as possible after forwarding it.
	NoStore bool

	// NoCache is a list of field names that should not be cached.
	//
	// Defined in https://datatracker.ietf.org/doc/html/rfc7234#section-5.2.2.2
	//
	// The "no-cache" response directive indicates that the response MUST
	// NOT be used to satisfy a subsequent request without successful
	// validation on the origin server.
	//
	// This allows an origin server to prevent a cache from using it to satisfy a request without contacting
	// it, even by caches that have been configured to send stale responses.
	//
	// If the no-cache response directive specifies one or more field-names,
	// then a cache MAY use the response to satisfy a subsequent request,
	// subject to any other restrictions on caching.  However, any header
	// fields in the response that have the field-name(s) listed MUST NOT be
	// sent in the response to a subsequent request without successful
	// revalidation with the origin server.  This allows an origin server to
	// prevent the re-use of certain header fields in a response, while
	// still allowing caching of the rest of the response.
	NoCache *FieldNames

	// Public indicates that any cache MAY store the response,
	// even if the response would normally be non-cacheable or cacheable only within a private cache.
	//
	// Defined in https://datatracker.ietf.org/doc/html/rfc7234#section-5.2.2.5
	Public bool

	// Private indicates that the response message is intended for a single user and MUST NOT be stored by a shared cache.
	// A private cache MAY store the response and reuse it for later requests, even if the response would normally be non-cacheable.
	//
	// Defined in https://datatracker.ietf.org/doc/html/rfc7234#section-5.2.2.6
	//
	// If the private response directive specifies one or more field-names,
	// this requirement is limited to the field-values associated with the
	// listed response header fields. That is, a shared cache MUST NOT
	// store the specified field-names(s), whereas it MAY store the
	// remainder of the response message.
	Private *FieldNames
}

func ParseCacheControlResponse(headers http.Header) (*CacheControlResponse, error) {
	if len(headers) == 0 {
		return nil, nil
	}

	cacheControl := headers[textproto.CanonicalMIMEHeaderKey("Cache-Control")]
	if len(cacheControl) == 0 {
		return nil, nil
	}

	// Repeated field lines are one comma-joined value (RFC 9110 §5.3).
	value := cacheControl[0]
	if len(cacheControl) > 1 {
		value = strings.Join(cacheControl, ",")
	}

	// Nothing but whitespace and delimiters carries no directives, which is
	// indistinguishable from the header being absent.
	if strings.Trim(value, " \t\r\n,") == "" {
		return nil, nil
	}

	return parse(value)
}

func parse(s string) (*CacheControlResponse, error) {
	l := newLexer(s)

	if err := l.tokenize(); err != nil {
		return nil, err
	}

	cc := &CacheControlResponse{}
	seen := seenDirectives{}

	for {
		token := l.readToken(0)

		switch token.tt {
		case tokenTypeIdent:
			if err := parseIdent(token, l, cc, &seen); err != nil {
				return nil, err
			}

			if after := l.peekToken(); after.tt != tokenTypeComma && after.tt != tokenTypeEOF {
				return nil, fmt.Errorf("expected , or end of input after directive %q at position %d", token.lit, after.start)
			}
		case tokenTypeComma:
			continue
		case tokenTypeEOF:
			return cc, nil
		default:
			return nil, fmt.Errorf("unexpected token %q at position %d", token.lit, token.start)
		}
	}
}

// deltaSecondsArgument interprets an argument as delta-seconds. The second
// result reports whether to record the directive at all: an unusable argument
// drops it rather than failing the field.
func deltaSecondsArgument(arg directiveArgument) (DeltaSeconds, bool) {
	if !arg.present || arg.text == "" {
		return 0, false
	}

	// delta-seconds is 1*DIGIT; strconv alone would also accept "+60".
	for i := 0; i < len(arg.text); i++ {
		if arg.text[i] < '0' || arg.text[i] > '9' {
			return 0, false
		}
	}

	value, err := parseDeltaSeconds(arg.text)
	if err != nil {
		return math.MaxInt32, true
	}

	if value < 0 {
		return 0, false
	}

	return value, true
}

// fieldNamesArgument merges an argument into a field-name list. An argument
// with no usable names collapses the directive to its unqualified form, which
// outranks any names present already or added later.
func fieldNamesArgument(arg directiveArgument, fields *FieldNames) {
	// Make sure we don't allow adding subsequent field names to the list. when we have no arguments.
	//
	// Example:
	// no-cache on its own is stricter than no-cache="A": it blocks reuse of the
	// whole response, not just a few fields. So an empty argument wins, and we
	// lock the set so a later occurrence can't weaken it.
	if !arg.present {
		fields.ClearAndLock()
		return
	}

	var added bool
	for _, field := range strings.Split(arg.text, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}

		fields.Add(field)
		added = true
	}

	if !added {
		fields.ClearAndLock()
	}
}

// seenDirectives tracks first-occurrence resolution. It is separate from the
// parsed value because a dropped first occurrence still counts as seen.
type seenDirectives struct {
	maxAge  bool
	sMaxAge bool
}

func parseIdent(name token, l *lexer, cc *CacheControlResponse, seen *seenDirectives) error {
	// Read the argument whatever the directive, so unknown and boolean ones
	// consume theirs instead of leaving it for the outer loop.
	arg, err := l.readArgument()
	if err != nil {
		return err
	}

	switch strings.ToLower(name.lit) {
	case "max-age":
		if seen.maxAge {
			return nil
		}
		seen.maxAge = true

		if value, ok := deltaSecondsArgument(arg); ok {
			cc.MaxAge = &value
		}

	case "s-maxage":
		if seen.sMaxAge {
			return nil
		}
		seen.sMaxAge = true

		if value, ok := deltaSecondsArgument(arg); ok {
			cc.SMaxAge = &value
		}

	case "no-store":
		// A stray argument means nothing to a boolean directive; discard it.
		cc.NoStore = true

	case "public":
		cc.Public = true

	case "no-cache":
		if cc.NoCache == nil {
			cc.NoCache = NewFieldNames()
		}
		fieldNamesArgument(arg, cc.NoCache)

	case "private":
		if cc.Private == nil {
			cc.Private = NewFieldNames()
		}
		fieldNamesArgument(arg, cc.Private)

	default:
		// We ignore directives that we don't specify here.
	}

	return nil
}
