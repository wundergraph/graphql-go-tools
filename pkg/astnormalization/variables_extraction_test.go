package astnormalization

import (
	"testing"
)

const (
	variablesExtractionDefinition = `
		schema { mutation: Mutation }
		type Mutation {
			httpBinPost(input: HttpBinPostInput): HttpBinPostResponse
		}
		input HttpBinPostInput {
			foo: String!
		}
		type HttpBinPostResponse {
			headers: Headers
			data: HttpBinPostResponseData
		}
		type HttpBinPostResponseData {
			foo: String
		}
		type Headers {
			userAgent: String!
			host: String!
			acceptEncoding: String
			Authorization: String
		}
		scalar String
	`
)

func TestVariablesExtraction(t *testing.T) {
	t.Run("simple http bin example", func(t *testing.T) {
		run(variablesExtraction, variablesExtractionDefinition, `
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
			mutation HttpBinPost($input: HttpBinPostInput){
			  httpBinPost(input: $input){
				headers {
				  userAgent
				}
				data {
				  foo
				}
			  }
			}`)
	})
}
