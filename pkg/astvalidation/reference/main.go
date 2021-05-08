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
		"KnownArgumentNamesRule", // OK
		"KnownDirectivesRule",    // OK
		// "KnownTypeNamesRule",
		// "LoneSchemaDefinitionRule",
		// "NoDeprecatedCustomRule",
		// "OverlappingFieldsCanBeMergedRule",
		// "PossibleTypeExtensionsRule",
		// "ProvidedRequiredArgumentsRule",
		// "UniqueDirectiveNamesRule",
		// "UniqueDirectivesPerLocationRule",
		// "UniqueEnumValueNamesRule",
		// "UniqueFieldDefinitionNamesRule",
		// "UniqueOperationTypesRule",
		// "UniqueTypeNamesRule",
		// "ValuesOfCorrectTypeRule",
		// "VariablesInAllowedPositionRule",
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

	result := iterateLines(testName, string(content))

	outFileName := testName + "_test.go"
	ioutil.WriteFile(filepath.Join(outDir, outFileName), []byte(result), os.ModePerm)
}

func iterateLines(testName string, content string) string {
	var outLines []string
	lines := strings.Split(content, "\n")

	outLines = append(outLines, header)
	outLines = append(outLines, fmt.Sprintf("func Test%s(t *testing.T) {", testName))

	insideImport := false
	for _, line := range lines {
		transformedLine, skip := transformLine(&insideImport, line)
		if !skip {
			outLines = append(outLines, transformedLine)
		}
	}

	outLines = append(outLines, "}")

	return strings.Join(outLines, "\n")
}

func transformLine(insideImport *bool, line string) (out string, skip bool) {
	switch {

	case strings.Contains(line, `'`) && !strings.Contains(line, `"`):
		transformedLine := strings.ReplaceAll(line, `'`, `"`)
		out, skip = transformLine(insideImport, transformedLine)

	case strings.Contains(line, "import { "):
		return "", true

	case strings.Contains(line, "import {"):
		*insideImport = true
		return "", true

	case strings.Contains(line, "} from"):
		*insideImport = false
		return "", true

	case strings.Contains(line, "buildSchema("):
		variableName :=
			strings.TrimPrefix(
				strings.TrimSpace(strings.Split(line, "=")[0]),
				"const",
			)
		out = fmt.Sprintf("%s := helpers.BuildSchema(`", variableName)

	case strings.Contains(line, "describe("):
		name := strings.TrimSuffix(strings.ReplaceAll(line, "describe(", ""), jsArrowFunction)
		out = fmt.Sprintf(`t.Run(%s, func(t *testing.T) {`, name)

	case strings.Contains(line, "it("):
		name := strings.TrimSuffix(strings.ReplaceAll(line, "it(", ""), jsArrowFunction)
		out = fmt.Sprintf(`t.Run(%s, func(t *testing.T) {`, name)

	case strings.Contains(line, "function expectErrors"):
		out = "\nexpectErrors := func(queryStr string) helpers.ResultCompare {"

	case strings.Contains(line, "function expectValidSDL"):
		out = "expectValidSDL :=  func(sdlStr string, schema ...string) {"

	case strings.Contains(line, "function expectValid"):
		out = "expectValid :=  func(queryStr string) {"

	case strings.Contains(line, "function expectSDLErrors"):
		out = `expectSDLErrors := func(sdlStr string, sch ...string) helpers.ResultCompare {
			schema := ""
if len(sch) > 0 { schema = sch[0] }`

	case strings.Contains(line, "return expectSDLValidationErrors("):
		transformedLine := strings.ReplaceAll(line,
			"expectSDLValidationErrors", "helpers.ExpectSDLValidationErrors")
		if strings.Contains(transformedLine, "Rule,") {
			ruleName := strings.TrimSpace(strings.Split(transformedLine, ",")[1])
			transformedLine = strings.ReplaceAll(transformedLine, ruleName, strconv.Quote(ruleName))
		}
		out = transformedLine

	case strings.Contains(line, "return expectValidationErrors("):
		ruleName := strings.TrimSpace(strings.TrimSuffix(strings.ReplaceAll(line, "return expectValidationErrors(", ""), ", queryStr);"))
		out = fmt.Sprintf(`return helpers.ExpectValidationErrors("%s", queryStr)`, ruleName)

	case strings.Contains(line, "expectSDLErrors(sdlStr, schema)"):
		transformedLine := strings.ReplaceAll(line, "expectSDLErrors(sdlStr, schema)", "expectSDLErrors(sdlStr, schema...)")
		out, skip = transformLine(insideImport, transformedLine)

	case strings.Contains(line, "to.deep.equal([])"):
		out = strings.ReplaceAll(line, ".to.deep.equal([])", "(`[]`)")

	case strings.Contains(line, "`).to.deep.equal(["):
		out = strings.ReplaceAll(line, ".to.deep.equal(", "(`")

	case strings.Contains(line, ").to.deep.equal(["):
		out = strings.ReplaceAll(line, ".to.deep.equal(", "(`")

	case strings.Contains(line, "])"):
		out = "]`)"

	case strings.HasSuffix(line, "Rule,"):
		if *insideImport {
			return "", true
		}
		ruleName := strconv.Quote(strings.TrimSpace(strings.TrimSuffix(line, ",")))
		out = ruleName + ","

	// case strings.Contains(line, "function"):
	// 	out = strings.ReplaceAll(line, "function", "func")

	default:
		if *insideImport {
			return "", true
		}
		out = line
	}

	return
}
