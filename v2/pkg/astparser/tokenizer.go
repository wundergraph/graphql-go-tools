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

// TokenizeWithDepthLimit tokenizes input with a depth limit to prevent excessive nesting
// that could lead to stack overflow or DoS attacks. It tracks both global depth across
// the entire document and local depth within individual operations/fragments.
//
// The depth limit applies to the total nesting depth of GraphQL constructs (objects,
// arrays, selection sets) across the entire document. When a new operation or fragment
// is encountered, the local depth peak is added to the global depth counter.
//
// Returns ErrDepthLimitExceeded if the depth limit is exceeded during tokenization.
func (t *Tokenizer) TokenizeWithDepthLimit(depthLimit int, input *ast.Input) error {
	t.lexer.SetInput(input)
	t.tokens = t.tokens[:0]

	// globalDepth tracks the cumulative depth across the entire document
	globalDepth := 0
	// localDepth tracks the current nesting depth within the current operation/fragment
	localDepth := 0
	// localDepthPeak tracks the maximum depth reached within the current operation/fragment
	localDepthPeak := 0

	for {
		next := t.lexer.Read()
		if next.Keyword == keyword.EOF {
			t.maxTokens = len(t.tokens)
			t.currentToken = -1
			return nil
		}
		switch next.Keyword {
		case keyword.LBRACE:
			globalDepth++
			if globalDepth > depthLimit {
				return ErrDepthLimitExceeded{
					limit: depthLimit,
				}
			}
			localDepth++
			if localDepth > localDepthPeak {
				localDepthPeak = localDepth
			}
		case keyword.RBRACE:
			globalDepth--
			localDepth--
		case keyword.IDENT:
			key := identkeyword.KeywordFromLiteral(input.ByteSlice(next.Literal))
			switch key {
			case identkeyword.FRAGMENT, identkeyword.QUERY, identkeyword.MUTATION, identkeyword.SUBSCRIPTION:
				// When starting a new operation or fragment, add the local depth peak
				// to global depth and reset local tracking
				globalDepth += localDepthPeak
				localDepth = 0
				localDepthPeak = 0
			}
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
