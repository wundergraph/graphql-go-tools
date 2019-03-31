package lexer

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/runes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

// Lexer emits tokens from a input reader
type Lexer struct {
	_storage                             [maxInput]byte
	input                                []byte
	inputPosition                        int
	typeSystemEndPosition                int
	textPosition                         position.Position
	beforeLastLineTerminatorTextPosition position.Position
}

// NewLexer initializes a new lexer
func NewLexer() *Lexer {
	return &Lexer{}
}

const (
	//maxInput = 655350
	maxInput = 1000000
)

// SetTypeSystemInput sets the new reader as input and resets all position stats
func (l *Lexer) SetTypeSystemInput(input []byte) error {

	if len(input) > maxInput {
		return fmt.Errorf("SetTypeSystemInput: input size must not be > %d, got: %d", maxInput, len(input))
	}

	l.input = l._storage[:0]
	l.input = append(l.input, input...)

	l.inputPosition = 0
	l.textPosition.LineStart = 1
	l.textPosition.CharStart = 1
	l.typeSystemEndPosition = len(input)

	return nil
}

func (l *Lexer) ResetTypeSystemInput() {
	l.input = l._storage[:0]
	l.inputPosition = 0
	l.textPosition.LineStart = 1
	l.textPosition.CharStart = 1
	l.typeSystemEndPosition = 0
}

func (l *Lexer) AppendBytes(input []byte) (err error) {
	currentLength := len(l.input)
	inputLength := len(input)
	totalLength := currentLength + inputLength
	if totalLength > maxInput {
		return fmt.Errorf("AppendBytes: input size must not be > %d, got: %d", maxInput, totalLength)
	}

	l.input = append(l.input, input...)
	return
}

func (l *Lexer) SetExecutableInput(input []byte) error {

	l.input = append(l.input[:l.typeSystemEndPosition], input...)

	if len(input) > maxInput {
		return fmt.Errorf("SetTypeSystemInput: input size must not be > %d, got: %d", maxInput, len(input))
	}

	l.inputPosition = l.typeSystemEndPosition
	l.textPosition.LineStart = 1
	l.textPosition.CharStart = 1

	return nil
}

func (l *Lexer) ByteSlice(reference document.ByteSliceReference) document.ByteSlice {
	return l.input[reference.Start:reference.End]
}

func (l *Lexer) TextPosition() position.Position {
	return l.textPosition
}

// Read emits the next token, this cannot be undone
func (l *Lexer) Read() (tok token.Token) {

	var next byte
	var inputPositionStart int

	for {
		inputPositionStart = l.inputPosition
		tok.SetStart(l.inputPosition, l.textPosition)
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
	case runes.DOLLAR:
		l.readVariable(&tok)
		return
	}

	if runeIsDigit(next) {
		l.readDigit(&tok)
		return
	}

	l.readIdent()
	tok.Keyword = l.keywordFromIdent(inputPositionStart, l.inputPosition)
	tok.SetEnd(l.inputPosition, l.textPosition)
	return
}

// Peek will emit the next keyword without advancing the reader position
func (l *Lexer) Peek(ignoreWhitespace bool) keyword.Keyword {
	next := l.peekRune(ignoreWhitespace)
	return l.keywordFromRune(next)
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
		return keyword.VARIABLE
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
	case runes.BRACKETOPEN:
		return keyword.BRACKETOPEN
	case runes.BRACKETCLOSE:
		return keyword.BRACKETCLOSE
	case runes.CURLYBRACKETOPEN:
		return keyword.CURLYBRACKETOPEN
	case runes.CURLYBRACKETCLOSE:
		return keyword.CURLYBRACKETCLOSE
	case runes.SQUAREBRACKETOPEN:
		return keyword.SQUAREBRACKETOPEN
	case runes.SQUAREBRACKETCLOSE:
		return keyword.SQUAREBRACKETCLOSE
	case runes.AND:
		return keyword.AND
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

	start := l.inputPosition + l.peekWhitespaceLength()

	for i := start; i < len(l.input); i++ {

		peeked = l.input[i]

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
	case runes.BRACKETOPEN:
		tok.Keyword = keyword.BRACKETOPEN
	case runes.BRACKETCLOSE:
		tok.Keyword = keyword.BRACKETCLOSE
	case runes.CURLYBRACKETOPEN:
		tok.Keyword = keyword.CURLYBRACKETOPEN
	case runes.CURLYBRACKETCLOSE:
		tok.Keyword = keyword.CURLYBRACKETCLOSE
	case runes.SQUAREBRACKETOPEN:
		tok.Keyword = keyword.SQUAREBRACKETOPEN
	case runes.SQUAREBRACKETCLOSE:
		tok.Keyword = keyword.SQUAREBRACKETCLOSE
	case runes.AND:
		tok.Keyword = keyword.AND
	default:
		return false
	}

	tok.SetEnd(l.inputPosition, l.textPosition)

	return true
}

func (l *Lexer) readIdent() {

	var r byte

	for {
		r = l.readRune()
		if !runeIsIdent(r) {
			if r != runes.EOF {
				l.unreadRune()
			}
			return
		}
	}
}

const identWantRunes = 13

func (l *Lexer) peekIdent() (k keyword.Keyword) {

	whitespaceOffset := l.peekWhitespaceLength()

	start := l.inputPosition + whitespaceOffset
	end := start + identWantRunes
	if end > len(l.input) {
		end = len(l.input)
	}

	for i := start; i < end; {
		if !runeIsIdent(l.input[i]) {
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
		if l.input[start] == 'o' && l.input[start+1] == 'n' {
			return keyword.ON
		}
	case 4:
		if l.input[start] == 'n' && l.input[start+1] == 'u' && l.input[start+2] == 'l' && l.input[start+3] == 'l' {
			return keyword.NULL
		}
		if l.input[start] == 'e' && l.input[start+1] == 'n' && l.input[start+2] == 'u' && l.input[start+3] == 'm' {
			return keyword.ENUM
		}
		if l.input[start] == 't' {
			if l.input[start+1] == 'r' && l.input[start+2] == 'u' && l.input[start+3] == 'e' {
				return keyword.TRUE
			}
			if l.input[start+1] == 'y' && l.input[start+2] == 'p' && l.input[start+3] == 'e' {
				return keyword.TYPE
			}
		}
	case 5:
		if l.input[start] == 'f' && l.input[start+1] == 'a' && l.input[start+2] == 'l' && l.input[start+3] == 's' && l.input[start+4] == 'e' {
			return keyword.FALSE
		}
		if l.input[start] == 'u' && l.input[start+1] == 'n' && l.input[start+2] == 'i' && l.input[start+3] == 'o' && l.input[start+4] == 'n' {
			return keyword.UNION
		}
		if l.input[start] == 'q' && l.input[start+1] == 'u' && l.input[start+2] == 'e' && l.input[start+3] == 'r' && l.input[start+4] == 'y' {
			return keyword.QUERY
		}
		if l.input[start] == 'i' && l.input[start+1] == 'n' && l.input[start+2] == 'p' && l.input[start+3] == 'u' && l.input[start+4] == 't' {
			return keyword.INPUT
		}
	case 6:
		if l.input[start] == 's' {
			if l.input[start+1] == 'c' && l.input[start+2] == 'h' && l.input[start+3] == 'e' && l.input[start+4] == 'm' && l.input[start+5] == 'a' {
				return keyword.SCHEMA
			}
			if l.input[start+1] == 'c' && l.input[start+2] == 'a' && l.input[start+3] == 'l' && l.input[start+4] == 'a' && l.input[start+5] == 'r' {
				return keyword.SCALAR
			}
		}
	case 8:
		if l.input[start] == 'm' && l.input[start+1] == 'u' && l.input[start+2] == 't' && l.input[start+3] == 'a' && l.input[start+4] == 't' && l.input[start+5] == 'i' && l.input[start+6] == 'o' && l.input[start+7] == 'n' {
			return keyword.MUTATION
		}
		if l.input[start] == 'f' && l.input[start+1] == 'r' && l.input[start+2] == 'a' && l.input[start+3] == 'g' && l.input[start+4] == 'm' && l.input[start+5] == 'e' && l.input[start+6] == 'n' && l.input[start+7] == 't' {
			return keyword.FRAGMENT
		}
	case 9:
		if l.input[start] == 'i' && l.input[start+1] == 'n' && l.input[start+2] == 't' && l.input[start+3] == 'e' && l.input[start+4] == 'r' && l.input[start+5] == 'f' && l.input[start+6] == 'a' && l.input[start+7] == 'c' && l.input[start+8] == 'e' {
			return keyword.INTERFACE
		}
		if l.input[start] == 'd' && l.input[start+1] == 'i' && l.input[start+2] == 'r' && l.input[start+3] == 'e' && l.input[start+4] == 'c' && l.input[start+5] == 't' && l.input[start+6] == 'i' && l.input[start+7] == 'v' && l.input[start+8] == 'e' {
			return keyword.DIRECTIVE
		}
	case 10:
		if l.input[start] == 'i' && l.input[start+1] == 'm' && l.input[start+2] == 'p' && l.input[start+3] == 'l' && l.input[start+4] == 'e' && l.input[start+5] == 'm' && l.input[start+6] == 'e' && l.input[start+7] == 'n' && l.input[start+8] == 't' && l.input[start+9] == 's' {
			return keyword.IMPLEMENTS
		}
	case 12:
		if l.input[start] == 's' && l.input[start+1] == 'u' && l.input[start+2] == 'b' && l.input[start+3] == 's' && l.input[start+4] == 'c' && l.input[start+5] == 'r' && l.input[start+6] == 'i' && l.input[start+7] == 'p' && l.input[start+8] == 't' && l.input[start+9] == 'i' && l.input[start+10] == 'o' && l.input[start+11] == 'n' {
			return keyword.SUBSCRIPTION
		}
	}

	return keyword.IDENT
}

func (l *Lexer) readVariable(tok *token.Token) {

	tok.SetStart(l.inputPosition, l.textPosition)

	tok.Keyword = keyword.VARIABLE

	l.readIdent()

	tok.SetEnd(l.inputPosition, l.textPosition)
	tok.TextPosition.CharStart -= 1
}

func (l *Lexer) readDotOrSpread(tok *token.Token) {

	isSpread := l.peekEquals(false, runes.DOT, runes.DOT)

	if isSpread {
		l.swallowAmount(2)
		tok.Keyword = keyword.SPREAD
	} else {
		tok.Keyword = keyword.DOT
	}

	tok.SetEnd(l.inputPosition, l.textPosition)
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
			tok.SetEnd(l.inputPosition, l.textPosition)
		}
	}
}

func (l *Lexer) readString(tok *token.Token) {

	tok.Keyword = keyword.STRING

	isMultiLineString := l.peekEquals(false, runes.QUOTE, runes.QUOTE)

	if isMultiLineString {
		l.swallowAmount(2)
		l.readMultiLineString(tok)
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

	start := l.inputPosition + whitespaceOffset
	end := l.inputPosition + len(equals) + whitespaceOffset

	if end > len(l.input) {
		return false
	}

	for i := 0; i < len(equals); i++ {
		if l.input[start+i] != equals[i] {
			return false
		}
	}

	return true
}

func (l *Lexer) peekWhitespaceLength() (amount int) {
	for i := l.inputPosition; i < len(l.input); i++ {
		if l.byteIsWhitespace(l.input[i]) {
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
		r = l.readRune()
		if !runeIsDigit(r) {
			break
		}
	}

	isFloat := r == runes.DOT

	if isFloat {
		l.swallowAmount(1)
		l.readFloat(tok)
		return
	}

	if r != runes.EOF {
		l.unreadRune()
	}

	tok.Keyword = keyword.INTEGER
	tok.SetEnd(l.inputPosition, l.textPosition)
}

func (l *Lexer) readFloat(tok *token.Token) {

	var r byte
	for {
		r = l.readRune()
		if !runeIsDigit(r) {
			break
		}
	}

	if r != runes.EOF {
		l.unreadRune()
	}

	tok.Keyword = keyword.FLOAT
	tok.SetEnd(l.inputPosition, l.textPosition)
}

func (l *Lexer) readRune() (r byte) {

	if l.inputPosition < len(l.input) {
		r = l.input[l.inputPosition]

		if r == runes.LINETERMINATOR {
			l.beforeLastLineTerminatorTextPosition = l.textPosition
			l.textPosition.LineStart++
			l.textPosition.CharStart = 1
		} else {
			l.textPosition.CharStart++
		}

		l.inputPosition++
	} else {
		r = runes.EOF
	}

	return
}

func (l *Lexer) unreadRune() {

	l.inputPosition--

	r := rune(l.input[l.inputPosition])
	if r == runes.LINETERMINATOR {
		l.textPosition = l.beforeLastLineTerminatorTextPosition
	} else {
		l.textPosition.CharStart--
	}
}

func (l *Lexer) peekRune(ignoreWhitespace bool) (r byte) {

	for i := l.inputPosition; i < len(l.input); i++ {
		r = l.input[i]
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
	case r == runes.NEGATIVESIGN:
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
		runes.BRACKETOPEN,
		runes.BRACKETCLOSE,
		runes.CURLYBRACKETOPEN,
		runes.CURLYBRACKETCLOSE,
		runes.SQUAREBRACKETOPEN,
		runes.SQUAREBRACKETCLOSE,
		runes.AND,
		runes.AT,
		runes.BANG,
		runes.COLON,
		runes.DOLLAR,
		runes.EQUALS,
		runes.HASHTAG,
		runes.NEGATIVESIGN,
		runes.PIPE,
		runes.QUOTE,
		runes.SLASH:
		return true
	default:
		return false
	}
}

func (l *Lexer) readMultiLineString(tok *token.Token) {

	tok.SetStart(l.inputPosition, l.textPosition)

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
					tok.SetEnd(l.inputPosition, l.textPosition)
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

	tok.SetStart(l.inputPosition, l.textPosition)

	var escaped bool

	for {

		nextRune := l.peekRune(false)

		switch nextRune {
		case runes.QUOTE, runes.EOF:
			if escaped {
				escaped = false
				l.readRune()
			} else {
				tok.SetEnd(l.inputPosition, l.textPosition)
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
