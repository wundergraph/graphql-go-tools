[![GoDoc](https://godoc.org/github.com/jensneuse/graphql-go-tools?status.svg)](https://godoc.org/github.com/jensneuse/graphql-go-tools)
[![CI](https://github.com/jensneuse/graphql-go-tools/workflows/ci/badge.svg)](https://github.com/jensneuse/graphql-go-tools/workflows/ci/badge.svg)
# graphql-go-tools

Looking for a ready to use GraphQL Server/Gateway?

Have a look at: https://github.com/jensneuse/graphql-gateway

This repository implements low level building blocks to write graphql services in Go.

With this library you could build tools like:
- proxies
- caches
- server/client implementations
- a WAF
- be creative! =)

Currently implemented:

- GraphQL AST as of https://graphql.github.io/graphql-spec/June2018
- Token lexing
- AST parsing: parse bytes/string into AST
- AST printing: print an AST to an io.Writer
    - supports indentation
- AST validation:
    - all rules from the spec implemented
- AST visitor:
    - simple visitor: fastest implementation, without field type definition information
    - visitor: a bit more overhead but has field type definitions and other quirks
- AST normalization
    - remove unnecessary include/skip directives
    - field deduplication
    - field selection merging
    - fragment definition removal
    - fragment spread inlining
    - inline fragment merging
    - remove self aliasing
    - type extension merging
    - remove type extensions
- Introspection: transforms a graphql schema into a resolvable Data Source
- AST execution
    - query planning: turns a Query AST into a cacheable execution plan
        - supported DataSources:
            - GraphQL (multiple GraphQL services can be combined)
            - static (static embedded data)
            - HTTP JSON
            - HTTP JSON Streaming (uses polling to create a stream)
            - MQTT
            - Nats
            - Webassembly (resolve a Request using WASI compliant modules)
    - query execution: takes a context object and executes an execution plan
- Middleware:
    - Operation Complexity: Calculates the complexity of an operation based on the GitHub algorithm
- OperationReport: Makes it easy to collect errors during all phases of a request and enables easy error printing according to the GraphQL spec
- Playground: Easy hosting of GraphQL Playground (no external dependencies, simple middleware) 
- Import Statements: combine multiple GraphQL files into one single schema using #import statements

## Go version Info

This repos uses go modules so make sure to use the latest version of Go.

## Docs

https://godoc.org/github.com/jensneuse/graphql-go-tools

## Usage

Look into the docs.
Other than that, tests definitely help understanding this library.

## Testing

`make test`

## Linting

`make lint`

## Performance

Most hot path operations have 0 allocations.
You should expect this library to exceed all alternatives in terms of performance.
I've compared my implementation vs. others but why trust my numbers?
Feel free to add comparisons via PR.

## Benchmarks

Parse Kitchen Sink (1020 chars, example from Facebook):
```shell script
pkg: github.com/jensneuse/graphql-go-tools/pkg/astparser
BenchmarkKitchenSink 	  189426	      5652 ns/op	       0 B/op	       0 allocs/op
BenchmarkKitchenSink 	  198253	      5526 ns/op	       0 B/op	       0 allocs/op
BenchmarkKitchenSink 	  199924	      5553 ns/op	       0 B/op	       0 allocs/op
BenchmarkKitchenSink 	  212695	      5804 ns/op	       0 B/op	       0 allocs/op
```

CPU and Memory consumption for lexing, parsing as well as most other operations is neglectable, even for larger queries.

## Contributors

- [Jens Neuse][jens-neuse-github] (Project Lead & Active Maintainer)
- [Mantas Vidutis][mantas-vidutis-github]
    - Contributions to the http proxy & the Context Middleware
- [Jonas Bergner][jonas-bergner-github]
    - Contributions to the initial version of the parser, contributions to the tests
    - Implemented Type Extension merging [#108](https://github.com/jensneuse/graphql-go-tools/pull/108)
- [Patric Vormstein][patric-vormstein-github]
    - Fixed lexer on windows [#92](https://github.com/jensneuse/graphql-go-tools/pull/92)
- [Sergey Petrunin][sergey-petrunin-github]
    - Helped cleaning up the API of the pipeline package [#166](https://github.com/jensneuse/graphql-go-tools/pull/166)

[jens-neuse-github]: https://github.com/jensneuse
[mantas-vidutis-github]: https://github.com/mvid
[jonas-bergner-github]: https://github.com/java-jonas
[patric-vormstein-github]: https://github.com/pvormste
[sergey-petrunin-github]: https://github.com/spetrunin

## Contributions

Feel free to file an issue in case of bugs.
We're open to your ideas to enhance the repository.

You are open to contribute via PR's.
Please open an issue to discuss your idea before implementing it so we can have a discussion.
Make sure to comply with the linting rules.
You must not add untested code.
