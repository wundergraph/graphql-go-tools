package resolve

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

func TestResolvable_NonNullableNullBubble_DedupsExistingError(t *testing.T) {
	// A nullable parent (order) with a non-nullable child (order.items).
	// When items resolves to null because of an error, only one error should
	// be present — the synthetic "Cannot return null for non-nullable field"
	// error must not be added on top of an error that already covers the path.
	plan := func() *Object {
		return &Object{
			Fields: []*Field{
				{
					Name: []byte("order"),
					Value: &Object{
						Nullable: true,
						Path:     []string{"order"},
						Fields: []*Field{
							{
								Name: []byte("items"),
								Value: &Object{
									Path: []string{"items"},
									Fields: []*Field{
										{
											Name:  []byte("totalCount"),
											Value: &Integer{Path: []string{"totalCount"}},
										},
									},
								},
							},
						},
					},
				},
			},
		}
	}

	seed := func(res *Resolvable, msg string, path ...fastjsonext.PathElement) {
		res.ensureErrorsInitialized()
		fastjsonext.AppendErrorToArray(res.astjsonArena, res.errors, msg, path)
	}

	t.Run("existing error covering the null field is not duplicated", func(t *testing.T) {
		res := NewResolvable(nil, ResolvableOptions{})
		err := res.Init(&Context{}, []byte(`{"order":{"items":null}}`), ast.OperationTypeQuery)
		assert.NoError(t, err)
		seed(res, "items are unavailable",
			fastjsonext.PathElement{Name: "order"}, fastjsonext.PathElement{Name: "items"})

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), plan(), nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"errors":[{"message":"items are unavailable","path":["order","items"]}],"data":{"order":null}}`, out.String())
	})

	t.Run("existing error descending from the null field is not duplicated", func(t *testing.T) {
		res := NewResolvable(nil, ResolvableOptions{})
		err := res.Init(&Context{}, []byte(`{"order":{"items":null}}`), ast.OperationTypeQuery)
		assert.NoError(t, err)
		seed(res, "boom",
			fastjsonext.PathElement{Name: "order"}, fastjsonext.PathElement{Name: "items"},
			fastjsonext.PathElement{Name: "nodes"}, fastjsonext.PathElement{Idx: 0})

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), plan(), nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"errors":[{"message":"boom","path":["order","items","nodes",0]}],"data":{"order":null}}`, out.String())
	})

	t.Run("unrelated sibling error still yields the synthetic error", func(t *testing.T) {
		res := NewResolvable(nil, ResolvableOptions{})
		err := res.Init(&Context{}, []byte(`{"order":{"items":null}}`), ast.OperationTypeQuery)
		assert.NoError(t, err)
		seed(res, "boom",
			fastjsonext.PathElement{Name: "order"}, fastjsonext.PathElement{Name: "shipments"})

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), plan(), nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"errors":[{"message":"boom","path":["order","shipments"]},{"message":"Cannot return null for non-nullable field 'Query.order.items'.","path":["order","items"]}],"data":{"order":null}}`, out.String())
	})
}
