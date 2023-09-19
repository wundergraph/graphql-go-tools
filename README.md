[![GoDoc](https://godoc.org/github.com/wundergraph/graphql-go-tools?status.svg)](https://godoc.org/github.com/wundergraph/graphql-go-tools)
[![v1-ci](https://github.com/wundergraph/graphql-go-tools/workflows/v1-ci/badge.svg)](https://github.com/wundergraph/graphql-go-tools/actions/workflows/v1.yml)
# graphql-go-tools

[<p align="center"><img height="auto" src="./assets/logo.png"></p>](https://wundergraph.com/)

## Apollo Federation Gateway Replacement

This library can be used as a replacement for the Apollo Federation Gateway.
It implements the [Apollo Federation Specification](https://www.apollographql.com/docs/apollo-server/federation/federation-spec/).

In addition to the scope of other implementations, this one supports **Subscriptions!**

Check out the [Demo](/examples/federation).

## Overview

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
            - REST
    - query execution: takes a context object and executes an execution plan
- Middleware:
    - Operation Complexity: Calculates the complexity of an operation based on the GitHub algorithm
- OperationReport: Makes it easy to collect errors during all phases of a request and enables easy error printing according to the GraphQL spec
- Playground: Easy hosting of GraphQL Playground (no external dependencies, simple middleware) 
- Import Statements: combine multiple GraphQL files into one single schema using #import statements
- Implements the Apollo Federation Specification: Replacement for Apollo Federation Gateway 

## Go version Info

This repos uses go modules so make sure to use the latest version of Go.

## Docs

https://godoc.org/github.com/wundergraph/graphql-go-tools

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
pkg: github.com/wundergraph/graphql-go-tools/pkg/astparser
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
    - Implemented Type Extension merging [#108](https://github.com/wundergraph/graphql-go-tools/pull/108)
- [Patric Vormstein][patric-vormstein-github] (Active Maintainer)
    - Fixed lexer on windows [#92](https://github.com/wundergraph/graphql-go-tools/pull/92)
    - Author of the graphql package to simplify the usage of the library
    - Refactored the http package to simplify usage with http servers
    - Author of the starwars package to enhance testing
- [Sergey Petrunin][sergey-petrunin-github] (Active Maintainer)
    - Helped cleaning up the API of the pipeline package [#166](https://github.com/wundergraph/graphql-go-tools/pull/166)
    - Refactored the ast package into multiple files
    - Author of the introspection converter (introspection JSON -> AST)
    - Fixed various bugs in the parser & visitor & printer
    - Refactored and enhanced the astimport package
- [Vasyl Domanchuk][vasyl-github]
    - Implemented the logic to generate a federation configuration
    - Added federation example

[jens-neuse-github]: https://github.com/jensneuse
[mantas-vidutis-github]: https://github.com/mvid
[jonas-bergner-github]: https://github.com/java-jonas
[patric-vormstein-github]: https://github.com/pvormste
[sergey-petrunin-github]: https://github.com/spetrunin
[vasyl-github]: https://github.com/chedom

## Contributions

Feel free to file an issue in case of bugs.
We're open to your ideas to enhance the repository.

You are open to contribute via PR's.
Please open an issue to discuss your idea before implementing it so we can have a discussion.
Make sure to comply with the linting rules.
You must not add untested code.
