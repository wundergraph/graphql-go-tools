package sdlmerge

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
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
	runMergeTest := func(expectedSchema string, sdls ...string) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()

			got, err := MergeSDLs(sdls...)
			if err != nil {
				t.Fatal(err)
			}

			expectedOutputDocument := unsafeparser.ParseGraphqlDocumentString(expectedSchema)
			want := mustString(astprinter.PrintString(&expectedOutputDocument, nil))

			assert.Equal(t, want, got)
		}
	}

	t.Run("should merge all sdls successfully", runMergeTest(
		federatedSchema,
		accountSchema, productSchema, reviewSchema, likeSchema, disLikeSchema, paymentSchema, onlinePaymentSchema, classicPaymentSchema,
	))

	t.Run("should merge product and review sdl and leave `extend type User` in the schema", runMergeTest(
		productAndReviewFederatedSchema,
		productSchema, reviewSchema,
	))

	t.Run("should merge product and extends directives sdl and leave the type extension definition in the schema", runMergeTest(
		productAndExtendsDirectivesFederatedSchema,
		productSchema, extendsDirectivesSchema,
	))
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
			name: String! @external
			reviews: [Review] @requires(fields: "name")
		}

		extend type Subscription {
			review: Review!
		}
	`
	likeSchema = `
		type Like @key(fields: "id") {
			id: ID!
			productId: ID!
			userId: ID!
		}
		type Query {
			likesCount(productID: ID!): Int!
			likes(productID: ID!): [Like]!
		}
	`
	disLikeSchema = `
		type Like @key(fields: "id") @extends {
			id: ID! @external
			isDislike: Boolean!
		}
	`
	paymentSchema = `
		interface PaymentType {
			name: String!
		}
	`
	onlinePaymentSchema = `
		interface PaymentType @extends {
			email: String!
		}
	`
	classicPaymentSchema = `
		extend interface PaymentType {
			number: String!
		}
	`
	extendsDirectivesSchema = `
		type Comment {
			body: String!
			author: User!
		}

		type User @extends @key(fields: "id") {
			id: ID! @external
			comments: [Comment]
		}

		interface PaymentType @extends {
			name: String!
		}
	`
	federatedSchema = `
		type Query {
			me: User
			topProducts(first: Int = 5): [Product]
			likesCount(productID: ID!): Int!
			likes(productID: ID!): [Like]!
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
			author: User!
			product: Product!
		}
		type Like {
			id: ID!
			productId: ID!
			userId: ID!
			isDislike: Boolean!
		}

		interface PaymentType {
			name: String!
			email: String!
			number: String!
		}
	`

	productAndReviewFederatedSchema = `
		type Query {
			topProducts(first: Int = 5): [Product]
		}

		type Subscription {
			review: Review!
		}
		
		type Product {
			upc: String!
			name: String!
			price: Int!
			reviews: [Review]
		}

		type Review {
			body: String!
			author: User!
			product: Product!
		}
		
		extend type User @key(fields: "id") {
			id: ID! @external
			reviews: [Review]
		}
	`

	productAndExtendsDirectivesFederatedSchema = `
		type Query {
			topProducts(first: Int = 5): [Product]
		}
		
		type Product {
			upc: String!
			name: String!
			price: Int!
		}

		type Comment {
			body: String!
			author: User!
		}

		extend type User @key(fields: "id") {
			id: ID! @external
			comments: [Comment]
		}

		extend interface PaymentType {
			name: String!
		}
	`
)
