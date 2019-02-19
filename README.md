[![CircleCI](https://circleci.com/gh/jensneuse/graphql-go-tools.svg?style=svg)](https://circleci.com/gh/jensneuse/graphql-go-tools)
# graphql-go-tools

This repository implements useful graphql tools in the golang programming language.
The major differentiation from other implementations is heavy use of testing to ensure high quality and maintainability.
The code is written in a way that enables easy refactoring. Feel free to submit a PR to improve it further.

Until the repository hits 1.0 the API might be subject to change!

Currently implemented:

- lexing
- parsing

TODO (1.0 planned @ 02/2019):

- [x] cleanup different styles of testing
- [ ] validation (WIP)
- [ ] introspection
- [ ] schema printer

## Usage

See pkg/parser/parser_test.go

## Testing

`make test`

## Linting

`make lint`

## Benchmarks

```
pkg: github.com/jensneuse/graphql-go-tools/pkg/parser
BenchmarkParser-4   	  100000	     19724 ns/op	       0 B/op	       0 allocs/op
BenchmarkParser-4   	  100000	     19548 ns/op	       0 B/op	       0 allocs/op
BenchmarkParser-4   	  100000	     19409 ns/op	       0 B/op	       0 allocs/op
BenchmarkParser-4   	  100000	     19233 ns/op	       0 B/op	       0 allocs/op
```

In a previous release I found that nested slice structs accounted for huge amounts of gc and decreased performance.
This is fixed. I've also added resource pooling to avoid slice grows. As a caveat the parser is not thread safe.
A possible solution would be to have a pool of parsers which should work fine as parser doesn't allocate a lot of memory.

Other than that I don't see any value in further optimizing for performance as it is "good enough".

For comparison (using the exact same input & hardware):

```
goos: darwin
goarch: amd64
pkg: github.com/vektah/gqlparser/parser
BenchmarkParser-4   	   50000	     36128 ns/op	   16112 B/op	     217 allocs/op
BenchmarkParser-4   	   50000	     35946 ns/op	   16112 B/op	     217 allocs/op
BenchmarkParser-4   	   50000	     36039 ns/op	   16112 B/op	     217 allocs/op
BenchmarkParser-4   	   50000	     35985 ns/op	   16112 B/op	     217 allocs/op
```

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
