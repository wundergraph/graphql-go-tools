package middleware

import (
	"testing"
)

func TestValidationMiddleware(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		query := `query myDocuments {documents {sensitiveInformation}}`
		_, err := InvokeMiddleware(&ValidationMiddleware{}, nil, publicSchema, query)
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		query := `query myDocuments {documents {fieldNotExists}}`
		_, err := InvokeMiddleware(&ValidationMiddleware{}, nil, publicSchema, query)
		if err == nil {
			t.Fatal("want err")
		}
	})
}
