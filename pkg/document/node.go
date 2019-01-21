package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexing/position"

type Node interface {
	NodeName() ByteSliceReference
	NodeAlias() ByteSliceReference
	NodeDescription() ByteSliceReference
	NodeArguments() []int
	NodeArgumentsDefinition() int
	NodeDirectives() []int
	NodeEnumValuesDefinition() []int
	NodeFields() []int
	NodeFieldsDefinition() []int
	NodeFragmentSpreads() []int
	NodeInlineFragments() []int
	NodeVariableDefinitions() []int
	NodeType() int
	NodeOperationType() OperationType
	NodeValue() int
	NodeDefaultValue() int
	NodeImplementsInterfaces() []ByteSliceReference
	InputValueDefinitionsNode
	TypeSystemDefinitionNode
	UnionTypeSystemDefinitionNode
	ValueNode
	PositionNode
}

type TypeSystemDefinitionNode interface {
	NodeSchemaDefinition() SchemaDefinition
	NodeScalarTypeDefinitions() []int
	NodeObjectTypeDefinitions() []int
	NodeInterfaceTypeDefinitions() []int
	NodeUnionTypeDefinitions() []int
	NodeEnumTypeDefinitions() []int
	NodeInputObjectTypeDefinitions() []int
	NodeDirectiveDefinitions() []int
}

type UnionTypeSystemDefinitionNode interface {
	NodeUnionMemberTypes() []ByteSliceReference
}

type ValueNode interface {
	NodeValueType() ValueType
	NodeValueReference() int
}

type PositionNode interface {
	NodePosition() position.Position
}

type InputValueDefinitionsNode interface {
	NodeInputValueDefinitions() []int
}
