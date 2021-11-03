package plan

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
)

const (
	federationKeyDirectiveName      = "key"
	federationRequireDirectiveName  = "requires"
	federationExternalDirectiveName = "external"
)

// LocalTypeFieldExtractor takes an ast.Document as input and generates the
// TypeField configuration for both root and child nodes. Root nodes are the
// root operation types (usually Query, Mutation and Schema--though these types
// can be configured via the schema keyword) plus "entities" as defined by the
// Apollo federation specification. In short, entities are types with a @key
// directive. Child nodes are field types recursively accessible via a root
// node. Nodes are either object or interface definitions or extensions. Root
// nodes only include "local" fields; they don't include fields that have the
// @external directive.
type LocalTypeFieldExtractor struct {
	document *ast.Document
}

func NewLocalTypeFieldExtractor(document *ast.Document) *LocalTypeFieldExtractor {
	return &LocalTypeFieldExtractor{document: document}
}

type nodeInformation struct {
	isInterface       bool
	isRoot            bool
	concreteTypeNames []string
	localFieldRefs    []int
	externalFieldRefs []int
}

// appendIfNotPresent appends a string to the given slice if the string isn't
// already present in the slice.
func appendIfNotPresent(slice []string, value string) []string {
	var hasValue bool
	for _, existingValue := range slice {
		if value == existingValue {
			hasValue = true
			break
		}
	}
	if !hasValue {
		return append(slice, value)
	}
	return slice
}

// GetAllNodes returns all root and child nodes in the document associated with
// the LocalTypeFieldExtractor. See LocalTypeFieldExtractor for a detailed
// explanation of what root and child nodes are.
func (e *LocalTypeFieldExtractor) GetAllNodes() ([]TypeField, []TypeField) {
	// The strategy for the extractor is as follows:
	//
	// 1. Loop over each node in the document and collect information into
	//    "node info" structs. All document nodes are processed before creating
	//    the final "root" and "child" plan nodes because multiple document
	//    nodes may correspond to a single "node info" struct. For example,
	//    `type User { ... }` and `extend type User { ... }` nodes will
	//    correspond to a single User struct.
	//
	// 2. Build root nodes for each node info struct identified as a root node.
	//
	// 3. Push the root node info structs into a queue and construct a child
	//    node for each info struct in the queue. After constructing a child
	//    node, loop over the fields of the child type and add any object or
	//    abstract type to the queue if the type hasn't yet been processed. An
	//    abstract type is either an interface or union. When processing
	//    abstract types, also add the corresponding concrete types to the
	//    queue (i.e. all the types that implement an interface and union
	//    members). Note that child nodes aren't created for union types--only
	//    union members--since it ISN'T possible to select directly from a
	//    union; union selection sets MUST contain fragments.

	nodeInfoMap := make(map[string]*nodeInformation, len(e.document.RootNodes))
	possibleInterfaceTypes := map[string][]string{}
	var rootNodeNames []string

	// https://spec.graphql.org/June2018/#sec-Root-Operation-Types
	queryType := string(e.document.Index.QueryTypeName)
	if queryType == "" {
		queryType = "Query"
	}
	mutationType := string(e.document.Index.MutationTypeName)
	if mutationType == "" {
		mutationType = "Mutation"
	}
	subscriptionType := string(e.document.Index.SubscriptionTypeName)
	if subscriptionType == "" {
		subscriptionType = "Subscription"
	}

	// 1. Loop over each node in the document (see description above).
	for _, astNode := range e.document.RootNodes {
		var isInterface bool
		var concreteTypeNames []string
		typeName := e.document.NodeNameString(astNode)

		switch astNode.Kind {
		case ast.NodeKindObjectTypeDefinition, ast.NodeKindObjectTypeExtension:
			for _, ref := range e.interfaceRefs(astNode) {
				interfaceName := e.document.ResolveTypeNameString(ref)
				// The document doesn't provide a way to directly look up the
				// types that implement an interface, so instead we track the
				// interfaces implemented for each type and after all nodes
				// have been processed record the concrete types for each
				// interface.
				possibleInterfaceTypes[interfaceName] = append(
					possibleInterfaceTypes[interfaceName], typeName)
			}
		case ast.NodeKindInterfaceTypeDefinition, ast.NodeKindInterfaceTypeExtension:
			isInterface = true
		case ast.NodeKindUnionTypeDefinition, ast.NodeKindUnionTypeExtension:
			for _, ref := range e.unionMemberRefs(astNode) {
				memberName := e.document.ResolveTypeNameString(ref)
				concreteTypeNames = append(concreteTypeNames, memberName)
			}
		default:
			continue
		}

		nodeInfo, ok := nodeInfoMap[typeName]
		if !ok {
			nodeInfo = &nodeInformation{}
			nodeInfoMap[typeName] = nodeInfo
		}

		hasKey := e.NodeHasKeyDirective(astNode)
		isFederationEntity := hasKey && !isInterface

		isRootNode := typeName == queryType ||
			typeName == mutationType ||
			typeName == subscriptionType ||
			isFederationEntity

		nodeInfo.isInterface = isInterface
		// A node may be a local extension of a root node. The node is
		// considered a root node if ANY node related to the type is a root
		// node.
		nodeInfo.isRoot = nodeInfo.isRoot || isRootNode
		// Local union extensions are disjoint. For details, see the GraphQL
		// spec: https://spec.graphql.org/October2021/#sec-Union-Extensions
		nodeInfo.concreteTypeNames = append(nodeInfo.concreteTypeNames, concreteTypeNames...)

		if isRootNode {
			rootNodeNames = appendIfNotPresent(rootNodeNames, typeName)
		}

		// Record the local and external fields separately for later
		// processing. Root nodes only include local fields, while child nodes
		// include both local and external fields.
		for _, ref := range e.document.NodeFieldDefinitions(astNode) {
			isExternal := e.document.FieldDefinitionHasNamedDirective(ref,
				federationExternalDirectiveName)

			if isExternal {
				nodeInfo.externalFieldRefs = append(nodeInfo.externalFieldRefs, ref)
			} else {
				nodeInfo.localFieldRefs = append(nodeInfo.localFieldRefs, ref)
			}
		}
	}

	// Record the concrete types for each interface.
	for interfaceName, concreteTypeNames := range possibleInterfaceTypes {
		if nodeInfo, ok := nodeInfoMap[interfaceName]; ok {
			nodeInfo.concreteTypeNames = concreteTypeNames
		}
	}

	// This is the queue used in step 3, child node construction.
	childrenSeen := make(map[string]struct{}, len(nodeInfoMap))
	childrenToProcess := make([]string, 0, len(nodeInfoMap))

	// pushChildIfNotAlreadyProcessed pushes a child type onto the queue if it
	// hasn't already been processed. Only types with node info are pushed onto
	// the queue. Recall that node info is limited to object types, interfaces
	// and union members above.
	pushChildIfNotAlreadyProcessed := func(typeName string) {
		if _, ok := childrenSeen[typeName]; !ok {
			if _, ok := nodeInfoMap[typeName]; ok {
				childrenToProcess = append(childrenToProcess, typeName)
			}
			childrenSeen[typeName] = struct{}{}
		}
	}

	// processFieldRef pushes node info for the field's type as well as--in the
	// case of abstract types--node info for each concrete type.
	processFieldRef := func(ref int) string {
		fieldType := e.document.FieldDefinitionType(ref)
		fieldTypeName := e.document.ResolveTypeNameString(fieldType)
		pushChildIfNotAlreadyProcessed(fieldTypeName)
		if nodeInfo, ok := nodeInfoMap[fieldTypeName]; ok {
			for _, name := range nodeInfo.concreteTypeNames {
				pushChildIfNotAlreadyProcessed(name)
			}
		}
		return e.document.FieldDefinitionNameString(ref)
	}

	var rootNodes, childNodes []TypeField

	// 2. Create the root nodes. Also, loop over the fields to find additional
	// child nodes to process.
	for _, typeName := range rootNodeNames {
		nodeInfo := nodeInfoMap[typeName]
		numFields := len(nodeInfo.localFieldRefs)
		if numFields == 0 {
			continue
		}
		fieldNames := make([]string, numFields)
		for i, ref := range nodeInfo.localFieldRefs {
			fieldNames[i] = processFieldRef(ref)
		}
		rootNodes = append(rootNodes, TypeField{
			TypeName:   typeName,
			FieldNames: fieldNames,
		})
	}

	// 3. Process the child node queue to create child nodes. When processing
	// child nodes, loop over the fields of the child to find additional
	// children to process.
	for len(childrenToProcess) > 0 {
		typeName := childrenToProcess[len(childrenToProcess)-1]
		childrenToProcess = childrenToProcess[:len(childrenToProcess)-1]
		nodeInfo, ok := nodeInfoMap[typeName]
		if !ok {
			continue
		}
		numFields := len(nodeInfo.localFieldRefs) + len(nodeInfo.externalFieldRefs)
		if numFields == 0 {
			continue
		}
		fieldNames := make([]string, 0, numFields)
		for _, ref := range nodeInfo.localFieldRefs {
			fieldNames = append(fieldNames, processFieldRef(ref))
		}
		for _, ref := range nodeInfo.externalFieldRefs {
			fieldNames = append(fieldNames, processFieldRef(ref))
		}
		childNodes = append(childNodes, TypeField{
			TypeName:   typeName,
			FieldNames: fieldNames,
		})
	}

	return rootNodes, childNodes
}

// interfaceRefs returns the interfaces implemented by the given node (this is
// only applicable to object kinds).
func (e *LocalTypeFieldExtractor) interfaceRefs(node ast.Node) []int {
	switch node.Kind {
	case ast.NodeKindObjectTypeDefinition:
		return e.document.ObjectTypeDefinitions[node.Ref].ImplementsInterfaces.Refs
	case ast.NodeKindObjectTypeExtension:
		return e.document.ObjectTypeExtensions[node.Ref].ImplementsInterfaces.Refs
	default:
		return nil
	}
}

// unionMemberRefs returns the union members of the given node (this is only
// applicable to union kinds).
func (e *LocalTypeFieldExtractor) unionMemberRefs(node ast.Node) []int {
	switch node.Kind {
	case ast.NodeKindUnionTypeDefinition:
		return e.document.UnionTypeDefinitions[node.Ref].UnionMemberTypes.Refs
	case ast.NodeKindUnionTypeExtension:
		return e.document.UnionTypeExtensions[node.Ref].UnionMemberTypes.Refs
	default:
		return nil
	}
}

// NodeHasKeyDirective returns whether the given node has a @key directive.
func (e *LocalTypeFieldExtractor) NodeHasKeyDirective(node ast.Node) bool {
	for _, directiveRef := range e.document.NodeDirectives(node) {
		if e.document.DirectiveNameString(directiveRef) == federationKeyDirectiveName {
			return true
		}
	}
	return false
}
