package sdlmerge

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type composeVisitor []Visitor

func (c composeVisitor) Register(walker *astvisitor.Walker) {
	for _, visitor := range c {
		visitor.Register(walker)
	}
}

var run = func(t *testing.T, visitor Visitor, operation, expectedOutput string) {

	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	expectedOutputDocument := unsafeparser.ParseGraphqlDocumentString(expectedOutput)
	report := operationreport.Report{}
	walker := astvisitor.NewWalker(48)

	visitor.Register(&walker)

	walker.Walk(&operationDocument, nil, &report)

	if report.HasErrors() {
		t.Fatal(report.Error())
	}

	got := mustString(astprinter.PrintStringIndent(&operationDocument, nil, " "))
	want := mustString(astprinter.PrintStringIndent(&expectedOutputDocument, nil, " "))

	assert.Equal(t, want, got)
}

func runMany(t *testing.T, operation, expectedOutput string, visitors ...Visitor) {
	run(t, composeVisitor(visitors), operation, expectedOutput)
}

func mustString(str string, err error) string {
	if err != nil {
		panic(err)
	}
	return str
}

func TestMergeSDLs(t *testing.T) {
	got, err := MergeSDLs(accountSchema, productSchema, reviewSchema)
	if err != nil {
		t.Fatal(err)
	}

	expectedOutputDocument := unsafeparser.ParseGraphqlDocumentString(federatedSchema)
	want := mustString(astprinter.PrintString(&expectedOutputDocument, nil))

	assert.Equal(t, want, got)
}

const (
	accountSchema = `
		extend type Query {
			me: User
		}
	
		type User @key(fields: "id") {
			id: ID!
			username: String!
		}
	`
	productSchema = `
		extend type Query {
			topProducts(first: Int = 5): [Product]
		}
		
		type Product @key(fields: "upc") {
			upc: String!
			name: String!
			price: Int!
		}
	`
	reviewSchema = `
		type Review {
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
		}
		
		extend type User @key(fields: "id") {
			id: ID! @external
			reviews: [Review]
		}
		
		extend type Product @key(fields: "upc") {
			upc: String! @external
			reviews: [Review]
		}

		extend type Subscription {
			review: Review!
		}
	`
	federatedSchema = `
		type Query {
			me: User
			topProducts(first: Int = 5): [Product]
		}
		
		type Subscription {
			review: Review!
		}
		
		type User {
			id: ID!
			username: String!
			reviews: [Review]
		}
		
		type Product {
			upc: String!
			name: String!
			price: Int!
			reviews: [Review]
		}
		
		type Review {
			body: String!
			product: Product!
		}
	`
)
