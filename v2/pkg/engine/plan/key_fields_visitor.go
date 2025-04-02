package plan

import (
	"slices"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type keyVisitorInput struct {
	typeName        string
	parentPath      string
	key, definition *ast.Document
	report          *operationreport.Report

	dataSource      DataSource
	providesEntries []*NodeSuggestion
}

func keyFieldPaths(input *keyVisitorInput) []string {
	walker := astvisitor.NewWalker(48)
	visitor := &isKeyFieldVisitor{
		walker: &walker,
		input:  input,
	}

	walker.RegisterEnterFieldVisitor(visitor)
	walker.Walk(input.key, input.definition, input.report)

	return visitor.keyPaths
}

type isKeyFieldVisitor struct {
	walker *astvisitor.Walker
	input  *keyVisitorInput

	keyPaths []string
}

func (v *isKeyFieldVisitor) EnterField(ref int) {
	fieldName := v.input.key.FieldNameUnsafeString(ref)

	parentPath := v.input.parentPath + strings.TrimPrefix(v.walker.Path.DotDelimitedString(), v.input.typeName)
	currentPath := parentPath + "." + fieldName

	v.keyPaths = append(v.keyPaths, currentPath)
}

type KeyInfo struct {
	DSHash       DSHash
	Source       bool
	Target       bool
	TypeName     string
	SelectionSet string
	FieldPaths   []string
}

/*

Type of keys

1. Explicit - source/target
# Source: true, Target: true
type Entity @key(fields: "id") {
	id: ID!
}

2. Explicit conditional - target only, conditional source

type Query {
	# Source: true, Target: true
	entitiesSource: [Entity] @provides(fields: "id")
	# Source: false, Target: true
	entitiesTargetOnly: [Entity]
}

type Entity @key(fields: "id") {
	id: ID! @external
}

3. Explicit resolvable false - source only

# Source: true, Target: false
type Entity @key(fields: "id", resolvable: false) {
	id: ID!
}

4. Implicit - source only

# Source: true, Target: false
type Entity {
	id: ID!
}

5. Implicit conditional - source only conditional

type Query {
	# Source: true, Target: false
	entitiesSource: [Entity] @provides(fields: "id")
	# Source: false, Target: false - such key should not be added
	entitiesTargetOnly: [Entity]
}

type Entity {
	id: ID! @external
}


*/

func (f *collectNodesVisitor) collectKeysForPath(typeName, parentPath string) {
	allKeys := f.dataSource.FederationConfiguration().Keys
	keys := allKeys.FilterByTypeAndResolvability(typeName, false)
	if len(keys) == 0 {
		return
	}

	typeNameKeys := make([]KeyInfo, 0, len(keys))

	for _, key := range keys {
		fieldSet, report := RequiredFieldsFragment(typeName, key.SelectionSet, false)
		if report.HasErrors() {
			// TODO: handle error
			return
		}

		input := &keyVisitorInput{
			typeName:   typeName,
			key:        fieldSet,
			definition: f.definition,
			report:     report,
			parentPath: parentPath,

			dataSource:      f.dataSource,
			providesEntries: f.providesEntries,
		}

		keyPaths, hasExternalFields := keyInfo(input)

		target := !key.DisableEntityResolver
		source := !hasExternalFields // provided counted as not external

		if !target && !source {
			// could not be a usable key
			continue
		}

		keyInfo := KeyInfo{
			DSHash:       f.dataSource.Hash(),
			Source:       source,
			Target:       target,
			TypeName:     typeName,
			SelectionSet: key.SelectionSet,
			FieldPaths:   keyPaths,
		}

		typeNameKeys = append(typeNameKeys, keyInfo)
	}

	f.keys = append(f.keys, DSKeyInfo{
		DSHash:   f.dataSource.Hash(),
		TypeName: typeName,
		Path:     parentPath,
		Keys:     typeNameKeys,
	})
}

func keyInfo(input *keyVisitorInput) (keyPaths []string, hasExternalFields bool) {
	walker := astvisitor.NewWalker(48)
	visitor := &keyInfoVisitor{
		walker: &walker,
		input:  input,
	}

	walker.RegisterEnterFieldVisitor(visitor)
	walker.Walk(input.key, input.definition, input.report)

	return visitor.keyPaths, visitor.hasExternalFields
}

type keyInfoVisitor struct {
	walker *astvisitor.Walker
	input  *keyVisitorInput

	keyPaths          []string
	hasExternalFields bool
}

func (v *keyInfoVisitor) EnterField(ref int) {
	fieldName := v.input.key.FieldNameUnsafeString(ref)
	typeName := v.walker.EnclosingTypeDefinition.NameString(v.input.definition)

	parentPath := v.input.parentPath + strings.TrimPrefix(v.walker.Path.DotDelimitedString(), v.input.typeName)
	currentPath := parentPath + "." + fieldName

	hasRootNode := v.input.dataSource.HasRootNode(typeName, fieldName)
	hasChildNode := v.input.dataSource.HasChildNode(typeName, fieldName)

	isExternalRootNode := v.input.dataSource.HasExternalRootNode(typeName, fieldName)
	isExternalChildNode := v.input.dataSource.HasExternalChildNode(typeName, fieldName)

	hasNode := hasRootNode || hasChildNode || isExternalRootNode || isExternalChildNode

	if !hasNode {
		// TODO: report an error
		return
	}

	isExternal := isExternalRootNode || isExternalChildNode
	if isExternal {
		isProvided := slices.ContainsFunc(v.input.providesEntries, func(suggestion *NodeSuggestion) bool {
			return suggestion.TypeName == typeName && suggestion.FieldName == fieldName && suggestion.Path == currentPath
		})

		if isProvided {
			isExternal = false
		}
	}

	v.keyPaths = append(v.keyPaths, currentPath)

	if isExternal {
		v.hasExternalFields = true
	}
}
