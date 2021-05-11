package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

//go:generate rm -rf ./testsgo/*_test.go
//go:generate go run main.go
//go:generate gofmt -w testsgo

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

	replacementsPath := workingDir + "/../replacements.yml"
	replacementContent, _ := ioutil.ReadFile(replacementsPath)

	var replacements []Replacement
	if err := yaml.Unmarshal(replacementContent, &replacements); err != nil {
		log.Fatal(err)
	}

	for _, fileInfo := range dir {
		processFile(workingDir, fileInfo.Name(), replacements)
	}
}

const (
	inputDir = "./__tests__"
	outDir   = "./testsgo"

	header = `
package testsgo

import (
  "testing"
)
`
)

var (
	jsArrowFunction = ", () => {"

	convertRules = []string{
		"ExecutableDefinitionsRule",
		"FieldsOnCorrectTypeRule",
		"FragmentsOnCompositeTypesRule",
		"KnownArgumentNamesRule",
		"KnownDirectivesRule",
		"KnownFragmentNamesRule",
		"KnownTypeNamesRule",
		"LoneAnonymousOperationRule",
		"LoneSchemaDefinitionRule",
		"NoFragmentCyclesRule",
		"NoUndefinedVariablesRule",
		"NoUnusedFragmentsRule",
		"NoUnusedVariablesRule",
		"OverlappingFieldsCanBeMergedRule",
		"PossibleFragmentSpreadsRule",
		"PossibleTypeExtensionsRule",
		"ProvidedRequiredArgumentsRule",
		"ScalarLeafsRule",
		"SingleFieldSubscriptionsRule",
		"UniqueArgumentNamesRule",
		"UniqueDirectiveNamesRule",
		"UniqueDirectivesPerLocationRule",
		"UniqueEnumValueNamesRule",
		"UniqueFieldDefinitionNamesRule",
		"UniqueFragmentNamesRule",
		"UniqueInputFieldNamesRule",
		"UniqueOperationNamesRule",
		"UniqueOperationTypesRule",
		"UniqueTypeNamesRule",
		"UniqueVariableNamesRule",
		"ValuesOfCorrectTypeRule",
		"VariablesAreInputTypesRule",
		"VariablesInAllowedPositionRule",
		// "validation", // should be rewritten manually
		// "NoDeprecatedCustomRule", // should be ignored we have no custom rules
		// "NoSchemaIntrospectionCustomRule", // should be ignored we have no custom rules
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

func processFile(workingDir string, filename string, replacements Replacements) {
	fPath := filepath.Join(workingDir, filename)
	fileContent, _ := ioutil.ReadFile(fPath)

	testName := strings.TrimSuffix(strings.Split(filepath.Base(filename), ".")[0], "-test")

	if skipRule(testName) {
		return
	}

	content := string(fileContent)
	for _, replacement := range replacements.ReplaceForRule(testName) {
		content = replacement.Do(content)
	}

	converter := &Converter{}
	result := converter.iterateLines(testName, content)

	outFileName := testName + "_test.go"
	err := ioutil.WriteFile(filepath.Join(outDir, outFileName), []byte(result), os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
}

type Converter struct {
	insideImport          bool
	insideMultilineString bool
	insideResultAssertion bool
	lineNumber            int
}

func (c *Converter) iterateLines(testName string, content string) string {
	var outLines []string

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

	return strings.Join(outLines, "\n")
}

func (c *Converter) transformLine(line string) (out string, skip bool) {
	switch {
	case strings.Contains(line, `'`):
		if strings.Contains(line, `"`) {
			transformedLine := strings.ReplaceAll(line, `'`, "`")
			out, skip = c.transformLine(transformedLine)
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
		out = "expectErrorsWithSchema := func(schema string, queryStr string) ResultCompare {"

	case strings.Contains(line, "function expectErrors"):
		out = "expectErrors := func(queryStr string) ResultCompare {"

	case strings.Contains(line, "function expectValidSDL"):
		out = "expectValidSDL := func(sdlStr string, schema ...string) {"

	case strings.Contains(line, "function expectValidWithSchema"):
		out = "expectValidWithSchema := func(schema string, queryStr string) {"

	case strings.Contains(line, "function expectValid"):
		out = "expectValid := func(queryStr string) {"

	case strings.Contains(line, "function expectSDLErrors"):
		out = `expectSDLErrors := func(sdlStr string, sch ...string) ResultCompare {
			schema := ""
if len(sch) > 0 { schema = sch[0] }`

	case strings.Contains(line, "expectErrorMessage(schema,"):
		out = strings.ReplaceAll(line, ")(", ")(t,")

	case strings.Contains(line, "buildSchema("):
		transformedLine := strings.ReplaceAll(line, "buildSchema", "BuildSchema")
		out, skip = c.transformLine(transformedLine)

	case strings.Contains(line, "expectValidationErrorsWithSchema"):
		transformedLine := strings.ReplaceAll(line,
			"expectValidationErrorsWithSchema", "ExpectValidationErrorsWithSchema")

		out, skip = c.transformLine(transformedLine)

	case strings.Contains(line, "expectSDLValidationErrors("):
		transformedLine := strings.ReplaceAll(line,
			"expectSDLValidationErrors", "ExpectSDLValidationErrors")

		out, skip = c.transformLine(transformedLine)

	case strings.Contains(line, "expectValidationErrors("):
		transformedLine := strings.ReplaceAll(line,
			"expectValidationErrors", "ExpectValidationErrors")
		out, skip = c.transformLine(transformedLine)

	case strings.Contains(line, "expectSDLErrors(sdlStr, schema)"):
		transformedLine := strings.ReplaceAll(line, "expectSDLErrors(sdlStr, schema)", "expectSDLErrors(sdlStr, schema...)")
		out, skip = c.transformLine(transformedLine)

	case strings.Contains(line, "to.deep.equal([])"):
		out = strings.ReplaceAll(line, ".to.deep.equal([])", "(t, []Err{})")

	case strings.Contains(line, "`).to.deep.equal(["):
		c.insideMultilineString = false
		fallthrough

	case strings.Contains(line, ").to.deep.equal(["):
		c.insideResultAssertion = true
		out = strings.ReplaceAll(line, ".to.deep.equal([", "(t, []Err{")

	case strings.Contains(line, "{ message,"):
		if c.insideResultAssertion {
			out = strings.ReplaceAll(line, "{ message,", `{ message: message,`)
			out, skip = c.transformLine(out)
		}

	case strings.Contains(line, "locations: ["):
		transformedLine := strings.ReplaceAll(line, "locations: [", `locations: []Loc{`)
		if strings.Contains(transformedLine, "}]") {
			transformedLine = strings.ReplaceAll(transformedLine, "}]", `}}`)
		}
		out = transformedLine

	case strings.Contains(line, "])"):
		if c.insideMultilineString {
			out = line
		} else {
			c.insideResultAssertion = false
			out = "})"
		}

	case strings.Contains(line, "],"):
		if c.insideResultAssertion {
			out = strings.ReplaceAll(line, "],", `},`)
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

	default:
		if c.insideImport {
			return "", true
		}
		out = line
	}

	return
}

type Replacement struct {
	Rule        string
	Source      string
	Replacement string
}

func (r Replacement) Do(content string) string {
	if !strings.Contains(content, r.Source) {
		log.Fatal("Could not find a replacement for Rule:", r.Rule, " Source:\n", r.Source)
	}
	out := strings.ReplaceAll(content, r.Source, r.Replacement)
	return out
}

type Replacements []Replacement

func (r Replacements) ReplaceForRule(rule string) (out []Replacement) {
	for _, replacement := range r {
		if replacement.Rule == rule {
			out = append(out, replacement)
		}
	}
	return
}
