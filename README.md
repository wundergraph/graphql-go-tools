[![CircleCI](https://circleci.com/gh/jensneuse/graphql-go-tools.svg?style=svg)](https://circleci.com/gh/jensneuse/graphql-go-tools)
# graphql-go-tools

This repository implements useful graphql tools in the golang programming language.
The major differentiation from other implementations is heavy use of testing to ensure high quality and maintainability.
The code is written in a way that enables easy refactoring. Feel free to submit a PR to improve it further.

Until the repository hits 1.0 the API might be subject to change!

Currently implemented:

- lexing
- parsing
- validation

## Usage

See pkg/parser/parser_test.go

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
BenchmarkValidator/test_valid_schema-4         	  200000	      7823 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidator/test_valid_schema-4         	  200000	      7836 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidator/test_valid_schema-4         	  200000	      7766 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidator/test_valid_schema-4         	  200000	      7777 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidator/introspection_query-4       	    3000	    407511 ns/op	      44 B/op	       0 allocs/op
BenchmarkValidator/introspection_query-4       	    3000	    410118 ns/op	      44 B/op	       0 allocs/op
BenchmarkValidator/introspection_query-4       	    3000	    405893 ns/op	      45 B/op	       0 allocs/op
BenchmarkValidator/introspection_query-4       	    3000	    403380 ns/op	      56 B/op	       0 allocs/op
```

To put these numbers into perspective. Parsing + validating the (quite complex) introspection query is < 0.5ms (on my 2013 MacBook) which should be acceptable for web applications.

It's important to note that gc is kept at a minimum which should enable applications built on top of this library to have almost zero deviation regarding latency.

You'll probably add bottlenecks at another layer, e.g. invoking a database. 

## Contributors

This repository was initially developed and maintained by one single person:
[Jens Neuse][jens-neuse-github].

These users are actively maintaining and/or developing as of today:

- [Jens Neuse][jens-neuse-github] (Project Lead)
- [Jonas Bergner][jonas-bergner-github] (Contributions to the initial version of the parser, contributions to the tests)

[jens-neuse-github]: https://github.com/jensneuse
[jonas-bergner-github]: https://github.com/java-jonas

## Contributions

Feel free to file an issue in case of bugs.
We're open to your ideas to enhance the repository.

You are open to contribute via PR's.
Make sure to comply with the linting rules.
You must not add untested code.
