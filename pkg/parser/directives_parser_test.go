package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestDirectivesParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseDirectives", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a single simple directive",
				input:     `@rename(index: 3)`,
				expectErr: BeNil(),
				expectValues: Equal(document.Directives{
					document.Directive{
						Name: []byte("rename"),
						Arguments: document.Arguments{
							document.Argument{
								Name: []byte("index"),
								Value: document.IntValue{
									Val: 3,
								},
							},
						},
					},
				}),
			},
			{
				it:        "should parse multiple simple directives",
				input:     `@rename(index: 3)@moveto(index: 4)`,
				expectErr: BeNil(),
				expectValues: Equal(document.Directives{
					document.Directive{
						Name: []byte("rename"),
						Arguments: document.Arguments{
							document.Argument{
								Name: []byte("index"),
								Value: document.IntValue{
									Val: 3,
								},
							},
						},
					},
					document.Directive{
						Name: []byte("moveto"),
						Arguments: document.Arguments{
							document.Argument{
								Name: []byte("index"),
								Value: document.IntValue{
									Val: 4,
								},
							},
						},
					},
				}),
			},
			{
				it:        "should parse a single simple directive with multiple Arguments",
				input:     `@rename(index: 3, count: 10)`,
				expectErr: BeNil(),
				expectValues: Equal(document.Directives{
					document.Directive{
						Name: []byte("rename"),
						Arguments: document.Arguments{
							document.Argument{
								Name: []byte("index"),
								Value: document.IntValue{
									Val: 3,
								},
							},
							document.Argument{
								Name: []byte("count"),
								Value: document.IntValue{
									Val: 10,
								},
							},
						},
					},
				}),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseDirectives()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
