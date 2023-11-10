package plan

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type SchemaUsageInfo struct {
	OperationType   ast.OperationType
	TypeFields      []TypeFieldUsageInfo
	Arguments       []ArgumentUsageInfo
	InputTypeFields []TypeFieldUsageInfo
}

type TypeFieldUsageInfo struct {
	Count      int
	FieldName  string
	NamedType  string
	TypeNames  []string
	Path       []string
	Source     TypeFieldSource
	EnumValues []string
}

func (t *TypeFieldUsageInfo) Equals(other TypeFieldUsageInfo) bool {
	if t.FieldName != other.FieldName {
		return false
	}
	if t.NamedType != other.NamedType {
		return false
	}
	if len(t.TypeNames) != len(other.TypeNames) {
		return false
	}
	for i := range t.TypeNames {
		if t.TypeNames[i] != other.TypeNames[i] {
			return false
		}
	}
	if len(t.Path) != len(other.Path) {
		return false
	}
	for i := range t.Path {
		if t.Path[i] != other.Path[i] {
			return false
		}
	}
	if len(t.Source.IDs) != len(other.Source.IDs) {
		return false
	}
	for i := range t.Source.IDs {
		if t.Source.IDs[i] != other.Source.IDs[i] {
			return false
		}
	}
	if len(t.EnumValues) != len(other.EnumValues) {
		return false
	}
	for i := range t.EnumValues {
		if t.EnumValues[i] != other.EnumValues[i] {
			return false
		}
	}
	return true
}

type ArgumentUsageInfo struct {
	FieldName        string
	NamedType        string
	ArgumentName     string
	ArgumentTypeName string
}

type TypeFieldSource struct {
	// IDs is a list of data source IDs that can be used to resolve the field
	IDs []string
}

func GetSchemaUsageInfo(plan Plan, operation, definition *ast.Document, variables []byte) (*SchemaUsageInfo, error) {
	js := astjson.Pool.Get()
	defer astjson.Pool.Put(js)
	err := js.ParseObject(variables)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	visitor := planVisitor{}
	switch p := plan.(type) {
	case *SynchronousResponsePlan:
		if p.Response.Info != nil {
			visitor.usage.OperationType = p.Response.Info.OperationType
		}
		visitor.visitNode(p.Response.Data, nil)
	case *SubscriptionResponsePlan:
		if p.Response.Response.Info != nil {
			visitor.usage.OperationType = p.Response.Response.Info.OperationType
		}
		visitor.visitNode(p.Response.Response.Data, nil)
	}
	walker := astvisitor.NewWalker(48)
	vis := &schemaUsageInfoVisitor{
		usage:      &visitor.usage,
		walker:     &walker,
		operation:  operation,
		definition: definition,
		variables:  js,
	}
	walker.RegisterInputValueDefinitionVisitor(vis)
	walker.RegisterArgumentVisitor(vis)
	walker.RegisterFieldVisitor(vis)
	walker.RegisterVariableDefinitionVisitor(vis)
	report := &operationreport.Report{}
	walker.Walk(operation, definition, report)
	if report.HasErrors() {
		return nil, errors.WithStack(fmt.Errorf("unable to generate schema usage info due to ast errors"))
	}
	return &visitor.usage, nil
}

type planVisitor struct {
	usage SchemaUsageInfo
}

func (p *planVisitor) visitNode(node resolve.Node, path []string) {
	switch t := node.(type) {
	case *resolve.Object:
		for _, field := range t.Fields {
			if field.Info == nil {
				continue
			}
			newPath := append([]string{}, append(path, field.Info.Name)...)
			p.usage.TypeFields = append(p.usage.TypeFields, TypeFieldUsageInfo{
				FieldName: field.Info.Name,
				TypeNames: field.Info.ParentTypeNames,
				NamedType: field.Info.NamedType,
				Path:      newPath,
				Source: TypeFieldSource{
					IDs: field.Info.Source.IDs,
				},
			})
			p.visitNode(field.Value, newPath)
		}
	case *resolve.Array:
		p.visitNode(t.Item, path)
	}
}

type schemaUsageInfoVisitor struct {
	usage              *SchemaUsageInfo
	walker             *astvisitor.Walker
	operation          *ast.Document
	definition         *ast.Document
	fieldEnclosingNode ast.Node
	variables          *astjson.JSON
}

func (s *schemaUsageInfoVisitor) EnterVariableDefinition(ref int) {
	varTypeRef := s.operation.VariableDefinitions[ref].Type
	varName := s.operation.VariableValueNameString(s.operation.VariableDefinitions[ref].VariableValue.Ref)
	varTypeName := s.operation.ResolveTypeNameString(varTypeRef)
	jsonField := s.variables.GetObjectField(s.variables.RootNode, varName)
	if jsonField == -1 {
		return
	}
	s.traverseVariable(jsonField, varName, varTypeName, "")
}

func (s *schemaUsageInfoVisitor) addUniqueInputFieldUsageInfoOrIncrementCount(info TypeFieldUsageInfo) {
	for i := range s.usage.InputTypeFields {
		if s.usage.InputTypeFields[i].Equals(info) {
			s.usage.InputTypeFields[i].Count++
			return
		}
	}
	info.Count = 1
	s.usage.InputTypeFields = append(s.usage.InputTypeFields, info)
}

func (s *schemaUsageInfoVisitor) traverseVariable(jsonNodeRef int, fieldName, typeName, parentTypeName string) {
	defNode, ok := s.definition.NodeByNameStr(typeName)
	if !ok {
		return
	}
	usageInfo := TypeFieldUsageInfo{
		FieldName: fieldName,
		NamedType: typeName,
	}
	switch defNode.Kind {
	case ast.NodeKindInputObjectTypeDefinition:
		for _, arrayValue := range s.variables.Nodes[jsonNodeRef].ArrayValues {
			s.traverseVariable(arrayValue, fieldName, typeName, parentTypeName)
		}
		for _, field := range s.variables.Nodes[jsonNodeRef].ObjectFields {
			key := s.variables.ObjectFieldKey(field)
			value := s.variables.ObjectFieldValue(field)
			fieldRef := s.definition.InputObjectTypeDefinitionInputValueDefinitionByName(defNode.Ref, key)
			if fieldRef == -1 {
				continue
			}
			fieldTypeName := s.definition.ResolveTypeNameString(s.definition.InputValueDefinitions[fieldRef].Type)
			if s.definition.TypeIsList(s.definition.InputValueDefinitions[fieldRef].Type) {
				for _, arrayValue := range s.variables.Nodes[value].ArrayValues {
					s.traverseVariable(arrayValue, string(key), fieldTypeName, typeName)
				}
			} else {
				s.traverseVariable(value, string(key), fieldTypeName, typeName)
			}
		}
	case ast.NodeKindEnumTypeDefinition:
		switch s.variables.Nodes[jsonNodeRef].Kind {
		case astjson.NodeKindString:
			usageInfo.EnumValues = []string{string(s.variables.Nodes[jsonNodeRef].ValueBytes(s.variables))}
		case astjson.NodeKindArray:
			usageInfo.EnumValues = make([]string, len(s.variables.Nodes[jsonNodeRef].ArrayValues))
			for i, arrayValue := range s.variables.Nodes[jsonNodeRef].ArrayValues {
				usageInfo.EnumValues[i] = string(s.variables.Nodes[arrayValue].ValueBytes(s.variables))
			}
		}
	}
	if parentTypeName != "" {
		usageInfo.TypeNames = []string{parentTypeName}
	} else {
		usageInfo.FieldName = ""
	}
	s.addUniqueInputFieldUsageInfoOrIncrementCount(usageInfo)
}

func (s *schemaUsageInfoVisitor) LeaveVariableDefinition(ref int) {

}

func (s *schemaUsageInfoVisitor) EnterField(ref int) {
	s.fieldEnclosingNode = s.walker.EnclosingTypeDefinition
}

func (s *schemaUsageInfoVisitor) LeaveField(ref int) {

}

func (s *schemaUsageInfoVisitor) EnterArgument(ref int) {
	argName := s.operation.ArgumentNameBytes(ref)
	anc := s.walker.Ancestors[len(s.walker.Ancestors)-1]
	if anc.Kind != ast.NodeKindField {
		return
	}
	fieldName := s.operation.FieldNameBytes(anc.Ref)
	enclosingTypeName := s.definition.NodeNameBytes(s.fieldEnclosingNode)
	argDef := s.definition.NodeFieldDefinitionArgumentDefinitionByName(s.fieldEnclosingNode, fieldName, argName)
	if argDef == -1 {
		return
	}
	argType := s.definition.InputValueDefinitionType(argDef)
	typeName := s.definition.ResolveTypeNameBytes(argType)
	s.usage.Arguments = append(s.usage.Arguments, ArgumentUsageInfo{
		FieldName:        string(fieldName),
		NamedType:        string(enclosingTypeName),
		ArgumentName:     string(argName),
		ArgumentTypeName: string(typeName),
	})
}

func (s *schemaUsageInfoVisitor) LeaveArgument(ref int) {

}

func (s *schemaUsageInfoVisitor) EnterInputValueDefinition(ref int) {

}

func (s *schemaUsageInfoVisitor) LeaveInputValueDefinition(ref int) {

}
