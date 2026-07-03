package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type areRequiredFieldsProvidedInput struct {
	typeName          string
	requiredFields    string
	definition        *ast.Document
	dataSource        DataSource
	providedSelection providesSelection
}

// areRequiredFieldsProvided checks if all required fields are provided on the given position in a query
// Example subgraph schema:
// type Address @key(fields: "zip") {
//  id: ID!
//  street: String @requires(fields: "zip")
//  zip: String @external
// }

// type User @key(fields: "id") {
//  id: ID!
//  address: Address @external
// }

//	type Query {
//	 me: User @provides(fields: "address { street zip }")
//	}
//
// Example query:
//
//	query {
//	  me {
//	    address {
//	      street
//	    }
//	  }
//	}
//
// When one of the parent nodes provides fields, which are mentioned in requires,
// we can skip fetching these requirements, because fields are already available.
// providedSelection is the provided selection applying to the field with @requires
// and its siblings, e.g. the fields of the entity holding the requirement.
func areRequiredFieldsProvided(input areRequiredFieldsProvidedInput) (bool, *operationreport.Report) {
	if len(input.providedSelection) == 0 {
		return false, operationreport.NewReport()
	}

	key, report := RequiredFieldsFragment(input.typeName, input.requiredFields, false)
	if report.HasErrors() {
		return false, report
	}

	walker := astvisitor.NewWalkerWithID(4, "RequiredFieldsProvidedVisitor")

	visitor := &requiredFieldsProvidedVisitor{
		walker:         &walker,
		input:          input,
		key:            key,
		selectionStack: []providesSelection{input.providedSelection},
		allProvided:    true,
	}

	walker.RegisterFieldVisitor(visitor)
	walker.Walk(key, input.definition, report)

	return visitor.allProvided, report
}

type requiredFieldsProvidedVisitor struct {
	walker *astvisitor.Walker
	input  areRequiredFieldsProvidedInput
	key    *ast.Document

	// selectionStack tracks the provided selection at each nesting level of the
	// requires fragment walk; entries are nil below levels which are not provided
	selectionStack []providesSelection
	allProvided    bool
}

func (v *requiredFieldsProvidedVisitor) EnterField(ref int) {
	fieldName := v.key.FieldNameString(ref)
	typeName := v.walker.EnclosingTypeDefinition.NameString(v.input.definition)

	currentSelection := v.selectionStack[len(v.selectionStack)-1]
	childSelection, provided := currentSelection.providedTypeSelection(fieldName, typeName)
	v.selectionStack = append(v.selectionStack, childSelection)

	if provided {
		return
	}

	// the field is not explicitly provided, but when it is a regular node of the
	// data source it is accessible under the provided parent,
	// e.g. implicitly provided
	if v.input.dataSource.HasRootNode(typeName, fieldName) || v.input.dataSource.HasChildNode(typeName, fieldName) {
		return
	}

	v.allProvided = false
	v.walker.Stop()
}

func (v *requiredFieldsProvidedVisitor) LeaveField(ref int) {
	v.selectionStack = v.selectionStack[:len(v.selectionStack)-1]
}
