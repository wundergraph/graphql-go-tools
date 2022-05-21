package graphql_datasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/pkg/fastbuffer"
)

func newBufPair(data string, err string) *resolve.BufPair {
	bufPair := resolve.NewBufPair()
	bufPair.Data.WriteString(data)

	if err != "" {
		bufPair.Errors.WriteString(err)
	}

	return bufPair
}

func runTestBatch(t *testing.T, inputs []string, expectedInput string, mappings []inputResponseBufferMappings, batchSize int) {
	expectedFastBuf := fastbuffer.New()
	expectedFastBuf.WriteBytes([]byte(expectedInput))

	expectedBatch := &Batch{
		resultedInput:    expectedFastBuf,
		batchSize:        batchSize,
		responseMappings: mappings,
	}

	convertedInputs := make([][]byte, len(inputs))
	for i := range inputs {
		convertedInputs[i] = []byte(inputs[i])
	}

	batchFactory := NewBatchFactory()
	batch, err := batchFactory.CreateBatch(convertedInputs)
	require.NoError(t, err)
	assert.Equal(t, expectedBatch, batch)
}

func runTestDemultiplex(t *testing.T, inputs []string, responseBufPair *resolve.BufPair, expectedBufPairs []*resolve.BufPair) {
	convertedInputs := make([][]byte, len(inputs))
	for i := range inputs {
		convertedInputs[i] = []byte(inputs[i])
	}

	batchFactory := NewBatchFactory()
	batch, err := batchFactory.CreateBatch(convertedInputs)
	require.NoError(t, err)

	gotBufPairs := make([]*resolve.BufPair, len(inputs))
	for i := range inputs {
		gotBufPairs[i] = resolve.NewBufPair()
	}

	require.NoError(t, batch.Demultiplex(responseBufPair, gotBufPairs))

	assert.Equal(t, expectedBufPairs, gotBufPairs)
}

func TestBatch(t *testing.T) {
	t.Run("create batch with unique args", func(t *testing.T) {
		runTestBatch(
			t,
			[]string{
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}}}`,
			},
			`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"},{"upc":"top-2","__typename":"Product"}]}}}`,
			[]inputResponseBufferMappings{
				{
					responseIndex:         0,
					originalInput:         []byte(`{"upc":"top-1","__typename":"Product"}`),
					assignedBufferIndices: []int{0},
				},
				{
					responseIndex:         1,
					originalInput:         []byte(`{"upc":"top-2","__typename":"Product"}`),
					assignedBufferIndices: []int{1},
				},
			},
			2,
		)
	})
	t.Run("deduplicate the same args", func(t *testing.T) {
		runTestBatch(
			t,
			[]string{
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}}}`,
			},
			`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}}}`,
			[]inputResponseBufferMappings{
				{
					responseIndex:         0,
					originalInput:         []byte(`{"upc":"top-2","__typename":"Product"}`),
					assignedBufferIndices: []int{0, 1},
				},
			},
			2,
		)
	})
	t.Run("deduplicate the same args with overlaps", func(t *testing.T) {
		runTestBatch(
			t,
			[]string{
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"},{"upc":"top-1","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"},{"upc":"top-3","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-3","__typename":"Product"},{"upc":"top-2","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"},{"upc":"top-2","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-3","__typename":"Product"},{"upc":"top-1","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-3","__typename":"Product"}]}}}`,
			},
			`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"},{"upc":"top-1","__typename":"Product"},{"upc":"top-3","__typename":"Product"}]}}}`,
			[]inputResponseBufferMappings{
				{
					responseIndex:         0,
					originalInput:         []byte(`{"upc":"top-2","__typename":"Product"}`),
					assignedBufferIndices: []int{0, 1, 2, 3, 5},
				},
				{
					responseIndex:         1,
					originalInput:         []byte(`{"upc":"top-1","__typename":"Product"}`),
					assignedBufferIndices: []int{0, 3, 4, 6},
				},
				{
					responseIndex:         2,
					originalInput:         []byte(`{"upc":"top-3","__typename":"Product"}`),
					assignedBufferIndices: []int{1, 2, 4, 7},
				},
			},
			8,
		)
	})
	t.Run("create batch with complex inputs", func(t *testing.T) {
		runTestBatch(
			t,
			[]string{ // Entity has multi key: category + name
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"category":"category-1", "name":"Top 1","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"category":"category-2", "name":"Top 1","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"category":"category-1", "name":"Top 1","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"category":"category-2", "name":"Top 2","__typename":"Product"}]}}}`,
			},
			`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"category":"category-1", "name":"Top 1","__typename":"Product"},{"category":"category-2", "name":"Top 1","__typename":"Product"},{"category":"category-2", "name":"Top 2","__typename":"Product"}]}}}`,
			[]inputResponseBufferMappings{
				{
					responseIndex:         0,
					originalInput:         []byte(`{"category":"category-1", "name":"Top 1","__typename":"Product"}`),
					assignedBufferIndices: []int{0, 2},
				},
				{
					responseIndex:         1,
					originalInput:         []byte(`{"category":"category-2", "name":"Top 1","__typename":"Product"}`),
					assignedBufferIndices: []int{1},
				},
				{
					responseIndex:         2,
					originalInput:         []byte(`{"category":"category-2", "name":"Top 2","__typename":"Product"}`),
					assignedBufferIndices: []int{3},
				},
			},
			4,
		)
	})
}

func TestBatch_Demultiplex(t *testing.T) {
	t.Run("demultiplex uniq inputs", func(t *testing.T) {
		runTestDemultiplex(
			t,
			[]string{
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}}}`,
			},
			newBufPair(`[{"name":"Name 1", "price": 1.01, "__typename":"Product"},{"name":"Name 2", "price": 2.01, "__typename":"Product"}]`, ""),
			[]*resolve.BufPair{
				newBufPair(`{"name":"Name 1", "price": 1.01, "__typename":"Product"}`, ""),
				newBufPair(`{"name":"Name 2", "price": 2.01, "__typename":"Product"}`, ""),
			},
		)
	})
	t.Run("demultiplex deduplicated inputs", func(t *testing.T) {
		runTestDemultiplex(
			t,
			[]string{
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`,
			},
			newBufPair(`[{"name":"Name 1", "price": 1.01, "__typename":"Product"},{"name":"Name 2", "price": 2.01, "__typename":"Product"}]`, ""),
			[]*resolve.BufPair{
				newBufPair(`{"name":"Name 1", "price": 1.01, "__typename":"Product"}`, ""),
				newBufPair(`{"name":"Name 2", "price": 2.01, "__typename":"Product"}`, ""),
				newBufPair(`{"name":"Name 1", "price": 1.01, "__typename":"Product"}`, ""),
			},
		)
	})
	t.Run("demultiplex response with error", func(t *testing.T) {
		runTestDemultiplex(
			t,
			[]string{
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}}}`,
			},
			newBufPair(`[null,{"name":"Name 2", "price": 2.01, "__typename":"Product"}]`, `{"message":"errorMessage"}`),
			[]*resolve.BufPair{
				newBufPair("null", `{"message":"errorMessage"}`),
				newBufPair(`{"name":"Name 2", "price": 2.01, "__typename":"Product"}`, ""),
			},
		)
	})
}
