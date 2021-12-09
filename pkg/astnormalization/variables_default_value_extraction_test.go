package astnormalization

import (
	"testing"
)

const (
	variablesDefaultValueExtractionDefinition = `
		schema { mutation: Mutation }
		type Mutation {
			simple(input: String = "foo"): String
			mixed(a: String, b: String, input: String = "foo"): String
		}
		scalar String
	`
)

func TestVariablesDefaultValueExtraction(t *testing.T) {
	t.Run("field argument default value", func(t *testing.T) {
		t.Run("no value provided", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			mutation simple {
			  simple
			}`, "", `
			mutation simple($a: String) {
			  simple(input: $a)
			}`, ``, `{"a":"foo"}`)
		})

		t.Run("value provided", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			mutation simple {
			  simple(input: "bazz")
			}`, "", `
			mutation simple {
			  simple(input: "bazz")
			}`, ``, ``)
		})

		t.Run("mixed", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			mutation simple($a: String) {
			  mixed(a: $a, b: "bar")
			}`, "", `
			mutation simple($a: String, $b: String) {
			  mixed(a: $a, b: "bar", input: $b)
			}`, `{"a":"aaa"}`, `{"b":"foo","a":"aaa"}`)
		})
	})

	t.Run("variable with default value", func(t *testing.T) {
		t.Run("no value provided", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			mutation simple($in: String = "bar" ) {
			  simple(input: $in)
			}`, "", `
			mutation simple($in: String) {
			  simple(input: $in)
			}`, ``, `{"in":"bar"}`)
		})
		t.Run("value provided", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			mutation simple($in: String = "bar" ) {
			  simple(input: $in)
			}`, "", `
			mutation simple($in: String) {
			  simple(input: $in)
			}`, `{"in":"foo"}`, `{"in":"foo"}`)
		})
	})

	t.Run("mixed", func(t *testing.T) {
		runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			mutation simple($a: String = "bar", $b: String = "bazz") {
			  mixed(a: $a, b: $b)
			}`, "", `
			mutation simple($a: String, $b: String, $c: String) {
			  mixed(a: $a, b: $b, input: $c)
			}`, `{"a":"aaa"}`, `{"c":"foo","b":"bazz","a":"aaa"}`)
	})
}
