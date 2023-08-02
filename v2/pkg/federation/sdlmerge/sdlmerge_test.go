package sdlmerge

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

var testEntitySet = entitySet{"Mammal": {}}

func newTestNormalizer(withEntity bool) entitySet {
	if withEntity {
		return testEntitySet
	}
	return make(entitySet)
}

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
		if report.InternalErrors == nil {
			got = report.ExternalErrors[0].Message
		} else {
			got = report.InternalErrors[0].Error()
		}
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
	t.Skip("TODO: FIXME")

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
			actual, _ := operationreport.ExternalErrorMessage(err, testFormatExternalErrorMessage)
			assert.Equal(t, expectedError, actual)
		}
	}

	t.Run("should merge all sdls successfully", runMergeTest(
		federatedSchema,
		accountSchema, productSchema, reviewSchema, likeSchema, disLikeSchema, paymentSchema, onlinePaymentSchema, classicPaymentSchema,
	))

	t.Run("When merging product and review, the unresolved orphan extension for User will return an error", runMergeTestAndExpectError(
		unresolvedExtensionOrphansErrorMessage("User"),
		productSchema, reviewSchema,
	))

	t.Run("When merging product and extendsDirectives, the unresolved orphan extension for User will return an error", runMergeTestAndExpectError(
		unresolvedExtensionOrphansErrorMessage("User"),
		productSchema, extendsDirectivesSchema,
	))

	t.Run("Non-identical duplicate enums should return an error", runMergeTestAndExpectError(
		nonIdenticalSharedTypeErrorMessage("Satisfaction"),
		productSchema, negativeTestingLikeSchema,
	))

	t.Run("Non-identical duplicate unions should return an error", runMergeTestAndExpectError(
		nonIdenticalSharedTypeErrorMessage("AlphaNumeric"),
		accountSchema, negativeTestingReviewSchema,
	))

	t.Run("Entity duplicates should return an error", runMergeTestAndExpectError(
		duplicateEntityErrorMessage("User"),
		accountSchema, negativeTestingAccountSchema,
	))

	t.Run("The first type encountered without a body should return an error", runMergeTestAndExpectError(
		emptyTypeBodyErrorMessage("object", "Message"),
		accountSchema, negativeTestingProductSchema,
	))

	t.Run("Fields should merge successfully", runMergeTest(
		`
			type Mammal {
				name: String!
				age: Int!
			}
		`,
		`
			type Mammal @key(fields: "name") {
				name: String!
				age: Int!
			}
		`, `
			extend type Mammal @key(fields: "name") {
				name: String! @external
				age: Int!
			}
		`,
	))

	t.Run("Operation fields should merge successfully", runMergeTest(
		`
			type Query {
			  _service: _Service!
			}

			type _Service {
			  sdl: String
			}
		`,
		`
			type Query {
			  _service: _Service!
			}

			type _Service {
			  sdl: String
			}
		`, `
			type Query {
		  		_service: _Service!
			}

			type _Service {
			  sdl: String
			}
		`,
	))

	t.Run("Non-identical fields should fail to merge #1", runMergeTestAndExpectError(
		unmergableDuplicateFieldsErrorMessage("age", "Mammal", "Int", "String"),
		`
			type Mammal @key(fields: "name") {
				name: String!
				age: Int
			}
		`, `
			extend type Mammal @key(fields: "name") {
				name: String! @external
				age: String
			}
		`,
	))

	t.Run("Non-identical fields should fail to merge #2", runMergeTestAndExpectError(
		unmergableDuplicateFieldsErrorMessage("age", "Mammal", "Int!", "String!"),
		`
			type Mammal @key(fields: "name") {
				name: String!
				age: Int!
			}
		`, `
			extend type Mammal @key(fields: "name") {
				name: String! @external
				age: String!
			}
		`,
	))

	t.Run("Non-dentical fields should fail to merge #3", runMergeTestAndExpectError(
		unmergableDuplicateFieldsErrorMessage("ages", "Mammal", "[Int!]!", "[String!]!"),
		`
			type Mammal @key(fields: "name") {
				name: String!
				ages: [Int!]!
			}
		`, `
			extend type Mammal @key(fields: "name") {
				name: String! @external
				ages: [String!]!
			}
		`,
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

	negativeTestingAccountSchema = `
		extend type Query {
			me: User
		}

		union AlphaNumeric = Int | String | Float

		scalar DateTime

		scalar CustomScalar

		type User {
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
			UNHAPPY,
			HAPPY,
			NEUTRAL,
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

		interface ProductInfo {
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
		}

		scalar BigInt
		
		type Product implements ProductInfo @key(fields: "upc") {
			upc: String!
			name: String!
			price: Int!
			worth: BigInt!
			reputation: CustomScalar!
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
		}

		union AlphaNumeric = Int | String | Float
	`

	negativeTestingProductSchema = `
		enum Satisfaction {
			UNHAPPY,
			HAPPY,
			NEUTRAL,
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

		interface ProductInfo {
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
		}

		type Message {
		}

		scalar BigInt
		
		type Product implements ProductInfo @key(fields: "upc") {
			upc: String!
			name: String!
			price: Int!
			worth: BigInt!
			reputation: CustomScalar!
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
		}
		
		extend type Message {
			content: String!
		}

		union AlphaNumeric = Int | String | Float
	`
	reviewSchema = `
		scalar DateTime
		union AlphaNumeric = Int | String | Float

		input ReviewInput {
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
			updated: DateTime!
			inputType: AlphaNumeric!
		}

		type Review {
			id: ID!
			created: DateTime!
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
			updated: DateTime!
			inputType: AlphaNumeric!
		}
		
		type Query {
			getReview(id: ID!): Review
		}

		type Mutation {
			createReview(input: ReviewInput): Review
			updateReview(id: ID!, input: ReviewInput): Review
		}
		
		enum Department {
			GROCERIES,
			COSMETICS,
			ELECTRONICS,
		}

		extend type User @key(fields: "id") {
			id: ID! @external
			reviews: [Review]
		}

		scalar BigInt

		extend type Product implements ProductInfo @key(fields: "upc") {
			upc: String! @external
			name: String! @external
			reviews: [Review] @requires(fields: "name")
			sales: BigInt!
		}

		enum Satisfaction {
			HAPPY,
			NEUTRAL,
			UNHAPPY,
		}

		extend type Subscription {
			review: Review!
		}

		interface ProductInfo {
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
		}
	`

	negativeTestingReviewSchema = `
		scalar DateTime

		input ReviewInput {
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
			updated: DateTime!
			inputType: AlphaNumeric!
		}

		type Review {
			id: ID!
			created: DateTime!
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
			updated: DateTime!
			inputType: AlphaNumeric!
		}
		
		type Query {
			getReview(id: ID!): Review
		}

		type Mutation {
			createReview(input: ReviewInput): Review
			updateReview(id: ID!, input: ReviewInput): Review
		}

		interface ProductInfo {
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
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

		type Product implements ProductInfo @key(fields: "upc") {
			upc: String!
			name: String!
			reviews: [Review] @requires(fields: "name")
			sales: BigInt!
		}

		scalar BigInt

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
		extend enum Satisfaction {
			UNHAPPY
		}

		scalar DateTime

		union AlphaNumeric = Int | String

		scalar BigInt

		interface PaymentType @extends {
			email: String!
			date: DateTime!
			amount: BigInt!
		}
		
		extend union AlphaNumeric = Float

		enum Satisfaction {
			HAPPY
			NEUTRAL
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
			getReview(id: ID!): Review
			likesCount(productID: ID!): Int!
			likes(productID: ID!): [Like]!
		}

		type Mutation {
			createReview(input: ReviewInput): Review
			updateReview(id: ID!, input: ReviewInput): Review
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

		interface ProductInfo {
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
		}
		
		scalar BigInt
		
		type Product implements ProductInfo {
			upc: String!
			name: String!
			price: Int!
			worth: BigInt!
			reputation: CustomScalar!
			departments: [Department!]!
			averageSatisfaction: Satisfaction!
			reviews: [Review]
			sales: BigInt!
		}

		input ReviewInput {
			body: String!
			author: User! @provides(fields: "username")
			product: Product!
			updated: DateTime!
			inputType: AlphaNumeric!
		}
		
		type Review {
			id: ID!
			created: DateTime!
			body: String!
			author: User!
			product: Product!
			updated: DateTime!
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
)

var testFormatExternalErrorMessage = func(report *operationreport.Report) string {
	if len(report.ExternalErrors) > 0 {
		return report.ExternalErrors[0].Message
	}
	return ""
}

func nonIdenticalSharedTypeErrorMessage(typeName string) string {
	return fmt.Sprintf("the shared type named '%s' must be identical in any subgraphs to federate", typeName)
}

func sharedTypeExtensionErrorMessage(typeName string) string {
	return fmt.Sprintf("the type named '%s' cannot be extended because it is a shared type", typeName)
}

func emptyTypeBodyErrorMessage(definitionType, typeName string) string {
	return fmt.Sprintf("the %s named '%s' is invalid due to an empty body", definitionType, typeName)
}

func unresolvedExtensionOrphansErrorMessage(typeName string) string {
	return fmt.Sprintf("the extension orphan named '%s' was never resolved in the supergraph", typeName)
}

func noKeyDirectiveErrorMessage(typeName string) string {
	return fmt.Sprintf("an extension of the entity named '%s' does not have a key directive", typeName)
}

func nonEntityExtensionErrorMessage(typeName string) string {
	return fmt.Sprintf("the extension named '%s' has a key directive but there is no entity of the same name", typeName)
}

func duplicateEntityErrorMessage(typeName string) string {
	return fmt.Sprintf("the entity named '%s' is defined in the subgraph(s) more than once", typeName)
}

func unmergableDuplicateFieldsErrorMessage(fieldName, parentName, typeOne, typeTwo string) string {
	return fmt.Sprintf("field '%s' on type '%s' is defined in multiple subgraphs "+
		"but the fields cannot be merged because the types of the fields are non-identical:\n"+
		"first subgraph: type '%s'\n second subgraph: type '%s'", fieldName, parentName, typeOne, typeTwo)
}

func Test_validateSubgraphs(t *testing.T) {
	basePath := filepath.Join(".", "testdata", "validate-subgraph")

	testcase := []struct {
		name           string
		schemaFileName string
		wantErr        bool
		errMsg         string
	}{
		{
			name:           "well-defined subgraph schema",
			schemaFileName: "well-defined.graphqls",
			wantErr:        false,
		},
		{
			name:           "well-defined subgraph schema (non-null field type)",
			schemaFileName: "well-defined-non-null.graphqls",
			wantErr:        false,
		},
		{
			name:           "a subgraph lacking the definition of 'User' type",
			schemaFileName: "lack-definition.graphqls",
			wantErr:        true,
			errMsg:         `external: Unknown type "User"`,
		},
		{
			name:           "a subgraph lacking the extend definition of 'Product' type",
			schemaFileName: "lack-extend-definition.graphqls",
			wantErr:        true,
			errMsg:         `external: Unknown type "Product"`,
		},
		{
			name:           "a subgraph lacking the definition of 'User' type (field type non-null)",
			schemaFileName: "lack-definition-non-null.graphqls",
			wantErr:        true,
			errMsg:         `external: Unknown type "User"`,
		},
		{
			name:           "a subgraph lacking the extend definition of 'Product' type (field type non-null)",
			schemaFileName: "lack-extend-definition-non-null.graphqls",
			wantErr:        true,
			errMsg:         `external: Unknown type "Product"`,
		},
	}
	for _, tt := range testcase {
		t.Run(tt.name, func(t *testing.T) {
			caseSchemaPath := filepath.Join(basePath, tt.schemaFileName)

			b, err := os.ReadFile(caseSchemaPath)
			require.NoError(t, err)

			subgraph := string(b)
			err = validateSubgraphs([]string{subgraph})

			if tt.wantErr {
				assert.Error(t, err, subgraph)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			assert.NoError(t, err, subgraph)
		})
	}
}
