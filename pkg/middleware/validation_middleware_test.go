package middleware

import (
	"testing"
)

func TestValidationMiddleware(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		query := `query myDocuments {documents {sensitiveInformation}}`
		_, err := InvokeMiddleware(&ValidationMiddleware{}, nil, validationMiddlewarePublicSchema, query)
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		query := `query myDocuments {documents {fieldNotExists}}`
		_, err := InvokeMiddleware(&ValidationMiddleware{}, nil, validationMiddlewarePublicSchema, query)
		if err == nil {
			t.Fatal("want err")
		}
	})
}

const validationMiddlewarePublicSchema = `
schema {
	query: Query
}

type Query {
	documents: [Document]
}

type Document implements Node {
	owner: String
	sensitiveInformation: String
}
`
