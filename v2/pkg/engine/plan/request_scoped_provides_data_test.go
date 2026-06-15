package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestPopulateRequestScopedFieldsProvidesDataNilPlannerObjectLeavesFieldsUnchanged(t *testing.T) {
	state := newCachingPlannerState(nil, nil, &Configuration{})
	fields := []resolve.RequestScopedField{
		{
			FieldName: "currentViewer",
			FieldPath: []string{
				"currentViewer",
			},
			L1Key: "viewer.currentViewer",
		},
	}

	out := state.populateRequestScopedFieldsProvidesData(fields, nil)

	assert.Equal(t, fields, out)
}

func TestPopulateRequestScopedFieldsProvidesDataNoMatchDropsHint(t *testing.T) {
	state := newCachingPlannerState(nil, nil, &Configuration{})
	plannerObj := &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name:  []byte("id"),
				Value: &resolve.Scalar{},
			},
		},
	}
	fields := []resolve.RequestScopedField{
		{
			FieldName: "currentViewer",
			FieldPath: []string{
				"currentViewer",
			},
			L1Key: "viewer.currentViewer",
		},
	}

	out := state.populateRequestScopedFieldsProvidesData(fields, plannerObj)

	assert.Equal(t, []resolve.RequestScopedField{}, out)
}

func TestPopulateRequestScopedFieldsProvidesDataScalarDropsHint(t *testing.T) {
	state := newCachingPlannerState(nil, nil, &Configuration{})
	plannerObj := &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name:  []byte("locale"),
				Value: &resolve.String{},
			},
		},
	}
	fields := []resolve.RequestScopedField{
		{
			FieldName: "locale",
			FieldPath: []string{
				"locale",
			},
			L1Key: "viewer.locale",
		},
	}

	out := state.populateRequestScopedFieldsProvidesData(fields, plannerObj)

	assert.Equal(t, []resolve.RequestScopedField{}, out)
}

func TestPopulateRequestScopedFieldsProvidesDataAliasMatchesByResponseKey(t *testing.T) {
	state := newCachingPlannerState(nil, nil, &Configuration{})
	viewerObj := &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name:  []byte("id"),
				Value: &resolve.Scalar{},
			},
			{
				Name:  []byte("name"),
				Value: &resolve.String{},
			},
		},
	}
	plannerObj := &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name:         []byte("viewer"),
				OriginalName: []byte("currentViewer"),
				Value:        viewerObj,
			},
		},
	}
	fields := []resolve.RequestScopedField{
		{
			FieldName: "viewer",
			FieldPath: []string{
				"viewer",
			},
			L1Key: "viewer.currentViewer",
		},
	}

	out := state.populateRequestScopedFieldsProvidesData(fields, plannerObj)

	assert.Equal(t, []resolve.RequestScopedField{
		{
			FieldName: "viewer",
			FieldPath: []string{
				"viewer",
			},
			L1Key:        "viewer.currentViewer",
			ProvidesData: viewerObj,
		},
	}, out)
	assert.Same(t, viewerObj, out[0].ProvidesData)
}

func TestPopulateRequestScopedFieldsProvidesDataEachFieldUsesMatchingSubObject(t *testing.T) {
	state := newCachingPlannerState(nil, nil, &Configuration{})
	viewerObj := &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name:  []byte("id"),
				Value: &resolve.Scalar{},
			},
		},
	}
	sessionObj := &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name:  []byte("token"),
				Value: &resolve.String{},
			},
		},
	}
	plannerObj := &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name:  []byte("currentViewer"),
				Value: viewerObj,
			},
			{
				Name:  []byte("viewerSession"),
				Value: sessionObj,
			},
		},
	}
	fields := []resolve.RequestScopedField{
		{
			FieldName: "currentViewer",
			FieldPath: []string{
				"currentViewer",
			},
			L1Key: "viewer.currentViewer",
		},
		{
			FieldName: "viewerSession",
			FieldPath: []string{
				"viewerSession",
			},
			L1Key: "viewer.session",
		},
	}

	out := state.populateRequestScopedFieldsProvidesData(fields, plannerObj)

	assert.Equal(t, []resolve.RequestScopedField{
		{
			FieldName: "currentViewer",
			FieldPath: []string{
				"currentViewer",
			},
			L1Key:        "viewer.currentViewer",
			ProvidesData: viewerObj,
		},
		{
			FieldName: "viewerSession",
			FieldPath: []string{
				"viewerSession",
			},
			L1Key:        "viewer.session",
			ProvidesData: sessionObj,
		},
	}, out)
	assert.Same(t, viewerObj, out[0].ProvidesData)
	assert.Same(t, sessionObj, out[1].ProvidesData)
}
