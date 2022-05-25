package astvalidation

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type populatedTypeBodiesVisitor struct {
	*astvisitor.Walker
	definition *ast.Document
}

func PopulatedTypeBodies() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := &populatedTypeBodiesVisitor{
			Walker:     walker,
			definition: nil,
		}

		walker.RegisterEnterDocumentVisitor(visitor)
		walker.RegisterEnterEnumTypeDefinitionVisitor(visitor)
		walker.RegisterEnterEnumTypeExtensionVisitor(visitor)
		walker.RegisterEnterInputObjectTypeDefinitionVisitor(visitor)
		walker.RegisterEnterInputObjectTypeExtensionVisitor(visitor)
		walker.RegisterEnterInterfaceTypeDefinitionVisitor(visitor)
		walker.RegisterEnterInterfaceTypeExtensionVisitor(visitor)
		walker.RegisterEnterObjectTypeDefinitionVisitor(visitor)
		walker.RegisterEnterObjectTypeExtensionVisitor(visitor)
	}
}

func (p *populatedTypeBodiesVisitor) EnterDocument(operation, _ *ast.Document) {
	p.definition = operation
}

func (p populatedTypeBodiesVisitor) EnterEnumTypeDefinition(ref int) {
	definition := p.definition
	if !definition.EnumTypeDefinitions[ref].HasEnumValuesDefinition {
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("enum", definition.EnumTypeDefinitionNameString(ref)))
		return
	}
}

func (p *populatedTypeBodiesVisitor) EnterEnumTypeExtension(ref int) {
	definition := p.definition
	if !definition.EnumTypeExtensions[ref].HasEnumValuesDefinition {
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("enum extension", definition.EnumTypeExtensionNameString(ref)))
		return
	}
}

func (p populatedTypeBodiesVisitor) EnterInputObjectTypeDefinition(ref int) {
	definition := p.definition
	if !definition.InputObjectTypeDefinitions[ref].HasInputFieldsDefinition {
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("input", definition.InputObjectTypeDefinitionNameString(ref)))
		return
	}
}

func (p *populatedTypeBodiesVisitor) EnterInputObjectTypeExtension(ref int) {
	definition := p.definition
	if !definition.InputObjectTypeExtensions[ref].HasInputFieldsDefinition {
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("input extension", definition.InputObjectTypeExtensionNameString(ref)))
		return
	}
}

func (p populatedTypeBodiesVisitor) EnterInterfaceTypeDefinition(ref int) {
	definition := p.definition
	switch definition.InterfaceTypeDefinitions[ref].HasFieldDefinitions {
	case true:
		if !doesTypeOnlyContainReservedFields(definition, definition.InterfaceTypeDefinitions[ref].FieldsDefinition.Refs) {
			return
		}
		fallthrough
	case false:
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("interface", definition.InterfaceTypeDefinitionNameString(ref)))
		return
	}
}

func (p *populatedTypeBodiesVisitor) EnterInterfaceTypeExtension(ref int) {
	definition := p.definition
	if !definition.InterfaceTypeExtensions[ref].HasFieldDefinitions {
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("interface extension", definition.InterfaceTypeExtensionNameString(ref)))
		return
	}
}

func (p populatedTypeBodiesVisitor) EnterObjectTypeDefinition(ref int) {
	definition := p.definition
	nameBytes := definition.ObjectTypeDefinitionNameBytes(ref)
	object := definition.ObjectTypeDefinitions[ref]
	switch object.HasFieldDefinitions {
	case true:
		if ast.IsRootType(nameBytes) || !doesTypeOnlyContainReservedFields(definition, definition.ObjectTypeDefinitions[ref].FieldsDefinition.Refs) {
			return
		}
		fallthrough
	case false:
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("object", string(nameBytes)))
		return
	}
}

func (p *populatedTypeBodiesVisitor) EnterObjectTypeExtension(ref int) {
	definition := p.definition
	if !definition.ObjectTypeExtensions[ref].HasFieldDefinitions {
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("object extension", definition.ObjectTypeExtensionNameString(ref)))
		return
	}
}

func doesTypeOnlyContainReservedFields(definition *ast.Document, refs []int) bool {
	for _, fieldRef := range refs {
		fieldNameBytes := definition.FieldDefinitionNameBytes(fieldRef)
		if len(fieldNameBytes) < 2 || !bytes.HasPrefix(fieldNameBytes, reservedFieldPrefix) {
			return false
		}
	}
	return true
}
