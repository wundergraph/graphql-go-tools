package parser

import (
	"bytes"
	. "github.com/franela/goblin"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestImplementsInterfacesParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("parser.parseImplementsInterfaces", func() {

		tests := []struct {
			it           string
			input        string
			expectErr    types.GomegaMatcher
			expectValues types.GomegaMatcher
		}{
			{
				it:        "should parse a simple ImplementsInterfaces",
				input:     "implements Dogs",
				expectErr: BeNil(),
				expectValues: Equal(document.ImplementsInterfaces{
					[]byte("Dogs"),
				}),
			},
			{
				it:        "should parse ImplementsInterfaces with multiple interfaces",
				input:     "implements Dogs & Cats & Mice",
				expectErr: BeNil(),
				expectValues: Equal(document.ImplementsInterfaces{
					[]byte("Dogs"),
					[]byte("Cats"),
					[]byte("Mice"),
				}),
			},
			{
				it:        "should not parse ImplementsInterfaces after a '&' is omitted",
				input:     "implements Dogs & Cats Mice",
				expectErr: BeNil(),
				expectValues: Equal(document.ImplementsInterfaces{
					[]byte("Dogs"),
					[]byte("Cats"),
				}),
			},
			{
				it:           "should not parse ImplementsInterfaces when not starting with 'implements'",
				input:        "implement Dogs & Cats Mice",
				expectErr:    BeNil(),
				expectValues: Equal(document.ImplementsInterfaces(nil)),
			},
		}

		for _, test := range tests {
			test := test

			g.It(test.it, func() {

				reader := bytes.NewReader([]byte(test.input))
				parser := NewParser()
				parser.l.SetInput(reader)

				val, err := parser.parseImplementsInterfaces()
				Expect(err).To(test.expectErr)
				Expect(val).To(test.expectValues)
			})
		}
	})
}
