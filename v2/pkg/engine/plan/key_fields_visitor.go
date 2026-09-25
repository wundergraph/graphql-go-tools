package plan

import (
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

	dataSource DataSource
	// providedSelection is the provided selection applying to the fields of the
	// entity at the parent path, nil when nothing is provided there
	providedSelection providesSelection
	keyIsConditional  bool
}

type KeyInfo struct {
	DSHash       DSHash
	Source       bool
	Target       bool
	TypeName     string
	SelectionSet string
	FieldPaths   []KeyInfoFieldPath
}

type KeyInfoFieldPath struct {
	Path       string
	IsExternal bool
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

func (f *collectNodesDSVisitor) collectKeysForPath(typeName, parentPath string, providedSelection providesSelection) error {
	indexKey := SeenKeyPath{
		TypeName: typeName,
		Path:     parentPath,
		DSHash:   f.dataSource.Hash(),
	}
	// global seen keys is used when we recollect nodes
	if _, ok := f.globalSeenKeys[indexKey]; ok {
		return nil
	}
	// local seen fields is used when we have multipe fields on a path, and we visit it first time
	if _, ok := f.localSeenKeys[indexKey]; ok {
		// we already collected keys for this path
		return nil
	}
	// WARNING: we are not writing to global map from go routine
	f.localSeenKeys[indexKey] = struct{}{}

	allKeys := f.dataSource.FederationConfiguration().Keys
	keys := allKeys.FilterByTypeAndResolvability(typeName, false)
	if len(keys) == 0 {
		return nil
	}

	typeNameKeys := make([]KeyInfo, 0, len(keys))

	report := &operationreport.Report{}

	for _, key := range keys {
		input := &keyVisitorInput{
			typeName:   typeName,
			key:        key.parsedSelectionSet,
			definition: f.definition,
			report:     report,
			parentPath: parentPath,

			dataSource:        f.dataSource,
			providedSelection: providedSelection,
			keyIsConditional:  len(key.Conditions) > 0,
		}

		keyPaths, hasExternalFields := getKeyPaths(input)
		if report.HasErrors() {
			return report
		}

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

		for _, path := range keyPaths {
			if path.IsExternal {
				continue
			}

			f.notExternalKeyPaths[path.Path] = struct{}{}
		}

		typeNameKeys = append(typeNameKeys, keyInfo)
	}

	f.keys = append(f.keys, DSKeyInfo{
		DSHash:   f.dataSource.Hash(),
		TypeName: typeName,
		Path:     parentPath,
		Keys:     typeNameKeys,
	})

	return nil
}

func getKeyPaths(input *keyVisitorInput) (keyPaths []KeyInfoFieldPath, hasExternalFields bool) {
	walker := astvisitor.NewWalkerWithID(48, "KeyInfoVisitor")
	visitor := &keyInfoVisitor{
		walker:         &walker,
		input:          input,
		selectionStack: []providesSelection{input.providedSelection},
	}

	walker.RegisterEnterFieldVisitor(visitor)
	walker.RegisterLeaveFieldVisitor(&keyInfoSelectionPopper{visitor: visitor})
	walker.Walk(input.key, input.definition, input.report)

	return visitor.keyPaths, visitor.hasExternalFields
}

type keyInfoVisitor struct {
	walker *astvisitor.Walker
	input  *keyVisitorInput

	keyPaths          []KeyInfoFieldPath
	hasExternalFields bool

	currentKeyPath []KeyInfoFieldPath

	// selectionStack tracks the provided selection at each nesting level of the key
	// fragment walk; entries are nil below levels which are not provided
	selectionStack []providesSelection
}

// keyInfoSelectionPopper keeps the selection stack of keyInfoVisitor in sync on field
// leave; a separate type as keyInfoVisitor has an unregistered LeaveField method
type keyInfoSelectionPopper struct {
	visitor *keyInfoVisitor
}

func (p *keyInfoSelectionPopper) LeaveField(ref int) {
	p.visitor.selectionStack = p.visitor.selectionStack[:len(p.visitor.selectionStack)-1]
}

func (v *keyInfoVisitor) EnterField(ref int) {
	fieldName := v.input.key.FieldNameUnsafeString(ref)
	typeName := v.walker.EnclosingTypeDefinition.NameString(v.input.definition)

	// resolve provided-ness against the provided selection of the current nesting
	// level and push the nested selection for the child fields
	currentSelection := v.selectionStack[len(v.selectionStack)-1]
	childSelection, isProvided := currentSelection.providedTypeSelection(fieldName, typeName)
	v.selectionStack = append(v.selectionStack, childSelection)

	parentPath := v.input.parentPath + strings.TrimPrefix(v.walker.Path.DotDelimitedString(), v.input.typeName)
	currentPath := parentPath + "." + fieldName

	hasRootNode := v.input.dataSource.HasRootNode(typeName, fieldName)
	hasChildNode := v.input.dataSource.HasChildNode(typeName, fieldName)

	hasExternalRootNode := v.input.dataSource.HasExternalRootNode(typeName, fieldName)
	hasExternalChildNode := v.input.dataSource.HasExternalChildNode(typeName, fieldName)

	hasNode := hasRootNode || hasChildNode || hasExternalRootNode || hasExternalChildNode

	if !hasNode {
		// TODO: report an error
		return
	}

	isExternal := hasExternalRootNode || hasExternalChildNode

	if isExternal {
		if isProvided {
			// if the field is provided, it should not be marked as external
			isExternal = false
		} else if hasRootNode || hasChildNode {
			// fallback for how we used to handle keys marked as external in the current composition version
			// if the key field present in both external fields and regular fields it should not be marked as external
			// this logic is parallel to what we have in collect fields visitor
			// but if key is implicit and conditional we do not apply such logic, as such keys should be provided
			// NOTE: edfs makes entity a child node so we need to have a child node check too

			if !v.input.keyIsConditional {
				isExternal = false
			}
		} else if !v.input.keyIsConditional && len(v.currentKeyPath) > 0 && !v.isRootKeyPathExternal() {

			// handles edge case when we mark direct child node as not external
			// but nested fields was external for implicit key
			// e.g.
			// type User @key(fields: "id info {name}") {
			//   id: ID!
			//   info: Info @external
			// }
			// type Info {
			//   name: String! @external
			// }
			// In the configuration User.info - will not be marked as external
			// But Info.name will be marked as external
			// so we have to bypass this case

			isExternal = false
		}

	}

	fieldKeyPath := KeyInfoFieldPath{
		Path:       currentPath,
		IsExternal: isExternal,
	}

	v.keyPaths = append(v.keyPaths, fieldKeyPath)

	if isExternal {
		v.hasExternalFields = true
	}

	v.currentKeyPath = append(v.keyPaths, fieldKeyPath)
}

func (v *keyInfoVisitor) LeaveField(ref int) {
	if len(v.currentKeyPath) == 0 {
		return
	}

	v.currentKeyPath = v.currentKeyPath[:len(v.currentKeyPath)-1]
}

func (v *keyInfoVisitor) isRootKeyPathExternal() bool {
	if len(v.currentKeyPath) == 0 {
		return false
	}

	return v.currentKeyPath[0].IsExternal
}
