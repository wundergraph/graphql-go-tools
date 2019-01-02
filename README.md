[![CircleCI](https://circleci.com/gh/jensneuse/graphql-go-tools.svg?style=svg)](https://circleci.com/gh/jensneuse/graphql-go-tools)
# graphql-go-tools

This repository tries to implement useful graphql tools in the golang programming language.
The major differentiation from other implementations is heavy use of testing to ensure high quality.

Currently implemented:

- lexing
- parsing

TODO:

- validation

## Usage

See pkg/parser/parser_test.go

## Testing

`make test`

## Linting

`make lint`

## Benchmarks

```
goos: darwin
goarch: amd64
pkg: github.com/jensneuse/graphql-go-tools/pkg/parser
BenchmarkParser-4   	   50000	     36178 ns/op	    9746 B/op	     130 allocs/op
BenchmarkParser-4   	   50000	     36630 ns/op	    9746 B/op	     130 allocs/op
BenchmarkParser-4   	   50000	     36620 ns/op	    9746 B/op	     130 allocs/op
BenchmarkParser-4   	   50000	     36444 ns/op	    9746 B/op	     130 allocs/op
```

Allocations could easily reduced below 70 allocs/op 7000 B/op by introducing resource pooling.
That being said I don't see any value in micro optimizing at this stage. <0.04 ms/op for parsing the Introspection Query seems good enough.

## Contributors

This repository was initially developed and maintained by one single person:
[Jens Neuse][jens-neuse-github].

These users are actively maintaining and/or developing as of today:

- [Jens Neuse][jens-neuse-github] (Project Lead)
- [Jonas Bergner][jonas-bergner-github] (Major contributions to the parser, extensive testing)

[jens-neuse-github]: https://github.com/jensneuse
[jonas-bergner-github]: https://github.com/java-jonas

## Contributions

Feel free to file an issue in case of bugs.
We're open to your ideas to enhance the repository.

You are open to contribute via PR's.
Make sure to comply with the linting rules.
You must not add untested code.
