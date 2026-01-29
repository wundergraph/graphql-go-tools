package astnormalization

import "github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization/uploads"

// FieldArgumentValue represents either a variable reference or a literal value.
// If LiteralValue is non-nil, it's a literal; otherwise use VariableName.
type FieldArgumentValue struct {
	VariableName string // Set when value is from a variable (e.g., "userId", "a")
	LiteralValue []byte // Set when value is a literal (JSON encoded, e.g., []byte("10"))
}

// FieldArgumentMapping maps field arguments to their values.
// Key: "fieldPath.argumentName" (e.g., "User.posts.limit")
// Value: either a variable name or a literal value
type FieldArgumentMapping map[string]FieldArgumentValue

// VariablesNormalizerResult contains the results of variable normalization.
type VariablesNormalizerResult struct {
	// UploadsMapping tracks file upload variables and how their paths change during normalization.
	UploadsMapping []uploads.UploadPathMapping
	// FieldArgumentMapping maps field arguments to their variable names for fast lookup.
	FieldArgumentMapping FieldArgumentMapping
}
