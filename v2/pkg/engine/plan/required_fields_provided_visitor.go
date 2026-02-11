package plan

import (
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type areRequiredFieldsProvidedInput struct {
	typeName       string
	requiredFields string
	definition     *ast.Document
	dataSource     DataSource
	providedFields map[string]struct{}
	parentPath     string
}

// areRequiredFieldsProvided checks if all required fields are provided on the given path in a query
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
// When one of the parent nodes provides fields, which are mentioned in requires.
// We can skip fetching these requirements, because fields are already available under the given path.
func areRequiredFieldsProvided(input areRequiredFieldsProvidedInput) (bool, *operationreport.Report) {
	if len(input.providedFields) == 0 {
		return false, operationreport.NewReport()
	}

	key, report := RequiredFieldsFragment(input.typeName, input.requiredFields, false)
	if report.HasErrors() {
		return false, report
	}

	walker := astvisitor.NewWalkerWithID(4, "RequiredFieldsProvidedVisitor")

	visitor := &requiredFieldsProvidedVisitor{
		walker:      &walker,
		input:       input,
		key:         key,
		allProvided: true,
	}

	walker.RegisterEnterFieldVisitor(visitor)
	walker.Walk(key, input.definition, report)

	return visitor.allProvided, report
}

type requiredFieldsProvidedVisitor struct {
	walker      *astvisitor.Walker
	input       areRequiredFieldsProvidedInput
	key         *ast.Document
	allProvided bool
}

func (v *requiredFieldsProvidedVisitor) EnterField(ref int) {
	typeName := v.walker.EnclosingTypeDefinition.NameString(v.input.definition)
	currentFieldName := v.key.FieldNameUnsafeString(ref)

	currentPathWithoutFragments := v.walker.Path.WithoutInlineFragmentNames().DotDelimitedString()
	// remove the parent type name from the path because we are walking a fragment with the required fields
	parentPath := v.input.parentPath + strings.TrimPrefix(currentPathWithoutFragments, v.input.typeName)
	currentPath := parentPath + "." + currentFieldName

	key := providedFieldKey(typeName, currentFieldName, currentPath)

	_, provided := v.input.providedFields[key]

	if !provided {
		// if we are on a nested path - it means that parent was provided as we reach this
		if parentPath != "" {
			hasRootNode := v.input.dataSource.HasRootNode(typeName, currentFieldName)
			hasChildNode := v.input.dataSource.HasChildNode(typeName, currentFieldName)

			// if the field is not external under the parent
			if hasRootNode || hasChildNode {
				// we consider it accessible.
				// e.g., implicitly provided
				return
			}
		}

		v.allProvided = false
		v.walker.Stop()
	}
}
