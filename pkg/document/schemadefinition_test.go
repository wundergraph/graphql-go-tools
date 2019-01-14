package document

import (
	"encoding/json"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"testing"
)

func TestSchemaDefinition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lexer")
}

var _ = Describe("SchemaDefinition", func() {

	type Case struct {
		input     SchemaDefinition
		expectOut types.GomegaMatcher
		expectErr types.GomegaMatcher
	}

	DescribeTable("MarshalJSONObject", func(c Case) {

		actualOut, err := json.Marshal(c.input)
		if c.expectErr != nil {
			Expect(err).To(c.expectErr)
		}

		Expect(string(actualOut)).To(c.expectOut)
	},
		Entry("should marshal simple SchemaDefinition", Case{
			input: SchemaDefinition{
				Query:        []byte("Query"),
				Mutation:     []byte("Mutation"),
				Subscription: []byte("Subscription"),
			},
			expectErr: Not(HaveOccurred()),
			expectOut: Equal(`{"Query":"Query","Mutation":"Mutation","Subscription":"Subscription","Directives":null}`),
		}),
	)
})
