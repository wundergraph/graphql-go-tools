package cache

import (
	"fmt"
	"iter"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// FieldNames is a map of field names.
type FieldNames struct {
	// noFields is a flag to prevent adding any fields.
	// This is set when an empty field list is parsed.
	noFields bool

	// fields is a list of field names.
	// Do not add any fields to this map directly. Use the add method instead
	fields map[string]struct{}
}

func NewFieldNames() *FieldNames {
	return &FieldNames{
		fields: make(map[string]struct{}),
	}
}

func (f *FieldNames) Add(name string) {
	if f.fields == nil {
		f.fields = make(map[string]struct{})
	}

	if f.noFields {
		return
	}

	// Stored verbatim: these may be GraphQL field names, which are case-sensitive.
	f.fields[name] = struct{}{}
}

// Fields returns an iterator over the field names.
//
// Example:
//
//	 ```
//	 fields := NewFieldNames()
//		fields.Add("field1")
//		fields.Add("field2")
//
//		for field := range f.Fields() {
//			fmt.Println(field)
//		}
//
// ```
func (f *FieldNames) Fields() iter.Seq[string] {
	return func(yield func(s string) bool) {
		if f.fields == nil || f.noFields {
			return
		}

		for field := range f.fields {
			if !yield(field) {
				return
			}
		}
	}
}

func (f *FieldNames) has(name string) bool {
	if f == nil {
		return false
	}

	_, ok := f.fields[name]
	return ok
}

func (f *FieldNames) LockFields() {
	f.noFields = true
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
	// Nothing but whitespace and delimiters carries no directives, which is
	// indistinguishable from the header being absent.
	value := strings.Trim(strings.Join(headers.Values("Cache-Control"), ","), "\t\r\n")
	if value == "" {
		return &CacheControlResponse{}, nil
	}

	return parse(value)
}

func parse(s string) (*CacheControlResponse, error) {
	l := newLexer(s)

	if err := l.tokenize(); err != nil {
		return nil, err
	}

	cc := &CacheControlResponse{}

	for {
		token := l.readToken(0)

		switch token.tt {
		case tokenTypeIdent:
			if err := parseIdent(token, l, cc); err != nil {
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

// deltaSecondsArgument interprets an argument as delta-seconds. An unusable
// argument is an error, so a non-nil MaxAge/SMaxAge always means we parsed one
// successfully.
func deltaSecondsArgument(name string, arg directiveArgument) (DeltaSeconds, error) {
	if !arg.present || arg.text == "" {
		return 0, fmt.Errorf("%s requires a value", name)
	}

	// delta-seconds is one or more digits, nothing else; strconv would also
	// accept "+60" and "-60".
	for i := 0; i < len(arg.text); i++ {
		if arg.text[i] < '0' || arg.text[i] > '9' {
			return 0, fmt.Errorf("%s value %q is not a number", name, arg.text)
		}
	}

	value, err := parseDeltaSeconds(arg.text)
	if err != nil {
		// Overflowing int64 is the only failure left, and RFC 9111 §1.2.2
		// says to clamp it.
		return math.MaxInt32, nil
	}

	return value, nil
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
		fields.LockFields()
		return
	}

	var added bool
	for field := range strings.SplitSeq(arg.text, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}

		fields.Add(field)
		added = true
	}

	if !added {
		fields.LockFields()
	}
}

func parseIdent(name token, l *lexer, cc *CacheControlResponse) error {
	// Read the argument whatever the directive, so unknown and boolean ones
	// consume theirs instead of leaving it for the outer loop.
	arg, err := l.readArgument()
	if err != nil {
		return err
	}

	switch strings.ToLower(name.lit) {
	case "max-age":
		// First occurrence wins, and a later one is ignored without being
		// validated.
		if cc.MaxAge != nil {
			return nil
		}

		value, err := deltaSecondsArgument("max-age", arg)
		if err != nil {
			return err
		}
		cc.MaxAge = &value

	case "s-maxage":
		if cc.SMaxAge != nil {
			return nil
		}

		value, err := deltaSecondsArgument("s-maxage", arg)
		if err != nil {
			return err
		}
		cc.SMaxAge = &value

	case "no-store":
		// A boolean directive cannot have an argument, so we ignore it.
		cc.NoStore = true

	case "public":
		// A boolean directive cannot have an argument, so we ignore it.
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
