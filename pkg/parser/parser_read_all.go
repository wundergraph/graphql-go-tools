package parser

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/token"

// ForeachToken is a callback func that gets invoked for each emitted token
type ForeachToken func(tok token.Token) bool

// ForeachTokens is a callback func that gets invoked for each emitted set of tokens
type ForeachTokens func(tokens []token.Token) error

// MatchPattern is a slice of tokens for pattern matching
type MatchPattern struct {
	match []token.Keyword
	input []token.Token
}

func (m *MatchPattern) clearInput() {
	m.input = nil
}

func (m *MatchPattern) canAcceptMore() bool {
	return len(m.input) < len(m.match)
}

func (m *MatchPattern) feedInput(tok token.Token) {
	m.input = append(m.input, tok)
}

func (m *MatchPattern) matchLast() bool {
	return m.match[len(m.input)-1] == m.input[len(m.input)-1].Keyword
}

func (m *MatchPattern) isSet() bool {
	return len(m.match) > 0
}

// Pattern returns a new MatchPattern
func Pattern(tok ...token.Keyword) MatchPattern {
	return MatchPattern{
		match: tok,
	}
}

type readAllOperation struct {
	p       *Parser
	until   token.Keyword
	options []ReadOption
}

func (p *Parser) makeReadAllOptions(until token.Keyword, options ...ReadOption) readOptions {
	preparedOptions := p.makeReadOptions(options...)
	if len(preparedOptions.whitelist) > 0 {
		// if the whitelist is empty we don't want to whitelist just one keyword
		// because this would essentially blacklist all other keywords
		preparedOptions.whitelist[until] = struct{}{}
	}

	return preparedOptions
}

func (r *readAllOperation) foreach(callback ForeachToken) error {

	options := r.p.makeReadAllOptions(r.until, r.options...)

	for {
		tok, err := r.p.readWithOptions(options)
		if err != nil {
			return err
		}

		if tok.Keyword == r.until {
			return nil
		}

		// as we're using the same bytes.Buffer for all operations
		// we need to copy all byte slices to a new byte slice
		cop := make([]byte, len(tok.Literal))
		copy(cop, tok.Literal)
		tok.Literal = cop

		if !callback(tok) {
			return nil
		}
	}
}

func (r *readAllOperation) foreachMatchedPattern(pattern MatchPattern, match ForeachTokens) (matched int, err error) {

	if !pattern.isSet() {
		return
	}

	options := r.p.makeReadAllOptions(r.until, r.options...)

	// read as long as we accept input
	for pattern.canAcceptMore() {

		tok, err := r.p.readWithOptions(options)
		if err != nil {
			return matched, err
		}

		if tok.Keyword == r.until {
			return matched, err
		}

		// as we're using the same bytes.Buffer for all operations
		// we need to copy all byte slices to a new byte slice
		cop := make([]byte, len(tok.Literal))
		copy(cop, tok.Literal)
		tok.Literal = cop

		// feed input
		pattern.feedInput(tok)

		// always match the last input item to instantly return if the pattern doesn't match
		if !pattern.matchLast() {
			return matched, err
		}

		if options.readRepeat {
			_, err := r.p.read()
			if err != nil {
				return matched, err
			}
		}

		if !pattern.canAcceptMore() {
			err = match(pattern.input)
			if err != nil {
				return matched, err
			}
			pattern.clearInput()
			matched++
		}
	}

	return
}

func (p *Parser) readAllUntil(until token.Keyword, option ...ReadOption) *readAllOperation {
	return &readAllOperation{
		p:       p,
		until:   until,
		options: option,
	}
}
