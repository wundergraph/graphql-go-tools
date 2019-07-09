package ast

import "github.com/jensneuse/graphql-go-tools/pkg/input"

type Visitor struct {
	definitionInput    *input.Input
	definitionDocument *Document
	operation          *Document
	operationInput     *input.Input
	fieldVisitor       FieldVisitor
}

type FieldVisitor func(field Field)

func (v *Visitor) Visit() {
	for _, operation := range v.operation.OperationDefinitions {
		if operation.OperationType == OperationTypeQuery {

			opName := v.operationInput.ByteSlice(operation.Name)
			_ = opName

			for operation.SelectionSet.Next(v.operation) {
				selection, _ := operation.SelectionSet.Value()
				if selection.Kind == SelectionKindField {
					field := v.operation.Fields[selection.Ref]

					v.fieldVisitor(field)
					v.traverseField(field)
				}
			}
		}
	}
}

func (v *Visitor) traverseField(field Field) {
	for field.SelectionSet.Next(v.operation) {
		selection, _ := field.SelectionSet.Value()
		if selection.Kind == SelectionKindField {
			field := v.operation.Fields[selection.Ref]
			v.fieldVisitor(field)
			v.traverseField(field)
		}
	}
}
