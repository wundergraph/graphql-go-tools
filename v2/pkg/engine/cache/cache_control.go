package cache

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/runes"
)

// FieldNames is a map of field names.
type FieldNames struct {
	// Present indicates whether the directive is present in the response.
	Present bool
	// Fields is a list of field names.
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

	f.Fields[strings.ToLower(name)] = struct{}{}
}

func (f *FieldNames) Contains(name string) bool {
	if f == nil || !f.Present {
		return false
	}
	_, ok := f.Fields[strings.ToLower(name)]
	return ok
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
	MaxAge DeltaSeconds

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
	SMaxAge DeltaSeconds

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

// THIS COULD GO INTO A SEPARATE FILE

func ParseCacheControlResponse(headers http.Header) (*CacheControlResponse, error) {
	if len(headers) == 0 {
		return nil, nil
	}

	cacheControl := headers[textproto.CanonicalMIMEHeaderKey("Cache-Control")]

	if len(cacheControl) == 1 {
		return parse(cacheControl[0])
	}

	var value string
	if len(cacheControl) > 1 {
		value = strings.Join(cacheControl, ",")
	}

	return parse(value)
}

func parse(s string) (*CacheControlResponse, error) {
	if len(s) == 0 {
		return nil, errors.New("Cache-Control header is empty")
	}

	l := newLexer(s)

	if err := l.tokenize(); err != nil {
		return nil, err
	}

	cc := &CacheControlResponse{}

outer:
	for {
		token := l.readToken(0)

		switch token.tt {
		case tokenTypeIdent:
			err := parseIdent(token, l, cc)
			if err != nil {
				return nil, err
			}
		case tokenTypeComma:
			continue
		case tokenTypeEOF:
			break outer
		default:
			return nil, fmt.Errorf("unexpected token: %s", token.lit)
		}

	}

	return cc, nil
}

type lexer struct {
	pos      int
	tokenPos int
	input    string
	tokens   []token
}

func newLexer(input string) *lexer {
	return &lexer{
		pos:      0,
		tokenPos: 0,
		input:    input,
		tokens:   make([]token, 0, 64),
	}
}

func (l *lexer) tokenize() error {
	for {
		token, err := l.nextToken()
		if err != nil {
			return err
		}

		if token.tt == tokenTypeEOF {
			return nil
		}

		l.tokens = append(l.tokens, token)
	}
}

func (l *lexer) peekToken() token {
	if l.tokenPos < len(l.tokens) {
		return l.tokens[l.tokenPos]
	}

	return token{tt: tokenTypeEOF}
}

func (l *lexer) readToken(offset int) token {
	if l.tokenPos+offset < len(l.tokens) {
		nextIndex := l.tokenPos + offset
		token := l.tokens[nextIndex]
		l.tokenPos = nextIndex + 1
		return token
	}

	return token{tt: tokenTypeEOF}
}

// isWhitespace checks if the rune is a whitespace character.
func (l *lexer) isWhitespace(r byte) bool {
	return r == runes.SPACE || r == runes.TAB || r == runes.CARRIAGERETURN || r == runes.LINETERMINATOR
}

// isControlCharacter checks if the rune is a control character.
// ASCII control characters are ranged from 0-31 and 127.
func (l *lexer) isControlCharacter(r byte) bool {
	return r <= 0x1F || r == 0x7F // 0-31 and 127
}

// isPrintableCharacter checks if the rune is a printable character.
// Printable characters are ranged from 32 to 126.
func (l *lexer) isPrintableCharacter(r byte) bool {
	return r >= 0x20 && r <= 0x7E // 32-126
}

func (l *lexer) isDigit(r byte) bool {
	return r >= 0x30 && r <= 0x39 // 0-9
}

func (l *lexer) nextToken() (token, error) {
	tok := token{}

	if l.pos >= len(l.input) {
		tok.tt = tokenTypeEOF
		tok.setEnd(l.pos, l.input)
		return tok, nil
	}

	var next byte

	for {
		tok.setStart(l.pos)
		next = l.read()

		// control characters are not allowed in the input
		if l.isControlCharacter(next) {
			return tok, errors.New("control character found in input at position " + strconv.Itoa(l.pos))
		}

		if !l.isWhitespace(next) {
			break
		}
	}

	if l.matchSingleByteToken(next, &tok) {
		tok.setEnd(l.pos, l.input)
		return tok, nil
	}

	switch next {
	case runes.QUOTE:
		err := l.readString(&tok)
		return tok, err
	}

	if l.isDigit(next) {
		err := l.readNumber(&tok)
		return tok, err
	}

	err := l.readIdent(&tok)
	return tok, err
}

func (l *lexer) matchSingleByteToken(r byte, tok *token) bool {
	switch r {
	case runes.EOF:
		tok.tt = tokenTypeEOF
	case runes.EQUALS:
		tok.tt = tokenTypeEquals
	case runes.COMMA:
		tok.tt = tokenTypeComma
	default:
		return false
	}

	return true
}

func (l *lexer) readIdent(tok *token) error {
	tok.tt = tokenTypeIdent

	for {
		next := l.peek(0)

		if next == runes.EOF {
			tok.setEnd(l.pos, l.input)
			return nil
		}

		if l.isControlCharacter(next) {
			return fmt.Errorf("control character %x found in input at position %d", next, l.pos)
		}

		switch next {
		case runes.QUOTE:
			return fmt.Errorf("unexpected character %x found in input at position %d", next, l.pos)
		case runes.CARRIAGERETURN, runes.LINETERMINATOR, runes.COMMA, runes.EQUALS, runes.SPACE, runes.TAB:
			tok.setEnd(l.pos, l.input)
			return nil
		}

		l.advance(1)
	}
}

func (l *lexer) readString(tok *token) error {
	tok.setStart(l.pos)
	tok.tt = tokenTypeString

	for {
		next := l.peek(0)

		switch next {
		case runes.EOF, runes.CARRIAGERETURN, runes.LINETERMINATOR:
			return fmt.Errorf("unexpected end of input at position %d", l.pos)
		}

		if l.isControlCharacter(next) {
			return fmt.Errorf("control character %x found in input at position %d", next, l.pos)
		}

		switch next {
		case runes.QUOTE, runes.CARRIAGERETURN, runes.LINETERMINATOR:
			tok.setEnd(l.pos, l.input)
			l.advance(1) // consume the quote
			return nil
		}

		l.advance(1)
	}
}

func (l *lexer) readNumber(tok *token) error {
	tok.tt = tokenTypeNumber

	for {
		next := l.peek(0)

		switch next {
		case runes.EOF, runes.CARRIAGERETURN, runes.LINETERMINATOR:
			tok.setEnd(l.pos, l.input)
			return nil

		// We only allow integer numbers in the Cache-Control header.
		// So we shouldn't allow a dot in the number input.
		case runes.DOT:
			return fmt.Errorf("unexpected character %x found in input at position %d - Only integer numbers are allowed", next, l.pos)

		case runes.COMMA:
			peek := l.peek(1)

			if l.isDigit(peek) {
				return fmt.Errorf("unexpected character %x found in input at position %d - Unsupported character in number", next, l.pos)
			}
		}

		if l.isControlCharacter(next) {
			return fmt.Errorf("control character %x found in input at position %d", next, l.pos)
		}

		// We allow only integer numbers in the Cache-Control header.
		if !l.isDigit(next) {
			tok.setEnd(l.pos, l.input)
			return nil
		}

		l.advance(1)

	}
}

func (l *lexer) peek(n int) byte {
	if l.pos+n >= len(l.input) {
		return byte(runes.EOF)
	}

	return l.input[l.pos+n]
}

func (l *lexer) advance(n int) {
	if l.pos+n >= len(l.input) {
		l.pos = len(l.input)
	} else {
		l.pos += n
	}
}

func (l *lexer) read() byte {
	if l.pos >= len(l.input) {
		return runes.EOF
	}

	b := l.input[l.pos]
	l.pos++
	return b
}

type token struct {
	tt    tokenType
	lit   string
	start int
	end   int
}

func (t *token) setStart(start int) {
	t.start = start
}

func (t *token) setEnd(end int, input string) {
	t.end = end
	t.lit = input[t.start:t.end]
}

type tokenType int

const (
	tokenTypeIdent tokenType = iota
	tokenTypeEOF

	tokenTypeComma
	tokenTypeEquals

	tokenTypeNumber
	tokenTypeString
)

func parseDeltaSecondsIDent(l *lexer) (DeltaSeconds, error) {
	next := l.peekToken()
	if next.tt == tokenTypeEquals {
		next := l.readToken(1)
		if next.tt != tokenTypeNumber {
			return 0, fmt.Errorf("unexpected token: %s", next.lit)
		}

		deltaSeconds, err := parseDeltaSeconds(next.lit)
		if err != nil {
			return 0, err
		}

		return deltaSeconds, nil

	}

	return -1, nil
}

func parseFieldNamesIdent(l *lexer, fieldNames *FieldNames) error {
	next := l.peekToken()

	if next.tt == tokenTypeEquals {
		next := l.readToken(1)
		if next.tt != tokenTypeString {
			return fmt.Errorf("unexpected token: %s", next.lit)
		}

		fields := strings.Split(next.lit, ",")
		for _, field := range fields {
			fieldNames.Add(strings.TrimSpace(field))
		}

		return nil

	}

	return nil
}

func parseIdent(token token, l *lexer, cc *CacheControlResponse) error {
	switch strings.ToLower(token.lit) {
	case "max-age":
		deltaSeconds, err := parseDeltaSecondsIDent(l)
		if err != nil {
			return err
		}
		cc.MaxAge = deltaSeconds
	case "s-maxage":
		deltaSeconds, err := parseDeltaSecondsIDent(l)
		if err != nil {
			return err
		}
		cc.SMaxAge = deltaSeconds
	case "no-store":
		cc.NoStore = true
	case "no-cache":
		cc.NoCache = NewFieldNames()
		err := parseFieldNamesIdent(l, cc.NoCache)
		if err != nil {
			return err
		}
	case "public":
		cc.Public = true
	case "private":
		cc.Private = NewFieldNames()
		err := parseFieldNamesIdent(l, cc.Private)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unexpected ident: %s", token.lit)
	}

	return nil
}
