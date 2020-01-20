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

Lexing & Parsing is neglectable, even for larger queries.

Overlapping Fields can merge Validation Benchmark:
```shell script
pkg: github.com/jensneuse/graphql-go-tools/pkg/astvalidation
BenchmarkOverlappingFieldsCanBeMerged/valid1         	  403772	      2950 ns/op	    1280 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid1       	  282883	      3986 ns/op	    1424 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid5         	  167020	      6652 ns/op	    1280 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid5       	  152618	      7768 ns/op	    1424 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid10        	   97315	     12127 ns/op	    1280 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid10      	   84303	     13523 ns/op	    1424 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid20        	   45145	     26941 ns/op	    1280 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid20      	   43556	     27883 ns/op	    1424 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid30        	   27928	     44090 ns/op	    1280 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid30      	   26857	     45379 ns/op	    1424 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid40        	   18298	     64951 ns/op	    1280 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid40      	   18244	     66808 ns/op	    1424 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid50        	   13846	     87803 ns/op	    1281 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid50      	   13272	     88558 ns/op	    1425 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid60        	   10000	    114803 ns/op	    1281 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid60      	   10000	    115831 ns/op	    1425 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid70        	    8439	    144624 ns/op	    1283 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid70      	    7879	    147608 ns/op	    1427 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid80        	    6108	    182149 ns/op	    1284 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid80      	    6740	    184045 ns/op	    1428 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid90        	    5209	    218830 ns/op	    1285 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid90      	    5452	    223565 ns/op	    1429 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid100       	    4058	    272589 ns/op	    1286 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid100     	    4449	    273063 ns/op	    1430 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid110       	    3837	    308023 ns/op	    1287 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid110     	    3842	    325062 ns/op	    1431 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid120       	    3322	    356372 ns/op	    1288 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid120     	    3334	    353856 ns/op	    1432 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid130       	    2853	    411152 ns/op	    1298 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid130     	    2904	    424221 ns/op	    1442 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid140       	    2486	    462372 ns/op	    1301 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid140     	    2509	    461915 ns/op	    1445 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid150       	    2260	    530658 ns/op	    1303 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid150     	    2250	    530150 ns/op	    1448 B/op	       4 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/valid200       	    1333	    877854 ns/op	    1320 B/op	       1 allocs/op
BenchmarkOverlappingFieldsCanBeMerged/invalid200     	    1316	    876438 ns/op	    1465 B/op	       4 allocs/op
```

Complex validation grows linearly, not exponentially.

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
