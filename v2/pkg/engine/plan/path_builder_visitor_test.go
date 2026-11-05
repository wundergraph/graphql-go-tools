package plan

import (
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestPathBuilderVisitor_EnterField_PlansTypenameOnExistingPlannerFallback(t *testing.T) {
	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(`
		type Query {
			a: A
		}

		type A {
			id: ID!
		}
	`)
	operation := unsafeparser.ParseGraphqlDocumentString(`
		query TestQuery {
			a {
				__typename
			}
		}
	`)

	rootSelectionSet := operation.OperationDefinitions[0].SelectionSet
	rootFieldRef := operation.Selections[operation.SelectionSets[rootSelectionSet].SelectionRefs[0]].Ref
	typenameSelectionSet := operation.Fields[rootFieldRef].SelectionSet
	typenameFieldRef := operation.Selections[operation.SelectionSets[typenameSelectionSet].SelectionRefs[0]].Ref

	planner := testPlannerConfiguration(
		t,
		"existing-planner",
		DataSourcePlanningBehavior{AllowPlanningTypeName: true},
		[]TypeField{{TypeName: "A", FieldNames: []string{"id"}}},
		"query.a",
		"query.a",
	)

	walker := astvisitor.NewWalker(8)
	report := &operationreport.Report{}

	visitor := newTestPathBuilderVisitor(planner)
	visitor.operation = &operation
	visitor.definition = &definition
	visitor.walker = &walker
	visitor.nodeSuggestions = NewNodeSuggestions()
	visitor.fieldsPlannedOn = make(map[int][]int)

	walker.RegisterEnterFieldVisitor(visitor)
	walker.RegisterLeaveFieldVisitor(visitor)
	walker.Walk(&operation, &definition, report)
	require.False(t, report.HasErrors(), report.Error())

	assert.True(t, planner.HasPath("query.a.__typename"))
	assert.Equal(t, []int{0}, visitor.fieldsPlannedOn[typenameFieldRef])
	assert.Equal(t, []resolve.GraphCoordinate{{
		TypeName:  "A",
		FieldName: "__typename",
	}}, planner.ObjectFetchConfiguration().rootFields)
}

func TestPathBuilderVisitor_PlanTypenameOnExistingPlanner_SkipsIneligiblePlanners(t *testing.T) {
	const (
		precedingParentPath = "query.c.a"
		parentPath          = "query.c.a.A1"
		currentPath         = "query.c.a.A1.__typename"
		typeName            = "A1"
		fieldRef            = 11
	)

	broaderOwningPlanner := testPlannerConfiguration(
		t,
		"broader-owning-planner",
		DataSourcePlanningBehavior{AllowPlanningTypeName: true},
		[]TypeField{{TypeName: typeName, FieldNames: []string{"id"}}},
		"query",
		"query.c",
		precedingParentPath,
		parentPath,
	)
	plannerWithoutParentPath := testPlannerConfiguration(
		t,
		"missing-parent-path",
		DataSourcePlanningBehavior{AllowPlanningTypeName: true},
		[]TypeField{{TypeName: typeName, FieldNames: []string{"id"}}},
		parentPath,
		precedingParentPath,
	)
	plannerWithoutTypenameSupport := testPlannerConfiguration(
		t,
		"missing-typename-support",
		DataSourcePlanningBehavior{AllowPlanningTypeName: true},
		[]TypeField{{TypeName: "Other", FieldNames: []string{"id"}}},
		parentPath,
		parentPath,
	)
	validPlanner := testPlannerConfiguration(
		t,
		"valid",
		DataSourcePlanningBehavior{AllowPlanningTypeName: true},
		[]TypeField{{TypeName: typeName, FieldNames: []string{"id"}}},
		parentPath,
		parentPath,
	)

	visitor := newTestPathBuilderVisitor(broaderOwningPlanner, plannerWithoutParentPath, plannerWithoutTypenameSupport, validPlanner)

	plannerIdx, planned := visitor.planTypenameOnExistingPlanner(fieldRef, typeName, typeNameField, currentPath, parentPath, precedingParentPath)

	require.True(t, planned)
	assert.Equal(t, 3, plannerIdx)
	assert.False(t, broaderOwningPlanner.HasPath(currentPath))
	assert.False(t, plannerWithoutParentPath.HasPath(currentPath))
	assert.False(t, plannerWithoutTypenameSupport.HasPath(currentPath))
	assert.True(t, validPlanner.HasPath(currentPath))
	assert.Len(t, visitor.addedPathTracker, 1)
	assert.Equal(t, currentPath, visitor.addedPathTracker[0].path)
}

func TestPathBuilderVisitor_PlanTypenameOnExistingPlanner_PrefersAnchoredPlanner(t *testing.T) {
	const (
		precedingParentPath = "query.c.a"
		parentPath          = "query.c.a.A1"
		currentPath         = "query.c.a.A1.__typename"
		typeName            = "A1"
		fieldRef            = 12
	)

	fragmentPlanner := testPlannerConfiguration(
		t,
		"fragment-planner",
		DataSourcePlanningBehavior{AllowPlanningTypeName: true},
		[]TypeField{{TypeName: typeName, FieldNames: []string{"id"}}},
		parentPath,
		parentPath,
	)
	anchoredPlanner := testPlannerConfiguration(
		t,
		"anchored-planner",
		DataSourcePlanningBehavior{AllowPlanningTypeName: true},
		[]TypeField{{TypeName: typeName, FieldNames: []string{"id"}}},
		precedingParentPath,
		precedingParentPath,
	)

	visitor := newTestPathBuilderVisitor(fragmentPlanner, anchoredPlanner)

	plannerIdx, planned := visitor.planTypenameOnExistingPlanner(fieldRef, typeName, typeNameField, currentPath, parentPath, precedingParentPath)

	require.True(t, planned)
	assert.Equal(t, 1, plannerIdx)
	assert.False(t, fragmentPlanner.HasPath(currentPath))
	assert.True(t, anchoredPlanner.HasPath(currentPath))
}

func TestPathBuilderVisitor_PlanTypenameOnExistingPlanner_ReturnsFalseWhenTypenamePathCannotBeAdded(t *testing.T) {
	const (
		precedingParentPath = "query.c"
		parentPath          = "query.c.a"
		currentPath         = "query.c.a.__typename"
		typeName            = "A"
		fieldRef            = 13
	)

	planner := testPlannerConfiguration(
		t,
		"disabled-typename-planning",
		DataSourcePlanningBehavior{AllowPlanningTypeName: false},
		[]TypeField{{TypeName: typeName, FieldNames: []string{"id"}}},
		precedingParentPath,
		precedingParentPath,
		parentPath,
	)

	visitor := newTestPathBuilderVisitor(planner)

	plannerIdx, planned := visitor.planTypenameOnExistingPlanner(fieldRef, typeName, typeNameField, currentPath, parentPath, precedingParentPath)

	assert.False(t, planned)
	assert.Equal(t, -1, plannerIdx)
	assert.False(t, planner.HasPath(currentPath))
	assert.Empty(t, visitor.addedPathTracker)
}

func TestPathBuilderVisitor_PlanTypenameOnExistingPlanner_ReturnsFalseWhenMultipleCandidatesMatch(t *testing.T) {
	const (
		precedingParentPath = "query.c"
		parentPath          = "query.c.a"
		currentPath         = "query.c.a.__typename"
		typeName            = "A"
		fieldRef            = 14
	)

	firstPlanner := testPlannerConfiguration(
		t,
		"first-candidate",
		DataSourcePlanningBehavior{AllowPlanningTypeName: true},
		[]TypeField{{TypeName: typeName, FieldNames: []string{"id"}}},
		precedingParentPath,
		precedingParentPath,
		parentPath,
	)
	secondPlanner := testPlannerConfiguration(
		t,
		"second-candidate",
		DataSourcePlanningBehavior{AllowPlanningTypeName: true},
		[]TypeField{{TypeName: typeName, FieldNames: []string{"id"}}},
		precedingParentPath,
		precedingParentPath,
		parentPath,
	)

	visitor := newTestPathBuilderVisitor(firstPlanner, secondPlanner)

	plannerIdx, planned := visitor.planTypenameOnExistingPlanner(fieldRef, typeName, typeNameField, currentPath, parentPath, precedingParentPath)

	assert.False(t, planned)
	assert.Equal(t, -1, plannerIdx)
	assert.False(t, firstPlanner.HasPath(currentPath))
	assert.False(t, secondPlanner.HasPath(currentPath))
	assert.Empty(t, visitor.addedPathTracker)
}

func newTestPathBuilderVisitor(planners ...PlannerConfiguration) *pathBuilderVisitor {
	return &pathBuilderVisitor{
		plannerConfiguration:  &Configuration{},
		planners:              planners,
		addedPathTrackerIndex: make(map[string][]int),
		missingPathTracker:    make(map[string]struct{}),
	}
}

func testPlannerConfiguration(
	t *testing.T,
	id string,
	behavior DataSourcePlanningBehavior,
	rootNodes []TypeField,
	parentPath string,
	paths ...string,
) PlannerConfiguration {
	t.Helper()

	ds, err := NewDataSourceConfiguration(
		id,
		&FakeFactory[struct{}]{
			behavior: &behavior,
		},
		&DataSourceMetadata{
			RootNodes: rootNodes,
		},
		struct{}{},
	)
	require.NoError(t, err)

	pathConfigs := make([]pathConfiguration, 0, len(paths))
	for _, path := range paths {
		pathConfigs = append(pathConfigs, pathConfiguration{
			parentPath:       parentPath,
			path:             path,
			shouldWalkFields: true,
			typeName:         "A",
			fieldRef:         1,
			dsHash:           ds.Hash(),
			fragmentRef:      -1,
			pathType:         PathTypeField,
		})
	}

	return ds.CreatePlannerConfiguration(
		abstractlogger.NoopLogger,
		&objectFetchConfiguration{},
		newPlannerPathsConfiguration(parentPath, PlannerPathObject, pathConfigs),
		&Configuration{},
	)
}
