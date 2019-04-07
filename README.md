[![CircleCI](https://circleci.com/gh/jensneuse/graphql-go-tools.svg?style=svg)](https://circleci.com/gh/jensneuse/graphql-go-tools)
# graphql-go-tools

This repository implements useful graphql tools in the golang programming language.
The major differentiation from other implementations is heavy use of testing to ensure high quality and maintainability.
The code is written in a way that enables easy refactoring. Feel free to submit a PR to improve it further.

Currently implemented:

- lexing
- parsing
- validation
- schema formatting
- proxying

## Go version Info

This repos used go modules so make sure to use the latest version of Go.

## Docs

https://jens-neuse.gitbook.io/graphql-go-tools

## WIP

- (remote) schema introspection
- graphql proxy

## Usage

Please see the tests to understand the library.

## CMD usage

pretty print/format a graphql schema:
```bash
graphql-go-tools fmt schema starwars.schema.graphql > formatted.graphql
```

## Testing

`make test`

## Linting

`make lint`

## Benchmarks

```
pkg: github.com/jensneuse/graphql-go-tools/pkg/parser
BenchmarkParser-4   	   50000	     29490 ns/op	       0 B/op	       0 allocs/op
BenchmarkParser-4   	   50000	     29931 ns/op	       0 B/op	       0 allocs/op
BenchmarkParser-4   	   50000	     28779 ns/op	       1 B/op	       0 allocs/op
BenchmarkParser-4   	   50000	     29176 ns/op	       0 B/op	       0 allocs/op
```

```
pkg: github.com/jensneuse/graphql-go-tools/pkg/validator
BenchmarkValidator/test_valid_schema-4         	  200000	      6091 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidator/test_valid_schema-4         	  200000	      6174 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidator/test_valid_schema-4         	  200000	      6119 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidator/test_valid_schema-4         	  200000	      5975 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidator/introspection_query-4       	   20000	     86069 ns/op	       2 B/op	       0 allocs/op
BenchmarkValidator/introspection_query-4       	   20000	     88226 ns/op	       4 B/op	       0 allocs/op
BenchmarkValidator/introspection_query-4       	   20000	     88185 ns/op	       2 B/op	       0 allocs/op
BenchmarkValidator/introspection_query-4       	   20000	     88447 ns/op	       2 B/op	       0 allocs/op
```

To put these numbers into perspective. Parsing + validating the (quite complex) introspection query is ~ 0.1ms (on my 2013 MacBook) which should be acceptable for web applications.

It's important to note that gc is kept at a minimum which should enable applications built on top of this library to have almost zero deviation regarding latency.

You'll probably add bottlenecks at another layer, e.g. invoking a database.

While adding the ast printing feature I found a severe logical error that made the walker walk way too many times the same values.
I've refactored the validator to properly handle fragment spreads which also helped to easily find the operation definitions a fragment is used by.
By fixing this issue validation time for the introspection query dropped from ~ 407k ns to ~ 88k ns. 

## Contributors

- [Jens Neuse][jens-neuse-github] (Project Lead & Active Maintainer)
- [Mantas Vidutis][mantas-vidutis-github](Contributions to the http proxy & the Context Middleware)
- [Jonas Bergner][jonas-bergner-github] (Contributions to the initial version of the parser, contributions to the tests)

[jens-neuse-github]: https://github.com/jensneuse
[mantas-vidutis-github]: https://github.com/mvid
[jonas-bergner-github]: https://github.com/java-jonas

## Contributions

Feel free to file an issue in case of bugs.
We're open to your ideas to enhance the repository.

You are open to contribute via PR's.
Make sure to comply with the linting rules.
You must not add untested code.
