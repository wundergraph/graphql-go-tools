package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeHasAliases(t *testing.T) {
	tests := []struct {
		name     string
		object   *Object
		expected bool
	}{
		{
			name:     "nil object",
			object:   nil,
			expected: false,
		},
		{
			name: "object with one plain field",
			object: &Object{
				Fields: []*Field{
					{
						Name:  []byte("realName"),
						Value: &String{},
					},
				},
			},
			expected: false,
		},
		{
			name: "object with one aliased field",
			object: &Object{
				Fields: []*Field{
					{
						Name:         []byte("a"),
						OriginalName: []byte("realName"),
						Value:        &String{},
					},
				},
			},
			expected: true,
		},
		{
			name: "object with one field carrying a CacheArg",
			object: &Object{
				Fields: []*Field{
					{
						Name:  []byte("realName"),
						Value: &String{},
						CacheArgs: []CacheFieldArg{
							{
								ArgName:      "format",
								VariableName: "format",
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "nested object with inner aliased field",
			object: &Object{
				Fields: []*Field{
					{
						Name: []byte("profile"),
						Value: &Object{
							Fields: []*Field{
								{
									Name:         []byte("a"),
									OriginalName: []byte("realName"),
									Value:        &String{},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "array of object with aliased item field",
			object: &Object{
				Fields: []*Field{
					{
						Name: []byte("profiles"),
						Value: &Array{
							Item: &Object{
								Fields: []*Field{
									{
										Name:         []byte("a"),
										OriginalName: []byte("realName"),
										Value:        &String{},
									},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := ComputeHasAliases(test.object)

			assert.Equal(t, test.expected, actual)
		})
	}
}
