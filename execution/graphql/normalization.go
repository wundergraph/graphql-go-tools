package graphql

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/graphqlerrors"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type NormalizationResult struct {
	Successful bool
	Errors     graphqlerrors.Errors
}

func (r *Request) Normalize(schema *Schema, options ...astnormalization.Option) (result NormalizationResult, err error) {
	if schema == nil {
		return NormalizationResult{Successful: false, Errors: nil}, ErrNilSchema
	}

	report := r.parseQueryOnce()
	if report.HasErrors() {
		return NormalizationResultFromReport(report)
	}

	r.document.Input.Variables = r.Variables

	// use default normalization options if none are provided
	if len(options) == 0 {
		options = []astnormalization.Option{
			astnormalization.WithExtractVariables(),
			astnormalization.WithRemoveFragmentDefinitions(),
			astnormalization.WithRemoveUnusedVariables(),
			astnormalization.WithInlineFragmentSpreads(),
		}
	}

	normalizer := astnormalization.NewWithOpts(options...)

	if r.OperationName != "" {
		normalizer.NormalizeNamedOperation(&r.document, &schema.document, []byte(r.OperationName), &report)
	} else {
		normalizer.NormalizeOperation(&r.document, &schema.document, &report)
	}

	if report.HasErrors() {
		return NormalizationResultFromReport(report)
	}

	r.isNormalized = true

	r.Variables = r.document.Input.Variables

	return NormalizationResult{Successful: true, Errors: nil}, nil
}

func NormalizationResultFromReport(report operationreport.Report) (NormalizationResult, error) {
	result := NormalizationResult{
		Successful: false,
		Errors:     nil,
	}

	if !report.HasErrors() {
		result.Successful = true
		return result, nil
	}

	result.Errors = graphqlerrors.RequestErrorsFromOperationReport(report)

	var err error
	if len(report.InternalErrors) > 0 {
		err = report.InternalErrors[0]
	}

	return result, err
}
