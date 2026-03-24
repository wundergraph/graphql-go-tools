package astparser

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/identkeyword"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/keyword"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/token"
)

// Tokenizer takes a raw input and turns it into set of tokens
type Tokenizer struct {
	lexer        *lexer.Lexer
	tokens       []token.Token
	maxTokens    int
	currentToken int
	skipComments bool
}

type TokenizerStats struct {
	TotalDepth  int
	TotalFields int
}

// NewTokenizer returns a new tokenizer
func NewTokenizer() *Tokenizer {
	return &Tokenizer{
		tokens:       make([]token.Token, 0, 64),
		lexer:        &lexer.Lexer{},
		skipComments: true,
	}
}

func (t *Tokenizer) Tokenize(input *ast.Input) {
	t.lexer.SetInput(input)
	t.tokens = t.tokens[:0]

	for {
		next := t.lexer.Read()
		if next.Keyword == keyword.EOF {
			t.maxTokens = len(t.tokens)
			t.currentToken = -1
			return
		}
		t.tokens = append(t.tokens, next)
	}
}

type TokenizerLimits struct {
	MaxDepth  int
	MaxFields int
}

func (t *Tokenizer) TokenizeWithLimits(limits TokenizerLimits, input *ast.Input) (TokenizerStats, error) {
	t.lexer.SetInput(input)
	t.tokens = t.tokens[:0]

	limitDepth := limits.MaxDepth > 0
	// globalDepth tracks the cumulative depth across the entire document
	globalDepth := 0
	// localDepth tracks the current nesting depth within the current operation/fragment
	localDepth := 0
	// localDepthPeak tracks the maximum depth reached within the current operation/fragment
	localDepthPeak := 0

	limitFields := limits.MaxFields > 0
	fieldsCount := 0
	lastWasSpread := false // used to dismiss an identifier after a spread operator

	for {
		next := t.lexer.Read()
		switch next.Keyword {
		case keyword.EOF:
			t.maxTokens = len(t.tokens)
			t.currentToken = -1
			return TokenizerStats{TotalDepth: globalDepth + localDepthPeak, TotalFields: fieldsCount}, nil
		case keyword.LBRACE:
			globalDepth++
			if limitDepth && globalDepth > limits.MaxDepth {
				return TokenizerStats{TotalDepth: globalDepth + localDepthPeak, TotalFields: fieldsCount}, ErrDepthLimitExceeded{
					limit: limits.MaxDepth,
				}
			}
			localDepth++
			if localDepth > localDepthPeak {
				localDepthPeak = localDepth
			}
			lastWasSpread = false
		case keyword.RBRACE:
			globalDepth--
			localDepth--
			lastWasSpread = false
		case keyword.SPREAD:
			lastWasSpread = true
		case keyword.IDENT:
			key := identkeyword.KeywordFromLiteral(input.ByteSlice(next.Literal))
			switch key {
			case identkeyword.FRAGMENT, identkeyword.QUERY, identkeyword.MUTATION, identkeyword.SUBSCRIPTION:
				// When starting a new operation or fragment, add the local depth peak
				// to global depth and reset local tracking
				globalDepth += localDepthPeak
				localDepth = 0
				localDepthPeak = 0
			default:
				// localDepth > 0 means that we are inside a selection set, otherwise we're not counting fields
				// if lastWasSpread, it means that the next token is an identifier of a fragment spread, we dismiss it
				if localDepth > 0 && !lastWasSpread {
					fieldsCount++
				}
				if limitFields && fieldsCount > limits.MaxFields {
					return TokenizerStats{TotalDepth: globalDepth + localDepthPeak, TotalFields: fieldsCount}, ErrFieldsLimitExceeded{
						limit: limits.MaxFields,
					}
				}
			}
			lastWasSpread = false
		}
		t.tokens = append(t.tokens, next)
	}
}

// Read - increments currentToken index and return token if hasNextToken
// otherwise returns keyword.EOF
func (t *Tokenizer) Read() token.Token {
	tok := t.read()
	if t.skipComments && tok.Keyword == keyword.COMMENT {
		tok = t.read()
	}

	return tok
}

func (t *Tokenizer) read() token.Token {
	if t.currentToken+1 < t.maxTokens {
		t.currentToken++
		return t.tokens[t.currentToken]
	}

	return token.Token{
		Keyword: keyword.EOF,
	}
}

// Peek - returns token next to currentToken if hasNextToken
// otherwise returns keyword.EOF
func (t *Tokenizer) Peek() token.Token {
	tok := t.peek(0)
	if t.skipComments && tok.Keyword == keyword.COMMENT {
		tok = t.peek(1)
	}

	return tok
}

func (t *Tokenizer) peek(skip int) token.Token {
	if t.currentToken+1+skip < t.maxTokens {
		nextIndex := t.currentToken + 1 + skip
		return t.tokens[nextIndex]
	}
	return token.Token{
		Keyword: keyword.EOF,
	}
}
