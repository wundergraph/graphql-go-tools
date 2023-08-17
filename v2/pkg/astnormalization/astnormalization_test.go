package astnormalization

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestNormalizeOperation(t *testing.T) {

	run := func(t *testing.T, definition, operation, expectedOutput, variablesInput, expectedVariables string) {
		t.Helper()

		definitionDocument := unsafeparser.ParseGraphqlDocumentString(definition)
		require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definitionDocument))

		operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
		expectedOutputDocument := unsafeparser.ParseGraphqlDocumentString(expectedOutput)
		report := operationreport.Report{}

		if variablesInput != "" {
			operationDocument.Input.Variables = []byte(variablesInput)
		}

		normalizer := NewWithOpts(
			WithInlineFragmentSpreads(),
			WithExtractVariables(),
			WithRemoveFragmentDefinitions(),
			WithRemoveUnusedVariables(),
			WithNormalizeDefinition(),
		)
		normalizer.NormalizeOperation(&operationDocument, &definitionDocument, &report)

		if report.HasErrors() {
			t.Fatal(report.Error())
		}

		got := mustString(astprinter.PrintString(&operationDocument, &definitionDocument))
		want := mustString(astprinter.PrintString(&expectedOutputDocument, &definitionDocument))

		assert.Equal(t, want, got)
		assert.Equal(t, expectedVariables, string(operationDocument.Input.Variables))
	}

	t.Run("complex", func(t *testing.T) {
		run(t, testDefinition, `	
				subscription sub {
					... multipleSubscriptions
					... on Subscription {
						newMessage {
							body
							sender
						}	
					}
				}
				fragment newMessageFields on Message {
					body: body
					sender
					... on Body {
						body
					}
				}
				fragment multipleSubscriptions on Subscription {
					newMessage {
						body
						sender
					}
					newMessage {
						... newMessageFields
					}
					newMessage {
						body
						body
						sender
					}
					... on Subscription {
						newMessage {
							body
							sender
						}	
					}
					disallowedSecondRootField
				}`, `
				subscription sub {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
				}`, "", "")
	})
	t.Run("inject default", func(t *testing.T) {
		run(t,
			injectDefaultValueDefinition, `
			query{elQuery(input:{fieldB: "dupa"})}`,
			`query($a: elInput){elQuery(input: $a)}`, "",
			`{"a":{"fieldB":"dupa","fieldA":"VALUE_A"}}`,
		)
	})
	t.Run("fragments", func(t *testing.T) {
		run(t, testDefinition, `
				query conflictingBecauseAlias ($unused: String) {
					dog {
						extras { ...frag }
						extras { ...frag2 }
					}
				}
				fragment frag on DogExtra { string1 }
				fragment frag2 on DogExtra { string1: string }`, `
				query conflictingBecauseAlias {
					dog {
						extras {
							string1
							string1: string
						}
					}
				}`, `{"unused":"foo"}`, `{}`)
	})
	t.Run("fragments", func(t *testing.T) {
		run(t, variablesExtractionDefinition, `
			mutation HttpBinPost{
			  httpBinPost(input: {foo: "bar"}){
				headers {
				  userAgent
				}
				data {
				  foo
				}
			  }
			}`, `
			mutation HttpBinPost($a: HttpBinPostInput){
			  httpBinPost(input: $a){
				headers {
				  userAgent
				}
				data {
				  foo
				}
			  }
			}`, ``, `{"a":{"foo":"bar"}}`)
	})
	t.Run("type extensions", func(t *testing.T) {
		run(t, typeExtensionsDefinition, `
			{
				findUserByLocation(loc: {lat: 1.000, lon: 2.000, planet: "EARTH"}) {
					id
					name
					age
					type {
						... on TrialUser {
							__typename
							enabled
						}
						... on SubscribedUser {
							__typename
							subscription
						}
					}
					metadata
				}
			}`, `query($a: Location){
				findUserByLocation(loc: $a) {
					id
					name
					age
					type {
						... on TrialUser {
							__typename
							enabled
						}
						... on SubscribedUser {
							__typename
							subscription
						}
					}
					metadata
				}
			}`,
			`{"a": {"lat": 1.000, "lon": 2.000, "planet": "EARTH"}}`,
			`{"a": {"lat":1.000,"lon":2.000,"planet":"EARTH"}}`)
	})
	t.Run("use extended Query without explicit schema definition", func(t *testing.T) {
		run(t, extendedRootOperationTypeDefinition, `
			{
				me
			}`, `{
				me
			}`, ``, ``)
	})
	t.Run("use extended Mutation without explicit schema definition", func(t *testing.T) {
		run(t, extendedRootOperationTypeDefinition, `
			mutation {
				increaseTextCounter
			}`, `mutation {
				increaseTextCounter
			}`, ``, ``)
	})
	t.Run("use extended Subscription without explicit schema definition", func(t *testing.T) {
		run(t, extendedRootOperationTypeDefinition, `
			subscription {
				textCounter
			}`, `subscription {
				textCounter
			}`, ``, ``)
	})

	t.Run("default values", func(t *testing.T) {
		run(t, testDefinition, `
			query {
				simple
			}`, `query($a: String) {
			  simple(input: $a)
			}`, ``, `{"a":"foo"}`)
	})
	t.Run("input list coercion inline", func(t *testing.T) {
		run(t, inputCoercionForListDefinition, `
			query Foo {
			  inputWithList(input: {list:{foo:"bar",list:{foo:"bar2",list:{nested:{foo:"bar3",list:{foo:"bar4"}}}}}}) {
				id
				name
			  }
			}`, `query Foo($a: InputWithList) {
			  inputWithList(input: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":{"list":[{"foo":"bar","list":[{"foo":"bar2","list":[{"nested":{"foo":"bar3","list":[{"foo":"bar4"}]}}]}]}]}}`)
	})
	t.Run("input list coercion with extracted variables", func(t *testing.T) {
		run(t, inputCoercionForListDefinition, `
			query ($input: InputWithListNestedList) {
			  inputWithListNestedList(input: $input) {
				id
				name
			  }
			}`, `query ($input: InputWithListNestedList) {
			  inputWithListNestedList(input: $input) {
				id
				name
			  }
			}`, `{"input":{"doubleList":{"foo":"bar","list":{"foo":"bar2","list":{"nested":{"foo":"bar3","list":{"foo":"bar4"}}}}}}}`,
			`{"input":{"doubleList":[[{"foo":"bar","list":[{"foo":"bar2","list":[{"nested":{"foo":"bar3","list":[{"foo":"bar4"}]}}]}]}]]}}`)
	})
}

func TestOperationNormalizer_NormalizeOperation(t *testing.T) {
	t.Run("should return an error once on normalization with missing field", func(t *testing.T) {
		schema := `
type Query {
	country: Country!
}

type Country {
	name: String!
}

schema {
    query: Query
}
`

		query := `
{
	country {
		nam
	}
}
`
		definition := unsafeparser.ParseGraphqlDocumentString(schema)
		operation := unsafeparser.ParseGraphqlDocumentString(query)

		report := operationreport.Report{}
		normalizer := NewNormalizer(true, true)
		normalizer.NormalizeOperation(&operation, &definition, &report)

		assert.True(t, report.HasErrors())
		assert.Equal(t, 1, len(report.ExternalErrors))
		assert.Equal(t, 0, len(report.InternalErrors))
		assert.Equal(t, "external: field: nam not defined on type: Country, locations: [], path: [query,country,nam]", report.Error())
	})
}

func TestOperationNormalizer_NormalizeNamedOperation(t *testing.T) {
	t.Run("should properly remove fragments and unmatched query", func(t *testing.T) {
		schema := `
			type Query {
				items: Attributes
			}
		
			type Attribute {
				name: String
				childAttributes: [Attribute]
			}
			
			type Attributes {
				name: String
				childAttributes: [Attribute]
			}`

		query := `
			query Items {
				items {
					...AttributesFragment
				}
			}
			query OtherItems {
				items {
					...AttributesFragment
				}
			}
			fragment AttributesFragment on Attributes {
				name
				childAttributes {
					...AttributeFragment
					childAttributes {
						...AttributeFragment
					}
				}
			}
			fragment AttributeFragment on Attribute {
				name
				childAttributes {
					name
				}
			}`

		expectedQuery := `query Items {
  items {
    name
    childAttributes {
      name
      childAttributes {
        name
        childAttributes {
          name
        }
      }
    }
  }
}`

		definition := unsafeparser.ParseGraphqlDocumentString(schema)
		require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definition))
		operation := unsafeparser.ParseGraphqlDocumentString(query)

		report := operationreport.Report{}
		NormalizeNamedOperation(&operation, &definition, []byte("Items"), &report)
		assert.False(t, report.HasErrors())

		actual, _ := astprinter.PrintStringIndent(&operation, &definition, " ")
		assert.Equal(t, expectedQuery, actual)
	})
}

func TestNewNormalizer(t *testing.T) {
	schema := `
scalar String

type Query {
	country: Country!
}

type Country {
	name: String!
}

schema {
    query: Query
}
`
	query := `fragment Fields on Country {name} query Q {country {...Fields}}`

	runNormalization := func(t *testing.T, removeFragmentDefinitions bool, expectedOperation string) {
		t.Helper()

		definition := unsafeparser.ParseGraphqlDocumentString(schema)
		operation := unsafeparser.ParseGraphqlDocumentString(query)

		report := operationreport.Report{}
		normalizer := NewNormalizer(removeFragmentDefinitions, true)
		normalizer.NormalizeOperation(&operation, &definition, &report)
		assert.False(t, report.HasErrors())
		fmt.Println(report)

		actualOperation := unsafeprinter.Print(&operation, nil)
		assert.NotEqual(t, query, actualOperation)
		assert.Equal(t, expectedOperation, actualOperation)
	}

	t.Run("should respect remove fragment definitions option", func(t *testing.T) {
		t.Run("when remove fragments: true", func(t *testing.T) {
			runNormalization(t, true, `query Q {country {name}}`)
		})

		t.Run("when remove fragments: false", func(t *testing.T) {
			runNormalization(t, false, `fragment Fields on Country {name} query Q {country {name}}`)
		})
	})
}

func TestParseMissingBaseSchema(t *testing.T) {
	const (
		schema = `type Query {
			hello: String!
		}`

		query = `query { hello }`
	)
	definition, report := astparser.ParseGraphqlDocumentString(schema)
	assert.False(t, report.HasErrors(), report.Error())
	doc := ast.NewDocument()
	doc.Input.ResetInputString(query)
	astparser.NewParser().Parse(doc, &report)
	assert.False(t, report.HasErrors(), report.Error())
	normalizer := NewNormalizer(false, false)
	normalizer.NormalizeOperation(doc, &definition, &report)
	assert.True(t, report.HasErrors(), "normalization should report an error")
	assert.Regexp(t, regexp.MustCompile("forget.*merge.*base.*schema"), report.Error(), "error should mention the user forgot to merge the base schema")
}

func BenchmarkAstNormalization(b *testing.B) {

	definition := unsafeparser.ParseGraphqlDocumentString(testDefinition)
	operation := unsafeparser.ParseGraphqlDocumentString(testOperation)
	report := operationreport.Report{}

	normalizer := NewNormalizer(false, false)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		report.Reset()
		normalizer.NormalizeOperation(&operation, &definition, &report)
	}
}

var mustString = func(str string, err error) string {
	if err != nil {
		panic(err)
	}
	return str
}

type registerNormalizeFunc func(walker *astvisitor.Walker)
type registerNormalizeVariablesFunc func(walker *astvisitor.Walker) *variablesExtractionVisitor
type registerNormalizeVariablesDefaulValueFunc func(walker *astvisitor.Walker) *variablesDefaultValueExtractionVisitor
type registerNormalizeDeleteVariablesFunc func(walker *astvisitor.Walker) *deleteUnusedVariablesVisitor

var runWithVariablesAssert = func(t *testing.T, registerVisitor func(walker *astvisitor.Walker), definition, operation, operationName, expectedOutput, variablesInput, expectedVariables string, additionalNormalizers ...registerNormalizeFunc) {
	t.Helper()

	definitionDocument := unsafeparser.ParseGraphqlDocumentString(definition)
	err := asttransform.MergeDefinitionWithBaseSchema(&definitionDocument)
	if err != nil {
		panic(err)
	}

	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	expectedOutputDocument := unsafeparser.ParseGraphqlDocumentString(expectedOutput)
	report := operationreport.Report{}
	walker := astvisitor.NewWalker(48)

	if variablesInput != "" {
		operationDocument.Input.Variables = []byte(variablesInput)
	}

	registerVisitor(&walker)

	for _, fn := range additionalNormalizers {
		fn(&walker)
	}

	walker.Walk(&operationDocument, &definitionDocument, &report)
	// we run this walker twice because some normalizers may depend on other normalizers
	// walking twice ensures that all prerequisites are met
	// additionally, walking twice also ensures that the normalizers are idempotent
	walker.Walk(&operationDocument, &definitionDocument, &report)

	if report.HasErrors() {
		panic(report.Error())
	}

	actualAST := mustString(astprinter.PrintString(&operationDocument, &definitionDocument))
	expectedAST := mustString(astprinter.PrintString(&expectedOutputDocument, &definitionDocument))
	assert.Equal(t, expectedAST, actualAST)
	actualVariables := string(operationDocument.Input.Variables)
	assert.Equal(t, expectedVariables, actualVariables)
}

var runWithVariables = func(t *testing.T, normalizeFunc registerNormalizeVariablesFunc, definition, operation, operationName, expectedOutput, variablesInput, expectedVariables string, additionalNormalizers ...registerNormalizeFunc) {
	t.Helper()

	runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
		visitor := normalizeFunc(walker)
		visitor.operationName = []byte(operationName)
	}, definition, operation, operationName, expectedOutput, variablesInput, expectedVariables, additionalNormalizers...)
}

var runWithVariablesDefaultValues = func(t *testing.T, normalizeFunc registerNormalizeVariablesDefaulValueFunc, definition, operation, operationName, expectedOutput, variablesInput, expectedVariables string) {
	t.Helper()

	runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
		visitor := normalizeFunc(walker)
		visitor.operationName = []byte(operationName)
	}, definition, operation, operationName, expectedOutput, variablesInput, expectedVariables)
}

var runWithDeleteUnusedVariables = func(t *testing.T, normalizeFunc registerNormalizeDeleteVariablesFunc, definition, operation, operationName, expectedOutput, variablesInput, expectedVariables string) {
	t.Helper()

	runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
		visitor := normalizeFunc(walker)
		visitor.operationName = []byte(operationName)
	}, definition, operation, operationName, expectedOutput, variablesInput, expectedVariables)
}

var run = func(t *testing.T, normalizeFunc registerNormalizeFunc, definition, operation, expectedOutput string, indent ...bool) {

	definitionDocument := unsafeparser.ParseGraphqlDocumentString(definition)
	err := asttransform.MergeDefinitionWithBaseSchema(&definitionDocument)
	if err != nil {
		panic(err)
	}

	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	expectedOutputDocument := unsafeparser.ParseGraphqlDocumentString(expectedOutput)
	report := operationreport.Report{}
	walker := astvisitor.NewWalker(48)

	normalizeFunc(&walker)

	walker.Walk(&operationDocument, &definitionDocument, &report)

	if report.HasErrors() {
		panic(report.Error())
	}

	var got, want string
	if len(indent) > 0 && indent[0] {
		got = mustString(astprinter.PrintStringIndent(&operationDocument, &definitionDocument, "  "))
		want = mustString(astprinter.PrintStringIndent(&expectedOutputDocument, &definitionDocument, "  "))
	} else {
		got = mustString(astprinter.PrintString(&operationDocument, &definitionDocument))
		want = mustString(astprinter.PrintString(&expectedOutputDocument, &definitionDocument))
	}

	assert.Equal(t, want, got)
}

var runWithExpectedErrors = func(t *testing.T, normalizeFunc registerNormalizeVariablesFunc, definition, operation, expectedError string, additionalNormalizers ...registerNormalizeFunc) {
	t.Helper()

	definitionDocument := unsafeparser.ParseGraphqlDocumentString(definition)
	err := asttransform.MergeDefinitionWithBaseSchema(&definitionDocument)
	if err != nil {
		panic(err)
	}

	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	report := operationreport.Report{}
	walker := astvisitor.NewWalker(48)

	normalizeFunc(&walker)

	for _, fn := range additionalNormalizers {
		fn(&walker)
	}

	walker.Walk(&operationDocument, &definitionDocument, &report)
	// we run this walker twice because some normalizers may depend on other normalizers
	// walking twice ensures that all prerequisites are met
	// additionally, walking twice also ensures that the normalizers are idempotent
	walker.Walk(&operationDocument, &definitionDocument, &report)

	assert.True(t, report.HasErrors())
	assert.Condition(t, func() bool {
		for i := range report.InternalErrors {
			if report.InternalErrors[i].Error() == expectedError {
				return true
			}
		}
		return false
	})
}

func runMany(t *testing.T, definition, operation, expectedOutput string, normalizeFuncs ...registerNormalizeFunc) {
	var runManyNormalizers = func(walker *astvisitor.Walker) {
		for _, normalizeFunc := range normalizeFuncs {
			normalizeFunc(walker)
		}
	}

	run(t, runManyNormalizers, definition, operation, expectedOutput)
}

func runManyOnDefinition(t *testing.T, definition, expectedOutput string, normalizeFuncs ...registerNormalizeFunc) {
	runMany(t, "", definition, expectedOutput, normalizeFuncs...)
}

const testOperation = `	
subscription sub {
	... multipleSubscriptions
	... on Subscription {
		newMessage {
			body
			sender
		}	
	}
}
fragment newMessageFields on Message {
	body: body
	sender
	... on Body {
		body
	}
}
fragment multipleSubscriptions on Subscription {
	newMessage {
		body
		sender
	}
	newMessage {
		... newMessageFields
	}
	newMessage {
		body
		body
		sender
	}
	... on Subscription {
		newMessage {
			body
			sender
		}	
	}
	disallowedSecondRootField
}`

const testDefinition = `
schema {
	query: Query
	subscription: Subscription
}

interface Body {
	body: String
}

type Message implements Body {
	body: String
	sender: String
}

type Subscription {
	newMessage: Message
	disallowedSecondRootField: Boolean
	frag2Field: String
}

input ComplexInput { name: String, owner: String }
input ComplexNonOptionalInput { name: String! }

type Field {
	subfieldA: String
	subfieldB: String
}

type Query {
	human: Human
  	pet: Pet
  	dog: Dog
	cat: Cat
	catOrDog: CatOrDog
	dogOrHuman: DogOrHuman
	humanOrAlien: HumanOrAlien
	arguments: ValidArguments
	findDog(complex: ComplexInput): Dog
	findDogNonOptional(complex: ComplexNonOptionalInput): Dog
  	booleanList(booleanListArg: [Boolean!]): Boolean
	extra: Extra
	field: Field
	simple(input: String = "foo"): String
}

type ValidArguments {
	multipleReqs(x: Int!, y: Int!): Int!
	booleanArgField(booleanArg: Boolean): Boolean
	floatArgField(floatArg: Float): Float
	intArgField(intArg: Int): Int
	nonNullBooleanArgField(nonNullBooleanArg: Boolean!): Boolean!
	booleanListArgField(booleanListArg: [Boolean]!): [Boolean]
	optionalNonNullBooleanArgField(optionalBooleanArg: Boolean! = false): Boolean!
}

enum DogCommand { SIT, DOWN, HEEL }

type Dog implements Pet {
	name: String!
	nickname: String
	barkVolume: Int
	doesKnowCommand(dogCommand: DogCommand!): Boolean!
	isHousetrained(atOtherHomes: Boolean): Boolean!
	owner: Human
	extra: DogExtra
	extras: [DogExtra]
	mustExtra: DogExtra!
	mustExtras: [DogExtra]!
	mustMustExtras: [DogExtra!]!
	doubleNested: Boolean
	nestedDogName: String
}

type DogExtra {
	string: String
	string1: String
	strings: [String]
	mustStrings: [String]!
	bool: Int
	noString: Boolean
}

interface Sentient {
  name: String!
}

interface Pet {
  name: String!
}

type Alien implements Sentient {
  name: String!
  homePlanet: String
}

type Human implements Sentient {
  name: String!
}

enum CatCommand { JUMP }

type Cat implements Pet {
	name: String!
	nickname: String
	doesKnowCommand(catCommand: CatCommand!): Boolean!
	meowVolume: Int
	extra: CatExtra
}

type CatExtra {
	string: String
	string2: String
	strings: [String]
	mustStrings: [String]!
	bool: Boolean
}

union CatOrDog = Cat | Dog
union DogOrHuman = Dog | Human
union HumanOrAlien = Human | Alien
union Extra = CatExtra | DogExtra`

const typeExtensionsDefinition = `
schema { query: Query }

extend scalar JSONPayload
extend union UserType = TrialUser | SubscribedUser

extend type Query {
	findUserByLocation(loc: Location): [User]
}

extend interface Entity {
	id: ID
}

type User {
	name: String
}

type TrialUser {
	enabled: Boolean
}

type SubscribedUser {
	subscription: SubscriptionType
}

enum SubscriptionType {
	BASIC
	PRO
	ULTIMATE
}

extend type User implements Entity {
	id: ID
	age: Int
	type: UserType
	metadata: JSONPayload
}

extend enum Planet {
	EARTH
	MARS
}

extend input Location {
	lat: Float 
	lon: Float
	planet: Planet
}
`

const extendedRootOperationTypeDefinition = `
extend type Query {
	me: String
}
extend type Mutation {
	increaseTextCounter: String
}
extend type Subscription {
	textCounter: String
}
`
const injectDefaultValueDefinition = `
type Query {
  elQuery(input: elInput): Boolean!
}

type Mutation{
  elMutation(input: elInput!): Boolean!
}

input elInput{
  fieldA: MyEnum! = VALUE_A
  fieldB: String
}

enum MyEnum {
	VALUE_A
	VALUE_B
}
`
