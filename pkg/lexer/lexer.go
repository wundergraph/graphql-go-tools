package lexer

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/runes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/token"
)

// Lexer emits tokens from a input reader
type Lexer struct {
	input *ast.Input
}

func (l *Lexer) SetInput(input *ast.Input) {
	l.input = input
}

// Read emits the next token
func (l *Lexer) Read() (tok token.Token) {

	var next byte

	for {
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
	tok.Keyword = keyword.IDENT
	tok.SetEnd(l.input.InputPosition, l.input.TextPosition)
	return
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
	for {
		if l.input.InputPosition < l.input.Length {
			if !l.runeIsIdent(l.input.RawBytes[l.input.InputPosition]) {
				return
			}
			l.input.TextPosition.CharStart++
			l.input.InputPosition++
		} else {
			return
		}
	}
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
	} else {
		l.readSingleLineString(tok)
	}
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

func (l *Lexer) runeIsIdent(r byte) bool {

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
	tok.TextPosition.CharStart -= 3

	escaped := false
	quoteCount := 0
	whitespaceCount := 0
	reachedFirstNonWhitespace := false
	leadingWhitespaceToken := 0

	for {
		next := l.readRune()
		switch next {
		case runes.SPACE, runes.TAB, runes.LINETERMINATOR:
			quoteCount = 0
			whitespaceCount++
		case runes.EOF:
			return
		case runes.QUOTE:
			if escaped {
				escaped = !escaped
				continue
			}

			quoteCount++

			if quoteCount == 3 {
				tok.SetEnd(l.input.InputPosition-3, l.input.TextPosition)
				tok.Literal.Start += uint32(leadingWhitespaceToken)
				tok.Literal.End -= uint32(whitespaceCount)
				return
			}

		case runes.BACKSLASH:
			escaped = !escaped
			quoteCount = 0
			whitespaceCount = 0
		default:
			if !reachedFirstNonWhitespace {
				reachedFirstNonWhitespace = true
				leadingWhitespaceToken = whitespaceCount
			}
			escaped = false
			quoteCount = 0
			whitespaceCount = 0
		}
	}
}

func (l *Lexer) readSingleLineString(tok *token.Token) {

	tok.Keyword = keyword.STRING

	tok.SetStart(l.input.InputPosition, l.input.TextPosition)
	tok.TextPosition.CharStart -= 1

	escaped := false
	whitespaceCount := 0
	reachedFirstNonWhitespace := false
	leadingWhitespaceToken := 0

	for {
		next := l.readRune()
		switch next {
		case runes.SPACE, runes.TAB:
			whitespaceCount++
		case runes.EOF:
			tok.SetEnd(l.input.InputPosition, l.input.TextPosition)
			tok.Literal.Start += uint32(leadingWhitespaceToken)
			tok.Literal.End -= uint32(whitespaceCount)
			return
		case runes.QUOTE, runes.LINETERMINATOR:
			if escaped {
				escaped = !escaped
				continue
			}

			tok.SetEnd(l.input.InputPosition-1, l.input.TextPosition)
			tok.Literal.Start += uint32(leadingWhitespaceToken)
			tok.Literal.End -= uint32(whitespaceCount)
			return

		case runes.BACKSLASH:
			escaped = !escaped
			whitespaceCount = 0
		default:
			if !reachedFirstNonWhitespace {
				reachedFirstNonWhitespace = true
				leadingWhitespaceToken = whitespaceCount
			}
			escaped = false
			whitespaceCount = 0
		}
	}
}
