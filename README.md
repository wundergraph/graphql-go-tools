[![GoDoc](https://pkg.go.dev/badge/github.com/wundergraph/graphql-go-tools/v2)](https://pkg.go.dev/github.com/wundergraph/graphql-go-tools/v2)
[![v2-ci](https://github.com/wundergraph/graphql-go-tools/workflows/v2-ci/badge.svg)](https://github.com/wundergraph/graphql-go-tools/actions/workflows/v2.yml)
# GraphQL Router / API Gateway Framework written in Golang

[<p align="center"><img height="auto" src="./assets/logo.png"></p>](https://wundergraph.com/)

## We're hiring!

Are you interested in working on graphql-go-tools?
We're looking for experienced Go developers and DevOps or Platform Engineering specialists to help us run Cosmo Cloud.
If you're more interested in working with Customers on their GraphQL Strategy,
we also offer Solution Architect positions.

Check out the [currently open positions](https://wundergraph.com/jobs#open-positions).

## Replacement for Apollo Router

If you're looking for a complete ready-to-use Open Source Router for Federation,
have a look at the [Cosmo Router](https://github.com/wundergraph/cosmo) which is based on this library.

Cosmo Router wraps this library and provides a complete solution for Federated GraphQL including the following features:
- [x] Federation Gateway
- [x] OpenTelemetry Metrics & Distributed Tracing
- [x] Prometheus Metrics
- [x] GraphQL Schema Usage Exporter
- [x] Health Checks
- [x] GraphQL Playground
- [x] Execution Tracing Exporter & UI in the Playground
- [x] Federated Subscriptions over WebSockets (graphql-ws & graphql-transport-ws protocol support) and SSE
- [x] Authentication using JWKS & JWT
- [x] Highly available & scalable using S3 as a backend for the Router Config
- [x] Persisted Operations / Trusted Documents
- [x] Traffic Shaping (Timeouts, Retries, Header & Body Size Limits, Subgraph Header forwarding)
- [x] Custom Modules & Middleware

## State of the packages

This repository contains multiple packages joined via [workspace](https://github.com/wundergraph/graphql-go-tools/blob/master/go.work).

| Package                                                                                                       | Description                                                                                                                                                                                                                        | Package dependencies                                                                                                                                                                            | Maintenance state                  |
|---------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------------------------|
| [graphql-go-tools v2](https://github.com/wundergraph/graphql-go-tools/blob/master/v2/go.mod)                  | GraphQL engine implementation consisting of lexer, parser, ast, ast validation, ast normalization, datasources, query planner and resolver. Supports GraphQL Federation. Has built-in support for batching federation entity calls | -                                                                                                                                                                                               | actual version, active development |
| [execution](https://github.com/wundergraph/graphql-go-tools/blob/master/execution/go.mod)                     | Execution helpers for the request handling and engine configuration builder                                                                                                                                                        | depends on [graphql-go-tools v2](https://github.com/wundergraph/graphql-go-tools/blob/master/v2/go.mod) and [composition](https://github.com/wundergraph/cosmo/blob/main/composition-go/go.mod) | actual version                     |
| [examples/federation](https://github.com/wundergraph/graphql-go-tools/blob/master/examples/federation/go.mod) | Example implementation of graphql federation gateway. This example is not production ready. For production ready solution please consider using [cosmo router](https://github.com/wundergraph/cosmo/tree/main)                     | depends on [execution](https://github.com/wundergraph/graphql-go-tools/blob/master/execution/go.mod) package                                                                                    | actual federation gateway example  |
| [graphql-go-tools v1](https://github.com/wundergraph/graphql-go-tools/blob/master/go.mod)                     | Legacy GraphQL engine implementation. This version 1 package is in maintenance mode and accepts only pull requests with critical bug fixes. All new features will be implemented in the version 2 package only.                    | -                                                                                                                                                                                               | deprecated, maintenance mode       |


## Notes

This library is used in production at [WunderGraph](https://wundergraph.com/).
We've recently introduced a v2 module that is not completely backwards compatible with v1, hence the major version bump.
The v2 module contains big rewrites in the engine package, mainly to better support GraphQL Federation.
Please consider the v1 module as deprecated and move to v2 as soon as possible.

We have customers who pay us to maintain this library and steer the direction of the project.
[Contact us](https://wundergraph.com/contact/sales) if you're looking for commercial support, features or consulting.

## Performance

The architecture of this library is designed for performance, high-throughput and low garbage collection overhead.
The following benchmark measures the "overhead" of loading and resolving a GraphQL response from four static in-memory Subgraphs at 0,007459 ms/op.
In more complete end-to-end benchmarks, we've measured up to 8x more requests per second and 8x lower p99 latency compared to Apollo Router, which is written in Rust.

```shell
cd v2/pkg/engine
go test -run=nothing -bench=Benchmark_NestedBatchingWithoutChecks -memprofile memprofile.out -benchtime 3s && go tool pprof memprofile.out
goos: darwin
goarch: arm64
pkg: github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve
Benchmark_NestedBatchingWithoutChecks-10          473186              7134 ns/op          52.00 MB/s        2086 B/op         36 allocs/op
```

## Tutorial

If you're here to learn how to use this library to build your own custom GraphQL Router or API Gateway,
here's a speed run tutorial for you, based on how we use this library in Cosmo Router.

```go
package main

import (
  "bytes"
  "context"
  "fmt"

  "github.com/cespare/xxhash/v2"
  "github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
  "github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
  "github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
  "github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
  "github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
  "github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
  "github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
  "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/staticdatasource"
  "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
  "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
  "github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

/*
ExampleParsePrintDocument shows you the most basic usage of the library.
It parses a GraphQL document and prints it back to a writer.
*/
func ExampleParsePrintDocument() {

	input := []byte(`query { hello }`)

	report := &operationreport.Report{}
	document := ast.NewSmallDocument()
	parser := astparser.NewParser()
	printer := &astprinter.Printer{}

	document.Input.ResetInputBytes(input)
	parser.Parse(document, report)

	if report.HasErrors() {
		panic(report.Error())
	}

	out := &bytes.Buffer{}
	err := printer.Print(document, nil, out)
	if err != nil {
		panic(err)
	}
	fmt.Println(out.String()) // Output: query { hello }
}

/*
Okay, that was easy, but also not very useful.
Let's try to parse a more complex document and print it back to a writer.
*/

// ExampleParseComplexDocument shows a special feature of the printer
func ExampleParseComplexDocument() {

	input := []byte(`
		query {
			hello
			foo {
				bar
			}
		}
	`)

	report := &operationreport.Report{}
	document := ast.NewSmallDocument()
	parser := astparser.NewParser()
	printer := &astprinter.Printer{}

	document.Input.ResetInputBytes(input)
	parser.Parse(document, report)

	if report.HasErrors() {
		panic(report.Error())
	}

	out := &bytes.Buffer{}
	err := printer.Print(document, nil, out)
	if err != nil {
		panic(err)
	}
	fmt.Println(out.String()) // Output: query { hello foo { bar } }
}

/*
You'll notice that the printer removes all whitespace and newlines.
But what if we wanted to print the document with indentation?
*/

func ExamplePrintWithIndentation() {

	input := []byte(`
		query {
			hello
			foo {
				bar
			}
		}
	`)

	report := &operationreport.Report{}
	document := ast.NewSmallDocument()
	parser := astparser.NewParser()

	document.Input.ResetInputBytes(input)
	parser.Parse(document, report)

	if report.HasErrors() {
		panic(report.Error())
	}

	out, err := astprinter.PrintStringIndent(document, nil, "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(out)
	// Output: query {
	//   hello
	//   foo {
	//     bar
	//   }
	// }
}

/*
Okay, fantastic. We can parse and print GraphQL documents.
As a next step, we could analyze the document and extract some information from it.
What if we wanted to know the name of the operation in the document, if any?
And what if we wanted to know about the Operation type?
*/

func ExampleParseOperationNameAndType() {

	input := []byte(`
		query MyQuery {
			hello
			foo {
				bar
			}
		}
	`)

	report := &operationreport.Report{}
	document := ast.NewSmallDocument()
	parser := astparser.NewParser()

	document.Input.ResetInputBytes(input)
	parser.Parse(document, report)

	if report.HasErrors() {
		panic(report.Error())
	}

	operationCount := 0
	var (
		operationNames []string
		operationTypes []ast.OperationType
	)

	for _, node := range document.RootNodes {
		if node.Kind != ast.NodeKindOperationDefinition {
			continue
		}
		operationCount++
		name := document.RootOperationTypeDefinitionNameString(node.Ref)
		operationNames = append(operationNames, name)
		operationType := document.RootOperationTypeDefinitions[node.Ref].OperationType
		operationTypes = append(operationTypes, operationType)
	}

	fmt.Println(operationCount) // Output: 1
	fmt.Println(operationNames) // Output: [MyQuery]
}

/*
We've now seen how to analyze the document and learn a bit about it.
We could now add some validation to our application,
e.g. we could check for the number of operations in the document,
and return an error if there are multiple anonymous operations.

We could also validate the Operation content against a schema.
But before we do this, we need to normalize the document.
This is important because validation relies on the document being normalized.
It was much easier to build the validation and many other features on top of a normalized document.

Normalization is the process of transforming the document into a canonical form.
This means that the document is transformed in a way that makes it easier to reason about it.
We inline fragments, we remove unused fragments,
we remove duplicate fields, we remove unused variables,
we remove unused operations etc...

So, let's normalize the document!
*/

func ExampleNormalizeDocument() {

	input := []byte(`
		query MyQuery {
			hello
			hello
			foo {
				bar
				bar
			}
			...MyFragment
		}

		fragment MyFragment on Query {
			hello
			foo {
				bar
			}
		}
	`)

	schema := []byte(`
		type Query {
			hello: String
			foo: Foo
		}
	
		type Foo {
			bar: String
		}
	`)

	report := &operationreport.Report{}
	document := ast.NewSmallDocument()
	parser := astparser.NewParser()

	document.Input.ResetInputBytes(input)
	parser.Parse(document, report)

	if report.HasErrors() {
		panic(report.Error())
	}

	schemaDocument := ast.NewSmallDocument()
	schemaParser := astparser.NewParser()
	schemaDocument.Input.ResetInputBytes(schema)
	schemaParser.Parse(schemaDocument, report)

	if report.HasErrors() {
		panic(report.Error())
	}

	// graphql-go-tools is very strict about the schema
	// the above GraphQL Schema is not fully valid, e.g. the `schema { query: Query }` part is missing
	// we can fix this automatically by merging the schema with a base schema
	err := asttransform.MergeDefinitionWithBaseSchema(schemaDocument)
	if err != nil {
		panic(err)
	}

	// you can customize what rules the normalizer should apply
	normalizer := astnormalization.NewWithOpts(
		astnormalization.WithExtractVariables(),
		astnormalization.WithInlineFragmentSpreads(),
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveNotMatchingOperationDefinitions(),
	)

	// It's generally recommended to always give your operation a name
	// If it doesn't have a name, just add one to the AST before normalizing it
	// This is not strictly necessary, but ensures that all normalization rules work as expected
	normalizer.NormalizeNamedOperation(document, schemaDocument, []byte("MyQuery"), report)

	if report.HasErrors() {
		panic(report.Error())
	}

	out, err := astprinter.PrintStringIndent(document, nil, "  ")
	if err != nil {
		panic(err)
	}

	fmt.Println(out)
	// Output: query MyQuery {
	//   hello
	//   foo {
	//     bar
	//   }
	// }
}

/*
Okay, that was a lot of work, but now we have a normalized document.
As you can see, all the duplicate fields have been removed and the fragment has been inlined.

What can we do with it?
Well, the possibilities are endless,
but why don't we start with validating the document against a schema?
Alright. Let's do it!
*/

func ExampleValidateDocument() {
	schemaDocument := ast.NewSmallDocument()
	operationDocument := ast.NewSmallDocument()
	report := &operationreport.Report{}
	validator := astvalidation.DefaultOperationValidator()
	validator.Validate(operationDocument, schemaDocument, report)
	if report.HasErrors() {
		panic(report.Error())
	}
}

/*
Fantastic, we've now got a GraphQL document that is valid against a schema.

As a next step, we could generate a cache key for the document.
This is very useful if we want to start doing expensive operations afterward that could be de-duplicated or cached.
At the same time, generating a cache key from a normalized document is not as trivial as it sounds.
Let's take a look!
*/

func ExampleGenerateCacheKey() {
	operationDocument := ast.NewSmallDocument()
	schemaDocument := ast.NewSmallDocument()
	report := &operationreport.Report{}

	normalizer := astnormalization.NewWithOpts(
		astnormalization.WithExtractVariables(),
		astnormalization.WithInlineFragmentSpreads(),
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveNotMatchingOperationDefinitions(),
	)

	normalizer.NormalizeNamedOperation(operationDocument, schemaDocument, []byte("MyQuery"), report)
	printer := &astprinter.Printer{}
	keyGen := xxhash.New()
	err := printer.Print(operationDocument, schemaDocument, keyGen)
	if err != nil {
		panic(err)
	}

	// you might be thinking that we're done now, but we're not
	// we've extracted the variables, so we need to add them to the cache key

	_, err = keyGen.Write(operationDocument.Input.Variables)
	if err != nil {
		panic(err)
	}

	key := keyGen.Sum64()
	fmt.Printf("%x", key) // Output: {cache key}
}

/*
Good job! We now have a correct cache key for the document.
We're using this ourselves in production to de-duplicate e.g. planning the execution of a GraphQL Operation.

There's just one problem with the above code.
An attacker could easily send the same document with a different Operation name and get a different cache key.
This could quite easily fill up our cache with duplicate entries.
To prevent this, we can make the operation name static.
Let's change out code to account for this.
*/

func ExampleGenerateCacheKeyWithStaticOperationName() {

	staticOperationName := []byte("O")

	operationDocument := ast.NewSmallDocument()
	schemaDocument := ast.NewSmallDocument()
	report := &operationreport.Report{}

	normalizer := astnormalization.NewWithOpts(
		astnormalization.WithExtractVariables(),
		astnormalization.WithInlineFragmentSpreads(),
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveNotMatchingOperationDefinitions(),
	)

	// First, we add the static operation name to the document and get an "address" to the byte slice (string) in the document
	// We cannot just add a string to an AST because the AST only stores references to byte slices
	// Storing strings in AST nodes would be very inefficient and would require a lot of allocations
	nameRef := operationDocument.Input.AppendInputBytes(staticOperationName)

	for _, node := range operationDocument.RootNodes {
		if node.Kind != ast.NodeKindOperationDefinition {
			continue
		}
		name := operationDocument.OperationDefinitionNameString(node.Ref)
		if name != "MyQuery" {
			continue
		}
		// Then we set the name of the operation to the address of the static operation name
		// Now we have renamed MyQuery to O
		operationDocument.OperationDefinitions[node.Ref].Name = nameRef
	}

	// Now we can normalize the modified document
	// All Operations that don't have the name O will be removed
	normalizer.NormalizeNamedOperation(operationDocument, schemaDocument, staticOperationName, report)

	printer := &astprinter.Printer{}
	keyGen := xxhash.New()
	err := printer.Print(operationDocument, schemaDocument, keyGen)
	if err != nil {
		panic(err)
	}

	_, err = keyGen.Write(operationDocument.Input.Variables)
	if err != nil {
		panic(err)
	}

	key := keyGen.Sum64()
	fmt.Printf("%x", key) // Output: {cache key}
}

/*
With these changes, the name of the operation doesn't matter anymore.
Independent of the name, the cache key will always be the same.

As a next step, we could start planning the execution of the operation.
This is a very complex topic, so we'll just show you how to plan the operation.
Going into detail would be beyond the scope of this example.
It took us years to get this right, so we won't be able to explain it in a few lines of code.

graphql-go-tools is not a GraphQL server by itself.
It's a library that you can use to build Routers, Gateways, or even GraphQL Server frameworks on top of it.
What this means is that there's no built-in support to define "resolvers".
Instead, you have to define DataSources that are used to resolve fields.

A DataSource can be anything, e.g. a static value, a HTTP JSON API, a GraphQL API, a WASM Lambda, a Database etc.
It's up to you to implement the DataSource interface.

The simplest DataSource is the StaticDataSource.
It's a DataSource that returns a static value for a field.
Let's see how to use it!

You have to attach the DataSource to one or more fields in the schema,
and you have to provide a config and a factory for the DataSource,
so that the planner knows how to create an execution plan for the DataSource and an "instance" of the DataSource.
*/

func ExamplePlanOperation() {
    staticDataSource, err := plan.NewDataSourceConfiguration[staticdatasource.Configuration](
      "StaticDataSource",
      &staticdatasource.Factory[staticdatasource.Configuration]{},
      &plan.DataSourceMetadata{
        RootNodes: []plan.TypeField{
          {
            TypeName:   "Query",
            FieldNames: []string{"hello"},
          },
        },
      },
      staticdatasource.Configuration{
        Data: `{"hello":"world"}`,
      },
    )
	if err != nil {
		panic(err)
    }
  
    config := plan.Configuration{
      DataSources: []plan.DataSource{
        staticDataSource,
      },
      Fields: []plan.FieldConfiguration{
        {
          TypeName:              "Query", // attach this config to the Query type and the field hello
          FieldName:             "hello",
          DisableDefaultMapping: true,              // disable the default mapping for this field which only applies to GraphQL APIs
          Path:                  []string{"hello"}, // returns the value of the field "hello" from the JSON data
        },
      },
      IncludeInfo: true,
    }

	operationDocument := ast.NewSmallDocument() // containing the following query: query O { hello }
	schemaDocument := ast.NewSmallDocument()
	report := &operationreport.Report{}
	operationName := "O"

	planner := plan.NewPlanner(context.Background(), config)
	executionPlan := planner.Plan(operationDocument, schemaDocument, operationName, report)
	if report.HasErrors() {
		panic(report.Error())
	}
	fmt.Printf("%+v", executionPlan) // Output: Plan...
}

/*
As you can see, the planner has created a plan for us.
This plan can now be executed by using the Resolver.
*/

func ExampleExecuteOperation() {
	var preparedPlan plan.Plan
	resolver := resolve.New(context.Background(), true)

	ctx := resolve.NewContext(context.Background())

	switch p := preparedPlan.(type) {
	case *plan.SynchronousResponsePlan:
		out := &bytes.Buffer{}
		err := resolver.ResolveGraphQLResponse(ctx, p.Response, nil, out)
		if err != nil {
			panic(err)
		}
		fmt.Println(out.String()) // Output: {"data":{"hello":"world"}}
	case *plan.SubscriptionResponsePlan:
		// this is a Query, so we ignore Subscriptions for now, but they are supported
	}
}

/*
Well done! You've now seen how to parse, print, validate, normalize, plan and execute a GraphQL document.
You've built a complete GraphQL API Gateway from scratch.
That said, this was really just the tip of the iceberg.

When you look under the hood of graphql-go-tools, you'll notice that a lot of its functionality is built on top of the AST,
more specifically on top of the "astvisitor" package.
It comes with a lot of useful bells and whistles that help you to solve complex problems.

You'll notice that almost everything, from normalization to printing, planning, validation, etc.
is built on top of the AST and the astvisitor package.

Let's take a look at a basic example of how to use the astvisitor package to build higher level functionality.
Here's a simple use case:

Let's walk through the AST of a GraphQL document and extract all tuples of (TypeName, FieldName).
This is useful, e.g. when you want to extract information about the fields that are used in a document.
*/

type visitor struct {
	walker                *astvisitor.Walker
	operation, definition *ast.Document
	typeFields            [][]string
}

func (v *visitor) EnterField(ref int) {
	// get the name of the enclosing type (Query)
	enclosingTypeName := v.walker.EnclosingTypeDefinition.NameString(v.definition)
	// get the name of the field (hello)
	fieldName := v.operation.FieldNameString(ref)
	// get the type definition of the field (String)
	definitionRef, exists := v.walker.FieldDefinition(ref)
	if !exists {
		return
	}
	// get the name of the field type (String)
	fieldTypeName := v.definition.FieldDefinitionTypeNameString(definitionRef)
	v.typeFields = append(v.typeFields, []string{enclosingTypeName, fieldName, fieldTypeName})
}

func ExampleWalkAST() {

	operationDocument := ast.NewSmallDocument() // containing the following query: query O { hello }
	schemaDocument := ast.NewSmallDocument()    // containing the following schema: type Query { hello: String }
	report := &operationreport.Report{}

	walker := astvisitor.NewWalker(24)

	vis := &visitor{
		walker:     &walker,
		operation:  operationDocument,
		definition: schemaDocument,
	}

	walker.RegisterEnterFieldVisitor(vis)
	walker.Walk(operationDocument, schemaDocument, report)
	if report.HasErrors() {
		panic(report.Error())
	}
	fmt.Printf("%+v", vis.typeFields) // Output: [[Query hello String]]
}

/*
This is just a very basic example of what you can do with the astvisitor package,
but you can see that it's very powerful and flexible.

You can register callbacks for every AST node and do whatever you want with it.
In addition, the walker helps you to keep track of the current position in the AST,
and it can help you to figure out the enclosing type of a field,or the ancestors or a node.
*/
```

I hope this tutorial gave you a good overview of what you can do with this library.
If you have any questions, feel free to open an issue.
Following, here's a list of all the important packages in this library and what problems they solve.

- ast: the GraphQL AST and all the logic to work with it.
- astimport: import GraphQL documents from one AST into another
- astnormalization: normalize a GraphQL document
- astparser: parse a string into a GraphQL AST
- astprinter: print a GraphQL AST into a string
- asttransform: transform a GraphQL AST, e.g. merge it with a base schema
- astvalidation: validate a GraphQL AST against a schema
- astvisitor: walk through a GraphQL AST and execute callbacks for every node
- engine/datasource: the DataSource interface and some implementations
- engine/datasource/graphql_datasource: the GraphQL DataSource implementation, including support for Federation
- engine/plan: plan the execution of a GraphQL document
- engine/resolve: execute the plan
- introspection: convert a GraphQL Schema into an introspection JSON document
- lexer: turn a string containing a GraphQL document into a list of tokens
- playground: add a GraphQL Playground to your Go HTTP server
- subscription: implements GraphQL Subscriptions over WebSockets and SSE

## Contributors

- [Jens Neuse][jens-neuse-github] (Project Lead & Active Maintainer)
  - Initial version of graphql-go-tools
  - Currently responsible for the loader and resolver implementation
- [Sergiy Petrunin 🇺🇦][sergiy-petrunin-github] (Active Maintainer)
  - Helped cleaning up the API of the pipeline package
  - Refactored the ast package into multiple files
  - Author of the introspection converter (introspection JSON -> AST)
  - Fixed various bugs in the parser & visitor & printer
  - Refactored and enhanced the astimport package
  - Current maintainer of the plan package
- [Patric Vormstein][patric-vormstein-github] (Active Maintainer)
  - Fixed lexer on windows
  - Author of the graphql package to simplify the usage of the library
  - Refactored the http package to simplify usage with http servers
  - Author of the starwars package to enhance testing
  - Refactor of the Subscriptions Implementation
- [Mantas Vidutis][mantas-vidutis-github] (Inactive)
  - Contributions to the http proxy & the Context Middleware
- [Jonas Bergner][jonas-bergner-github] (Inactive)
  - Contributions to the initial version of the parser, contributions to the tests
  - Implemented Type Extension merging (deprecated)
- [Vasyl Domanchuk][vasyl-github] (Inactive)
  - Implemented the logic to generate a federation configuration
  - Added federation example
  - Added the initial version of the batching implementation

[jens-neuse-github]: https://github.com/jensneuse
[mantas-vidutis-github]: https://github.com/mvid
[jonas-bergner-github]: https://github.com/java-jonas
[patric-vormstein-github]: https://github.com/pvormste
[sergiy-petrunin-github]: https://github.com/devsergiy
[vasyl-github]: https://github.com/chedom

## Contributions

Feel free to file an issue in case of bugs.
We're open to your ideas to enhance the repository.

You are open to contribute via PR's.
Please open an issue to discuss your idea before implementing it so we can have a discussion.
Make sure to comply with the linting rules.
You must not add untested code.
