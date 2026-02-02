package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization/uploads"
)

// FieldArgumentMapping maps the path of field arguments inside an operation
// to the name of their mapped variables.
// Key: "rootOperationType.fieldPath.argumentName" (e.g., "query.posts.limit")
// Value: name of variable of field argument after variable normalization.
type FieldArgumentMapping map[string]string

// VariablesNormalizerResult contains the results of variable normalization.
type VariablesNormalizerResult struct {
	// UploadsMapping tracks file upload variables and how their paths change during normalization.
	UploadsMapping []uploads.UploadPathMapping
	// FieldArgumentMapping maps field arguments to their variable names for fast lookup.
	FieldArgumentMapping FieldArgumentMapping
}
