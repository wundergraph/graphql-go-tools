package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

//go:generate rm -rf ./out
//go:generate mkdir out
//go:generate go run main.go
//go:generate gofmt -w out

const (
	inputDir = "./__tests__"
	outDir   = "./out"

	header = `
package out

import (
  "testing"

  "github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)
`

	buildAssertion = `function buildAssertion(sdlStr: string) {
  const schema = buildSchema(sdlStr);
  return { expectErrors, expectValid };

  function expectErrors(queryStr: string) {
    return expectValidationErrorsWithSchema(
      schema,
      NoDeprecatedCustomRule,
      queryStr,
    );
  }

  function expectValid(queryStr: string) {
    expectErrors(queryStr).to.deep.equal([]);
  }
}`
	buildAssertionGo = `
type AssertQuery func(queryStr string) helpers.ResultCompare

func buildAssertion(sdlStr string) (AssertQuery, func(queryStr string)){
  schema := helpers.BuildSchema(sdlStr)

  expectErrors := func(queryStr string) helpers.ResultCompare {
    return helpers.ExpectValidationErrorsWithSchema(
      schema,
      "NoDeprecatedCustomRule",
      queryStr,
    )
  }

  expectValid := func(queryStr string) {
    expectErrors(queryStr)("[]")
  }

  return expectErrors, expectValid
}`

	expectErrMsgJs = `
    function expectErrorMessage(schema: GraphQLSchema, queryStr: string) {
      const errors = validate(schema, parse(queryStr), [
        FieldsOnCorrectTypeRule,
      ]);
      expect(errors.length).to.equal(1);
      return expect(errors[0].message);
    }`

	expectErrMsgGo = `
    expectErrorMessage := func(schema string, queryStr string) func(string) {
    }`
)

func main() {
	currDir, _ := os.Getwd()
	println(currDir)

	workingDir := inputDir
	if !strings.Contains(currDir, "reference") {
		workingDir = filepath.Join(currDir, "pkg/astvalidation/reference/__tests__")
	}

	dir, err := ioutil.ReadDir(workingDir)
	if err != nil {
		log.Fatal(err)
	}

	for _, fileInfo := range dir {
		processFile(workingDir, fileInfo.Name())
	}
}

var (
	jsArrowFunction = ", () => {"

	// convertRules = []string{
	// 	"ExecutableDefinitionsRule",
	// 	"FieldsOnCorrectTypeRule",
	// 	"FragmentsOnCompositeTypesRule",
	// 	"KnownArgumentNamesRule",
	// 	"KnownDirectivesRule",
	// 	"KnownFragmentNamesRule",
	// 	// "KnownTypeNamesRule",
	// 	"LoneAnonymousOperationRule",
	// 	// "LoneSchemaDefinitionRule",
	// 	// "NoDeprecatedCustomRule",
	// 	"NoFragmentCyclesRule",
	// 	"NoSchemaIntrospectionCustomRule",
	// 	"NoUndefinedVariablesRule",
	// 	"NoUnusedFragmentsRule",
	// 	"NoUnusedVariablesRule",
	// 	// "OverlappingFieldsCanBeMergedRule",
	// 	"PossibleFragmentSpreadsRule",
	// 	// "PossibleTypeExtensionsRule",
	// 	// "ProvidedRequiredArgumentsRule",
	// 	"ScalarLeafsRule",
	// 	"SingleFieldSubscriptionsRule",
	// 	"UniqueArgumentNamesRule",
	// 	// "UniqueDirectiveNamesRule",
	// 	// "UniqueDirectivesPerLocationRule",
	// 	// "UniqueEnumValueNamesRule",
	// 	// "UniqueFieldDefinitionNamesRule",
	// 	"UniqueFragmentNamesRule",
	// 	"UniqueInputFieldNamesRule",
	// 	"UniqueOperationNamesRule",
	// 	// "UniqueOperationTypesRule",
	// 	// "UniqueTypeNamesRule",
	// 	"UniqueVariableNamesRule",
	// 	// "ValuesOfCorrectTypeRule",
	// 	"VariablesAreInputTypesRule",
	// 	// "VariablesInAllowedPositionRule",
	// 	// "validation",
	// }
	convertRules = []string{
		// "FieldsOnCorrectTypeRule",
		// "NoDeprecatedCustomRule", // syntax ok
		// "PossibleTypeExtensionsRule", // syntax ok
		"ValuesOfCorrectTypeRule", // need a skip
		// "VariablesInAllowedPositionRule", // OK
		// "validation",
	}
)

func skipRule(name string) bool {
	for _, rule := range convertRules {
		if rule == name {
			return false
		}
	}
	return true
}

func processFile(workingDir string, filename string) {
	fPath := filepath.Join(workingDir, filename)
	content, _ := ioutil.ReadFile(fPath)

	testName := strings.TrimSuffix(strings.Split(filepath.Base(filename), ".")[0], "-test")

	if skipRule(testName) {
		return
	}

	converter := &Converter{}
	result := converter.iterateLines(testName, string(content))

	outFileName := testName + "_test.go"
	ioutil.WriteFile(filepath.Join(outDir, outFileName), []byte(result), os.ModePerm)
}

type Converter struct {
	insideImport          bool
	insideMultilineString bool
	insideResultAssertion bool
	lineNumber            int
}

func (c *Converter) iterateLines(testName string, content string) string {
	var outLines []string

	hasBuildAss := strings.Contains(content, buildAssertion)
	hasExpectErr := strings.Contains(content, expectErrMsgJs)
	hasIgnoreTest := strings.Contains(content, ignoreTest)

	hh := ignoreTest
	_ = hh

	if hasIgnoreTest {
		content = strings.ReplaceAll(content, ignoreTest, "")
	}

	if hasBuildAss {
		content = strings.ReplaceAll(content, buildAssertion, "")
		content = strings.ReplaceAll(content, "{ expectValid, expectErrors }", "expectValid, expectErrors")
	}

	if hasExpectErr {
		content = strings.ReplaceAll(content, expectErrMsgJs, expectErrMsgGo)
		content = strings.ReplaceAll(content, ".to.equal", "")
	}

	content = strings.ReplaceAll(content, ";", "")
	lines := strings.Split(content, "\n")

	outLines = append(outLines, header)
	outLines = append(outLines, fmt.Sprintf("func Test%s(t *testing.T) {", testName))

	for i, line := range lines {
		c.lineNumber = i + 1
		transformedLine, skip := c.transformLine(line)
		if !skip {
			outLines = append(outLines, transformedLine)
		}
	}

	outLines = append(outLines, "}")

	if hasBuildAss {
		outLines = append(outLines, buildAssertionGo)
	}

	return strings.Join(outLines, "\n")
}

func (c *Converter) transformLine(line string) (out string, skip bool) {
	switch {
	case strings.Contains(line, `'`):
		if strings.Contains(line, `"`) {
			if !c.insideResultAssertion {
				transformedLine := strings.ReplaceAll(line, `'`, "`")
				out, skip = c.transformLine(transformedLine)
			} else {
				out = line
			}
		} else {
			transformedLine := strings.ReplaceAll(line, `'`, `"`)
			out, skip = c.transformLine(transformedLine)
		}

	case strings.Contains(line, "import { "):
		return "", true

	case strings.Contains(line, "import {"):
		c.insideImport = true
		return "", true

	case strings.Contains(line, "} from"):
		c.insideImport = false
		return "", true

	case strings.Contains(line, "const "):
		parts := strings.Split(line, "=")
		variableName := strings.TrimPrefix(strings.TrimSpace(parts[0]), "const")
		transformedLine := fmt.Sprintf("%s := %s", variableName, parts[1])
		out, skip = c.transformLine(transformedLine)

	case strings.Contains(line, "describe("):
		name := strings.TrimSuffix(strings.ReplaceAll(line, "describe(", ""), jsArrowFunction)
		out = fmt.Sprintf(`t.Run(%s, func(t *testing.T) {`, name)

	case strings.Contains(line, "it("):
		name := strings.TrimSuffix(strings.ReplaceAll(line, "it(", ""), jsArrowFunction)
		out = fmt.Sprintf(`t.Run(%s, func(t *testing.T) {`, name)

	case strings.Contains(line, "function expectErrorsWithSchema"):
		out = "expectErrorsWithSchema := func(schema string, queryStr string) helpers.ResultCompare {"

	case strings.Contains(line, "function expectErrors"):
		out = "expectErrors := func(queryStr string) helpers.ResultCompare {"

	case strings.Contains(line, "function expectValidSDL"):
		out = "expectValidSDL := func(sdlStr string, schema ...string) {"

	case strings.Contains(line, "function expectValidWithSchema"):
		out = "expectValidWithSchema := func(schema string, queryStr string) {"

	case strings.Contains(line, "function expectValid"):
		out = "expectValid := func(queryStr string) {"

	case strings.Contains(line, "function expectSDLErrors"):
		out = `expectSDLErrors := func(sdlStr string, sch ...string) helpers.ResultCompare {
			schema := ""
if len(sch) > 0 { schema = sch[0] }`

	case strings.Contains(line, "buildSchema("):
		transformedLine := strings.ReplaceAll(line, "buildSchema", "helpers.BuildSchema")
		out, skip = c.transformLine(transformedLine)

	case strings.Contains(line, "expectValidationErrorsWithSchema"):
		transformedLine := strings.ReplaceAll(line,
			"expectValidationErrorsWithSchema", "helpers.ExpectValidationErrorsWithSchema")

		out, skip = c.transformLine(transformedLine)

	case strings.Contains(line, "expectSDLValidationErrors("):
		transformedLine := strings.ReplaceAll(line,
			"expectSDLValidationErrors", "helpers.ExpectSDLValidationErrors")

		out, skip = c.transformLine(transformedLine)

	case strings.Contains(line, "expectValidationErrors("):
		transformedLine := strings.ReplaceAll(line,
			"expectValidationErrors", "helpers.ExpectValidationErrors")
		out, skip = c.transformLine(transformedLine)

	case strings.Contains(line, "expectSDLErrors(sdlStr, schema)"):
		transformedLine := strings.ReplaceAll(line, "expectSDLErrors(sdlStr, schema)", "expectSDLErrors(sdlStr, schema...)")
		out, skip = c.transformLine(transformedLine)

	case strings.Contains(line, "to.deep.equal([])"):
		out = strings.ReplaceAll(line, ".to.deep.equal([])", "(`[]`)")

	case strings.Contains(line, "`).to.deep.equal(["):
		c.insideMultilineString = false
		c.insideResultAssertion = true
		out = strings.ReplaceAll(line, ".to.deep.equal(", "(`")

	case strings.Contains(line, ").to.deep.equal(["):
		c.insideResultAssertion = true
		out = strings.ReplaceAll(line, ".to.deep.equal(", "(`")

	case strings.Contains(line, "])"):
		if c.insideMultilineString {
			out = line
		} else {
			c.insideResultAssertion = false
			out = "]`)"
		}

	case strings.Contains(line, "`"):
		if strings.Contains(line, "to.deep.equal") {
			out, skip = c.transformLine(line)
		} else {
			if strings.Count(line, "`") == 1 {
				c.insideMultilineString = !c.insideMultilineString
			}
			out = line
		}

	case strings.Contains(line, "Rule,"):
		if c.insideImport {
			return "", true
		}
		var ruleName string
		for _, s := range strings.Split(line, ",") {
			if strings.Contains(s, "Rule") {
				ruleName = strings.TrimSpace(s)
				break
			}
		}
		if strings.Contains(ruleName, "(") {
			ruleName = strings.Split(ruleName, "(")[1]
		}
		out = strings.ReplaceAll(line, ruleName, strconv.Quote(ruleName))

	// case strings.Contains(line, "function"):
	// 	out = strings.ReplaceAll(line, "function", "func")

	default:
		if c.insideImport {
			return "", true
		}
		out = line
	}

	return
}

const ignoreTest = `    it('reports original error for custom scalar which throws', () => {
      const customScalar = new GraphQLScalarType({
        name: 'Invalid',
        parseValue(value) {
          throw new Error(` +
	"\n            `Invalid scalar is always invalid: ${inspect(value)}`," + `
          );
        },
      });

      const schema = new GraphQLSchema({
        query: new GraphQLObjectType({
          name: 'Query',
          fields: {
            invalidArg: {
              type: GraphQLString,
              args: { arg: { type: customScalar } },
            },
          },
        }),
      });

      const expectedErrors = expectErrorsWithSchema(
        schema,
        '{ invalidArg(arg: 123) }',
      );

      expectedErrors.to.deep.equal([
        {
          message:
            'Expected value of type "Invalid", found 123; Invalid scalar is always invalid: 123',
          locations: [{ line: 1, column: 19 }],
        },
      ]);

      expectedErrors.to.have.nested.property(
        '[0].originalError.message',
        'Invalid scalar is always invalid: 123',
      );
    });

    it('reports error for custom scalar that returns undefined', () => {
      const customScalar = new GraphQLScalarType({
        name: 'CustomScalar',
        parseValue() {
          return undefined;
        },
      });

      const schema = new GraphQLSchema({
        query: new GraphQLObjectType({
          name: 'Query',
          fields: {
            invalidArg: {
              type: GraphQLString,
              args: { arg: { type: customScalar } },
            },
          },
        }),
      });

      expectErrorsWithSchema(schema, '{ invalidArg(arg: 123) }').to.deep.equal([
        {
          message: 'Expected value of type "CustomScalar", found 123.',
          locations: [{ line: 1, column: 19 }],
        },
      ]);
    });

    it('allows custom scalar to accept complex literals', () => {
      const customScalar = new GraphQLScalarType({ name: 'Any' });
      const schema = new GraphQLSchema({
        query: new GraphQLObjectType({
          name: 'Query',
          fields: {
            anyArg: {
              type: GraphQLString,
              args: { arg: { type: customScalar } },
            },
          },
        }),
      });

      expectValidWithSchema(
        schema,` +
	"\n        `" + `
          {
            test1: anyArg(arg: 123)
            test2: anyArg(arg: "abc")
            test3: anyArg(arg: [123, "abc"])
            test4: anyArg(arg: {deep: [123, "abc"]})
          }` +
	"\n        `," + `
      );
    });
`
