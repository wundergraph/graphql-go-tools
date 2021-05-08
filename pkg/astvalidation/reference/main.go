package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
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

	convertRules = []string{
		"ExecutableDefinitionsRule",
		// "FieldsOnCorrectTypeRule",
		"FragmentsOnCompositeTypesRule",
		// "KnownArgumentNamesRule",
		// "KnownDirectivesRule",
		"KnownFragmentNamesRule",
		// "KnownTypeNamesRule",
		"LoneAnonymousOperationRule",
		// "LoneSchemaDefinitionRule",
		// "NoDeprecatedCustomRule",
		"NoFragmentCyclesRule",
		"NoSchemaIntrospectionCustomRule",
		"NoUndefinedVariablesRule",
		"NoUnusedFragmentsRule",
		"NoUnusedVariablesRule",
		// "OverlappingFieldsCanBeMergedRule",
		"PossibleFragmentSpreadsRule",
		// "PossibleTypeExtensionsRule",
		// "ProvidedRequiredArgumentsRule",
		// "ScalarLeafsRule",
		// "SingleFieldSubscriptionsRule",
		// "UniqueArgumentNamesRule",
		// "UniqueDirectiveNamesRule",
		// "UniqueDirectivesPerLocationRule",
		// "UniqueEnumValueNamesRule",
		// "UniqueFieldDefinitionNamesRule",
		// "UniqueFragmentNamesRule",
		// "UniqueInputFieldNamesRule",
		// "UniqueOperationNamesRule",
		// "UniqueOperationTypesRule",
		// "UniqueTypeNamesRule",
		// "UniqueVariableNamesRule",
		// "ValuesOfCorrectTypeRule",
		// "VariablesAreInputTypesRule",
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
	result = doReplace(result)

	outFileName := testName + "_test.go"
	ioutil.WriteFile(filepath.Join(outDir, outFileName), []byte(result), os.ModePerm)
}

func doReplace(content string) string {
	content = strings.ReplaceAll(content, `'`, `"`)

	content = strings.ReplaceAll(content, ";", "")

	content = strings.ReplaceAll(content, "function", "func")

	return content
}

func iterateLines(testName string, content string) string {
	var outLines []string
	lines := strings.Split(content, "\n")

	outLines = append(outLines, header)
	outLines = append(outLines, fmt.Sprintf("func Test%s(t *testing.T) {", testName))

	for _, line := range lines {
		switch {

		case strings.Contains(line, "import"):
			continue

		case strings.Contains(line, "describe("):
			name := strings.TrimSuffix(strings.ReplaceAll(line, "describe(", ""), jsArrowFunction)
			outLines = append(outLines, fmt.Sprintf(`t.Run(%s, func(t *testing.T) {`, name))
			continue

		case strings.Contains(line, "it("):
			name := strings.TrimSuffix(strings.ReplaceAll(line, "it(", ""), jsArrowFunction)
			outLines = append(outLines, fmt.Sprintf(`t.Run(%s, func(t *testing.T) {`, name))
			continue

		case strings.Contains(line, "function expectErrors"):
			outLines = append(outLines, "\nexpectErrors := func(queryStr string) helpers.ResultCompare {")

		case strings.Contains(line, "function expectValid"):
			outLines = append(outLines, "expectValid :=  func(queryStr string) {")

		case strings.Contains(line, "return expectValidationErrors("):
			ruleName := strings.TrimSpace(strings.TrimSuffix(strings.ReplaceAll(line, "return expectValidationErrors(", ""), ", queryStr);"))
			outLines = append(outLines, fmt.Sprintf(`return helpers.ExpectValidationErrors("%s", queryStr)`, ruleName))

		case strings.Contains(line, "to.deep.equal([])"):
			outLines = append(outLines, strings.ReplaceAll(line, ".to.deep.equal([])", "(`[]`)"))

		case strings.Contains(line, "`).to.deep.equal(["):
			outLines = append(outLines, strings.ReplaceAll(line, ".to.deep.equal(", "(`"))

		case strings.Contains(line, "])"):
			outLines = append(outLines, "]`)")

		default:
			outLines = append(outLines, line)
		}
	}

	outLines = append(outLines, "}")

	return strings.Join(outLines, "\n")
}
