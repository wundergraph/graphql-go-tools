package ast

import "github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"

type SchemaExtension struct {
	SchemaDefinition

	ExtendLiteral position.Position
}

func (d *Document) ExtendSchemaDefinitionBySchemaExtension(schemaDefinitionRef, schemaExtensionRef int) {
	if d.SchemaExtensions[schemaExtensionRef].HasDirectives {
		d.SchemaDefinitions[schemaDefinitionRef].Directives.Refs = append(d.SchemaDefinitions[schemaDefinitionRef].Directives.Refs, d.SchemaExtensions[schemaExtensionRef].Directives.Refs...)
		d.SchemaDefinitions[schemaDefinitionRef].HasDirectives = true
	}

	d.SchemaDefinitions[schemaDefinitionRef].AddRootOperationTypeDefinitionRefs(d.SchemaExtensions[schemaExtensionRef].RootOperationTypeDefinitions.Refs...)

	d.Index.MergedTypeExtensions = append(d.Index.MergedTypeExtensions, Node{Ref: schemaExtensionRef, Kind: NodeKindSchemaExtension})
}

// ImportAndExtendSchemaDefinitionBySchemaExtension turns an orphan schema extension into a schema definition.
// The definition is appended to the root nodes, as prepending would shift the root nodes of an ongoing walk
// and make the walker visit the same schema extension again.
func (d *Document) ImportAndExtendSchemaDefinitionBySchemaExtension(schemaExtensionRef int) {
	schemaDefinitionRef := d.AddSchemaDefinition(SchemaDefinition{})
	d.RootNodes = append(d.RootNodes, Node{Kind: NodeKindSchemaDefinition, Ref: schemaDefinitionRef})
	d.ExtendSchemaDefinitionBySchemaExtension(schemaDefinitionRef, schemaExtensionRef)
}
