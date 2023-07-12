package plan

import (
	"context"
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/resolve"
)

type configurationVisitor struct {
	operationName         string
	operation, definition *ast.Document
	walker                *astvisitor.Walker
	config                Configuration
	planners              []plannerConfiguration
	fetches               []objectFetchConfiguration
	currentBufferId       int
	fieldBuffers          map[int]int

	parentTypeNodes []ast.Node

	ctx context.Context
}

type objectFetchConfiguration struct {
	object             *resolve.Object
	trigger            *resolve.GraphQLSubscriptionTrigger
	planner            DataSourcePlanner
	bufferID           int
	isSubscription     bool
	fieldRef           int
	fieldDefinitionRef int
}

func (c *configurationVisitor) addPath(i int, configuration pathConfiguration) {
	if pp, ok := c.planners[i].planner.(GqlPlanner); ok {
		pp.DebugPrint("[configurationVisitor.addPath] parentPath:", "path:", configuration.String())
	}

	configuration.depth = c.walker.Depth

	c.planners[i].addPath(configuration)
}

func (c *configurationVisitor) debug(args ...any) {
	printArgs := []any{"[configurationVisitor]: "}
	printArgs = append(printArgs, args...)
	fmt.Println(printArgs...)
}

func (c *configurationVisitor) EnterDocument(operation, definition *ast.Document) {
	c.operation, c.definition = operation, definition
	c.currentBufferId = -1
	c.parentTypeNodes = c.parentTypeNodes[:0]
	if c.planners == nil {
		c.planners = make([]plannerConfiguration, 0, 8)
	} else {
		c.planners = c.planners[:0]
	}
	if c.fetches == nil {
		c.fetches = []objectFetchConfiguration{}
	} else {
		c.fetches = c.fetches[:0]
	}
	if c.fieldBuffers == nil {
		c.fieldBuffers = map[int]int{}
	} else {
		for i := range c.fieldBuffers {
			delete(c.fieldBuffers, i)
		}
	}
}

func (c *configurationVisitor) EnterOperationDefinition(ref int) {
	operationName := c.operation.OperationDefinitionNameString(ref)
	if c.operationName != operationName {
		c.walker.SkipNode()
		return
	}
}

func (c *configurationVisitor) EnterSelectionSet(ref int) {
	c.debug("EnterSelectionSet ref:", ref)
	c.parentTypeNodes = append(c.parentTypeNodes, c.walker.EnclosingTypeDefinition)

	// When selection is the inline fragment
	// We have to add a fragment path to the planner paths
	ancestor := c.walker.Ancestor()
	if ancestor.Kind == ast.NodeKindInlineFragment {
		parentPath := c.walker.Path[:len(c.walker.Path)-1].DotDelimitedString()
		currentPath := c.walker.Path.DotDelimitedString()
		typeName := c.operation.InlineFragmentTypeConditionNameString(ancestor.Ref)

		for i, planner := range c.planners {
			if !planner.hasPath(parentPath) {
				continue
			}

			hasRootNode := planner.dataSourceConfiguration.HasRootNodeWithTypename(typeName)
			hasChildNode := planner.dataSourceConfiguration.HasChildNodeWithTypename(typeName)
			if !(hasRootNode || hasChildNode) {
				continue
			}

			path := pathConfiguration{
				path:             currentPath,
				shouldWalkFields: true,
			}

			c.addPath(i, path)
		}
	}
}

func (c *configurationVisitor) LeaveSelectionSet(ref int) {
	c.debug("LeaveSelectionSet ref:", ref)
	c.parentTypeNodes = c.parentTypeNodes[:len(c.parentTypeNodes)-1]
}

func (c *configurationVisitor) EnterField(ref int) {
	fieldName := c.operation.FieldNameUnsafeString(ref)
	fieldAliasOrName := c.operation.FieldAliasOrNameString(ref)
	typeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)

	c.debug("EnterField ref:", ref, "fieldName:", fieldName, "typeName:", typeName)

	parentPath := c.walker.Path.DotDelimitedString()
	// we need to also check preceding path for inline fragments
	// as for the field within inline fragment the parent path will include type condition in a path
	// but planner path still will not include it
	// this required to not produce multiple planners for the inline fragments
	precedingParentPath := parentPath
	if c.walker.Path[len(c.walker.Path)-1].Kind == ast.InlineFragmentName {
		precedingParentPath = c.walker.Path[:len(c.walker.Path)-1].DotDelimitedString()
	}

	currentPath := parentPath + "." + fieldAliasOrName
	root := c.walker.Ancestors[0]
	if root.Kind != ast.NodeKindOperationDefinition {
		return
	}
	isSubscription := c.isSubscription(root.Ref, currentPath)
	for i, plannerConfig := range c.planners {
		planningBehaviour := plannerConfig.planner.DataSourcePlanningBehavior()
		if (plannerConfig.hasParent(parentPath) || plannerConfig.hasParent(precedingParentPath)) &&
			plannerConfig.dataSourceConfiguration.HasRootNode(typeName, fieldName) &&
			planningBehaviour.MergeAliasedRootNodes {
			// same parent + root node = root sibling

			c.addPath(i, pathConfiguration{
				path:             currentPath,
				shouldWalkFields: true,
				typeName:         typeName,
				fieldRef:         ref,
				enclosingNode:    c.walker.EnclosingTypeDefinition,
			})
			c.fieldBuffers[ref] = plannerConfig.bufferID

			return
		}
		if plannerConfig.hasPath(parentPath) || plannerConfig.hasParent(precedingParentPath) {
			if plannerConfig.dataSourceConfiguration.HasChildNode(typeName, fieldName) {

				// has parent path + has child node = child
				c.addPath(i, pathConfiguration{
					path:             currentPath,
					shouldWalkFields: true,
					typeName:         typeName,
					fieldRef:         ref,
					enclosingNode:    c.walker.EnclosingTypeDefinition,
				})

				return
			}

			if pathAdded := c.addPlannerPathForUnionChildOfObjectParent(i, currentPath, ref, typeName); pathAdded {
				return
			}

			if pathAdded := c.addPlannerPathForChildOfAbstractParent(i, currentPath, ref, typeName, fieldName); pathAdded {
				return
			}
		}
	}
	for i, config := range c.config.DataSources {
		if !config.HasRootNode(typeName, fieldName) {
			continue
		}
		var (
			bufferID int
		)
		if !isSubscription {
			bufferID = c.nextBufferID()
			c.fieldBuffers[ref] = bufferID
		}
		planner := c.config.DataSources[i].Factory.Planner(c.ctx)
		isParentAbstract := c.isParentTypeNodeAbstractType()
		paths := []pathConfiguration{
			{
				path:             currentPath,
				shouldWalkFields: true,
				typeName:         typeName,
				fieldRef:         ref,
				enclosingNode:    c.walker.EnclosingTypeDefinition,
			},
		}

		if isParentAbstract {
			// if the parent is abstract, we add the parent path as well
			// this will ensure that we're walking into and out of the root inline fragments
			// otherwise, we'd only walk into the fields inside the inline fragments in the root,
			// so we'd miss the selection sets and inline fragments in the root
			paths = append([]pathConfiguration{
				{
					path:             parentPath,
					shouldWalkFields: false,
				},
			}, paths...)
		} else {
			// add potentially missing parent path
			// this could happen when the parent is a fragment and we walking nested selection sets
			paths = append([]pathConfiguration{
				{
					path:             parentPath,
					shouldWalkFields: true,
				},
			}, paths...)
		}

		plannerPath := parentPath

		isParentFragment := c.walker.Path[len(c.walker.Path)-1].Kind == ast.InlineFragmentName
		if isParentFragment {
			precedingFragmentPath := c.walker.Path[:len(c.walker.Path)-1].DotDelimitedString()
			// if the parent is a fragment, we add the preceding parent path as well
			// to be able to walk selection sets in the fragment
			paths = append([]pathConfiguration{
				{
					path:             precedingFragmentPath,
					shouldWalkFields: false,
				},
			}, paths...)

			// if the parent is a fragment, we use the preceding parent path as the planner path
			// to avoid creating multiple planners for the same upstream
			plannerPath = precedingFragmentPath
		}

		c.planners = append(c.planners, plannerConfiguration{
			bufferID:                bufferID,
			parentPath:              plannerPath,
			planner:                 planner,
			paths:                   paths,
			dataSourceConfiguration: config,
		})
		fieldDefinition, ok := c.walker.FieldDefinition(ref)
		if !ok {
			continue
		}
		c.fetches = append(c.fetches, objectFetchConfiguration{
			bufferID:           bufferID,
			planner:            planner,
			isSubscription:     isSubscription,
			fieldRef:           ref,
			fieldDefinitionRef: fieldDefinition,
		})
		return
	}
}

func (c *configurationVisitor) LeaveField(ref int) {
	fieldName := c.operation.FieldNameUnsafeString(ref)
	fieldAliasOrName := c.operation.FieldAliasOrNameString(ref)
	typeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)
	c.debug("LeaveField ref:", ref, "fieldName:", fieldName, "typeName:", typeName)

	// fieldAliasOrName := c.operation.FieldAliasOrNameString(ref)
	parent := c.walker.Path.DotDelimitedString()
	current := parent + "." + fieldAliasOrName
	for i, planner := range c.planners {
		if planner.hasPath(current) && !planner.hasPathPrefix(current) {
			c.planners[i].setPathExit(current)
			return
		}
	}
}

func (c *configurationVisitor) addPlannerPathForUnionChildOfObjectParent(
	plannerIndex int, currentPath string, fieldRef int, typeName string,
) (pathAdded bool) {

	if c.walker.EnclosingTypeDefinition.Kind != ast.NodeKindObjectTypeDefinition {
		return false
	}
	fieldDefRef, exists := c.definition.NodeFieldDefinitionByName(c.walker.EnclosingTypeDefinition, c.operation.FieldNameBytes(fieldRef))
	if !exists {
		return false
	}

	fieldDefTypeRef := c.definition.FieldDefinitionType(fieldDefRef)
	fieldDefTypeName := c.definition.TypeNameBytes(fieldDefTypeRef)
	node, ok := c.definition.NodeByName(fieldDefTypeName)
	if !ok {
		return false
	}

	if node.Kind == ast.NodeKindUnionTypeDefinition {
		c.planners[plannerIndex].addPath(pathConfiguration{
			path:             currentPath,
			shouldWalkFields: true,
			typeName:         typeName,
			fieldRef:         fieldRef,
			enclosingNode:    c.walker.EnclosingTypeDefinition,
		})
		return true
	}
	return false
}

func (c *configurationVisitor) addPlannerPathForChildOfAbstractParent(
	plannerIndex int, currentPath string, fieldRef int, typeName, fieldName string,
) (pathAdded bool) {

	if !c.isParentTypeNodeAbstractType() {
		return false
	}
	// If the field is a root node in any of the data sources, the path shouldn't be handled here
	for _, d := range c.config.DataSources {
		if d.HasRootNode(typeName, fieldName) {
			return false
		}
	}
	// The path for this field should only be added if the parent path also exists on this planner

	c.planners[plannerIndex].addPath(pathConfiguration{
		path:             currentPath,
		shouldWalkFields: true,
		typeName:         typeName,
		fieldRef:         fieldRef,
		enclosingNode:    c.walker.EnclosingTypeDefinition,
	})
	return true
}

func (c *configurationVisitor) isParentTypeNodeAbstractType() bool {
	if len(c.parentTypeNodes) < 2 {
		return false
	}
	parentTypeNode := c.parentTypeNodes[len(c.parentTypeNodes)-2]
	return parentTypeNode.Kind.IsAbstractType()
}

func (c *configurationVisitor) isSubscription(root int, path string) bool {
	rootOperationType := c.operation.OperationDefinitions[root].OperationType
	if rootOperationType != ast.OperationTypeSubscription {
		return false
	}
	return strings.Count(path, ".") == 1
}

func (c *configurationVisitor) nextBufferID() int {
	c.currentBufferId++
	return c.currentBufferId
}
