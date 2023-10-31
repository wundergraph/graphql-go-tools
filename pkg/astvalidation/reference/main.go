package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
)

//go:generate ./gen.sh

func main() {
	currDir, _ := os.Getwd()
	println(currDir)

	workingDir := inputDir
	if !strings.Contains(currDir, "reference") {
		workingDir = filepath.Join(currDir, "pkg/astvalidation/reference/__tests__")
	}

	dir, err := os.ReadDir(workingDir)
	if err != nil {
		log.Fatal(err)
	}

	replacementsPath := workingDir + "/../replacements.yml"
	replacementContent, _ := os.ReadFile(replacementsPath)

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
	// jsArrowFunction - represents a function passed to mocha describe and it
	jsArrowFunction = ", () => {"

	// convertRules - list of reference rules which should be converted
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

// skipRule - determine do we have to convert particular rule e.g. test file from reference tests
func skipRule(name string) bool {
	for _, rule := range convertRules {
		if rule == name {
			return false
		}
	}
	return true
}

// processFile - convert and save reference test file
func processFile(workingDir string, filename string, replacements Replacements) {
	fPath := filepath.Join(workingDir, filename)
	fileContent, _ := os.ReadFile(fPath)

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
	err := os.WriteFile(filepath.Join(outDir, outFileName), []byte(result), os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
}

// Converter - is a line by line js-to-go converter of a reference test file
// global replacements should be done before running Converter
type Converter struct {
	insideImport          bool
	insideMultilineString bool
	insideResultAssertion bool
	lineNumber            int
}

// iterateLines - iterates over js file content line by line and wraps a content into go test function
func (c *Converter) iterateLines(testName string, content string) string {
	var outLines []string

	// remove semicolons from content
	content = strings.ReplaceAll(content, ";", "")
	// splits content into separate lines
	lines := strings.Split(content, "\n")

	// appends package header
	outLines = append(outLines, header)
	// appends go test function wrapper
	outLines = append(outLines, fmt.Sprintf("func Test%s(t *testing.T) {", testName))

	for i, line := range lines {
		c.lineNumber = i + 1
		transformedLine, skip := c.transformLine(line)
		if !skip {
			outLines = append(outLines, transformedLine)
		}
	}

	// appends go test function closing bracket
	outLines = append(outLines, "}")

	// joins processed lines into resulting string
	return strings.Join(outLines, "\n")
}

// transformLine - doing required replacement for the particular js code line
// could be called recursively in some cases
// returns:
// out - a processed line
// skip - a flag to skip appending line to results
func (c *Converter) transformLine(line string) (out string, skip bool) {
	// fmt.Println("#transformLine lineNumber: ", c.lineNumber, " line: ", line)
	// defer func() {
	// 	fmt.Println("#transformLine RESULT ", c.lineNumber, " skip: ", skip, " out: ", out)
	// }()

	// selects a required line transformation
	// NOTE: Order of transformation is matters!!!
	switch {

	// handles js string wrapped into "'" which in go should be presented with "`" or `"`
	case strings.Contains(line, `'`):
		// when js string contains double quote
		if strings.Contains(line, `"`) {
			// wrap result into "`"
			transformedLine := strings.ReplaceAll(line, `'`, "`")
			out, skip = c.transformLine(transformedLine)
		} else {
			// wrap result into `"`
			transformedLine := strings.ReplaceAll(line, `'`, `"`)
			out, skip = c.transformLine(transformedLine)
		}

		// skip js single line import
	case strings.Contains(line, "import { "):
		return "", true

		// skip js multi line import
	case strings.Contains(line, "import {"):
		c.insideImport = true
		return "", true

		// skip js multi line end of import
	case strings.Contains(line, "} from"):
		c.insideImport = false
		return "", true

		// replace js const with a go variable
	case strings.Contains(line, "const "):
		parts := strings.Split(line, "=")
		variableName := strings.TrimPrefix(strings.TrimSpace(parts[0]), "const")
		transformedLine := fmt.Sprintf("%s := %s", variableName, parts[1])
		out, skip = c.transformLine(transformedLine)

		// replace mocha "describe" with go t.Run
	case strings.Contains(line, "describe("):
		name := strings.TrimSuffix(strings.ReplaceAll(line, "describe(", ""), jsArrowFunction)
		out = fmt.Sprintf(`t.Run(%s, func(t *testing.T) {`, name)

		// replace mocha "it" with go t.Run
	case strings.Contains(line, "it("):
		name := strings.TrimSuffix(strings.ReplaceAll(line, "it(", ""), jsArrowFunction)
		out = fmt.Sprintf(`t.Run(%s, func(t *testing.T) {`, name)

		/*
			#  Start of section for processing reference test helper functions
			in reference implementation each test has a wrapper of harness helpers with rule name
			we do not need a rule name as it could be derived from *testing.T but we preserving rule name to not heavily modify a file
			Here we replacing test helper function definitions with go function variables
		*/

		// rewrite test helpers
	case strings.Contains(line, "function"):
		out = c.transformHelperFunctions(line)

	// rewrite calls to test helpers
	case !strings.Contains(line, ":=") && regexp.MustCompile(`expect.*\(`).MatchString(line):
		transformedLine := c.transformUsageOfHelperFunctions(line)
		out, skip = c.transformLine(transformedLine)

	case strings.Contains(line, "buildSchema("):
		transformedLine := strings.ReplaceAll(line, "buildSchema", "BuildSchema")
		out, skip = c.transformLine(transformedLine)

		/*
			# End of section for processing reference test helper functions
		*/

		// replace chai deep equal compare for an empty errors array
	case strings.Contains(line, "to.deep.equal([])"):
		out = strings.ReplaceAll(line, ".to.deep.equal([])", "([]Err{})")

		// detects end of schema sdl multiline string and steps into replacing errors matcher
	case strings.Contains(line, "`).to.deep.equal(["):
		// set insideMultilineString to false as schema sdl multiline string finished
		c.insideMultilineString = false
		fallthrough

		// replace chai deep equal compare of errors with a call to ResultCompare
	case strings.Contains(line, ").to.deep.equal(["):
		// set insideResultAssertion as we are entering errors assertion
		c.insideResultAssertion = true
		out = strings.ReplaceAll(line, ".to.deep.equal([", "([]Err{")

		// replace js object field inlining with message property of Err object
	case strings.Contains(line, "{ message,"):
		if c.insideResultAssertion {
			out = strings.ReplaceAll(line, "{ message,", `{ message: message,`)
			out, skip = c.transformLine(out)
		}

		// replace locations with Loc slice property
		// when locations in a single line we should replace ending brackets to close slice
	case strings.Contains(line, "locations: ["):
		transformedLine := strings.ReplaceAll(line, "locations: [", `locations: []Loc{`)
		if strings.Contains(transformedLine, "}]") {
			transformedLine = strings.ReplaceAll(transformedLine, "}]", `}}`)
		}
		out = transformedLine

		// closes result assertion with a slice close
	case strings.Contains(line, "])"):
		// if we are inside a multiline string leave a line as is
		if c.insideMultilineString {
			out = line
		} else {
			// set insideResultAssertion to false as we are leaving result assertion
			c.insideResultAssertion = false
			out = "})"
		}

		// handles closing slice when we are inside a multiline assertion
	case strings.Contains(line, "],"):
		switch {
		case c.insideMultilineString:
			out = line
		case c.insideResultAssertion:
			out = strings.ReplaceAll(line, "],", `},`)
		}

		// handles js multiline strings
	case strings.Contains(line, "`"):
		if strings.Contains(line, "to.deep.equal") {
			// in case string contains chai comparison proceed with a different convertion
			out, skip = c.transformLine(line)
		} else {
			// change insideMultilineString in case we are at start or end of js multiline string
			if strings.Count(line, "`") == 1 {
				c.insideMultilineString = !c.insideMultilineString
			}
			// add line as is
			out = line
		}

		// replacing a rule js function name with a simple string rule name
	case strings.Contains(line, "Rule,"):
		// if we are inside importing of rule function just skip a line
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
		out = strings.ReplaceAll(line, ruleName, ruleName)

		// do not transform a line when no conditions met
		// in case we are inside an import just skip a line
	default:
		if c.insideImport {
			return "", true
		}
		out = line
	}

	return
}

// transformHelperFunctions - creates helper function variables
func (c *Converter) transformHelperFunctions(line string) (out string) {
	// fmt.Println("#transformHelperFunctions lineNumber: ", c.lineNumber, " line: ", line)
	// defer func() {
	// 	fmt.Println("#transformHelperFunctions lineNumber: ", c.lineNumber, " out: ", out)
	// }()

	switch {
	case strings.Contains(line, "function expectErrorsWithSchema"):
		out = "ExpectErrorsWithSchema := func(t *testing.T, schema string, queryStr string) ResultCompare {"

	case strings.Contains(line, "function expectErrors"):
		out = "ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {"

	case strings.Contains(line, "function expectValidSDL"):
		out = "ExpectValidSDL := func(t *testing.T, sdlStr string, schema ...string) {"

	case strings.Contains(line, "function expectValidWithSchema"):
		out = "ExpectValidWithSchema := func(t *testing.T, schema string, queryStr string) {"

	case strings.Contains(line, "function expectValid"):
		out = "ExpectValid := func(t *testing.T, queryStr string) {"

	}

	return
}

// transformUsageOfHelperFunctions - adds a *testing.T as a first argument to helpers call
func (c *Converter) transformUsageOfHelperFunctions(line string) (out string) {
	// fmt.Println("#transformUsageOfHelperFunctions lineNumber: ", c.lineNumber, " line: ", line)
	// defer func() {
	// 	fmt.Println("#transformUsageOfHelperFunctions lineNumber: ", c.lineNumber, " out: ", out)
	// }()

	switch {
	case strings.Contains(line, "expectSDLErrors("):
		out = strings.ReplaceAll(line,
			"expectSDLErrors(sdlStr, schema)", "ExpectSDLErrors(t, sdlStr, schema...)")
		out = strings.ReplaceAll(out,
			"expectSDLErrors(", "ExpectSDLErrors(t,")

	// add *testing.T arg to expectErrorMessage call
	case strings.Contains(line, "expectErrorMessage("):
		out = strings.ReplaceAll(line, "expectErrorMessage(", "ExpectErrorMessage(t,")

	case strings.Contains(line, "expectValidationErrorsWithSchema("):
		out = strings.ReplaceAll(line,
			"expectValidationErrorsWithSchema(", "ExpectValidationErrorsWithSchema(t,")

	case strings.Contains(line, "expectErrorsWithSchema("):
		out = strings.ReplaceAll(line,
			"expectErrorsWithSchema(", "ExpectErrorsWithSchema(t,")

	case strings.Contains(line, "expectSDLValidationErrors("):
		out = strings.ReplaceAll(line,
			"expectSDLValidationErrors(", "ExpectSDLValidationErrors(t,")

	case strings.Contains(line, "expectValidationErrors("):
		out = strings.ReplaceAll(line,
			"expectValidationErrors(", "ExpectValidationErrors(t,")

	case strings.Contains(line, "expectErrors("):
		out = strings.ReplaceAll(line,
			"expectErrors(", "ExpectErrors(t,")

	case strings.Contains(line, "expectValid("):
		out = strings.ReplaceAll(line,
			"expectValid(", "ExpectValid(t,")

	case strings.Contains(line, "expectValidSDL("):
		out = strings.ReplaceAll(line,
			"expectValidSDL(", "ExpectValidSDL(t,")

	case strings.Contains(line, "expectValidWithSchema("):
		out = strings.ReplaceAll(line,
			"expectValidWithSchema(", "ExpectValidWithSchema(t,")
	}

	return
}

// Replacement - is a type holding global replacements for a particular rule e.g. test file in references tests
type Replacement struct {
	Rule        string
	Source      string
	Replacement string
}

func (r Replacement) Do(content string) string {
	if !strings.Contains(content, r.Source) {
		return content
	}
	out := strings.ReplaceAll(content, r.Source, r.Replacement)
	return out
}

// Replacements - list of global replacements for rules
type Replacements []Replacement

// ReplaceForRule - returns array of Replacement for particular rule
func (r Replacements) ReplaceForRule(rule string) (out []Replacement) {
	for _, replacement := range r {
		if replacement.Rule == "__ALL__" || replacement.Rule == rule {
			out = append(out, replacement)
		}
	}
	return
}
