// Package graphql-go-tools is library to create GraphQL services using the go programming language.
//
// # About GraphQL
//
// GraphQL is a query language for APIs and a runtime for fulfilling those queries with your existing data. GraphQL provides a complete and understandable description of the data in your API, gives clients the power to ask for exactly what they need and nothing more, makes it easier to evolve APIs over time, and enables powerful developer tools.
//
// Source: https://graphql.org
//
// # About this library
//
// This library is intended to be a set of low level building blocks to write high performance and secure GraphQL applications.
// Use cases could range from writing layer seven GraphQL proxies, firewalls, caches etc..
// You would usually not use this library to write a GraphQL server yourself but to build tools for the GraphQL ecosystem.
//
// To achieve this goal the library has zero dependencies at its core functionality.
// It has a full implementation of the GraphQL AST and supports lexing, parsing, validation, normalization, introspection, query planning as well as query execution etc.
//
// With the execution package it's possible to write a fully functional GraphQL server that is capable to mediate between various protocols and formats.
// In it's current state you can use the following DataSources to resolve fields:
// - Static data (embed static data into a schema to extend a field in a simple way)
// - HTTP JSON APIs (combine multiple Restful APIs into one single GraphQL Endpoint, nesting is possible)
// - GraphQL APIs (you can combine multiple GraphQL APIs into one single GraphQL Endpoint, nesting is possible)
// - Webassembly/WASM Lambdas (e.g. resolve a field using a Rust lambda)
//
// If you're looking for a ready to use solution that has all this functionality packaged as a Gateway have a look at: https://wundergraph.com
//
// Created by Jens Neuse
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
	fmt.Println(operationTypes) // Output: [query]
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
	validator.Validate(schemaDocument, operationDocument, report)
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
	staticDataSource, _ := plan.NewDataSourceConfiguration[staticdatasource.Configuration](
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
	resolver := resolve.New(context.Background(), resolve.ResolverOptions{
		MaxConcurrency: 1024,
	})

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
