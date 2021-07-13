package graphql_datasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/fastbuffer"
)

func newBufPair(data string, err string) *resolve.BufPair {
	bufPair := resolve.NewBufPair()
	bufPair.Data.WriteString(data)

	if err != "" {
		bufPair.Errors.WriteString(err)
	}

	return bufPair
}

func runTestBatch(t *testing.T, inputs []string, expectedInput string, outToInPos map[int][]int, batchSize int) {
	expectedFastBuf := fastbuffer.New()
	expectedFastBuf.WriteBytes([]byte(expectedInput))

	expectedBatch := &Batch{
		resultedInput:    expectedFastBuf,
		outToInPositions: outToInPos,
		batchSize:        batchSize,
	}

	convertedInputs := make([][]byte, len(inputs))
	for i := range inputs {
		convertedInputs[i] = []byte(inputs[i])
	}

	batchFactory := NewBatchFactory()
	batch, err := batchFactory.CreateBatch(convertedInputs...)
	require.NoError(t, err)
	assert.Equal(t, expectedBatch, batch)
}

func runTestDemultiplex(t *testing.T, inputs []string, responseBufPair *resolve.BufPair, expectedBufPairs []*resolve.BufPair) {
	convertedInputs := make([][]byte, len(inputs))
	for i := range inputs {
		convertedInputs[i] = []byte(inputs[i])
	}


	batchFactory := NewBatchFactory()
	batch, err := batchFactory.CreateBatch(convertedInputs...)
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
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}},"extract_entities":true}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}},"extract_entities":true}`,
			},
			`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"},{"upc":"top-2","__typename":"Product"}]}},"extract_entities":true}`,
			map[int][]int{0: {0}, 1: {1}},
			2,
		)
	})
	t.Run("deduplicate the same args", func(t *testing.T) {
		runTestBatch(
			t,
			[]string{
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}},"extract_entities":true}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}},"extract_entities":true}`,
			},
			`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}},"extract_entities":true}`,
			map[int][]int{0: {0, 1}},
			2,
		)
	})
	t.Run("create batch with complex inputs", func(t *testing.T) {
		runTestBatch(
			t,
			[]string{ // Entity has multi key: category + name
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"category":"category-1", "name":"Top 1","__typename":"Product"}]}},"extract_entities":true}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"category":"category-2", "name":"Top 1","__typename":"Product"}]}},"extract_entities":true}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"category":"category-1", "name":"Top 1","__typename":"Product"}]}},"extract_entities":true}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"category":"category-2", "name":"Top 2","__typename":"Product"}]}},"extract_entities":true}`,
			},
			`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"category":"category-1", "name":"Top 1","__typename":"Product"},{"category":"category-2", "name":"Top 1","__typename":"Product"},{"category":"category-2", "name":"Top 2","__typename":"Product"}]}},"extract_entities":true}`,
			map[int][]int{0: {0, 2}, 1: {1}, 2: {3}},
			4,
		)
	})
}

func TestBatch_Demultiplex(t *testing.T) {
	t.Run("demultiplex uniq inputs", func(t *testing.T) {
		runTestDemultiplex(
			t,
			[]string{
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}},"extract_entities":true}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}},"extract_entities":true}`,
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
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}},"extract_entities":true}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}},"extract_entities":true}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}},"extract_entities":true}`,
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
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}},"extract_entities":true}`,
				`{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}},"extract_entities":true}`,
			},
			newBufPair(`[null,{"name":"Name 2", "price": 2.01, "__typename":"Product"}]`,`{"message":"errorMessage"}`),
			[]*resolve.BufPair{
				newBufPair("null", `{"message":"errorMessage"}`),
				newBufPair(`{"name":"Name 2", "price": 2.01, "__typename":"Product"}`, ""),
			},
		)
	})
}
