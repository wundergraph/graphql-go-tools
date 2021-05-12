package testsgo

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

// testSchema - represents schema definition used in reference tests
const testSchema = `
  interface Being {
    name(surname: Boolean): String
  }

  interface Mammal {
    mother: Mammal
    father: Mammal
  }

  interface Pet implements Being {
    name(surname: Boolean): String
  }

  interface Canine implements Mammal & Being {
    name(surname: Boolean): String
    mother: Canine
    father: Canine
  }

  enum DogCommand {
    SIT
    HEEL
    DOWN
  }

  type Dog implements Being & Pet & Mammal & Canine {
    name(surname: Boolean): String
    nickname: String
    barkVolume: Int
    barks: Boolean
    doesKnowCommand(dogCommand: DogCommand): Boolean
    isHouseTrained(atOtherHomes: Boolean = true): Boolean
    isAtLocation(x: Int, y: Int): Boolean
    mother: Dog
    father: Dog
  }

  type Cat implements Being & Pet {
    name(surname: Boolean): String
    nickname: String
    meows: Boolean
    meowsVolume: Int
    furColor: FurColor
  }

  union CatOrDog = Cat | Dog

  interface Intelligent {
    iq: Int
  }

  type Human implements Being & Intelligent {
    name(surname: Boolean): String
    pets: [Pet]
    relatives: [Human]
    iq: Int
  }

  type Alien implements Being & Intelligent {
    name(surname: Boolean): String
    numEyes: Int
    iq: Int
  }

  union DogOrHuman = Dog | Human

  union HumanOrAlien = Human | Alien

  enum FurColor {
    BROWN
    BLACK
    TAN
    SPOTTED
    NO_FUR
    UNKNOWN
  }

  input ComplexInput {
    requiredField: Boolean!
    nonNullField: Boolean! = false
    intField: Int
    stringField: String
    booleanField: Boolean
    stringListField: [String]
  }

  type ComplicatedArgs {
    # TODO List
    # TODO Coercion
    # TODO NotNulls
    intArgField(intArg: Int): String
    nonNullIntArgField(nonNullIntArg: Int!): String
    stringArgField(stringArg: String): String
    booleanArgField(booleanArg: Boolean): String
    enumArgField(enumArg: FurColor): String
    floatArgField(floatArg: Float): String
    idArgField(idArg: ID): String
    stringListArgField(stringListArg: [String]): String
    stringListNonNullArgField(stringListNonNullArg: [String!]): String
    complexArgField(complexArg: ComplexInput): String
    multipleReqs(req1: Int!, req2: Int!): String
    nonNullFieldWithDefault(arg: Int! = 0): String
    multipleOpts(opt1: Int = 0, opt2: Int = 0): String
    multipleOptAndReq(req1: Int!, req2: Int!, opt1: Int = 0, opt2: Int = 0): String
  }

  type QueryRoot {
    human(id: ID): Human
    alien: Alien
    dog: Dog
    cat: Cat
    pet: Pet
    catOrDog: CatOrDog
    dogOrHuman: DogOrHuman
    humanOrAlien: HumanOrAlien
    complicatedArgs: ComplicatedArgs
  }

  schema {
    query: QueryRoot
  }

  directive @onQuery on QUERY
  directive @onMutation on MUTATION
  directive @onSubscription on SUBSCRIPTION
  directive @onField on FIELD
  directive @onFragmentDefinition on FRAGMENT_DEFINITION
  directive @onFragmentSpread on FRAGMENT_SPREAD
  directive @onInlineFragment on INLINE_FRAGMENT
  directive @onVariableDefinition on VARIABLE_DEFINITION
`

// Loc - local type representing location of validation error message
type Loc struct {
	line, column uint32
}

// Err - local type representing validation error message
type Err struct {
	message   string
	locations []Loc
}

// MessageCompare - is a function which allows to check that report has an expectedErrMsg
type MessageCompare func(expectedErrMsg string)

// ResultCompare - is a function to compare report errors with expectedErrors
type ResultCompare func(expectedErrors []Err)

// ExpectValidationErrorsWithSchema - is a helper to run operation validation
// returns ResultCompare function
func ExpectValidationErrorsWithSchema(t *testing.T, schema string, rule string, queryStr string) ResultCompare {
	op := unsafeparser.ParseGraphqlDocumentString(queryStr)
	def := unsafeparser.ParseGraphqlDocumentString(schema)

	report := operationreport.Report{}
	validator := astvalidation.DefaultOperationValidator()
	validator.Validate(&op, &def, &report)

	return compareReportErrors(t, report)
}

// ExpectValidationErrors - a wrapper for ExpectValidationErrorsWithSchema which uses default testSchema
// returns ResultCompare function
func ExpectValidationErrors(t *testing.T, rule string, queryStr string) ResultCompare {
	return ExpectValidationErrorsWithSchema(t, testSchema, rule, queryStr)
}

// ExpectSDLValidationErrors - is a helper to run schema definition validation
// returns ResultCompare function
func ExpectSDLValidationErrors(t *testing.T, schema string, rule string, sdlStr string) ResultCompare {
	definition, report := astparser.ParseGraphqlDocumentString(schema)
	// require.False(t, report.HasErrors())

	// merge schema additions
	definition.Input.AppendInputBytes([]byte(sdlStr))
	parser := astparser.NewParser()
	parser.Parse(&definition, &report)

	// validate schema sdl
	report = operationreport.Report{}
	validator := astvalidation.DefaultDefinitionValidator()
	validator.Validate(&definition, &report)

	return compareReportErrors(t, report)
}

// BuildSchema - helper used in reference test.
// As we handle validation differently return same schema string
func BuildSchema(sdl string) string {
	return sdl
}

// ExpectValidationErrorMessage - is a helper to run operation validation and check single error message
// returns MessageCompare
func ExpectValidationErrorMessage(t *testing.T, schema string, queryStr string) MessageCompare {
	op := unsafeparser.ParseGraphqlDocumentString(queryStr)
	def := unsafeparser.ParseGraphqlDocumentString(schema)

	report := operationreport.Report{}
	validator := astvalidation.DefaultOperationValidator()
	validator.Validate(&op, &def, &report)

	return hasReportError(t, report)
}

// ExtendSchema - helper to extend schema with provided sdl
func ExtendSchema(schema string, sdlStr string) string {
	definition, report := astparser.ParseGraphqlDocumentString(schema)
	// TODO: handle error

	definition.Input.AppendInputBytes([]byte(sdlStr))
	parser := astparser.NewParser()
	parser.Parse(&definition, &report)
	// TODO: handle error

	res, _ := astprinter.PrintStringIndent(&definition, nil, "  ")
	// TODO: handle error
	return res
}

// externalErrors - converts external errors to simple local type Err
// convertor could be adjusted to use exact type
func externalErrors(report operationreport.Report) (out []Err) {
	for _, externalError := range report.ExternalErrors {
		var locations []Loc

		for _, location := range externalError.Locations {
			locations = append(locations, Loc{
				line:   location.Line,
				column: location.Column,
			})
		}

		out = append(out, Err{
			message:   externalError.Message,
			locations: locations,
		})
	}

	return
}

// compareReportErrors - helper returns ResultCompare function for operationreport.Report
func compareReportErrors(t *testing.T, report operationreport.Report) ResultCompare {
	return func(expectedErrors []Err) {
		fmt.Println("expectedErrors", expectedErrors)
		actualErrors := externalErrors(report)

		fmt.Println("actualErrors", actualErrors)

		assert.Equal(t, expectedErrors, actualErrors)
	}
}

// hasReportError - helper returns MessageCompare function for operationreport.Report
func hasReportError(t *testing.T, report operationreport.Report) MessageCompare {
	return func(msg string) {
		actualErrors := externalErrors(report)

		var messages []string
		for _, actualError := range actualErrors {
			messages = append(messages, actualError.message)
		}

		// TODO check that error has msg
		assert.Contains(t, messages, msg)
	}
}
