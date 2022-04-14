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

		scalar DateTime

		scalar CustomScalar
	
		type User @key(fields: "id") {
			id: ID!
			username: String!
			created: DateTime!
			reputation: CustomScalar!
		}
	`

	productSchema = `
		scalar CustomScalar

		extend type Query {
			topProducts(first: Int = 5): [Product]
		}

		scalar BigInt
		
		type Product @key(fields: "upc") {
			upc: String!
			name: String!
			price: Int!
			worth: BigInt!
			reputation: CustomScalar!
		}
	`
	reviewSchema = `
		scalar DateTime

		type Review {
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
			created: DateTime!
		}
		
		extend type User @key(fields: "id") {
			id: ID! @external
			reviews: [Review]
		}

		extend type Product @key(fields: "upc") {
			upc: String! @external
			name: String! @external
			reviews: [Review] @requires(fields: "name")
			sales: BigInt!
		}

		extend type Subscription {
			review: Review!
		}
	`
	likeSchema = `
		scalar DateTime

		type Like @key(fields: "id") {
			id: ID!
			productId: ID!
			userId: ID!
			date: DateTime!
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
		scalar DateTime

		scalar BigInt

		interface PaymentType @extends {
			email: String!
			date: DateTime!
			amount: BigInt!
		}
	`
	classicPaymentSchema = `
		scalar CustomScalar

		extend interface PaymentType {
			number: String!
			reputation: CustomScalar!
		}
	`
	extendsDirectivesSchema = `
		scalar DateTime

		type Comment {
			body: String!
			author: User!
			created: DateTime!
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

		scalar DateTime

		scalar CustomScalar
		
		type User {
			id: ID!
			username: String!
			created: DateTime!
			reputation: CustomScalar!
			reviews: [Review]
		}
		
		scalar BigInt
		
		type Product {
			upc: String!
			name: String!
			price: Int!
			worth: BigInt!
			reputation: CustomScalar!
			reviews: [Review]
			sales: BigInt!
		}
		
		type Review {
			body: String!
			author: User!
			product: Product!
			created: DateTime!
		}
		type Like {
			id: ID!
			productId: ID!
			userId: ID!
			date: DateTime!
			isDislike: Boolean!
		}

		interface PaymentType {
			name: String!
			email: String!
			date: DateTime!
			amount: BigInt!
			number: String!
			reputation: CustomScalar!
		}
	`

	productAndReviewFederatedSchema = `
		type Query {
			topProducts(first: Int = 5): [Product]
		}

		type Subscription {
			review: Review!
		}
		
		scalar CustomScalar
		
		scalar BigInt

		type Product {
			upc: String!
			name: String!
			price: Int!
			worth: BigInt!
			reputation: CustomScalar!
			reviews: [Review]
			sales: BigInt!
		}
		
		scalar DateTime

		type Review {
			body: String!
			author: User!
			product: Product!
			created: DateTime!
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

		scalar CustomScalar

		scalar BigInt
		
		type Product {
			upc: String!
			name: String!
			price: Int!
			worth: BigInt!
			reputation: CustomScalar!
		}

	scalar DateTime

		type Comment {
			body: String!
			author: User!
			created: DateTime!
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
