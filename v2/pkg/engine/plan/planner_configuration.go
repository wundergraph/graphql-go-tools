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

func (p *plannerConfiguration) fragmentPaths() (out []string) {
	for i := range p.paths {
		if p.paths[i].pathType == PathTypeFragment {
			out = append(out, p.paths[i].path)
		}
	}
	return
}

func (p *plannerConfiguration) removePath(path string) {
	for i := range p.paths {
		if p.paths[i].path == path {
			p.paths = append(p.paths[:i], p.paths[i+1:]...)
			return
		}
	}
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
	pathType   PathType
}

type PathType int

const (
	PathTypeField PathType = iota
	PathTypeFragment
	PathTypeParent
)

func (p *pathConfiguration) String() string {
	pathType := "field"
	if p.pathType == PathTypeField {
		return fmt.Sprintf(`{"ds":%d,"path":"%s","fieldRef":%3d,"typeName":"%s","shouldWalkFields":%t,"exitPlannerOnNode":%t,"isRootNode":%t,"pathType":"%s"}`, p.dsHash, p.path, p.fieldRef, p.typeName, p.shouldWalkFields, p.exitPlannerOnNode, p.isRootNode, pathType)
	}
	switch p.pathType {
	case PathTypeFragment:
		pathType = "fragment"
	case PathTypeParent:
		pathType = "parent"
	}

	return fmt.Sprintf(`{"ds":%d,"path":"%s","shouldWalkFields":%t,"exitPlannerOnNode":%t,"pathType":"%s"}`, p.dsHash, p.path, p.shouldWalkFields, p.exitPlannerOnNode, pathType)
}
