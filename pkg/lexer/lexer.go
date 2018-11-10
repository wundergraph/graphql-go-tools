package lexer

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/runes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	"github.com/jensneuse/graphql-go-tools/pkg/rules"
	"github.com/jensneuse/graphql-go-tools/pkg/runestringer"
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
	"io"
)

// Lexer is the struct to coordinate the lexing process
type Lexer struct {
	reader                 *bufio.Reader
	current                token.Token
	runeStringer           runestringer.RuneStringer
	lexedRunes             []lexedRune
	readFromLexed          int
	readRepeatCurrentToken bool
}

type lexedRune struct {
	rune     rune
	position token.Position
}

// NewLexer returns a new *Lexer, a runestringer must be supplied
func NewLexer(stringer runestringer.RuneStringer) *Lexer {

	lexer := Lexer{
		runeStringer: stringer,
	}

	return &lexer
}

// SetInput (re-)sets the lexer's bufio.Reader
func (l *Lexer) SetInput(reader io.Reader) {
	if l.reader == nil {
		l.reader = bufio.NewReader(reader)
		return
	}

	l.reset(reader)
}

// ReadRepeatCurrentToken makes the lexer re read the emitted token
func (l *Lexer) ReadRepeatCurrentToken() {
	l.readRepeatCurrentToken = true
}

func (l *Lexer) reset(reader io.Reader) {
	l.reader.Reset(reader)
	l.lexedRunes = []lexedRune{}
	l.readFromLexed = 0
}

func (l *Lexer) unread() error {
	l.readFromLexed++

	if l.readFromLexed > len(l.lexedRunes) {
		return fmt.Errorf("unread: Unread too many times / out of bounds")
	}

	return nil
}

// Read emits the next token
func (l *Lexer) Read() (token.Token, error) {

	if l.readRepeatCurrentToken {
		l.readRepeatCurrentToken = false
		return l.current, nil
	}

	l.current.Literal = nil

	r := l.readRune()
	pos := r.position
	l.current.Position = pos

	switch r.rune {
	case runes.EOF:
		l.current.Keyword = token.EOF
		l.current.Literal = literal.EOF
		return l.current, nil
	case runes.COMMA, runes.SPACE, runes.TAB, runes.LINETERMINATOR:
		return l.Read()
	case runes.DOT:
		isSpread, err := l.scanSpread()
		if isSpread {
			l.current.Keyword = token.SPREAD
			l.current.Literal = literal.SPREAD
		} else {
			l.current.Keyword = token.DOT
			l.current.Literal = literal.DOT
		}
		return l.current, err
	case runes.PIPE:
		l.current.Keyword = token.PIPE
		l.current.Literal = literal.PIPE
		return l.current, nil
	case runes.EQUALS:
		l.current.Keyword = token.EQUALS
		l.current.Literal = literal.EQUALS
		return l.current, nil
	case runes.QUOTE:
		var err error
		l.current.Keyword = token.STRING
		l.current.Literal, err = l.scanString()
		return l.current, err
	case runes.AT:
		l.current.Keyword = token.AT
		l.current.Literal = literal.AT
		return l.current, nil
	case runes.COLON:
		l.current.Keyword = token.COLON
		l.current.Literal = literal.COLON
		return l.current, nil
	case runes.BANG:
		l.current.Keyword = token.BANG
		l.current.Literal = literal.BANG
		return l.current, nil
	case runes.HASHTAG:
		l.current.Keyword = token.COMMENT
		l.current.Literal = l.scanComment()
		return l.current, nil
	case runes.BRACKETOPEN:
		l.current.Keyword = token.BRACKETOPEN
		l.current.Literal = literal.BRACKETOPEN
		return l.current, nil
	case runes.BRACKETCLOSE:
		l.current.Keyword = token.BRACKETCLOSE
		l.current.Literal = literal.BRACKETCLOSE
		return l.current, nil
	case runes.CURLYBRACKETOPEN:
		l.current.Keyword = token.CURLYBRACKETOPEN
		l.current.Literal = literal.CURLYBRACKETOPEN
		return l.current, nil
	case runes.CURLYBRACKETCLOSE:
		l.current.Keyword = token.CURLYBRACKETCLOSE
		l.current.Literal = literal.CURLYBRACKETCLOSE
		return l.current, nil
	case runes.SQUAREBRACKETOPEN:
		l.current.Keyword = token.SQUAREBRACKETOPEN
		l.current.Literal = literal.SQUAREBRACKETOPEN
		return l.current, nil
	case runes.SQUAREBRACKETCLOSE:
		l.current.Keyword = token.SQUAREBRACKETCLOSE
		l.current.Literal = literal.SQUAREBRACKETCLOSE
		return l.current, nil
	case runes.AND:
		l.current.Keyword = token.AND
		l.current.Literal = literal.AND
		return l.current, nil
	case runes.DOLLAR:
		var err error
		l.current.Keyword = token.VARIABLE
		l.current.Literal, err = l.scanVariable()
		return l.current, err
	}

	if rules.IsDigit(r.rune) || r.rune == runes.NEGATIVESIGN {
		var err error
		l.current.Keyword, l.current.Literal, err = l.scanNumber(r.rune)
		return l.current, err
	}

	var err error
	l.current.Keyword, l.current.Literal, err = l.scanLiteral(r.rune)

	return l.current, err
}

func (l *Lexer) scanVariable() (lit []byte, err error) {

	first := l.readRune()
	if rules.IsLiteral(first.rune) {
		l.runeStringer.Write(first.rune)
	} else {
		err = fmt.Errorf("scanVariable: unexpected rune '%s' @ %s (wanted literal)", string(first.rune), first.position)
	}

	for {
		next := l.readRune()
		if rules.IsLiteral(next.rune) {
			l.runeStringer.Write(next.rune)
		} else {
			l.readFromLexed = 1
			lit = l.runeStringer.Bytes()
			return
		}
	}

}

func (l *Lexer) scanComment() []byte {

	for {
		run := l.readRune()
		if run.rune == runes.LINETERMINATOR || run.rune == runes.EOF {
			return transform.TrimWhitespace(l.runeStringer.Bytes())
		}

		l.runeStringer.Write(run.rune)
	}
}

func (l *Lexer) scanString() (lit []byte, err error) {

	isBlockString, err := l.PeekMatchRunes(runes.QUOTE, 2)
	if err != nil {
		return lit, err
	}

	if isBlockString {
		lit, err = l.scanBlockString()
	} else {
		lit, err = l.scanSingleLineString()
	}

	return
}

func (l *Lexer) scanSingleLineString() (lit []byte, err error) {

	var escaped bool

	for {
		run := l.readRune()

		switch run.rune {
		case runes.LINETERMINATOR, runes.EOF:
			err = fmt.Errorf("scanSingleLineString: unexpected Lineterminator/EOF @ %s", run.position)
			return lit, err
		case runes.QUOTE:

			if escaped {
				l.runeStringer.Write(run.rune)
				escaped = false
				continue
			}

			lit = transform.TrimWhitespace(l.runeStringer.Bytes())
			return lit, err
		case runes.BACKSLASH:
			escaped = true
		default:
			l.runeStringer.Write(run.rune)
		}
	}

}

func (l *Lexer) scanBlockString() (lit []byte, err error) {

	err = l.SwallowRunes(2)
	if err != nil {
		return
	}

	for {
		run := l.readRune()

		switch run.rune {
		case runes.EOF:
			err = fmt.Errorf("scanBlockString: unexpected EOF @ %s", run.position)
			return
		case runes.QUOTE:
			done, err := l.PeekMatchRunes(runes.QUOTE, 2)
			if err != nil {
				return lit, err
			}

			if done {
				l.SwallowRunes(2)
				lit = transform.TrimWhitespace(l.runeStringer.Bytes())
				return lit, err
			}
		case runes.BACKSLASH:
			continue
		}

		l.runeStringer.Write(run.rune)
	}

}

func (l *Lexer) scanNumber(beginWith rune) (keyword token.Keyword, lit []byte, err error) {

	l.runeStringer.Write(beginWith)

	isFloat := false

	for {
		r := l.readRune()

		if rules.IsDigit(r.rune) {
			l.runeStringer.Write(r.rune)
		} else if r.rune == runes.DOT {
			if !isFloat {
				isFloat = true
				l.runeStringer.Write(r.rune)
			} else {
				err = fmt.Errorf("scanNumber: unexpected . (DOT)")
				return
			}

		} else {
			err = l.unread()
			lit = l.runeStringer.Bytes()

			if isFloat {
				keyword = token.FLOAT
			} else {
				keyword = token.INTEGER
			}

			return
		}
	}

}

func (l *Lexer) scanLiteral(beginWith rune) (key token.Keyword, lit []byte, err error) {

	l.runeStringer.Write(beginWith)

	for {
		r := l.readRune()

		if !rules.IsLiteral(r.rune) {
			ll := string(r.rune)
			_ = ll

			err = l.unread()
			if err != nil {
				return key, lit, err
			}

			lit = l.runeStringer.Bytes()
			key = getLiteralKeyword(lit)

			return
		}

		l.runeStringer.Write(r.rune)
	}
}

func (l *Lexer) scanSpread() (isSpread bool, err error) {

	run := l.readRune()
	if run.rune != runes.DOT {
		return false, nil
	}

	run = l.readRune()
	if run.rune != runes.DOT {
		return false, fmt.Errorf("scanSpread: unexpected amount of DOTs: %s @ %s (wanted 1 or 3)", string(2), run.position)
	}

	return true, nil
}

func getLiteralKeyword(lit []byte) token.Keyword {

	if bytes.Equal(lit, literal.TRUE) {
		return token.TRUE
	} else if bytes.Equal(lit, literal.FALSE) {
		return token.FALSE
	} else if bytes.Equal(lit, literal.NULL) {
		return token.NULL
	}

	return token.IDENT
}

func (l *Lexer) readRune() (lexed lexedRune) {

	if l.readFromLexed != 0 {
		lexed = l.lexedRunes[len(l.lexedRunes)-l.readFromLexed]
		l.readFromLexed--
		return
	}

	run, _, err := l.reader.ReadRune()
	if err != nil {
		lexed.rune = runes.EOF
	}

	if len(l.lexedRunes) > 0 {
		last := l.lexedRunes[len(l.lexedRunes)-1]
		if last.rune == runes.LINETERMINATOR {
			lexed.position.Line = last.position.Line + 1
			lexed.position.Char = 1
		} else {
			lexed.position.Char = last.position.Char + 1
			lexed.position.Line = last.position.Line
		}
	} else {
		lexed.position.Line = 1
		lexed.position.Char = 1
	}

	lexed.rune = run

	l.lexedRunes = append(l.lexedRunes, lexed)

	return
}

// PeekRunes emits the desired amount of runes and unreads them
func (l *Lexer) PeekRunes(amount int) (runes []rune, err error) {
	for i := 0; i < amount; i++ {
		next := l.readRune()
		runes = append(runes, next.rune)
	}

	for k := 0; k < amount; k++ {
		err = l.unread()
		if err != nil {
			return
		}
	}

	return
}

// PeekMatchRunes returns true if the desired amount of runes peeked match a specified rune
func (l *Lexer) PeekMatchRunes(match rune, amount int) (matches bool, err error) {

	peeked, err := l.PeekRunes(amount)
	if err != nil {
		return false, err
	}

	for _, r := range peeked {
		if match != r {
			return false, err
		}
	}

	return true, nil
}

// SwallowRunes swallows the desired amount of runes
func (l *Lexer) SwallowRunes(amount int) (err error) {

	for i := 0; i < amount; i++ {
		l.readRune()
	}

	return
}
