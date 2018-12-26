package lexer

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/runes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	"io"
	"unicode"
	"unicode/utf8"
)

// Lexer emits tokens from a input reader
type Lexer struct {
	reader                           *bufio.Reader
	buffer                           *bytes.Buffer
	equalsBuffer                     *bytes.Buffer
	line                             int
	char                             int
	charPositionBeforeLineTerminator int
}

// NewLexer initializes a new lexer
func NewLexer() *Lexer {
	return &Lexer{
		buffer:       &bytes.Buffer{},
		equalsBuffer: &bytes.Buffer{},
	}
}

// SetInput sets the new reader as input and resets all position stats
func (l *Lexer) SetInput(reader io.Reader) {
	if l.reader == nil {
		l.reader = bufio.NewReader(reader)
	} else {
		l.reader.Reset(reader)
	}

	l.line = 1
	l.char = 1
}

// Read emits the next token, this cannot be undone
func (l *Lexer) Read() (tok token.Token, err error) {

	run, position, err := l.readRune()
	if err == io.EOF {
		tok = token.EOF
		tok.Position = position
		return tok, nil
	} else if err != nil {
		return tok, err
	}

	if tok, matched := l.switchSimpleTokens(position, run); matched {
		return tok, nil
	}

	switch run {
	case runes.COMMA, runes.SPACE, runes.TAB, runes.LINETERMINATOR:
		return l.Read()
	case runes.QUOTE:
		return l.readString(position)
	case runes.DOT:
		return l.readSpread(position)
	case runes.DOLLAR:
		return l.readVariable(position)
	}

	if unicode.IsDigit(run) {
		return l.readDigit(position, run)
	}

	return l.readIdent(position, run)
}

func (l *Lexer) swallowWhitespace() error {
	for {
		next, err := l.reader.Peek(1)
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}

		if bytes.Equal(next, literal.SPACE) ||
			bytes.Equal(next, literal.TAB) ||
			bytes.Equal(next, literal.LINETERMINATOR) ||
			bytes.Equal(next, literal.COMMA) {
			_, _, err = l.readRune()
			if err != nil {
				return err
			}
		} else {
			return nil
		}
	}
}

// Peek will emit the next token without advancing the reader position
func (l *Lexer) Peek(ignoreWhitespace bool) (key keyword.Keyword, err error) {

	if ignoreWhitespace {
		err = l.swallowWhitespace()
		if err != nil {
			return key, err
		}
	}

	peeked, err := l.reader.Peek(1)
	if err == io.EOF {
		return keyword.EOF, nil
	} else if err != nil {
		return key, err
	}

	return l.keyFromBytes(peeked)
}

func (l *Lexer) keyFromBytes(b []byte) (key keyword.Keyword, err error) {

	r, _ := utf8.DecodeRune(b)

	switch r {
	case runes.SPACE:
		return keyword.SPACE, nil
	case runes.TAB:
		return keyword.TAB, nil
	case runes.COMMA:
		return keyword.COMMA, nil
	case runes.LINETERMINATOR:
		return runes.LINETERMINATOR, nil
	case runes.QUOTE:
		return keyword.STRING, nil
	case runes.DOLLAR:
		return keyword.VARIABLE, nil
	case runes.PIPE:
		return keyword.PIPE, nil
	case runes.EQUALS:
		return keyword.EQUALS, nil
	case runes.AT:
		return keyword.AT, nil
	case runes.COLON:
		return keyword.COLON, nil
	case runes.BANG:
		return keyword.BANG, nil
	case runes.BRACKETOPEN:
		return keyword.BRACKETOPEN, nil
	case runes.BRACKETCLOSE:
		return keyword.BRACKETCLOSE, nil
	case runes.CURLYBRACKETOPEN:
		return keyword.CURLYBRACKETOPEN, nil
	case runes.CURLYBRACKETCLOSE:
		return keyword.CURLYBRACKETCLOSE, nil
	case runes.SQUAREBRACKETOPEN:
		return keyword.SQUAREBRACKETOPEN, nil
	case runes.SQUAREBRACKETCLOSE:
		return keyword.SQUAREBRACKETCLOSE, nil
	case runes.AND:
		return keyword.AND, nil
	case runes.DOT:
		return l.peekSpread()
	}

	if unicode.IsDigit(r) {
		isFloat, err := l.peekIsFloat()
		if err != nil {
			return key, err
		} else if isFloat {
			return keyword.FLOAT, nil
		} else {
			return keyword.INTEGER, nil
		}
	}

	return l.peekIdent()
}

func (l *Lexer) peekSpread() (key keyword.Keyword, err error) {

	actual, err := l.reader.Peek(len(literal.SPREAD))
	if err != nil {
		return key, err
	}

	if bytes.Equal(actual, literal.SPREAD) {
		return keyword.SPREAD, nil
	}

	return keyword.UNDEFINED, nil
}

func (l *Lexer) peekIsFloat() (isFloat bool, err error) {
	var numBytesToRead int
	var bytesPeeked []byte
	for err == nil {
		numBytesToRead++

		bytesPeeked, err = l.reader.Peek(numBytesToRead)
		if bytes.HasSuffix(bytesPeeked, literal.SPACE) ||
			bytes.HasSuffix(bytesPeeked, literal.TAB) ||
			bytes.HasSuffix(bytesPeeked, literal.LINETERMINATOR) ||
			bytes.HasSuffix(bytesPeeked, literal.COMMA) {
			return
		} else if bytes.HasSuffix(bytesPeeked, literal.DOT) {
			return true, nil
		}
	}

	if err == io.EOF {
		err = nil
	}

	return
}

func (l *Lexer) switchSimpleTokens(position position.Position, run rune) (tok token.Token, matched bool) {

	matched = true

	switch run {
	case runes.PIPE:
		tok = token.Pipe
	case runes.EQUALS:
		tok = token.Equals
	case runes.AT:
		tok = token.At
	case runes.COLON:
		tok = token.Colon
	case runes.BANG:
		tok = token.Bang
	case runes.BRACKETOPEN:
		tok = token.BracketOpen
	case runes.BRACKETCLOSE:
		tok = token.BracketClose
	case runes.CURLYBRACKETOPEN:
		tok = token.CurlyBracketOpen
	case runes.CURLYBRACKETCLOSE:
		tok = token.CurlyBracketClose
	case runes.SQUAREBRACKETOPEN:
		tok = token.SquaredBracketOpen
	case runes.SQUAREBRACKETCLOSE:
		tok = token.SquaredBracketClose
	case runes.AND:
		tok = token.And
	default:
		matched = false
	}

	tok.Position = position

	return
}

func (l *Lexer) readIdent(position position.Position, beginWith rune) (tok token.Token, err error) {

	tok.Position = position

	_, err = l.buffer.WriteRune(beginWith)
	if err != nil {
		return tok, err
	}

	var nextRune rune

	for {
		nextRune, _, err = l.readRune()
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			l.buffer.Reset()
			return
		}

		if unicode.IsLetter(nextRune) ||
			unicode.IsDigit(nextRune) ||
			nextRune == runes.UNDERSCORE ||
			nextRune == runes.NEGATIVESIGN {
			_, err = l.buffer.WriteRune(nextRune)
			if err != nil {
				return tok, err
			}
		} else {
			err = l.unreadRune()
			if err != nil {
				return tok, err
			}
			break
		}
	}

	tok.Literal = l.buffer.Bytes()
	l.buffer.Reset()

	if bytes.Equal(tok.Literal, literal.TRUE) {
		tok.Keyword = keyword.TRUE
	} else if bytes.Equal(tok.Literal, literal.FALSE) {
		tok.Keyword = keyword.FALSE
	} else if bytes.Equal(tok.Literal, literal.NULL) {
		tok.Keyword = keyword.NULL
	} else if bytes.Equal(tok.Literal, literal.ON) {
		tok.Keyword = keyword.ON
	} else if bytes.Equal(tok.Literal, literal.IMPLEMENTS) {
		tok.Keyword = keyword.IMPLEMENTS
	} else if bytes.Equal(tok.Literal, literal.SCHEMA) {
		tok.Keyword = keyword.SCHEMA
	} else if bytes.Equal(tok.Literal, literal.SCALAR) {
		tok.Keyword = keyword.SCALAR
	} else if bytes.Equal(tok.Literal, literal.TYPE) {
		tok.Keyword = keyword.TYPE
	} else if bytes.Equal(tok.Literal, literal.INTERFACE) {
		tok.Keyword = keyword.INTERFACE
	} else if bytes.Equal(tok.Literal, literal.UNION) {
		tok.Keyword = keyword.UNION
	} else if bytes.Equal(tok.Literal, literal.ENUM) {
		tok.Keyword = keyword.ENUM
	} else if bytes.Equal(tok.Literal, literal.INPUT) {
		tok.Keyword = keyword.INPUT
	} else if bytes.Equal(tok.Literal, literal.DIRECTIVE) {
		tok.Keyword = keyword.DIRECTIVE
	} else if bytes.Equal(tok.Literal, literal.QUERY) {
		tok.Keyword = keyword.QUERY
	} else if bytes.Equal(tok.Literal, literal.MUTATION) {
		tok.Keyword = keyword.MUTATION
	} else if bytes.Equal(tok.Literal, literal.SUBSCRIPTION) {
		tok.Keyword = keyword.SUBSCRIPTION
	} else if bytes.Equal(tok.Literal, literal.FRAGMENT) {
		tok.Keyword = keyword.FRAGMENT
	} else {
		tok.Keyword = keyword.IDENT
	}

	return
}

func (l *Lexer) isTerminated(input []byte) bool {
	return bytes.HasSuffix(input, literal.SPACE) ||
		bytes.HasSuffix(input, literal.TAB) ||
		bytes.HasSuffix(input, literal.LINETERMINATOR) ||
		bytes.HasSuffix(input, literal.COMMA) ||
		bytes.HasSuffix(input, literal.EQUALS) ||
		bytes.HasSuffix(input, literal.COLON) ||
		bytes.HasSuffix(input, literal.CURLYBRACKETOPEN) ||
		bytes.HasSuffix(input, literal.CURLYBRACKETCLOSE) ||
		bytes.HasSuffix(input, literal.BRACKETOPEN) ||
		bytes.HasSuffix(input, literal.BRACKETCLOSE) ||
		bytes.HasSuffix(input, literal.SQUAREBRACKETOPEN) ||
		bytes.HasSuffix(input, literal.SQUAREBRACKETCLOSE) ||
		bytes.HasSuffix(input, literal.PIPE) ||
		bytes.HasSuffix(input, literal.BANG) ||
		bytes.HasSuffix(input, literal.AND) ||
		bytes.HasSuffix(input, literal.DOLLAR) ||
		bytes.HasSuffix(input, literal.QUOTE) ||
		bytes.HasSuffix(input, literal.SLASH) ||
		bytes.HasSuffix(input, literal.BACKSLASH) ||
		bytes.HasSuffix(input, literal.AT)
}

func (l *Lexer) peekEOFSafe(n int) ([]byte, error) {
	peeked, err := l.reader.Peek(n)
	if err == nil || err == io.EOF {
		return peeked, nil
	}

	return nil, err
}

func (l *Lexer) peekIdent2() (done bool, k keyword.Keyword, err error) {

	peeked, err := l.peekEOFSafe(3)
	if err != nil {
		return true, k, err
	}

	if len(peeked) == 3 && !l.isTerminated(peeked) {
		return false, k, err
	}

	if bytes.HasPrefix(peeked, literal.ON) {
		return true, keyword.ON, nil
	}

	return true, keyword.IDENT, nil
}

func (l *Lexer) peekIdent4() (done bool, k keyword.Keyword, err error) {

	peeked, err := l.peekEOFSafe(5)
	if err != nil {
		return true, k, err
	}

	if len(peeked) == 5 && !l.isTerminated(peeked) {
		return false, k, err
	}

	if bytes.HasPrefix(peeked, literal.TRUE) {
		return true, keyword.TRUE, nil
	} else if bytes.HasPrefix(peeked, literal.FALSE) {
		return true, keyword.FALSE, nil
	} else if bytes.HasPrefix(peeked, literal.NULL) {
		return true, keyword.NULL, nil
	} else if bytes.HasPrefix(peeked, literal.TYPE) {
		return true, keyword.TYPE, nil
	} else if bytes.HasPrefix(peeked, literal.ENUM) {
		return true, keyword.ENUM, nil
	}

	return true, keyword.IDENT, nil
}

func (l *Lexer) peekIdent5() (done bool, k keyword.Keyword, err error) {

	peeked, err := l.peekEOFSafe(6)
	if err != nil {
		return true, k, err
	}

	if len(peeked) == 6 && !l.isTerminated(peeked) {
		return false, k, err
	}

	if bytes.HasPrefix(peeked, literal.FALSE) {
		return true, keyword.FALSE, nil
	} else if bytes.HasPrefix(peeked, literal.UNION) {
		return true, keyword.UNION, nil
	} else if bytes.HasPrefix(peeked, literal.INPUT) {
		return true, keyword.INPUT, nil
	} else if bytes.HasPrefix(peeked, literal.QUERY) {
		return true, keyword.QUERY, nil
	}

	return true, keyword.IDENT, nil
}

func (l *Lexer) peekIdent6() (done bool, k keyword.Keyword, err error) {

	peeked, err := l.peekEOFSafe(7)
	if err != nil {
		return true, k, err
	}

	if len(peeked) == 7 && !l.isTerminated(peeked) {
		return false, k, err
	}

	if bytes.HasPrefix(peeked, literal.SCHEMA) {
		return true, keyword.SCHEMA, nil
	} else if bytes.HasPrefix(peeked, literal.SCALAR) {
		return true, keyword.SCALAR, nil
	}

	return true, keyword.IDENT, nil
}

func (l *Lexer) peekIdent8() (done bool, k keyword.Keyword, err error) {

	peeked, err := l.peekEOFSafe(9)
	if err != nil {
		return true, k, err
	}

	if len(peeked) == 9 && !l.isTerminated(peeked) {
		return false, k, err
	}

	if bytes.HasPrefix(peeked, literal.MUTATION) {
		return true, keyword.MUTATION, nil
	} else if bytes.HasPrefix(peeked, literal.FRAGMENT) {
		return true, keyword.FRAGMENT, nil
	}

	return true, keyword.IDENT, nil
}

func (l *Lexer) peekIdent9() (done bool, k keyword.Keyword, err error) {

	peeked, err := l.peekEOFSafe(10)
	if err != nil {
		return true, k, err
	}

	if len(peeked) == 10 && !l.isTerminated(peeked) {
		return false, k, err
	}

	if bytes.HasPrefix(peeked, literal.INTERFACE) {
		return true, keyword.INTERFACE, nil
	} else if bytes.HasPrefix(peeked, literal.DIRECTIVE) {
		return true, keyword.DIRECTIVE, nil
	}

	return true, keyword.IDENT, nil
}

func (l *Lexer) peekIdent10() (done bool, k keyword.Keyword, err error) {

	peeked, err := l.peekEOFSafe(11)
	if err != nil {
		return true, k, err
	}

	if len(peeked) == 11 && !l.isTerminated(peeked) {
		return false, k, err
	}

	if bytes.HasPrefix(peeked, literal.IMPLEMENTS) {
		return true, keyword.IMPLEMENTS, nil
	}

	return true, keyword.IDENT, nil
}

func (l *Lexer) peekIdent12() (done bool, k keyword.Keyword, err error) {

	peeked, err := l.peekEOFSafe(13)
	if err != nil {
		return true, k, err
	}

	if len(peeked) == 13 && !l.isTerminated(peeked) {
		return false, k, err
	}

	if bytes.HasPrefix(peeked, literal.SUBSCRIPTION) {
		return true, keyword.SUBSCRIPTION, nil
	}

	return true, keyword.IDENT, nil
}

func (l *Lexer) peekIdent() (k keyword.Keyword, err error) {

	if done, k, err := l.peekIdent2(); done {
		return k, err
	}

	if done, k, err := l.peekIdent4(); done {
		return k, err
	}

	if done, k, err := l.peekIdent5(); done {
		return k, err
	}

	if done, k, err := l.peekIdent6(); done {
		return k, err
	}

	if done, k, err := l.peekIdent8(); done {
		return k, err
	}

	if done, k, err := l.peekIdent9(); done {
		return k, err
	}

	if done, k, err := l.peekIdent10(); done {
		return k, err
	}

	if done, k, err := l.peekIdent12(); done {
		return k, err
	}

	return keyword.IDENT, nil
}

func (l *Lexer) readVariable(position position.Position) (tok token.Token, err error) {

	tok.Position = position
	tok.Keyword = keyword.VARIABLE

	peeked, err := l.Peek(false)
	if err != nil {
		return tok, err
	}

	if peeked == keyword.SPACE ||
		peeked == keyword.TAB ||
		peeked == keyword.COMMA ||
		peeked == keyword.LINETERMINATOR {
		return tok, fmt.Errorf("readVariable: must not have whitespace after $")
	}

	ident, err := l.readIdent(position, runes.DOLLAR)
	if err != nil {
		return tok, err
	}

	tok.Literal = ident.Literal[1:]
	return
}

func (l *Lexer) readSpread(position position.Position) (tok token.Token, err error) {

	tok.Position = position

	isSpread, err := l.peekEquals([]rune{runes.DOT, runes.DOT}, true, false)
	if err != nil {
		return tok, err
	}

	if !isSpread {
		return tok, fmt.Errorf("readSpread: invalid '.' at position %s", position.String())
	}

	tok = token.Spread
	tok.Position = position
	return
}

func (l *Lexer) readString(position position.Position) (tok token.Token, err error) {

	tok.Keyword = keyword.STRING
	tok.Position = position

	isMultiLineString, err := l.peekEquals([]rune{runes.QUOTE, runes.QUOTE}, true, true)
	if err != nil {
		return tok, err
	}

	if isMultiLineString {

		var escaped bool

		for {

			nextRune, _, err := l.readRune()
			if err != nil {
				return tok, err
			}

			switch nextRune {
			case runes.QUOTE:
				if escaped {
					l.buffer.WriteRune(nextRune)
					escaped = false
				} else {

					isMultiLineStringEnd, err := l.peekEquals([]rune{runes.QUOTE, runes.QUOTE}, true, true)
					if err != nil {
						return tok, err
					}

					if !isMultiLineStringEnd {
						l.buffer.WriteRune(nextRune)
						escaped = false
					} else {
						tok.Literal = l.buffer.Bytes()
						l.buffer.Reset()
						tok.Literal = l.trimStartEnd(tok.Literal, literal.LINETERMINATOR)
						return tok, nil
					}
				}
			case runes.BACKSLASH:
				if escaped {
					l.buffer.WriteRune(nextRune)
					escaped = false
				} else {
					escaped = true
				}
			default:
				l.buffer.WriteRune(nextRune)
				escaped = false
			}
		}

		//tok.Literal, err = l.readWriteUntil([]rune{runes.QUOTE, runes.QUOTE, runes.QUOTE}, true)
		//tok.Literal = l.trimStartEnd(tok.Literal, literal.LINETERMINATOR)
		return
	}

	var escaped bool

	for {

		nextRune, _, err := l.readRune()
		if err != nil {
			return tok, err
		}

		switch nextRune {
		case runes.QUOTE:
			if escaped {
				l.buffer.WriteRune(nextRune)
				escaped = false
			} else {
				tok.Literal = l.buffer.Bytes()
				l.buffer.Reset()
				return tok, nil
			}
		case runes.BACKSLASH:
			if escaped {
				l.buffer.WriteRune(nextRune)
				escaped = false
			} else {
				escaped = true
			}
		default:
			l.buffer.WriteRune(nextRune)
			escaped = false
		}
	}
}

func (l *Lexer) swallow(amount int) error {
	for i := 0; i < amount; i++ {
		_, _, err := l.readRune()
		if err != nil {
			return err
		}
	}

	return nil
}

func (l *Lexer) peekEquals(equals []rune, swallow, returnErrorOnEOF bool) (bool, error) {

	for _, r := range equals {
		_, err := l.equalsBuffer.WriteRune(r)
		if err != nil {
			return false, err
		}
	}

	equalBytes := l.equalsBuffer.Bytes()
	l.equalsBuffer.Reset()

	var matches bool
	peeked, err := l.reader.Peek(len(equalBytes))
	if !returnErrorOnEOF && err == io.EOF {
		return false, nil
	}

	if err != nil {
		return matches, err
	}

	matches = bytes.Equal(equalBytes, peeked)
	if swallow && matches {
		err = l.swallow(len(equals))
	}

	return matches, err
}

func (l *Lexer) readDigit(position position.Position, beginWith rune) (tok token.Token, err error) {

	tok.Position = position

	_, err = l.buffer.WriteRune(beginWith)
	if err != nil {
		return tok, err
	}

	tok.Keyword = keyword.INTEGER

	tok.Literal, _, err = l.readWriteWhileMatching(unicode.IsDigit, false)

	if err != nil {
		return tok, err
	}

	isFloat, err := l.peekEquals([]rune{runes.DOT}, true, false)
	if err != nil {
		return tok, err
	}

	if isFloat {
		return l.readFloat(position, tok.Literal)
	}

	return
}

func (l *Lexer) readFloat(position position.Position, integerPart []byte) (tok token.Token, err error) {

	tok.Position = position

	_, err = l.buffer.Write(integerPart)
	if err != nil {
		return tok, err
	}

	_, err = l.buffer.WriteRune(runes.DOT)
	if err != nil {
		return tok, err
	}

	lit, totalMatches, err := l.readWriteWhileMatching(unicode.IsDigit, false)

	if err != nil {
		return tok, err
	}

	if totalMatches == 0 {
		return tok, fmt.Errorf("readFloat: expected float part after '.'")
	}

	tok.Keyword = keyword.FLOAT
	tok.Literal = lit

	return
}

func (l *Lexer) readWriteWhileMatching(matcher func(rune) bool, returnErrorOnEOF bool) (lit []byte, totalMatches int, err error) {

	for {
		run, _, err := l.readRune()
		if !returnErrorOnEOF && err == io.EOF {
			lit = l.buffer.Bytes()
			return lit, totalMatches, nil
		} else if err != nil {
			return lit, totalMatches, err
		}

		if matcher(run) {

			totalMatches++

			_, err = l.buffer.WriteRune(run)
			if err != nil {
				return lit, totalMatches, err
			}

		} else {

			err = l.unreadRune()
			if err != nil {
				return lit, totalMatches, err
			}

			lit = l.buffer.Bytes()
			l.buffer.Reset()
			return lit, totalMatches, nil
		}
	}
}

func (l *Lexer) trimStartEnd(input, trim []byte) []byte {
	return bytes.TrimSuffix(bytes.TrimPrefix(input, trim), trim)
}

func (l *Lexer) readRune() (r rune, position position.Position, err error) {

	position.Line = l.line
	position.Char = l.char

	r, size, err := l.reader.ReadRune()

	if r == runes.LINETERMINATOR {
		l.charPositionBeforeLineTerminator = l.char
		l.line++
		l.char = 1
	} else {
		l.char += size
	}

	return r, position, err
}

func (l *Lexer) unreadRune() error {

	err := l.reader.UnreadRune()
	if err != nil {
		return err
	}

	isLineTerminator, err := l.peekEquals([]rune{runes.LINETERMINATOR}, false, false)
	if err != nil {
		return err
	}

	if isLineTerminator {
		l.line = l.line - 1
		l.char = l.charPositionBeforeLineTerminator
	} else {
		l.char = l.char - 1
	}

	return nil
}
