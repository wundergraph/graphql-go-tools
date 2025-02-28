package ast

import (
	"bytes"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
)

type VariableDefinitionList struct {
	LPAREN position.Position // (
	Refs   []int             // VariableDefinition
	RPAREN position.Position // )
}

// VariableDefinition
// example:
// $devicePicSize: Int = 100 @small
type VariableDefinition struct {
	VariableValue Value             // $ Name
	Colon         position.Position // :
	Type          int               // e.g. String
	DefaultValue  DefaultValue      // optional, e.g. = "Default"
	HasDirectives bool
	Directives    DirectiveList // optional, e.g. @foo
}

func (d *Document) VariableDefinitionNameBytes(ref int) ByteSlice {
	return d.VariableValueNameBytes(d.VariableDefinitions[ref].VariableValue.Ref)
}

func (d *Document) VariableDefinitionNameString(ref int) string {
	return unsafebytes.BytesToString(d.VariableValueNameBytes(d.VariableDefinitions[ref].VariableValue.Ref))
}

func (d *Document) VariableDefinitionHasDefaultValue(ref int) bool {
	return d.VariableDefinitions[ref].DefaultValue.IsDefined
}

func (d *Document) VariableDefinitionDefaultValue(ref int) Value {
	return d.VariableDefinitions[ref].DefaultValue.Value
}

func (d *Document) VariableDefinitionType(ref int) int {
	return d.VariableDefinitions[ref].Type
}

func (d *Document) VariableDefinitionByNameAndOperation(operationDefinitionRef int, name ByteSlice) (definition int, exists bool) {
	if !d.OperationDefinitions[operationDefinitionRef].HasVariableDefinitions {
		return -1, false
	}
	for _, i := range d.OperationDefinitions[operationDefinitionRef].VariableDefinitions.Refs {
		definitionName := d.VariableValueNameBytes(d.VariableDefinitions[i].VariableValue.Ref)
		if bytes.Equal(name, definitionName) {
			return i, true
		}
	}
	return -1, false
}

func (d *Document) VariableDefinitionsBefore(variableDefinition int) bool {
	for i := range d.OperationDefinitions {
		for j, k := range d.OperationDefinitions[i].VariableDefinitions.Refs {
			if k == variableDefinition {
				return j != 0
			}
		}
	}
	return false
}

func (d *Document) VariableDefinitionsAfter(variableDefinition int) bool {
	for i := range d.OperationDefinitions {
		for j, k := range d.OperationDefinitions[i].VariableDefinitions.Refs {
			if k == variableDefinition {
				return j != len(d.OperationDefinitions[i].VariableDefinitions.Refs)-1
			}
		}
	}
	return false
}

func (d *Document) VariablePathByArgumentRefAndArgumentPath(argumentRef int, argumentPath []string, operationDefinitionRef int) ([]string, error) {
	argumentValue := d.ArgumentValue(argumentRef)
	if argumentValue.Kind != ValueKindVariable {
		return nil, fmt.Errorf(`expected argument to be kind "ValueKindVariable" but received "%s"`, argumentValue.Kind)
	}
	variableNameBytes := d.VariableValueNameBytes(argumentValue.Ref)
	if _, ok := d.VariableDefinitionByNameAndOperation(operationDefinitionRef, variableNameBytes); !ok {
		return nil, fmt.Errorf(`expected definition for variable "%s" to exist`, variableNameBytes)
	}
	// The variable path should be the variable name, e.g., "a", and then the 2nd element from the path onwards
	return append([]string{string(variableNameBytes)}, argumentPath[1:]...), nil
}
