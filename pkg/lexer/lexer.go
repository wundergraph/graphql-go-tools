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
	line                             int
	char                             int
	charPositionBeforeLineTerminator int
}

// NewLexer initializes a new lexer
func NewLexer() *Lexer {
	return &Lexer{
		buffer: &bytes.Buffer{},
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

	var r rune
	var pos position.Position

	for {
		r, pos, err = l.readRune()
		if err == io.EOF {
			tok = token.EOF
			tok.Position = pos
			return tok, nil
		} else if err != nil {
			return tok, err
		}

		if !l.runeIsWhitespace(r) {
			break
		}
	}

	if tok, matched := l.matchSingleRuneToken(pos, r); matched {
		return tok, nil
	}

	switch r {
	case runes.QUOTE:
		return l.readString(pos)
	case runes.DOT:
		return l.readSpread(pos)
	case runes.DOLLAR:
		return l.readVariable(pos)
	}

	if unicode.IsDigit(r) {
		return l.readDigit(pos, r)
	}

	return l.readIdent(pos, r)
}

func (l *Lexer) swallowWhitespace() (err error) {

	var peeked []byte

	for {
		peeked, err = l.reader.Peek(1)
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}

		if l.bytesIsWhitespace(peeked) {
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

	return l.keywordFromBytes(peeked)
}

func (l *Lexer) keywordFromBytes(b []byte) (key keyword.Keyword, err error) {

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

	peeked, err := l.reader.Peek(32)
	if err == io.EOF {
		err = nil
	} else if err != nil {
		return false, err
	}

	for pos := range peeked {
		r, _ := utf8.DecodeRune(peeked[pos : pos+1])

		if !isFloat && r == runes.DOT {
			isFloat = true
		} else if isFloat && r == runes.DOT {
			return false, fmt.Errorf("peekIsFloat: invalid input")
		} else if !unicode.IsDigit(r) {
			break
		}
	}

	return isFloat, err
}

func (l *Lexer) matchSingleRuneToken(position position.Position, run rune) (tok token.Token, matched bool) {

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

	var peeked []byte
	var r rune

	for {
		peeked, err = l.reader.Peek(1)
		if err == io.EOF {
			err = nil
			break
		} else if err != nil {
			return
		}

		if l.bytesIsIdent(peeked) {
			r, _, err = l.readRune()
			if err != nil {
				return tok, err
			}

			_, err = l.buffer.WriteRune(r)
			if err != nil {
				return tok, err
			}
		} else {
			break
		}
	}

	tok.Literal = make([]byte, l.buffer.Len())
	copy(tok.Literal, l.buffer.Bytes())
	l.buffer.Reset()

	tok.Keyword = l.identKeywordFromBytes(tok.Literal)

	return
}

const identWantBytes = 13

func (l *Lexer) peekIdent() (k keyword.Keyword, err error) {

	peeked, err := l.peekEOFSafe(identWantBytes)
	if err != nil {
		return k, err
	}

	nonIdentPosition := bytes.IndexFunc(peeked, func(r rune) bool {
		return !l.runeIsIdent(r)
	})

	if l.isUnterminatedIdent(identWantBytes, len(peeked), nonIdentPosition) {
		return keyword.IDENT, nil
	}

	if !l.isIndexFuncResultUnsatisfied(nonIdentPosition) {
		peeked = peeked[:nonIdentPosition]
	}

	return l.identKeywordFromBytes(peeked), nil
}

func (l *Lexer) isUnterminatedIdent(nWantBytes, nGotBytes, nonIdentPosition int) bool {
	return l.isIndexFuncResultUnsatisfied(nonIdentPosition) && nWantBytes == nGotBytes
}

func (l *Lexer) isIndexFuncResultUnsatisfied(result int) bool {
	return result == -1
}

func (l *Lexer) peekEOFSafe(n int) ([]byte, error) {
	peeked, err := l.reader.Peek(n)
	if err == nil || err == io.EOF {
		return peeked, nil
	}

	return nil, err
}

func (l *Lexer) identKeywordFromBytes(ident []byte) (k keyword.Keyword) {
	switch len(ident) {
	case 2:
		if bytes.Equal(ident, literal.ON) {
			k = keyword.ON
			return
		}
	case 4:
		if bytes.Equal(ident, literal.TRUE) {
			k = keyword.TRUE
			return
		} else if bytes.Equal(ident, literal.NULL) {
			k = keyword.NULL
			return
		} else if bytes.Equal(ident, literal.TYPE) {
			k = keyword.TYPE
			return
		} else if bytes.Equal(ident, literal.ENUM) {
			k = keyword.ENUM
			return
		}
	case 5:
		if bytes.Equal(ident, literal.FALSE) {
			k = keyword.FALSE
			return
		} else if bytes.Equal(ident, literal.UNION) {
			k = keyword.UNION
			return
		} else if bytes.Equal(ident, literal.INPUT) {
			k = keyword.INPUT
			return
		} else if bytes.Equal(ident, literal.QUERY) {
			k = keyword.QUERY
			return
		}
	case 6:
		if bytes.Equal(ident, literal.SCHEMA) {
			k = keyword.SCHEMA
			return
		} else if bytes.Equal(ident, literal.SCALAR) {
			k = keyword.SCALAR
			return
		}
	case 8:
		if bytes.Equal(ident, literal.MUTATION) {
			k = keyword.MUTATION
			return
		} else if bytes.Equal(ident, literal.FRAGMENT) {
			k = keyword.FRAGMENT
			return
		}
	case 9:
		if bytes.Equal(ident, literal.INTERFACE) {
			k = keyword.INTERFACE
			return
		} else if bytes.Equal(ident, literal.DIRECTIVE) {
			k = keyword.DIRECTIVE
			return
		}
	case 10:
		if bytes.Equal(ident, literal.IMPLEMENTS) {
			k = keyword.IMPLEMENTS
			return
		}
	case 12:
		if bytes.Equal(ident, literal.SUBSCRIPTION) {
			k = keyword.SUBSCRIPTION
			return
		}
	}

	return keyword.IDENT
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

	isSpread, err := l.peekEquals([]byte(".."), true, false)
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

func (l *Lexer) readString(pos position.Position) (tok token.Token, err error) {

	isMultiLineString, err := l.peekEquals([]byte(`""`), true, true)
	if err != nil {
		return tok, err
	}

	if isMultiLineString {
		return l.readMultiLineString(pos)
	}

	return l.readSingleLineString(pos)
}

func (l *Lexer) swallowAmount(amount int) error {
	for i := 0; i < amount; i++ {
		_, _, err := l.readRune()
		if err != nil {
			return err
		}
	}

	return nil
}

func (l *Lexer) peekEquals(equals []byte, swallow, returnErrorOnEOF bool) (bool, error) {

	var matches bool
	peeked, err := l.reader.Peek(len(equals))
	if !returnErrorOnEOF && err == io.EOF {
		return false, nil
	}

	if err != nil {
		return matches, err
	}

	matches = bytes.Equal(equals, peeked)
	if swallow && matches {
		err = l.swallowAmount(len(equals))
	}

	return matches, err
}

func (l *Lexer) readDigit(position position.Position, beginWith rune) (tok token.Token, err error) {

	tok.Position = position

	_, err = l.buffer.WriteRune(beginWith)
	if err != nil {
		return tok, err
	}

	totalMatches, err := l.writeNextDigitsToBuffer()
	if err != nil {
		l.buffer.Reset()
		return tok, err
	}

	if totalMatches == 0 {
		l.buffer.Reset()
		return tok, fmt.Errorf("readDigit: expected float part after '.'")
	}

	isFloat, err := l.peekEquals([]byte("."), true, false)
	if err != nil {
		return tok, err
	}

	if isFloat {
		return l.readFloat(position, tok.Literal)
	}

	tok.Keyword = keyword.INTEGER

	tok.Literal = make([]byte, l.buffer.Len())
	copy(tok.Literal, l.buffer.Bytes())
	l.buffer.Reset()

	return
}

func (l *Lexer) readFloat(position position.Position, integerPart []byte) (tok token.Token, err error) {

	tok.Position = position

	_, err = l.buffer.WriteRune(runes.DOT)
	if err != nil {
		l.buffer.Reset()
		return tok, err
	}

	totalMatches, err := l.writeNextDigitsToBuffer()
	if err != nil {
		l.buffer.Reset()
		return tok, err
	}

	if totalMatches == 0 {
		l.buffer.Reset()
		return tok, fmt.Errorf("readFloat: expected float part after '.'")
	}

	tok.Keyword = keyword.FLOAT
	tok.Literal = make([]byte, l.buffer.Len())
	copy(tok.Literal, l.buffer.Bytes())
	l.buffer.Reset()

	return
}

func (l *Lexer) writeNextDigitsToBuffer() (totalMatches int, err error) {

	var r rune

	for {
		r, _, err = l.readRune()
		if err == io.EOF {
			err = nil
			break
		} else if err != nil {
			return totalMatches, err
		}

		if unicode.IsDigit(r) {
			_, err = l.buffer.WriteRune(r)
			if err != nil {
				return totalMatches, err
			}

			totalMatches++

		} else {
			err = l.unreadRune()
			if err != nil {
				return totalMatches, err
			}
			break
		}
	}

	return
}

func (l *Lexer) trimStartEnd(input, trim []byte) []byte {
	return bytes.TrimSuffix(bytes.TrimPrefix(input, trim), trim)
}

func (l *Lexer) readRune() (r rune, position position.Position, err error) {

	if l.reader == nil {
		return r, position, fmt.Errorf("readRune: reader must not be nil")
	}

	position.Line = l.line
	position.Char = l.char

	r, size, err := l.reader.ReadRune()
	if err != nil {
		return r, position, err
	}

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

	isLineTerminator, err := l.peekEquals([]byte("\n"), false, false)
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

func (l *Lexer) runeIsIdent(r rune) bool {
	return unicode.IsLetter(r) ||
		unicode.IsDigit(r) ||
		r == runes.NEGATIVESIGN ||
		r == runes.UNDERSCORE
}

func (l *Lexer) bytesIsIdent(b []byte) bool {
	r, _ := utf8.DecodeRune(b)
	return l.runeIsIdent(r)
}

func (l *Lexer) runeIsWhitespace(r rune) bool {
	return r == runes.SPACE ||
		r == runes.TAB ||
		r == runes.LINETERMINATOR ||
		r == runes.COMMA
}

func (l *Lexer) bytesIsWhitespace(b []byte) bool {
	return bytes.Equal(b, literal.SPACE) ||
		bytes.Equal(b, literal.TAB) ||
		bytes.Equal(b, literal.LINETERMINATOR) ||
		bytes.Equal(b, literal.COMMA)
}

func (l *Lexer) readMultiLineString(pos position.Position) (tok token.Token, err error) {

	tok.Keyword = keyword.STRING
	tok.Position = pos

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

				isMultiLineStringEnd, err := l.peekEquals([]byte(`""`), true, true)
				if err != nil {
					return tok, err
				}

				if !isMultiLineStringEnd {
					l.buffer.WriteRune(nextRune)
					escaped = false
				} else {
					tok.Literal = make([]byte, l.buffer.Len())
					copy(tok.Literal, l.buffer.Bytes())
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
}

func (l *Lexer) readSingleLineString(pos position.Position) (tok token.Token, err error) {

	tok.Keyword = keyword.STRING
	tok.Position = pos

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
				tok.Literal = make([]byte, l.buffer.Len())
				copy(tok.Literal, l.buffer.Bytes())
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
