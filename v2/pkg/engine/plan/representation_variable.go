package plan

import (
	"bytes"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type representationObjectFields struct {
	popOnField int
	isRoot     bool
	fields     *[]*resolve.Field
}

// BuildRepresentationVariableNode builds a resolve.Object node from a FederationFieldConfiguration
// and the given AST definition. It creates a representation variable with __typename and the fields
// specified in the configuration's SelectionSet.
func BuildRepresentationVariableNode(definition *ast.Document, cfg FederationFieldConfiguration, federationCfg FederationMetaData) (*resolve.Object, error) {
	key, report := RequiredFieldsFragment(cfg.TypeName, cfg.SelectionSet, false)
	if report.HasErrors() {
		return nil, report
	}

	walker := astvisitor.WalkerFromPool()
	defer walker.Release()

	var interfaceObjectTypeName *string
	for _, interfaceObjCfg := range federationCfg.InterfaceObjects {
		if slices.Contains(interfaceObjCfg.ConcreteTypeNames, cfg.TypeName) {
			interfaceObjectTypeName = &interfaceObjCfg.InterfaceTypeName
			break
		}
	}
	var entityInterfaceTypeName *string
	for _, entityInterfaceCfg := range federationCfg.EntityInterfaces {
		if slices.Contains(entityInterfaceCfg.ConcreteTypeNames, cfg.TypeName) {
			entityInterfaceTypeName = &entityInterfaceCfg.InterfaceTypeName
			break
		}
	}

	visitor := &planRepresentationVariableVisitor{
		typeName:                cfg.TypeName,
		interfaceObjectTypeName: interfaceObjectTypeName,
		entityInterfaceTypeName: entityInterfaceTypeName,
		addOnType:               true,
		addTypeName:             true,
		remapPaths:              cfg.RemappedPaths,
		Walker:                  walker,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)

	walker.Walk(key, definition, report)
	if report.HasErrors() {
		return nil, report
	}

	return visitor.rootObject, nil
}

// MergeRepresentationVariableNodes merges multiple representation variable objects into one.
func MergeRepresentationVariableNodes(objects []*resolve.Object) *resolve.Object {
	fieldCount := 0
	for _, object := range objects {
		fieldCount += len(object.Fields)
	}

	fields := make([]*resolve.Field, 0, fieldCount)

	for _, object := range objects {
		for _, field := range object.Fields {
			if i, ok := representationFieldsHasField(fields, field); ok {
				fields[i] = mergeRepresentationFields(fields[i], field)
			} else {
				fields = append(fields, field)
			}
		}
	}

	return &resolve.Object{
		Nullable: true,
		Fields:   fields,
	}
}

func mergeRepresentationFields(left, right *resolve.Field) *resolve.Field {
	switch left.Value.NodeKind() {
	case resolve.NodeKindObject:
		left.Value = mergeRepresentationObjects(left.Value, right.Value)
	case resolve.NodeKindArray:
		left.Value = mergeRepresentationArrays(left.Value, right.Value)
	}
	return left
}

func mergeRepresentationArrays(left, right resolve.Node) resolve.Node {
	leftArray, _ := left.(*resolve.Array)
	rightArray, _ := right.(*resolve.Array)
	if leftArray.Item.NodeKind() == resolve.NodeKindObject {
		leftArray.Item = mergeRepresentationObjects(leftArray.Item, rightArray.Item)
	}
	return leftArray
}

func mergeRepresentationObjects(left, right resolve.Node) resolve.Node {
	leftObject, _ := left.(*resolve.Object)
	rightObject, _ := right.(*resolve.Object)
	for _, field := range rightObject.Fields {
		if i, ok := representationFieldsHasField(leftObject.Fields, field); ok {
			leftObject.Fields[i] = mergeRepresentationFields(leftObject.Fields[i], field)
		} else {
			leftObject.Fields = append(leftObject.Fields, field)
		}
	}
	return leftObject
}

func representationIsOnTypeEqual(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

func representationFieldsHasField(fields []*resolve.Field, field *resolve.Field) (int, bool) {
	for i, f := range fields {
		if bytes.Equal(f.Name, field.Name) && representationIsOnTypeEqual(f.OnTypeNames, field.OnTypeNames) {
			return i, true
		}
	}
	return -1, false
}

type planRepresentationVariableVisitor struct {
	*astvisitor.Walker

	key, definition *ast.Document

	currentFields []representationObjectFields
	rootObject    *resolve.Object

	typeName                string
	interfaceObjectTypeName *string
	entityInterfaceTypeName *string

	addOnType   bool
	addTypeName bool
	remapPaths  map[string]string
}

func (v *planRepresentationVariableVisitor) EnterDocument(key, definition *ast.Document) {
	v.key = key
	v.definition = definition

	fields := make([]*resolve.Field, 0, 2)
	if v.addTypeName {
		typeNameField := &resolve.Field{
			Name: []byte("__typename"),
		}

		if v.interfaceObjectTypeName != nil {
			typeNameField.Value = &resolve.StaticString{
				Path:  []string{"__typename"},
				Value: *v.interfaceObjectTypeName,
			}
		} else {
			typeNameField.Value = &resolve.String{
				Path: []string{"__typename"},
			}
		}

		if v.addOnType {
			v.addTypeNameToField(typeNameField)
		}

		fields = append(fields, typeNameField)
	}

	v.rootObject = &resolve.Object{
		Nullable: true,
		Fields:   fields,
	}

	v.currentFields = append(v.currentFields, representationObjectFields{
		fields:     &v.rootObject.Fields,
		popOnField: -1,
		isRoot:     true,
	})
}

func (v *planRepresentationVariableVisitor) EnterField(ref int) {
	fieldName := v.key.FieldNameBytes(ref)

	fieldDefinition, ok := v.Walker.FieldDefinition(ref)
	if !ok {
		return
	}
	fieldDefinitionType := v.definition.FieldDefinitionType(fieldDefinition)

	currentPath := v.Walker.Path.DotDelimitedString() + "." + string(fieldName)

	fieldPath := string(fieldName)
	if remapPath, ok := v.remapPaths[currentPath]; ok {
		fieldPath = remapPath
	}

	currentField := &resolve.Field{
		Name:        fieldName,
		Value:       v.resolveFieldValue(ref, fieldDefinitionType, true, []string{fieldPath}),
		OnTypeNames: v.resolveOnTypeNames(ref),
	}

	if v.addOnType && v.currentFields[len(v.currentFields)-1].isRoot {
		v.addTypeNameToField(currentField)
	}

	*v.currentFields[len(v.currentFields)-1].fields = append(*v.currentFields[len(v.currentFields)-1].fields, currentField)
}

func (v *planRepresentationVariableVisitor) addTypeNameToField(field *resolve.Field) {
	switch {
	case v.interfaceObjectTypeName != nil:
		field.OnTypeNames = [][]byte{[]byte(v.typeName), []byte(*v.interfaceObjectTypeName)}
	case v.entityInterfaceTypeName != nil:
		field.OnTypeNames = [][]byte{[]byte(v.typeName), []byte(*v.entityInterfaceTypeName)}
	default:
		field.OnTypeNames = [][]byte{[]byte(v.typeName)}
	}
}

func (v *planRepresentationVariableVisitor) LeaveField(ref int) {
	if v.currentFields[len(v.currentFields)-1].popOnField == ref {
		v.currentFields = v.currentFields[:len(v.currentFields)-1]
	}
}

func (v *planRepresentationVariableVisitor) resolveFieldValue(fieldRef, typeRef int, nullable bool, path []string) resolve.Node {
	ofType := v.definition.Types[typeRef].OfType

	switch v.definition.Types[typeRef].TypeKind {
	case ast.TypeKindNonNull:
		return v.resolveFieldValue(fieldRef, ofType, false, path)
	case ast.TypeKindList:
		listItem := v.resolveFieldValue(fieldRef, ofType, true, nil)
		return &resolve.Array{
			Nullable: nullable,
			Path:     path,
			Item:     listItem,
		}
	case ast.TypeKindNamed:
		typeName := v.definition.ResolveTypeNameString(typeRef)
		typeDefinitionNode, ok := v.definition.Index.FirstNodeByNameStr(typeName)
		if !ok {
			return &resolve.Null{}
		}
		switch typeDefinitionNode.Kind {
		case ast.NodeKindScalarTypeDefinition:
			switch typeName {
			case "String":
				return &resolve.String{Path: path, Nullable: nullable}
			case "Boolean":
				return &resolve.Boolean{Path: path, Nullable: nullable}
			case "Int":
				return &resolve.Integer{Path: path, Nullable: nullable}
			case "Float":
				return &resolve.Float{Path: path, Nullable: nullable}
			default:
				return &resolve.Scalar{Path: path, Nullable: nullable}
			}
		case ast.NodeKindEnumTypeDefinition:
			return &resolve.String{Path: path, Nullable: nullable}
		case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition, ast.NodeKindUnionTypeDefinition:
			object := &resolve.Object{
				Nullable: nullable,
				Path:     path,
				Fields:   []*resolve.Field{},
			}
			v.Walker.DefferOnEnterField(func() {
				v.currentFields = append(v.currentFields, representationObjectFields{
					popOnField: fieldRef,
					fields:     &object.Fields,
				})
			})
			return object
		default:
			return &resolve.Null{}
		}
	default:
		return &resolve.Null{}
	}
}

func (v *planRepresentationVariableVisitor) resolveOnTypeNames(fieldRef int) [][]byte {
	if len(v.Walker.Ancestors) < 2 {
		return nil
	}
	inlineFragment := v.Walker.Ancestors[len(v.Walker.Ancestors)-2]
	if inlineFragment.Kind != ast.NodeKindInlineFragment {
		return nil
	}
	typeName := v.key.InlineFragmentTypeConditionName(inlineFragment.Ref)
	if typeName == nil {
		typeName = v.Walker.EnclosingTypeDefinition.NameBytes(v.definition)
	}
	node, exists := v.definition.NodeByName(typeName)
	if !exists || !node.Kind.IsAbstractType() {
		return [][]byte{typeName}
	}
	if node.Kind == ast.NodeKindUnionTypeDefinition {
		panic("resolveOnTypeNames called with a union type")
	}
	onTypeNames := make([][]byte, 0, 2)
	for objectTypeDefinitionRef := range v.definition.ObjectTypeDefinitions {
		if v.definition.ObjectTypeDefinitionImplementsInterface(objectTypeDefinitionRef, typeName) {
			onTypeNames = append(onTypeNames, v.definition.ObjectTypeDefinitionNameBytes(objectTypeDefinitionRef))
		}
	}
	if len(onTypeNames) == 0 {
		return nil
	}

	if len(v.Walker.TypeDefinitions) > 1 {
		grandParent := v.Walker.TypeDefinitions[len(v.Walker.TypeDefinitions)-2]
		if grandParent.Kind == ast.NodeKindUnionTypeDefinition {
			for i := 0; i < len(onTypeNames); i++ {
				possibleMember, exists := v.definition.Index.FirstNodeByNameStr(string(onTypeNames[i]))
				if !exists {
					continue
				}
				if !v.definition.NodeIsUnionMember(possibleMember, grandParent) {
					onTypeNames = append(onTypeNames[:i], onTypeNames[i+1:]...)
					i--
				}
			}
		}
		if grandParent.Kind == ast.NodeKindInterfaceTypeDefinition {
			objectTypesImplementingGrandParent, _ := v.definition.InterfaceTypeDefinitionImplementedByObjectWithNames(grandParent.Ref)
			for i := 0; i < len(onTypeNames); i++ {
				if !slices.Contains(objectTypesImplementingGrandParent, string(onTypeNames[i])) {
					onTypeNames = append(onTypeNames[:i], onTypeNames[i+1:]...)
					i--
				}
			}
		}
	}

	return onTypeNames
}
