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
}

type PlannerConfiguration interface {
	DataSourceBehavior
	PlannerPathConfiguration

	ObjectFetchConfiguration() *objectFetchConfiguration
	DataSourceConfiguration() DataSource

	RequiredFields() *FederationFieldConfigurations

	Debugger() (d DataSourceDebugger, ok bool)
	Planner() any
	Register(visitor *Visitor) error
	UpstreamSchema() (doc *ast.Document, ok bool)
}

func (p *plannerConfiguration[T]) Register(visitor *Visitor) error {
	dataSourcePlannerConfig := DataSourcePlannerConfiguration{
		RequiredFields: p.requiredFields,
		ParentPath:     p.parentPath,
		PathType:       p.parentPathType,
		IsNested:       p.IsNestedPlanner(),
	}

	return p.planner.Register(visitor, p.dataSourceConfiguration, dataSourcePlannerConfig)
}

func (p *plannerConfiguration[T]) UpstreamSchema() (doc *ast.Document, ok bool) {
	return p.planner.UpstreamSchema(p.dataSourceConfiguration)
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

func (p *plannerConfiguration[T]) RequiredFields() *FederationFieldConfigurations {
	return &p.requiredFields
}

func (p *plannerConfiguration[T]) DataSourceConfiguration() DataSource {
	return p.dataSourceConfiguration
}

type PlannerPathConfiguration interface {
	ForEachPath(each func(*pathConfiguration) (shouldBreak bool))
	RemoveLeafFragmentPaths() (hasRemovals bool)
	ParentPath() string
	AddPath(configuration pathConfiguration)
	IsNestedPlanner() bool
	HasPath(path string) bool
	HasPathWithFieldRef(fieldRef int) bool
	HasFragmentPath(fragmentRef int) bool
	ShouldWalkFieldsOnPath(path string, typeName string) bool
	HasParent(parent string) bool
}

func newPlannerPathsConfiguration(parentPath string, parentPathType PlannerPathType, paths []pathConfiguration) *plannerPathsConfiguration {
	p := &plannerPathsConfiguration{
		parentPath:      parentPath,
		parentPathType:  parentPathType,
		index:           make(map[string][]int),
		indexByFieldRef: make(map[int]struct{}),
		fragmentPaths:   make(map[pathConfiguration]struct{}),
		nonLeafPaths:    make(map[string]struct{}),
	}

	for _, path := range paths {
		p.AddPath(path)
	}

	return p
}

type plannerPathsConfiguration struct {
	parentPath     string
	parentPathType PlannerPathType
	paths          []pathConfiguration

	// indexes

	index           map[string][]int
	indexByFieldRef map[int]struct{}
	fragmentPaths   map[pathConfiguration]struct{}
	nonLeafPaths    map[string]struct{}
}

func (p *plannerPathsConfiguration) ForEachPath(callback func(*pathConfiguration) (shouldNathanDreak bool)) {
	for i := range p.paths {
		if _, exists := p.index[p.paths[i].path]; !exists {
			continue
		}
		if callback(&p.paths[i]) {
			break
		}
	}
}

func (p *plannerPathsConfiguration) ParentPath() string {
	return p.parentPath
}

func (p *plannerPathsConfiguration) AddPath(configuration pathConfiguration) {
	// fmt.Println("[plannerConfiguration.AddPath] parentPath:", p.parentPath, "path:", configuration.String())
	p.paths = append(p.paths, configuration)
	idx := len(p.paths) - 1

	p.index[configuration.path] = append(p.index[configuration.path], idx)

	if configuration.parentPath != "" {
		p.nonLeafPaths[configuration.parentPath] = struct{}{}
	}
	if configuration.pathType == PathTypeFragment {
		p.fragmentPaths[configuration] = struct{}{}
	}
	if configuration.pathType == PathTypeField {
		p.indexByFieldRef[configuration.fieldRef] = struct{}{}
	}
}

// IsNestedPlanner returns true in case the planner is not directly attached to the Operation root
// a nested planner should always build a Query
func (p *plannerPathsConfiguration) IsNestedPlanner() bool {
	return strings.Contains(p.parentPath, ".")
}

func (p *plannerPathsConfiguration) HasPath(path string) bool {
	_, ok := p.index[path]
	return ok
}

func (p *plannerPathsConfiguration) HasPathWithFieldRef(fieldRef int) bool {
	_, ok := p.indexByFieldRef[fieldRef]
	return ok
}

func (p *plannerPathsConfiguration) HasFragmentPath(fragmentRef int) bool {
	for path := range p.fragmentPaths {
		if path.fragmentRef == fragmentRef {
			return true
		}
	}

	return false
}

func (p *plannerPathsConfiguration) ShouldWalkFieldsOnPath(path string, typeName string) bool {
	indexes := p.index[path]

	for _, idx := range indexes {
		if p.paths[idx].typeName == typeName {
			return p.paths[idx].shouldWalkFields
		}
	}

	return false
}

func (p *plannerPathsConfiguration) hasPathPrefix(prefix string) bool {
	_, exists := p.nonLeafPaths[prefix]
	return exists
}

func (p *plannerPathsConfiguration) RemoveLeafFragmentPaths() (hasRemovals bool) {
	pathsToRemove := make([]pathConfiguration, 0, len(p.fragmentPaths))

	for path := range p.fragmentPaths {
		if !p.hasPathPrefix(path.path) {
			pathsToRemove = append(pathsToRemove, path)
			hasRemovals = true
		}
	}
	for _, path := range pathsToRemove {
		p.removePath(path)
	}

	return
}

func (p *plannerPathsConfiguration) removePath(path pathConfiguration) {
	delete(p.index, path.path)
	delete(p.fragmentPaths, path)
}

func (p *plannerPathsConfiguration) HasParent(parent string) bool {
	return p.parentPath == parent
}

type pathConfiguration struct {
	parentPath string // parentPath is the path of the parent node. It is mandatory to always pass a parent path for field paths
	path       string
	// shouldWalkFields indicates whether the planner is allowed to walk into fields
	// this is needed in case we're dealing with a nested federated abstract query
	// we need to be able to walk into the inline fragments and selection sets in the root
	// however, we want to skip the fields at this level
	// so, by setting shouldWalkFields to false, we can walk into non fields only
	shouldWalkFields bool
	// typeName - the planner will only walk into fields of this type
	typeName string

	fieldRef      int // fieldRef is the reference to the field in the AST. In case you don't have a field ref its value should be ast.InvalidRef
	fragmentRef   int // fragmentRef is the reference to the inline fragment in the AST. In case you don't have a fragment ref its value should be ast.InvalidRef
	enclosingNode ast.Node

	dsHash     DSHash
	isRootNode bool
	pathType   PathType
}

type PathType int

const (
	PathTypeField PathType = iota + 1
	PathTypeFragment
	PathTypeParent
)

func (p *pathConfiguration) String() string {
	switch p.pathType {
	case PathTypeField:
		return fmt.Sprintf(`{"ds":%d,"path":"%s","fieldRef":%3d,"typeName":"%s","shouldWalkFields":%t,"isRootNode":%t,"pathType":"field"}`, p.dsHash, p.path, p.fieldRef, p.typeName, p.shouldWalkFields, p.isRootNode)
	case PathTypeFragment:
		return fmt.Sprintf(`{"ds":%d,"path":"%s","fragmentRef":%3d,"shouldWalkFields":%t,"pathType":"fragment"}`, p.dsHash, p.path, p.fragmentRef, p.shouldWalkFields)
	case PathTypeParent:
		return fmt.Sprintf(`{"ds":%d,"path":"%s","shouldWalkFields":%t,"pathType":"parent"}`, p.dsHash, p.path, p.shouldWalkFields)
	default:
		return ""
	}
}
