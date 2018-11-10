package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

// ReadOption is a func to add options to the readOptions
type ReadOption func(option readOptions) readOptions

type tokenMap map[token.Keyword]struct{}

func (tm tokenMap) contains(keyword token.Keyword) bool {
	_, contains := tm[keyword]
	return contains
}

func (tm tokenMap) merge(with tokenMap) tokenMap {

	if tm == nil {
		return with
	}

	for key, val := range with {
		tm[key] = val
	}

	return tm
}

func tokenMapFromKeylist(keyword ...token.Keyword) tokenMap {
	tm := tokenMap{}

	for _, key := range keyword {
		tm[key] = struct{}{}
	}

	return tm
}

type readOptions struct {
	ignore    tokenMap
	hasIgnore bool

	whitelist    tokenMap
	hasWhitelist bool

	blacklist    tokenMap
	hasBlacklist bool

	excludeLiteral    token.Literal
	hasExcludeLiteral bool

	hasDescription bool

	readRepeat bool
}

// WithIgnore makes the read operation ignore keywords
func WithIgnore(keyword ...token.Keyword) ReadOption {
	return func(option readOptions) readOptions {

		option.ignore = option.ignore.merge(tokenMapFromKeylist(keyword...))
		option.hasIgnore = true
		return option
	}
}

// WithWhitelist will run the read operation with a whitelist
func WithWhitelist(keyword ...token.Keyword) ReadOption {
	return func(option readOptions) readOptions {

		option.whitelist = option.whitelist.merge(tokenMapFromKeylist(keyword...))
		option.hasWhitelist = true
		return option
	}
}

// WithBlacklist will run the read operation with a blacklist
func WithBlacklist(keyword ...token.Keyword) ReadOption {
	return func(option readOptions) readOptions {

		option.blacklist = option.blacklist.merge(tokenMapFromKeylist(keyword...))
		option.hasBlacklist = true
		return option
	}
}

// WithExcludeLiteral will run the read operation with excluding a certain
// literal
func WithExcludeLiteral(literal token.Literal) ReadOption {
	return func(option readOptions) readOptions {

		option.excludeLiteral = literal
		option.hasExcludeLiteral = true
		return option
	}
}

// WithDescription will read a description in front of the next valid token
// the description will be added to the token object
func WithDescription() ReadOption {
	return func(option readOptions) readOptions {

		if option.hasDescription {
			panic("WithDescription: you must not call this function twice")
		}

		option.hasDescription = true
		return option
	}
}

// WithReadRepeat will make the reader re read the emitted token after the operation
// this is useful e.g. for peeking
func WithReadRepeat() ReadOption {
	return func(option readOptions) readOptions {
		option.readRepeat = true
		return option
	}
}

func (p *Parser) makeReadOptions(option ...ReadOption) readOptions {
	var options readOptions
	for _, op := range option {
		options = op(options)
	}

	return options
}

func (p *Parser) readWithOptions(options readOptions) (tok token.Token, err error) {
	var description string

	for {
		tok, err = p.l.Read()

		if options.hasDescription && tok.Keyword == token.STRING {
			description = string(tok.Literal)
			continue
		}

		if options.hasIgnore && options.ignore.contains(tok.Keyword) {
			// ignore token
			continue
		}

		if options.hasWhitelist && !options.whitelist.contains(tok.Keyword) {
			// whitelist is set but token is not whitelisted
			err = newErrInvalidType(tok.Position, "readWithOptions/whitelist", fmt.Sprintf("%v", options.whitelist), string(tok.Keyword))
			return
		}

		if options.hasBlacklist && options.blacklist.contains(tok.Keyword) {
			// token is blacklisted
			err = newErrInvalidType(tok.Position, "readWithOptions/blacklist", fmt.Sprintf("anything but %v", options.blacklist), string(tok.Keyword))
			return
		}

		if options.hasExcludeLiteral && options.excludeLiteral.Equals(tok.Literal) {
			// token.Literal is excluded
			err = newErrInvalidType(tok.Position, "readWithOptions/excludeLiteral", string(options.excludeLiteral), string(tok.Literal))
			return
		}

		// all rules OK -> return token
		tok.Description = description

		if options.readRepeat {
			p.l.ReadRepeatCurrentToken()
		}

		return
	}
}

func (p *Parser) read(option ...ReadOption) (tok token.Token, err error) {
	return p.readWithOptions(p.makeReadOptions(option...))
}
