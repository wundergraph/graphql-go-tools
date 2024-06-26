package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestMergeFields_Process(t *testing.T) {

	runTest := func(in, out resolve.Node) func(t *testing.T) {
		return func(t *testing.T) {
			m := &MergeFields{}
			m.Process(in)
			assert.Equal(t, out, in)
		}
	}

	t.Run("merge fields at the end of an object", runTest(
		&resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:  []byte(`a`),
					Value: &resolve.Integer{},
				},
				{
					Name:  []byte(`a`),
					Value: &resolve.Integer{},
					OnTypeNames: [][]byte{
						[]byte(`A`),
					},
				},
			},
		},
		&resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:  []byte(`a`),
					Value: &resolve.Integer{},
				},
			},
		},
	))

	t.Run("merge fields at the end of an object reverse", runTest(
		&resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:  []byte(`a`),
					Value: &resolve.Integer{},
					OnTypeNames: [][]byte{
						[]byte(`A`),
					},
				},
				{
					Name:  []byte(`a`),
					Value: &resolve.Integer{},
					OnTypeNames: [][]byte{
						[]byte(`A`),
					},
				},
				{
					Name:  []byte(`a`),
					Value: &resolve.Integer{},
				},
			},
		},
		&resolve.Object{
			Fields: []*resolve.Field{
				{
					Name:  []byte(`a`),
					Value: &resolve.Integer{},
				},
			},
		},
	))

	t.Run("merge fields at the end of an object nested", runTest(
		&resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte(`a`),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name:  []byte(`b`),
								Value: &resolve.Integer{},
							},
						},
					},
					OnTypeNames: [][]byte{
						[]byte(`A`),
					},
				},
				{
					Name: []byte(`a`),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name:  []byte(`b`),
								Value: &resolve.Integer{},
							},
						},
					},
				},
			},
		},
		&resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte(`a`),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name:  []byte(`b`),
								Value: &resolve.Integer{},
							},
						},
					},
				},
			},
		},
	))

	t.Run("merge fields at the end of an object nested reverse", runTest(
		&resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte(`a`),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name:  []byte(`b`),
								Value: &resolve.Integer{},
							},
						},
					},
				},
				{
					Name: []byte(`a`),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name:  []byte(`b`),
								Value: &resolve.Integer{},
							},
						},
					},
					OnTypeNames: [][]byte{
						[]byte(`A`),
					},
				},
			},
		},
		&resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte(`a`),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name:  []byte(`b`),
								Value: &resolve.Integer{},
							},
						},
					},
				},
			},
		},
	))

	t.Run("merge fields nested object differing onTypeNames", runTest(
		&resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte(`a`),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name:  []byte(`b`),
								Value: &resolve.Integer{},
							},
						},
					},
					OnTypeNames: [][]byte{
						[]byte(`A`),
					},
				},
				{
					Name: []byte(`a`),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name:  []byte(`c`),
								Value: &resolve.Integer{},
							},
						},
					},
					OnTypeNames: [][]byte{
						[]byte(`B`),
					},
				},
			},
		},
		&resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte(`a`),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name:  []byte(`b`),
								Value: &resolve.Integer{},
							},
							{
								Name:  []byte(`c`),
								Value: &resolve.Integer{},
								OnTypeNames: [][]byte{
									[]byte(`A`),
								},
							},
						},
					},
				},
			},
		},
	))
}
