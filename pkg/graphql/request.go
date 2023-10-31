package graphql

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/pkg/middleware/operation_complexity"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

const (
	schemaIntrospectionFieldName = "__schema"
	typeIntrospectionFieldName   = "__type"
)

type OperationType ast.OperationType

const (
	OperationTypeUnknown      OperationType = OperationType(ast.OperationTypeUnknown)
	OperationTypeQuery        OperationType = OperationType(ast.OperationTypeQuery)
	OperationTypeMutation     OperationType = OperationType(ast.OperationTypeMutation)
	OperationTypeSubscription OperationType = OperationType(ast.OperationTypeSubscription)
)

var (
	ErrEmptyRequest = errors.New("the provided request is empty")
	ErrNilSchema    = errors.New("the provided schema is nil")
)

type Request struct {
	OperationName string          `json:"operationName"`
	Variables     json.RawMessage `json:"variables,omitempty"`
	Query         string          `json:"query"`

	document     ast.Document
	isParsed     bool
	isNormalized bool
	request      resolve.Request

	validForSchema map[uint64]ValidationResult
}

func UnmarshalRequest(reader io.Reader, request *Request) error {
	requestBytes, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	if len(requestBytes) == 0 {
		return ErrEmptyRequest
	}

	return json.Unmarshal(requestBytes, &request)
}

func UnmarshalHttpRequest(r *http.Request, request *Request) error {
	request.request.Header = r.Header
	return UnmarshalRequest(r.Body, request)
}

func (r *Request) SetHeader(header http.Header) {
	r.request.Header = header
}

func (r *Request) CalculateComplexity(complexityCalculator ComplexityCalculator, schema *Schema) (ComplexityResult, error) {
	if schema == nil {
		return ComplexityResult{}, ErrNilSchema
	}

	report := r.parseQueryOnce()
	if report.HasErrors() {
		return complexityResult(
			operation_complexity.OperationStats{},
			[]operation_complexity.RootFieldStats{},
			report,
		)
	}

	return complexityCalculator.Calculate(&r.document, &schema.document)
}

func (r Request) Print(writer io.Writer) (n int, err error) {
	report := r.parseQueryOnce()
	if report.HasErrors() {
		return 0, report
	}

	return writer.Write(r.document.Input.RawBytes)
}

func (r *Request) IsNormalized() bool {
	return r.isNormalized
}

func (r *Request) parseQueryOnce() (report operationreport.Report) {
	if r.isParsed {
		return report
	}

	r.document, report = astparser.ParseGraphqlDocumentString(r.Query)
	if !report.HasErrors() {
		// If the given query has problems, and we failed to parse it,
		// we shouldn't mark it as parsed. It can be misleading for
		// the rest of the components.
		r.isParsed = true
	}
	return report
}

func (r *Request) IsIntrospectionQuery() (result bool, err error) {
	report := r.parseQueryOnce()
	if report.HasErrors() {
		return false, report
	}

	var operationDefinitionRef = ast.InvalidRef
	var possibleOperationDefinitionRefs = make([]int, 0)

	for i := 0; i < len(r.document.RootNodes); i++ {
		if r.document.RootNodes[i].Kind == ast.NodeKindOperationDefinition {
			possibleOperationDefinitionRefs = append(possibleOperationDefinitionRefs, r.document.RootNodes[i].Ref)
		}
	}

	if len(possibleOperationDefinitionRefs) == 0 {
		return
	} else if len(possibleOperationDefinitionRefs) == 1 {
		operationDefinitionRef = possibleOperationDefinitionRefs[0]
	} else {
		for i := 0; i < len(possibleOperationDefinitionRefs); i++ {
			ref := possibleOperationDefinitionRefs[i]
			name := r.document.OperationDefinitionNameString(ref)

			if r.OperationName == name {
				operationDefinitionRef = ref
				break
			}
		}
	}

	if operationDefinitionRef == ast.InvalidRef {
		return
	}

	operationDef := r.document.OperationDefinitions[operationDefinitionRef]
	if operationDef.OperationType != ast.OperationTypeQuery {
		return
	}
	if !operationDef.HasSelections {
		return
	}

	selectionSet := r.document.SelectionSets[operationDef.SelectionSet]
	if len(selectionSet.SelectionRefs) == 0 {
		return
	}

	for i := 0; i < len(selectionSet.SelectionRefs); i++ {
		selection := r.document.Selections[selectionSet.SelectionRefs[i]]
		if selection.Kind != ast.SelectionKindField {
			continue
		}

		fieldName := r.document.FieldNameUnsafeString(selection.Ref)
		switch fieldName {
		case schemaIntrospectionFieldName, typeIntrospectionFieldName:
			continue
		default:
			return
		}
	}

	return true, nil
}

func (r *Request) OperationType() (OperationType, error) {
	report := r.parseQueryOnce()
	if report.HasErrors() {
		return OperationTypeUnknown, report
	}

	for _, rootNode := range r.document.RootNodes {
		if rootNode.Kind != ast.NodeKindOperationDefinition {
			continue
		}

		if r.OperationName != "" && r.document.OperationDefinitionNameString(rootNode.Ref) != r.OperationName {
			continue
		}

		opType := r.document.OperationDefinitions[rootNode.Ref].OperationType
		return OperationType(opType), nil
	}

	return OperationTypeUnknown, nil
}
