package astnormalization

import (
	"testing"
)

func TestUnusedVariableDeletion(t *testing.T) {
	t.Run("delete unused variables", func(t *testing.T) {
		runWithDeleteUnusedVariables(t, deleteUnusedVariables, variablesExtractionDefinition, `
			mutation HttpBinPost($a: HttpBinPostInput $b: String){
			  httpBinPost(input: $a){
				headers {
				  userAgent
				}
				data {
				  foo
				}
			  }
			}`, "HttpBinPost", `
			mutation HttpBinPost($a: HttpBinPostInput){
			  httpBinPost(input: $a){
				headers {
				  userAgent
				}
				data {
				  foo
				}
			  }
			}`, `{"a":{"foo":"bar"},"b":"bat"}`, `{"a":{"foo":"bar"}}`)
	})
}
