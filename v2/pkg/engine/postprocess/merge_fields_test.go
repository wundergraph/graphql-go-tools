package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestMergeFields_Process(t *testing.T) {

	runTest := func(in, expected resolve.Node) func(t *testing.T) {
		return func(t *testing.T) {
			m := &mergeFields{}
			m.Process(in)
			assert.Equal(t, expected, in)
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

	t.Run("merge enum fields", runTest(
		&resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte(`interfaceField`),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte(`enumField`),
								Value: &resolve.Enum{
									Values:             []string{`a`},
									InaccessibleValues: []string{},
									TypeName:           `Enum`,
								},
							},
						},
					},
					OnTypeNames: [][]byte{[]byte(`A`), []byte(`B`)},
				},
				{
					Name: []byte(`interfaceField`),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name:  []byte(`stringField`),
								Value: &resolve.String{},
							},
						},
					},
					OnTypeNames: [][]byte{[]byte(`A`), []byte(`B`)},
				},
			},
		},
		&resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte(`interfaceField`),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte(`enumField`),
								Value: &resolve.Enum{
									Values:             []string{`a`},
									InaccessibleValues: []string{},
									TypeName:           `Enum`,
								},
								ParentOnTypeNames: []resolve.ParentOnTypeNames{
									{
										Depth: 1,
										Names: [][]byte{[]byte(`A`)},
									},
								},
							},
							{
								Name:  []byte(`stringField`),
								Value: &resolve.String{},
								ParentOnTypeNames: []resolve.ParentOnTypeNames{
									{
										Depth: 1,
										Names: [][]byte{[]byte(`A`)},
									},
								},
							},
						},
					},
					OnTypeNames: [][]byte{[]byte(`A`)},
				},
				{
					Name: []byte(`interfaceField`),
					Value: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte(`enumField`),
								Value: &resolve.Enum{
									Values:             []string{`a`},
									InaccessibleValues: []string{},
									TypeName:           `Enum`,
								},
								ParentOnTypeNames: []resolve.ParentOnTypeNames{
									{
										Depth: 1,
										Names: [][]byte{[]byte(`B`)},
									},
								},
							},
							{
								Name:  []byte(`stringField`),
								Value: &resolve.String{},
								ParentOnTypeNames: []resolve.ParentOnTypeNames{
									{
										Depth: 1,
										Names: [][]byte{[]byte(`B`)},
									},
								},
							},
						},
					},
					OnTypeNames: [][]byte{[]byte(`B`)},
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
}
