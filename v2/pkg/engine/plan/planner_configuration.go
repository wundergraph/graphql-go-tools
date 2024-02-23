package plan

import (
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type plannerConfiguration[T any] struct {
	*plannerPathsConfiguration

	planner                  DataSourcePlanner[T]
	dataSourceConfiguration  *dataSourceConfiguration[T]
	objectFetchConfiguration *objectFetchConfiguration

	requiredFields FederationFieldConfigurations
	providedFields *NodeSuggestions
}

type PlannerConfiguration interface {
	DataSourceBehavior
	PlannerPathConfiguration

	ObjectFetchConfiguration() *objectFetchConfiguration
	DataSourceConfiguration() DataSource
	modifyObjectFetchConfiguration(func(o *objectFetchConfiguration))

	RequiredFields() *FederationFieldConfigurations
	ProvidedFields() *NodeSuggestions

	Debugger() (d DataSourceDebugger, ok bool)
	Planner() any
	Register(visitor *Visitor) error
}

func (p *plannerConfiguration[T]) Register(visitor *Visitor) error {
	dataSourcePlannerConfig := DataSourcePlannerConfiguration{
		RequiredFields: p.requiredFields,
		ProvidedFields: p.providedFields,
		ParentPath:     p.parentPath,
		PathType:       p.parentPathType,
		IsNested:       p.IsNestedPlanner(),
	}

	return p.planner.Register(visitor, p.dataSourceConfiguration, dataSourcePlannerConfig)
}

func (p *plannerConfiguration[T]) Planner() any {
	return p.planner
}

func (p *plannerConfiguration[T]) Debugger() (d DataSourceDebugger, ok bool) {
	d, ok = p.planner.(DataSourceDebugger)
	return
}

func (p *plannerConfiguration[T]) ObjectFetchConfiguration() *objectFetchConfiguration {
	return p.objectFetchConfiguration
}

func (p *plannerConfiguration[T]) DataSourcePlanningBehavior() DataSourcePlanningBehavior {
	return p.planner.DataSourcePlanningBehavior()
}

func (p *plannerConfiguration[T]) DownstreamResponseFieldAlias(downstreamFieldRef int) (alias string, exists bool) {
	return p.planner.DownstreamResponseFieldAlias(downstreamFieldRef)
}

func (p *plannerConfiguration[T]) ProvidedFields() *NodeSuggestions {
	return p.providedFields
}

func (p *plannerConfiguration[T]) RequiredFields() *FederationFieldConfigurations {
	return &p.requiredFields
}

func (p *plannerConfiguration[T]) modifyObjectFetchConfiguration(f func(o *objectFetchConfiguration)) {
	f(p.objectFetchConfiguration)
}

func (p *plannerConfiguration[T]) DataSourceConfiguration() DataSource {
	return p.dataSourceConfiguration
}

type PlannerPathConfiguration interface {
	Paths() []pathConfiguration
	ParentPath() string
	AddPath(configuration pathConfiguration)
	IsNestedPlanner() bool
	HasPath(path string) bool
	IsExitPath(path string) bool
	ShouldWalkFieldsOnPath(path string, typeName string) bool
	SetPathExit(path string)
	HasPathPrefix(prefix string) bool
	FragmentPaths() (out []string)
	RemovePath(path string)
	HasParent(parent string) bool
}

type plannerPathsConfiguration struct {
	parentPath     string
	parentPathType PlannerPathType
	paths          []pathConfiguration
}

func (p *plannerPathsConfiguration) Paths() []pathConfiguration {
	return p.paths
}

func (p *plannerPathsConfiguration) ParentPath() string {
	return p.parentPath
}

func (p *plannerPathsConfiguration) AddPath(configuration pathConfiguration) {
	// fmt.Println("[plannerConfiguration.AddPath] parentPath:", p.parentPath, "path:", configuration.String())
	p.paths = append(p.paths, configuration)
}

// IsNestedPlanner returns true in case the planner is not directly attached to the Operation root
// a nested planner should always build a Query
func (p *plannerPathsConfiguration) IsNestedPlanner() bool {
	return strings.Contains(p.parentPath, ".")
}

func (p *plannerPathsConfiguration) HasPath(path string) bool {
	for i := range p.paths {
		if p.paths[i].path == path {
			return true
		}
	}
	return false
}

func (p *plannerPathsConfiguration) IsExitPath(path string) bool {
	for i := range p.paths {
		if p.paths[i].path == path {
			return p.paths[i].exitPlannerOnNode
		}
	}
	return false
}

func (p *plannerPathsConfiguration) ShouldWalkFieldsOnPath(path string, typeName string) bool {
	for i := range p.paths {
		if p.paths[i].path == path && p.paths[i].typeName == typeName {
			return p.paths[i].shouldWalkFields
		}
	}
	return false
}

func (p *plannerPathsConfiguration) SetPathExit(path string) {
	for i := range p.paths {
		if p.paths[i].path == path {
			p.paths[i].exitPlannerOnNode = true
			return
		}
	}
}

func (p *plannerPathsConfiguration) HasPathPrefix(prefix string) bool {
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

func (p *plannerPathsConfiguration) FragmentPaths() (out []string) {
	for i := range p.paths {
		if p.paths[i].pathType == PathTypeFragment {
			out = append(out, p.paths[i].path)
		}
	}
	return
}

func (p *plannerPathsConfiguration) RemovePath(path string) {
	for i := range p.paths {
		if p.paths[i].path == path {
			p.paths = append(p.paths[:i], p.paths[i+1:]...)
			return
		}
	}
}

func (p *plannerPathsConfiguration) HasParent(parent string) bool {
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
