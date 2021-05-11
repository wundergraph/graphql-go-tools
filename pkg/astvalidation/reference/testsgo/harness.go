package testsgo

import (
	"fmt"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

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

type Loc struct {
	line, column uint32
}

type Err struct {
	message   string
	locations []Loc
}

type MessageCompare func(msg string)
type ResultCompare func(result []Err)

func ExpectValidationErrorsWithSchema(schema string, rule string, queryStr string) ResultCompare {
	op := unsafeparser.ParseGraphqlDocumentString(queryStr)
	def := unsafeparser.ParseGraphqlDocumentString(schema)

	report := operationreport.Report{}
	validator := astvalidation.DefaultOperationValidator()
	validator.Validate(&op, &def, &report)

	return compareReportErrors(report)
}

func ExpectValidationErrors(rule string, queryStr string) ResultCompare {
	return ExpectValidationErrorsWithSchema(testSchema, rule, queryStr)
}

func ExpectSDLValidationErrors(schema string, rule string, sdlStr string) ResultCompare {
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

	return compareReportErrors(report)
}

func BuildSchema(sdl string) string {
	return sdl
}

func ExpectErrorMessage(schema string, queryStr string) MessageCompare {
	op := unsafeparser.ParseGraphqlDocumentString(queryStr)
	def := unsafeparser.ParseGraphqlDocumentString(schema)

	report := operationreport.Report{}
	validator := astvalidation.DefaultOperationValidator()
	validator.Validate(&op, &def, &report)

	return hasReportError(report)
}

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

// externalErrors - converted external errors to simple local type
// converted could be adjusted to use exact type
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

func compareReportErrors(report operationreport.Report) ResultCompare {
	return func(expectedErrors []Err) {
		fmt.Println("expectedErrors", expectedErrors)
		actualErrors := externalErrors(report)

		fmt.Println("actualErrors", actualErrors)
	}
}

func hasReportError(report operationreport.Report) MessageCompare {
	return func(msg string) {
		actualErrors := externalErrors(report)

		var messages []string
		for _, actualError := range actualErrors {
			messages = append(messages, actualError.message)
		}

		// TODO check that error has msg
	}
}
