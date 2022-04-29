package sdlmerge

import (
	"fmt"
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

var runAndExpectError = func(t *testing.T, visitor Visitor, operation, expectedError string) {

	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	report := operationreport.Report{}
	walker := astvisitor.NewWalker(48)

	visitor.Register(&walker)

	walker.Walk(&operationDocument, nil, &report)

	var got string
	if report.HasErrors() {
		got = report.Error()
	}

	assert.Equal(t, expectedError, got)
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

	runMergeTestAndExpectError := func(expectedError string, sdls ...string) func(t *testing.T) {
		return func(t *testing.T) {
			_, err := MergeSDLs(sdls...)

			assert.Equal(t, expectedError, err.Error())
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

	t.Run("Non-identical duplicate enums should return an error", runMergeTestAndExpectError(
		FederatingFieldlessValueTypeMergeErrorMessage("Satisfaction"),
		productSchema, negativeTestingLikeSchema,
	))

	t.Run("Non-identical duplicate unions should return an error", runMergeTestAndExpectError(
		FederatingFieldlessValueTypeMergeErrorMessage("AlphaNumeric"),
		accountSchema, negativeTestingReviewSchema,
	))
}

const (
	accountSchema = `
		extend type Query {
			me: User
		}

		union AlphaNumeric = Int | String | Float

		scalar DateTime

		scalar CustomScalar
	
		type User @key(fields: "id") {
			id: ID!
			username: String!
			created: DateTime!
			reputation: CustomScalar!
		}

		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}
	`

	productSchema = `
		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}

		scalar CustomScalar

		extend type Query {
			topProducts(first: Int = 5): [Product]
		}

		enum Department {
			COSMETICS,
			ELECTRONICS,
			GROCERIES,
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
			inputType: AlphaNumeric!
		}
		
		enum Department {
			COSMETICS,
			ELECTRONICS,
			GROCERIES,
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

		union AlphaNumeric = Int | String | Float
		
		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}

		extend type Subscription {
			review: Review!
		}
	`

	negativeTestingReviewSchema = `
		scalar DateTime

		type Review {
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
			created: DateTime!
			inputType: AlphaNumeric!
		}
		
		enum Department {
			COSMETICS,
			ELECTRONICS,
			GROCERIES,
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

		union AlphaNumeric = BigInt | String
		
		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
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
		
		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}

		type Query {
			likesCount(productID: ID!): Int!
			likes(productID: ID!): [Like]!
		}
	`
	negativeTestingLikeSchema = `
		scalar DateTime

		type Like @key(fields: "id") {
			id: ID!
			productId: ID!
			userId: ID!
			date: DateTime!
		}
		
		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
			DEVASTATED,
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

		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}

	`
	paymentSchema = `
		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}

		interface PaymentType {
			name: String!
		}
	`
	onlinePaymentSchema = `
		scalar DateTime

		union AlphaNumeric = Int | String | Float

		scalar BigInt

		interface PaymentType @extends {
			email: String!
			date: DateTime!
			amount: BigInt!
		}

		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}
	`
	classicPaymentSchema = `
		union AlphaNumeric = Int | String | Float

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

		union AlphaNumeric = Int | String | Float

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

		union AlphaNumeric = Int | String | Float

		scalar DateTime

		scalar CustomScalar
		
		type User {
			id: ID!
			username: String!
			created: DateTime!
			reputation: CustomScalar!
			reviews: [Review]
		}
		
		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}

		enum Department {
			COSMETICS,
			ELECTRONICS,
			GROCERIES,
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
			inputType: AlphaNumeric!
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
		
		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}
		
		scalar CustomScalar
		
		enum Department {
			COSMETICS,
			ELECTRONICS,
			GROCERIES,
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
		
		scalar DateTime

		type Review {
			body: String!
			author: User!
			product: Product!
			created: DateTime!
			inputType: AlphaNumeric!
		}
		
		extend type User @key(fields: "id") {
			id: ID! @external
			reviews: [Review]
		}

		union AlphaNumeric = Int | String | Float
	`

	productAndExtendsDirectivesFederatedSchema = `
		type Query {
			topProducts(first: Int = 5): [Product]
		}

		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}

		scalar CustomScalar

		enum Department {
			COSMETICS,
			ELECTRONICS,
			GROCERIES,
		}

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

		union AlphaNumeric = Int | String | Float

		extend interface PaymentType {
			name: String!
		}
	`
)

func FederatingFieldlessValueTypeErrorMessage(typeName string) string {
	return fmt.Sprintf("external: the value type named '%s' must be identical in any subgraphs to federate, locations: [], path: []", typeName)
}

func FederatingFieldlessValueTypeMergeErrorMessage(typeName string) string {
	return fmt.Sprintf("merge ast: walk: external: the value type named '%s' must be identical in any subgraphs to federate, locations: [], path: []", typeName)
}
