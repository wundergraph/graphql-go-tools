package lexer

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/runes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

// Lexer emits tokens from a input reader
type Lexer struct {
	input ast.Input
}

func (l *Lexer) SetInput(input ast.Input) {
	l.input = input
}

// Read emits the next token
func (l *Lexer) Read() (tok token.Token) {

	var next byte
	var inputPositionStart int

	for {
		inputPositionStart = l.input.InputPosition
		tok.SetStart(l.input.InputPosition, l.input.TextPosition)
		next = l.readRune()
		if !l.byteIsWhitespace(next) {
			break
		}
	}

	if l.matchSingleRuneToken(next, &tok) {
		return
	}

	switch next {
	case runes.HASHTAG:
		l.readComment(&tok)
		return
	case runes.QUOTE:
		l.readString(&tok)
		return
	case runes.DOT:
		l.readDotOrSpread(&tok)
		return
	}

	if runeIsDigit(next) {
		l.readDigit(&tok)
		return
	}

	l.readIdent()
	tok.Keyword = l.keywordFromIdent(inputPositionStart, l.input.InputPosition)
	tok.SetEnd(l.input.InputPosition, l.input.TextPosition)
	return
}

func (l *Lexer) keywordFromRune(r byte) keyword.Keyword {

	switch r {
	case runes.EOF:
		return keyword.EOF
	case runes.SPACE:
		return keyword.SPACE
	case runes.HASHTAG:
		return keyword.COMMENT
	case runes.TAB:
		return keyword.TAB
	case runes.COMMA:
		return keyword.COMMA
	case runes.LINETERMINATOR:
		return keyword.LINETERMINATOR
	case runes.QUOTE:
		return keyword.STRING
	case runes.DOLLAR:
		return keyword.DOLLAR
	case runes.PIPE:
		return keyword.PIPE
	case runes.EQUALS:
		return keyword.EQUALS
	case runes.AT:
		return keyword.AT
	case runes.COLON:
		return keyword.COLON
	case runes.BANG:
		return keyword.BANG
	case runes.LPAREN:
		return keyword.LPAREN
	case runes.RPAREN:
		return keyword.RPAREN
	case runes.LBRACE:
		return keyword.LBRACE
	case runes.RBRACE:
		return keyword.RBRACE
	case runes.LBRACK:
		return keyword.LBRACK
	case runes.RBRACK:
		return keyword.RBRACK
	case runes.AND:
		return keyword.AND
	case runes.SUB:
		return keyword.SUB
	case runes.DOT:
		if l.peekEquals(true, runes.DOT, runes.DOT, runes.DOT) {
			return keyword.SPREAD
		}
		return keyword.DOT
	}

	if runeIsDigit(r) {
		if l.peekIsFloat() {
			return keyword.FLOAT
		}
		return keyword.INTEGER
	}

	return l.peekIdent()
}

func (l *Lexer) peekIsFloat() (isFloat bool) {

	var hasDot bool
	var peeked byte

	start := l.input.InputPosition + l.peekWhitespaceLength()

	for i := start; i < l.input.Length; i++ {

		peeked = l.input.RawBytes[i]

		if l.byteTerminatesSequence(peeked) {
			return hasDot
		} else if peeked == runes.DOT && !hasDot {
			hasDot = true
		} else if peeked == runes.DOT && hasDot {
			return false
		} else if !runeIsDigit(peeked) {
			return false
		}
	}

	return hasDot
}

func (l *Lexer) matchSingleRuneToken(r byte, tok *token.Token) bool {

	switch r {
	case runes.EOF:
		tok.Keyword = keyword.EOF
	case runes.PIPE:
		tok.Keyword = keyword.PIPE
	case runes.EQUALS:
		tok.Keyword = keyword.EQUALS
	case runes.AT:
		tok.Keyword = keyword.AT
	case runes.COLON:
		tok.Keyword = keyword.COLON
	case runes.BANG:
		tok.Keyword = keyword.BANG
	case runes.LPAREN:
		tok.Keyword = keyword.LPAREN
	case runes.RPAREN:
		tok.Keyword = keyword.RPAREN
	case runes.LBRACE:
		tok.Keyword = keyword.LBRACE
	case runes.RBRACE:
		tok.Keyword = keyword.RBRACE
	case runes.LBRACK:
		tok.Keyword = keyword.LBRACK
	case runes.RBRACK:
		tok.Keyword = keyword.RBRACK
	case runes.AND:
		tok.Keyword = keyword.AND
	case runes.SUB:
		tok.Keyword = keyword.SUB
	case runes.DOLLAR:
		tok.Keyword = keyword.DOLLAR
	default:
		return false
	}

	tok.SetEnd(l.input.InputPosition, l.input.TextPosition)

	return true
}

func (l *Lexer) readIdent() {

	var r byte

	for {
		r = l.peekRune(false)
		if !runeIsIdent(r) {
			return
		}
		l.readRune()
	}
}

const identWantRunes = 13

func (l *Lexer) peekIdent() (k keyword.Keyword) {

	whitespaceOffset := l.peekWhitespaceLength()

	start := l.input.InputPosition + whitespaceOffset
	end := start + identWantRunes
	if end > l.input.Length {
		end = l.input.Length
	}

	for i := start; i < end; {
		if !runeIsIdent(l.input.RawBytes[i]) {
			end = i
			break
		}

		i++
	}

	return l.keywordFromIdent(start, end)
}

func (l *Lexer) keywordFromIdent(start, end int) (k keyword.Keyword) {

	switch end - start {
	case 2:
		if l.input.RawBytes[start] == 'o' && l.input.RawBytes[start+1] == 'n' {
			return keyword.ON
		}
	case 4:
		if l.input.RawBytes[start] == 'n' && l.input.RawBytes[start+1] == 'u' && l.input.RawBytes[start+2] == 'l' && l.input.RawBytes[start+3] == 'l' {
			return keyword.NULL
		}
		if l.input.RawBytes[start] == 'e' && l.input.RawBytes[start+1] == 'n' && l.input.RawBytes[start+2] == 'u' && l.input.RawBytes[start+3] == 'm' {
			return keyword.ENUM
		}
		if l.input.RawBytes[start] == 't' {
			if l.input.RawBytes[start+1] == 'r' && l.input.RawBytes[start+2] == 'u' && l.input.RawBytes[start+3] == 'e' {
				return keyword.TRUE
			}
			if l.input.RawBytes[start+1] == 'y' && l.input.RawBytes[start+2] == 'p' && l.input.RawBytes[start+3] == 'e' {
				return keyword.TYPE
			}
		}
	case 5:
		if l.input.RawBytes[start] == 'f' && l.input.RawBytes[start+1] == 'a' && l.input.RawBytes[start+2] == 'l' && l.input.RawBytes[start+3] == 's' && l.input.RawBytes[start+4] == 'e' {
			return keyword.FALSE
		}
		if l.input.RawBytes[start] == 'u' && l.input.RawBytes[start+1] == 'n' && l.input.RawBytes[start+2] == 'i' && l.input.RawBytes[start+3] == 'o' && l.input.RawBytes[start+4] == 'n' {
			return keyword.UNION
		}
		if l.input.RawBytes[start] == 'q' && l.input.RawBytes[start+1] == 'u' && l.input.RawBytes[start+2] == 'e' && l.input.RawBytes[start+3] == 'r' && l.input.RawBytes[start+4] == 'y' {
			return keyword.QUERY
		}
		if l.input.RawBytes[start] == 'i' && l.input.RawBytes[start+1] == 'n' && l.input.RawBytes[start+2] == 'p' && l.input.RawBytes[start+3] == 'u' && l.input.RawBytes[start+4] == 't' {
			return keyword.INPUT
		}
	case 6:
		if l.input.RawBytes[start] == 'e' && l.input.RawBytes[start+1] == 'x' && l.input.RawBytes[start+2] == 't' && l.input.RawBytes[start+3] == 'e' && l.input.RawBytes[start+4] == 'n' && l.input.RawBytes[start+5] == 'd' {
			return keyword.EXTEND
		}
		if l.input.RawBytes[start] == 's' {
			if l.input.RawBytes[start+1] == 'c' && l.input.RawBytes[start+2] == 'h' && l.input.RawBytes[start+3] == 'e' && l.input.RawBytes[start+4] == 'm' && l.input.RawBytes[start+5] == 'a' {
				return keyword.SCHEMA
			}
			if l.input.RawBytes[start+1] == 'c' && l.input.RawBytes[start+2] == 'a' && l.input.RawBytes[start+3] == 'l' && l.input.RawBytes[start+4] == 'a' && l.input.RawBytes[start+5] == 'r' {
				return keyword.SCALAR
			}
		}
	case 8:
		if l.input.RawBytes[start] == 'm' && l.input.RawBytes[start+1] == 'u' && l.input.RawBytes[start+2] == 't' && l.input.RawBytes[start+3] == 'a' && l.input.RawBytes[start+4] == 't' && l.input.RawBytes[start+5] == 'i' && l.input.RawBytes[start+6] == 'o' && l.input.RawBytes[start+7] == 'n' {
			return keyword.MUTATION
		}
		if l.input.RawBytes[start] == 'f' && l.input.RawBytes[start+1] == 'r' && l.input.RawBytes[start+2] == 'a' && l.input.RawBytes[start+3] == 'g' && l.input.RawBytes[start+4] == 'm' && l.input.RawBytes[start+5] == 'e' && l.input.RawBytes[start+6] == 'n' && l.input.RawBytes[start+7] == 't' {
			return keyword.FRAGMENT
		}
	case 9:
		if l.input.RawBytes[start] == 'i' && l.input.RawBytes[start+1] == 'n' && l.input.RawBytes[start+2] == 't' && l.input.RawBytes[start+3] == 'e' && l.input.RawBytes[start+4] == 'r' && l.input.RawBytes[start+5] == 'f' && l.input.RawBytes[start+6] == 'a' && l.input.RawBytes[start+7] == 'c' && l.input.RawBytes[start+8] == 'e' {
			return keyword.INTERFACE
		}
		if l.input.RawBytes[start] == 'd' && l.input.RawBytes[start+1] == 'i' && l.input.RawBytes[start+2] == 'r' && l.input.RawBytes[start+3] == 'e' && l.input.RawBytes[start+4] == 'c' && l.input.RawBytes[start+5] == 't' && l.input.RawBytes[start+6] == 'i' && l.input.RawBytes[start+7] == 'v' && l.input.RawBytes[start+8] == 'e' {
			return keyword.DIRECTIVE
		}
	case 10:
		if l.input.RawBytes[start] == 'i' && l.input.RawBytes[start+1] == 'm' && l.input.RawBytes[start+2] == 'p' && l.input.RawBytes[start+3] == 'l' && l.input.RawBytes[start+4] == 'e' && l.input.RawBytes[start+5] == 'm' && l.input.RawBytes[start+6] == 'e' && l.input.RawBytes[start+7] == 'n' && l.input.RawBytes[start+8] == 't' && l.input.RawBytes[start+9] == 's' {
			return keyword.IMPLEMENTS
		}
	case 12:
		if l.input.RawBytes[start] == 's' && l.input.RawBytes[start+1] == 'u' && l.input.RawBytes[start+2] == 'b' && l.input.RawBytes[start+3] == 's' && l.input.RawBytes[start+4] == 'c' && l.input.RawBytes[start+5] == 'r' && l.input.RawBytes[start+6] == 'i' && l.input.RawBytes[start+7] == 'p' && l.input.RawBytes[start+8] == 't' && l.input.RawBytes[start+9] == 'i' && l.input.RawBytes[start+10] == 'o' && l.input.RawBytes[start+11] == 'n' {
			return keyword.SUBSCRIPTION
		}
	}

	return keyword.IDENT
}

func (l *Lexer) readDotOrSpread(tok *token.Token) {

	isSpread := l.peekEquals(false, runes.DOT, runes.DOT)

	if isSpread {
		l.swallowAmount(2)
		tok.Keyword = keyword.SPREAD
	} else {
		tok.Keyword = keyword.DOT
	}

	tok.SetEnd(l.input.InputPosition, l.input.TextPosition)
}

func (l *Lexer) readComment(tok *token.Token) {

	tok.Keyword = keyword.COMMENT

	for {
		next := l.readRune()
		switch next {
		case runes.EOF:
			return
		case runes.LINETERMINATOR:
			if l.peekRune(true) != runes.HASHTAG {
				return
			}
		default:
			tok.SetEnd(l.input.InputPosition, l.input.TextPosition)
		}
	}
}

func (l *Lexer) readString(tok *token.Token) {

	if l.peekEquals(false, runes.QUOTE, runes.QUOTE) {
		l.swallowAmount(2)
		l.readBlockString(tok)
		return
	}

	l.readSingleLineString(tok)
}

func (l *Lexer) swallowAmount(amount int) {
	for i := 0; i < amount; i++ {
		l.readRune()
	}
}

func (l *Lexer) peekEquals(ignoreWhitespace bool, equals ...byte) bool {

	var whitespaceOffset int
	if ignoreWhitespace {
		whitespaceOffset = l.peekWhitespaceLength()
	}

	start := l.input.InputPosition + whitespaceOffset
	end := l.input.InputPosition + len(equals) + whitespaceOffset

	if end > l.input.Length {
		return false
	}

	for i := 0; i < len(equals); i++ {
		if l.input.RawBytes[start+i] != equals[i] {
			return false
		}
	}

	return true
}

func (l *Lexer) peekWhitespaceLength() (amount int) {
	for i := l.input.InputPosition; i < l.input.Length; i++ {
		if l.byteIsWhitespace(l.input.RawBytes[i]) {
			amount++
		} else {
			break
		}
	}

	return amount
}

func (l *Lexer) readDigit(tok *token.Token) {

	var r byte
	for {
		r = l.peekRune(false)
		if !runeIsDigit(r) {
			break
		}
		l.readRune()
	}

	isFloat := r == runes.DOT

	if isFloat {
		l.swallowAmount(1)
		l.readFloat(tok)
		return
	}

	tok.Keyword = keyword.INTEGER
	tok.SetEnd(l.input.InputPosition, l.input.TextPosition)
}

func (l *Lexer) readFloat(tok *token.Token) {

	var r byte
	for {
		r = l.peekRune(false)
		if !runeIsDigit(r) {
			break
		}
		l.readRune()
	}

	tok.Keyword = keyword.FLOAT
	tok.SetEnd(l.input.InputPosition, l.input.TextPosition)
}

func (l *Lexer) readRune() (r byte) {

	if l.input.InputPosition < l.input.Length {
		r = l.input.RawBytes[l.input.InputPosition]

		if r == runes.LINETERMINATOR {
			l.input.TextPosition.LineStart++
			l.input.TextPosition.CharStart = 1
		} else {
			l.input.TextPosition.CharStart++
		}

		l.input.InputPosition++
	} else {
		r = runes.EOF
	}

	return
}

func (l *Lexer) peekRune(ignoreWhitespace bool) (r byte) {

	for i := l.input.InputPosition; i < l.input.Length; i++ {
		r = l.input.RawBytes[i]
		if !ignoreWhitespace {
			return r
		} else if !l.byteIsWhitespace(r) {
			return r
		}
	}

	return runes.EOF
}

func runeIsIdent(r byte) bool {

	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == runes.SUB:
		return true
	case r == runes.UNDERSCORE:
		return true
	default:
		return false
	}
}

func runeIsDigit(r byte) bool {
	switch {
	case r >= '0' && r <= '9':
		return true
	default:
		return false
	}
}

func (l *Lexer) byteIsWhitespace(r byte) bool {
	switch r {
	case runes.SPACE, runes.TAB, runes.LINETERMINATOR, runes.COMMA:
		return true
	default:
		return false
	}
}

func (l *Lexer) byteTerminatesSequence(r byte) bool {
	switch r {
	case runes.SPACE,
		runes.TAB,
		runes.LINETERMINATOR,
		runes.COMMA,
		runes.LPAREN,
		runes.RPAREN,
		runes.LBRACE,
		runes.RBRACE,
		runes.LBRACK,
		runes.RBRACK,
		runes.AND,
		runes.AT,
		runes.BANG,
		runes.COLON,
		runes.DOLLAR,
		runes.EQUALS,
		runes.HASHTAG,
		runes.SUB,
		runes.PIPE,
		runes.QUOTE,
		runes.SLASH:
		return true
	default:
		return false
	}
}

func (l *Lexer) readBlockString(tok *token.Token) {

	tok.Keyword = keyword.BLOCKSTRING
	tok.SetStart(l.input.InputPosition, l.input.TextPosition)

	var escaped bool

	for {

		nextRune := l.peekRune(false)

		switch nextRune {
		case runes.QUOTE, runes.EOF:
			if escaped {
				escaped = false
				l.readRune()
			} else {

				isMultiLineStringEnd := l.peekEquals(false, runes.QUOTE, runes.QUOTE, runes.QUOTE)

				if !isMultiLineStringEnd {
					escaped = false
					l.readRune()
				} else {
					tok.SetEnd(l.input.InputPosition, l.input.TextPosition)
					tok.TextPosition.CharStart -= 3
					tok.TextPosition.CharEnd += 3
					l.swallowAmount(3)
					return
				}
			}
		case runes.BACKSLASH:
			l.readRune()
			if escaped {
				escaped = false
			} else {
				escaped = true
			}
		default:
			l.readRune()
			escaped = false
		}
	}
}

func (l *Lexer) readSingleLineString(tok *token.Token) {

	tok.Keyword = keyword.STRING
	tok.SetStart(l.input.InputPosition, l.input.TextPosition)

	var escaped bool

	for {

		nextRune := l.peekRune(false)

		switch nextRune {
		case runes.QUOTE, runes.EOF:
			if escaped {
				escaped = false
				l.readRune()
			} else {
				tok.SetEnd(l.input.InputPosition, l.input.TextPosition)
				tok.TextPosition.CharStart -= 1
				tok.TextPosition.CharEnd += 1
				l.swallowAmount(1)
				return
			}
		case runes.BACKSLASH:
			l.readRune()
			if escaped {
				escaped = false
			} else {
				escaped = true
			}
		default:
			l.readRune()
			escaped = false
		}
	}
}
