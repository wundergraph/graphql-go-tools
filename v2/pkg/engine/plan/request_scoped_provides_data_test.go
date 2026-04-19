package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestPopulateRequestScopedFieldsProvidesData verifies that the function correctly
// locates requestScoped fields in the planner's response Object tree by their
// response key (alias or schema name) and populates ProvidesData.
func TestPopulateRequestScopedFieldsProvidesData(t *testing.T) {
	t.Parallel()

	t.Run("no plannerObj leaves fields unchanged", func(t *testing.T) {
		t.Parallel()
		fields := []resolve.RequestScopedField{
			{FieldName: "currentViewer", FieldPath: []string{"currentViewer"}, L1Key: "k"},
		}
		out := populateRequestScopedFieldsProvidesData(fields, nil)
		assert.Equal(t, fields, out)
	})

	t.Run("no matching field leaves ProvidesData nil", func(t *testing.T) {
		t.Parallel()
		plannerObj := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id"), Value: &resolve.Scalar{}},
			},
		}
		fields := []resolve.RequestScopedField{
			{FieldName: "currentViewer", FieldPath: []string{"currentViewer"}, L1Key: "k"},
		}
		out := populateRequestScopedFieldsProvidesData(fields, plannerObj)
		assert.Len(t, out, 1)
		assert.Equal(t, "currentViewer", out[0].FieldName)
		assert.Nil(t, out[0].ProvidesData)
	})

	t.Run("matching field by response key populates ProvidesData", func(t *testing.T) {
		t.Parallel()
		viewerObj := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id"), Value: &resolve.Scalar{}},
				{Name: []byte("name"), Value: &resolve.Scalar{}},
			},
		}
		plannerObj := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("currentViewer"), Value: viewerObj},
			},
		}
		fields := []resolve.RequestScopedField{
			{FieldName: "currentViewer", FieldPath: []string{"currentViewer"}, L1Key: "k"},
		}
		out := populateRequestScopedFieldsProvidesData(fields, plannerObj)
		assert.Len(t, out, 1)
		assert.Equal(t, "currentViewer", out[0].FieldName)
		assert.Equal(t, []string{"currentViewer"}, out[0].FieldPath)
		assert.Same(t, viewerObj, out[0].ProvidesData)
	})

	t.Run("aliased field matched by alias (response key)", func(t *testing.T) {
		t.Parallel()
		viewerObj := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("id"), Value: &resolve.Scalar{}},
				{Name: []byte("name"), Value: &resolve.Scalar{}},
			},
		}
		// Outer query: { articles { viewer: currentViewer { id name } } }
		// The datasource planner already resolved the alias, so FieldName="viewer".
		// plannerObj has the field under the alias "viewer".
		plannerObj := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:         []byte("viewer"),        // alias (= response key)
					OriginalName: []byte("currentViewer"), // schema name
					Value:        viewerObj,
				},
			},
		}
		fields := []resolve.RequestScopedField{
			{FieldName: "viewer", FieldPath: []string{"viewer"}, L1Key: "k"},
		}
		out := populateRequestScopedFieldsProvidesData(fields, plannerObj)
		assert.Len(t, out, 1)
		assert.Equal(t, "viewer", out[0].FieldName)
		assert.Equal(t, []string{"viewer"}, out[0].FieldPath)
		assert.Same(t, viewerObj, out[0].ProvidesData)
	})

	t.Run("multiple fields, mix of aliased and unaliased", func(t *testing.T) {
		t.Parallel()
		viewerObj := &resolve.Object{Fields: []*resolve.Field{{Name: []byte("id"), Value: &resolve.Scalar{}}}}
		tenantObj := &resolve.Object{Fields: []*resolve.Field{{Name: []byte("id"), Value: &resolve.Scalar{}}}}
		plannerObj := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("viewer"), OriginalName: []byte("currentViewer"), Value: viewerObj},
				{Name: []byte("tenantConfig"), Value: tenantObj},
			},
		}
		fields := []resolve.RequestScopedField{
			{FieldName: "viewer", FieldPath: []string{"viewer"}, L1Key: "k1"},
			{FieldName: "tenantConfig", FieldPath: []string{"tenantConfig"}, L1Key: "k2"},
		}
		out := populateRequestScopedFieldsProvidesData(fields, plannerObj)
		assert.Len(t, out, 2)
		assert.Same(t, viewerObj, out[0].ProvidesData)
		assert.Same(t, tenantObj, out[1].ProvidesData)
	})

	t.Run("scalar field value does not populate ProvidesData", func(t *testing.T) {
		t.Parallel()
		plannerObj := &resolve.Object{
			Fields: []*resolve.Field{
				{Name: []byte("locale"), Value: &resolve.Scalar{}},
			},
		}
		fields := []resolve.RequestScopedField{
			{FieldName: "locale", FieldPath: []string{"locale"}, L1Key: "k"},
		}
		out := populateRequestScopedFieldsProvidesData(fields, plannerObj)
		assert.Len(t, out, 1)
		assert.Nil(t, out[0].ProvidesData) // Scalar, not Object
	})
}

// TestFindObjectFieldByResponseKey verifies the response-key lookup helper.
func TestFindObjectFieldByResponseKey(t *testing.T) {
	t.Parallel()

	obj := &resolve.Object{
		Fields: []*resolve.Field{
			{Name: []byte("id"), Value: &resolve.Scalar{}},
			{Name: []byte("cv"), OriginalName: []byte("currentViewer"), Value: &resolve.Object{}},
		},
	}

	t.Run("matches by response key", func(t *testing.T) {
		sub := findObjectFieldByResponseKey(obj, "cv")
		assert.NotNil(t, sub)
	})

	t.Run("schema name does not match when aliased", func(t *testing.T) {
		sub := findObjectFieldByResponseKey(obj, "currentViewer")
		assert.Nil(t, sub)
	})

	t.Run("scalar field returns nil", func(t *testing.T) {
		sub := findObjectFieldByResponseKey(obj, "id")
		assert.Nil(t, sub)
	})

	t.Run("not found returns nil", func(t *testing.T) {
		sub := findObjectFieldByResponseKey(obj, "unknown")
		assert.Nil(t, sub)
	})

	t.Run("nil obj returns nil", func(t *testing.T) {
		sub := findObjectFieldByResponseKey(nil, "anything")
		assert.Nil(t, sub)
	})
}
