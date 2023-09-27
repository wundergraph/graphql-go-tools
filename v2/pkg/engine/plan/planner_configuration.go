package plan

import (
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type plannerConfiguration struct {
	parentPath     string
	parentPathType PlannerPathType

	planner                 DataSourcePlanner
	paths                   []pathConfiguration
	dataSourceConfiguration DataSourceConfiguration

	requiredFields FederationFieldConfigurations
	providedFields NodeSuggestions
}

func (p *plannerConfiguration) addPath(configuration pathConfiguration) {
	// fmt.Println("[plannerConfiguration.addPath] parentPath:", p.parentPath, "path:", configuration.String())
	p.paths = append(p.paths, configuration)
}

// isNestedPlanner returns true in case the planner is not directly attached to the Operation root
// a nested planner should always build a Query
func (p *plannerConfiguration) isNestedPlanner() bool {
	return strings.Contains(p.parentPath, ".")
}

func (p *plannerConfiguration) hasPath(path string) bool {
	for i := range p.paths {
		if p.paths[i].path == path {
			return true
		}
	}
	return false
}

func (p *plannerConfiguration) isExitPath(path string) bool {
	for i := range p.paths {
		if p.paths[i].path == path {
			return p.paths[i].exitPlannerOnNode
		}
	}
	return false
}

func (p *plannerConfiguration) shouldWalkFieldsOnPath(path string, typeName string) bool {
	for i := range p.paths {
		if p.paths[i].path == path && p.paths[i].typeName == typeName {
			return p.paths[i].shouldWalkFields
		}
	}
	return false
}

func (p *plannerConfiguration) setPathExit(path string) {
	for i := range p.paths {
		if p.paths[i].path == path {
			p.paths[i].exitPlannerOnNode = true
			return
		}
	}
}

func (p *plannerConfiguration) hasPathPrefix(prefix string) bool {
	for i := range p.paths {
		if p.paths[i].path == prefix {
			continue
		}
		if strings.HasPrefix(p.paths[i].path, prefix) {
			return true
		}
	}
	return false
}

func (p *plannerConfiguration) hasParent(parent string) bool {
	return p.parentPath == parent
}

type pathConfiguration struct {
	path              string
	exitPlannerOnNode bool
	// shouldWalkFields indicates whether the planner is allowed to walk into fields
	// this is needed in case we're dealing with a nested federated abstract query
	// we need to be able to walk into the inline fragments and selection sets in the root
	// however, we want to skip the fields at this level
	// so, by setting shouldWalkFields to false, we can walk into non fields only
	shouldWalkFields bool
	// typeName - the planner will only walk into fields of this type
	typeName string

	fieldRef      int
	enclosingNode ast.Node

	depth      int
	dsHash     DSHash
	isRootNode bool
}

func (p *pathConfiguration) String() string {
	return fmt.Sprintf(`{"ds":%d,"fieldRef":%3d,"path":"%s","typeName":"%s","shouldWalkFields":%t,"exitPlannerOnNode":%t,"isRootNode":%t}`, p.dsHash, p.fieldRef, p.path, p.typeName, p.shouldWalkFields, p.exitPlannerOnNode, p.isRootNode)
}
