package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization/uploads"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type VariablesNormalizer struct {
	firstDetectUnused          *astvisitor.Walker
	secondExtract              *astvisitor.Walker
	thirdDeleteUnused          *astvisitor.Walker
	fourthCoerce               *astvisitor.Walker
	variablesExtractionVisitor *variablesExtractionVisitor
}

// VariablesNormalizerOptions allows to configure a VariablesNormalizer.
type VariablesNormalizerOptions struct {
	// EnableFieldArgumentMapping enables field argument mapping.
	// If true it contains the map as part of the NormalizeOperation result.
	EnableFieldArgumentMapping bool
}

// NewVariablesNormalizer creates a new variable normalizer.
func NewVariablesNormalizer(options VariablesNormalizerOptions) *VariablesNormalizer {
	// delete unused modifying variables refs,
	// so it is safer to run it sequentially with the extraction
	thirdDeleteUnused := astvisitor.NewWalkerWithID(8, "DeleteUnusedVariables")
	del := deleteUnusedVariables(&thirdDeleteUnused)

	// register variable usage detection on the first stage
	// and pass usage information to the deletion visitor
	// so it keeps variables that are defined but not used at all
	// ensuring that validation can still catch them
	firstDetectUnused := astvisitor.NewWalkerWithID(8, "DetectVariableUsage")
	detectVariableUsage(&firstDetectUnused, del)

	secondExtract := astvisitor.NewWalkerWithID(8, "ExtractVariables")
	variablesExtractionVisitor := extractVariables(&secondExtract, options.EnableFieldArgumentMapping)
	extractVariablesDefaultValue(&secondExtract)

	fourthCoerce := astvisitor.NewWalkerWithID(0, "VariablesCoercion")
	inputCoercionForList(&fourthCoerce)

	return &VariablesNormalizer{
		firstDetectUnused:          &firstDetectUnused,
		secondExtract:              &secondExtract,
		thirdDeleteUnused:          &thirdDeleteUnused,
		fourthCoerce:               &fourthCoerce,
		variablesExtractionVisitor: variablesExtractionVisitor,
	}
}

// NormalizeOperation processes GraphQL operation variables.
// It detects and removes unused variables, extracts variables from inline values
// and coerces variable values.
// https://spec.graphql.org/September2025/#sec-Coercing-Variable-Values
// It modifies the operation in place and
// returns metadata including field argument mappings and upload paths.
// Field argument mapping is done only when it is enabled in v,
// else VariablesNormalizerResult.FieldArgumentMapping will be nil.
// Any errors encountered during normalization are reported via the report parameter.
func (v *VariablesNormalizer) NormalizeOperation(operation, definition *ast.Document, report *operationreport.Report) VariablesNormalizationResult {
	v.firstDetectUnused.Walk(operation, definition, report)
	if report.HasErrors() {
		return VariablesNormalizationResult{}
	}
	v.secondExtract.Walk(operation, definition, report)
	if report.HasErrors() {
		return VariablesNormalizationResult{}
	}
	v.thirdDeleteUnused.Walk(operation, definition, report)
	if report.HasErrors() {
		return VariablesNormalizationResult{}
	}
	v.fourthCoerce.Walk(operation, definition, report)

	return VariablesNormalizationResult{
		UploadsMapping:       v.variablesExtractionVisitor.uploadsPath,
		FieldArgumentMapping: v.variablesExtractionVisitor.fieldArgumentMapping.result,
	}
}

// FieldArgumentMapping maps the path of field arguments inside an operation
// to the name of their mapped variables.
// Key: "rootOperationType.fieldPath.argumentName" (e.g., "query.posts.limit")
// Value: name of variable of field argument after variable normalization.
type FieldArgumentMapping map[string]string

// VariablesNormalizationResult contains the results of variable normalization.
type VariablesNormalizationResult struct {
	// UploadsMapping tracks file upload variables and how their paths change during normalization.
	UploadsMapping []uploads.UploadPathMapping
	// FieldArgumentMapping maps field arguments to their variable names for fast lookup.
	FieldArgumentMapping FieldArgumentMapping
}
