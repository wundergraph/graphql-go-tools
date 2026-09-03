package cache

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/runes"
)

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

	tokenTypeString
)

// directiveArgument is the raw text following "=" for a single directive.
type directiveArgument struct {
	// present is true even when the "=" was followed by nothing usable.
	present bool
	text    string
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

// OWS is SP / HTAB only. CR and LF are excluded on purpose: they are malformed
// inside a field value, not skippable.
func (l *lexer) isWhitespace(r rune) bool {
	return r == runes.SPACE || r == runes.TAB
}

// HTAB is a control character but is legal as both OWS and qdtext, so it is
// excluded here.
func (l *lexer) isForbiddenCharacter(r rune) bool {
	return r != runes.TAB && l.isControlCharacter(r)
}

// isControlCharacter checks if the rune is a control character.
// ASCII control characters are ranged from 0-31 and 127.
//
// The r >= 0 guard keeps the end-of-input sentinel out: -1 is not a character
// at all, and without it every caller that reads past the end would be told it
// found a control byte.
func (l *lexer) isControlCharacter(r rune) bool {
	return r >= 0 && (r <= 0x1F || r == 0x7F) // 0-31 and 127
}

func (l *lexer) isInvalidTokenCharacter(r rune) bool {
	switch r {
	case '(', ')', '<', '>', '@', ',', ';', ':', '\\', '"', '/', '[', ']', '?', '=', '{', '}', ' ', '\t':
		return true
	}

	return false
}

// isPrintableCharacter checks if the rune is a printable character.
// Printable characters are ranged from 32 to 126.
func (l *lexer) isPrintableCharacter(r rune) bool {
	return r >= 0x20 && r <= 0x7E // 32-126
}

func (l *lexer) nextToken() (token, error) {
	tok := token{}

	var next rune

	// Consume all whitespace characters until we encounter a valid token character.
	for {
		// Set the start position of the token.
		tok.setStart(l.pos)

		// Check if we have reached the end of the input.
		// If so, we return an EOF token.
		if l.atEnd() {
			tok.tt = tokenTypeEOF
			tok.setEnd(l.pos, l.input)
			return tok, nil
		}

		// Read the next byte from the input.
		next = l.read()

		// Check if the next byte is a forbidden character.
		if l.isForbiddenCharacter(next) {
			return tok, fmt.Errorf("control character %#x found in input at position %d", next, l.pos-1)
		}

		// When we encounter a non-whitespace character, we break out of the loop.
		if !l.isWhitespace(next) {
			break
		}
	}

	// Check if our next byte is a single byte token.
	if l.matchSingleRuneToken(next, &tok) {
		tok.setEnd(l.pos, l.input)
		return tok, nil
	}

	if next == runes.QUOTE {
		// We expect a string token here.
		err := l.readString(&tok)
		return tok, err
	}

	// All other tokens are identifiers, we also treat numbers as identifiers instead of defining
	// an unquoted value token type.
	//
	// TODO: might be better for readability to define an unquoted value token type.
	err := l.readIdent(&tok)
	return tok, err
}

// matchSingleByteToken matches a single byte token.
func (l *lexer) matchSingleRuneToken(r rune, tok *token) bool {
	switch r {
	case -1:
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
		if l.atEnd() {
			tok.setEnd(l.pos, l.input)
			return nil
		}

		next := l.peek(0)

		if l.isForbiddenCharacter(next) {
			return fmt.Errorf("control character %#x found in input at position %d", next, l.pos)
		}

		switch next {
		case runes.COMMA, runes.EQUALS, runes.SPACE, runes.TAB:
			tok.setEnd(l.pos, l.input)
			return nil
		}

		// obs-text is legal in a quoted string but never in a token.
		if !l.isPrintableCharacter(next) || l.isInvalidTokenCharacter(next) {
			return fmt.Errorf("invalid character %q found in input at position %d", next, l.pos)
		}

		l.advance(1)
	}
}

func (l *lexer) readString(tok *token) error {
	tok.setStart(l.pos)
	tok.tt = tokenTypeString

	for {
		if l.atEnd() {
			return fmt.Errorf("unexpected end of input at position %d", l.pos)
		}

		next := l.peek(0)

		if l.isForbiddenCharacter(next) {
			return fmt.Errorf("control character %#x found in input at position %d", next, l.pos)
		}

		if next == runes.QUOTE {
			tok.setEnd(l.pos, l.input)
			l.advance(1) // consume the quote
			return nil
		}

		l.advance(1)
	}
}

func (l *lexer) peek(n int) rune {
	if l.pos+n >= len(l.input) {
		return -1
	}

	return rune(l.input[l.pos+n])
}

func (l *lexer) advance(n int) {
	if l.pos+n >= len(l.input) {
		l.pos = len(l.input)
	} else {
		l.pos += n
	}
}

func (l *lexer) read() rune {
	if l.pos >= len(l.input) {
		return -1
	}

	b := l.input[l.pos]
	l.pos++
	return rune(b)
}

func (l *lexer) atEnd() bool {
	return l.pos >= len(l.input)
}

// readArgument consumes the "=" and value that may follow a directive name.
// Whitespace around the "=" is tolerated even though the grammar forbids it,
// so that a cosmetic space does not discard the whole field.
func (l *lexer) readArgument() (directiveArgument, error) {
	if l.peekToken().tt != tokenTypeEquals {
		return directiveArgument{}, nil
	}

	l.readToken(0)
	arg := directiveArgument{present: true}

	value := l.peekToken()
	switch value.tt {
	case tokenTypeString:
		l.readToken(0)
		arg.text = value.lit

		// Nothing may follow a closing quote except a delimiter.
		if after := l.peekToken(); after.tt != tokenTypeComma && after.tt != tokenTypeEOF {
			return arg, fmt.Errorf("unexpected token %q after quoted value at position %d", after.lit, after.start)
		}

		return arg, nil

	case tokenTypeIdent:
		l.readToken(0)
		arg.text = value.lit

		return arg, nil
	}

	// "=" with nothing usable after it.
	return arg, nil
}
