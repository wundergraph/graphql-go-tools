package astvalidation

import (
	"bytes"
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
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
	if !p.definition.EnumTypeDefinitions[ref].HasEnumValuesDefinition {
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("enum", p.definition.EnumTypeDefinitionNameString(ref)))
		return
	}
}

func (p *populatedTypeBodiesVisitor) EnterEnumTypeExtension(ref int) {
	if !p.definition.EnumTypeExtensions[ref].HasEnumValuesDefinition {
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("enum extension", p.definition.EnumTypeExtensionNameString(ref)))
		return
	}
}

func (p populatedTypeBodiesVisitor) EnterInputObjectTypeDefinition(ref int) {
	if !p.definition.InputObjectTypeDefinitions[ref].HasInputFieldsDefinition {
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("input", p.definition.InputObjectTypeDefinitionNameString(ref)))
		return
	}
}

func (p *populatedTypeBodiesVisitor) EnterInputObjectTypeExtension(ref int) {
	if !p.definition.InputObjectTypeExtensions[ref].HasInputFieldsDefinition {
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("input extension", p.definition.InputObjectTypeExtensionNameString(ref)))
		return
	}
}

func (p populatedTypeBodiesVisitor) EnterInterfaceTypeDefinition(ref int) {
	switch p.definition.InterfaceTypeDefinitions[ref].HasFieldDefinitions {
	case true:
		if !p.doesTypeOnlyContainReservedFields(p.definition.InterfaceTypeDefinitions[ref].FieldsDefinition.Refs) {
			return
		}
		fallthrough
	case false:
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("interface", p.definition.InterfaceTypeDefinitionNameString(ref)))
		return
	}
}

func (p *populatedTypeBodiesVisitor) EnterInterfaceTypeExtension(ref int) {
	if !p.definition.InterfaceTypeExtensions[ref].HasFieldDefinitions {
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("interface extension", p.definition.InterfaceTypeExtensionNameString(ref)))
		return
	}
}

func (p populatedTypeBodiesVisitor) EnterObjectTypeDefinition(ref int) {
	nameBytes := p.definition.ObjectTypeDefinitionNameBytes(ref)
	object := p.definition.ObjectTypeDefinitions[ref]
	switch object.HasFieldDefinitions {
	case true:
		if ast.IsRootType(nameBytes) || !p.doesTypeOnlyContainReservedFields(p.definition.ObjectTypeDefinitions[ref].FieldsDefinition.Refs) {
			return
		}
		fallthrough
	case false:
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("object", string(nameBytes)))
		return
	}
}

func (p *populatedTypeBodiesVisitor) EnterObjectTypeExtension(ref int) {
	if !p.definition.ObjectTypeExtensions[ref].HasFieldDefinitions {
		p.Report.AddExternalError(operationreport.ErrTypeBodyMustNotBeEmpty("object extension", p.definition.ObjectTypeExtensionNameString(ref)))
		return
	}
}

func (p *populatedTypeBodiesVisitor) doesTypeOnlyContainReservedFields(refs []int) bool {
	for _, fieldRef := range refs {
		fieldNameBytes := p.definition.FieldDefinitionNameBytes(fieldRef)
		if len(fieldNameBytes) < 2 || !bytes.HasPrefix(fieldNameBytes, reservedFieldPrefix) {
			return false
		}
	}
	return true
}
