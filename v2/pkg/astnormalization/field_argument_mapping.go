package astnormalization

import "github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization/uploads"

// FieldArgumentMapping maps field arguments to their variable names.
// Key: "fieldPath.argumentName" (e.g., "user.posts.limit")
// Value: variable name after extraction (e.g., "a", "b", "userId")
type FieldArgumentMapping map[string]string

// VariablesNormalizerResult contains the results of variable normalization.
type VariablesNormalizerResult struct {
	// UploadsMapping tracks file upload variables and how their paths change during normalization.
	UploadsMapping []uploads.UploadPathMapping
	// FieldArgumentMapping maps field arguments to their variable names for fast lookup.
	FieldArgumentMapping FieldArgumentMapping
}
