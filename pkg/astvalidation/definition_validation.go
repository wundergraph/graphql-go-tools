package astvalidation

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func DefaultDefinitionValidator() *DefinitionValidator {
	return NewDefinitionValidator(
		PopulatedTypeBodies(),
		UniqueOperationTypes(),
		UniqueTypeNames(),
		UniqueFieldDefinitionNames(),
		UniqueEnumValueNames(),
		UniqueUnionMemberTypes(),
		KnownTypeNames(),
		RequireDefinedTypesForExtensions(),
		ImplementTransitiveInterfaces(),
		ImplementingTypesAreSupersets(),
	)
}

func NewDefinitionValidator(rules ...Rule) *DefinitionValidator {
	validator := &DefinitionValidator{
		walker: astvisitor.NewWalker(48),
	}

	for _, rule := range rules {
		validator.RegisterRule(rule)
	}

	return validator
}

type DefinitionValidator struct {
	walker astvisitor.Walker
}

func (d *DefinitionValidator) RegisterRule(rule Rule) {
	rule(&d.walker)
}

func (d *DefinitionValidator) Validate(definition *ast.Document, report *operationreport.Report) ValidationState {
	if report == nil {
		report = &operationreport.Report{}
	}

	d.walker.Walk(definition, nil, report)

	if report.HasErrors() {
		return Invalid
	}
	return Valid
}

func IsRootType(nameBytes []byte) bool {
	length := len(nameBytes)
	return isQuery(length, nameBytes) || isMutation(length, nameBytes) || isSubscription(length, nameBytes)
}

func isQuery(length int, b []byte) bool {
	return length == 5 && b[0] == 'Q' && b[1] == 'u' && b[2] == 'e' && b[3] == 'r' && b[4] == 'y'
}

func isMutation(length int, b []byte) bool {
	return length == 8 && b[0] == 'M' && b[1] == 'u' && b[2] == 't' && b[3] == 'a' && b[4] == 't' && b[5] == 'i' && b[6] == 'o' && b[7] == 'n'
}

func isSubscription(length int, b []byte) bool {
	return length == 12 && b[0] == 'S' && b[1] == 'u' && b[2] == 'b' && b[3] == 's' && b[4] == 'c' && b[5] == 'r' && b[6] == 'i' && b[7] == 'p' && b[8] == 't' && b[9] == 'i' && b[10] == 'o' && b[11] == 'n'
}
