package graphql_datasource

import (
	"bytes"
	"fmt"
	"hash"

	"github.com/buger/jsonparser"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/fastbuffer"
	"github.com/jensneuse/graphql-go-tools/pkg/pool"
)

var representationPath = []string{"body", "variables", "representations"}

type Batch struct {
	resultedInput    *fastbuffer.FastBuffer
	outToInPositions map[int][]int
	batchSize        int
}

func NewBatchFactory() *BatchFactory {
	return &BatchFactory{}
}

type BatchFactory struct {
}

func (b *BatchFactory) CreateBatch(inputs ...[]byte) (resolve.DataSourceBatch, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	resultedInput := pool.FastBuffer.Get()

	outToInPositions, err := multiplexBatch(resultedInput, inputs...)
	if err != nil {
		return nil, nil
	}

	return &Batch{
		resultedInput:    resultedInput,
		outToInPositions: outToInPositions,
		batchSize:        len(inputs),
	}, nil
}

func (b *Batch) Input() *fastbuffer.FastBuffer {
	return b.resultedInput
}

func (b *Batch) Demultiplex(responseBufPair *resolve.BufPair, bufPairs []*resolve.BufPair) (err error) {
	defer pool.FastBuffer.Put(b.resultedInput)

	if b.batchSize != len(bufPairs) {
		return fmt.Errorf("expected %d buf pairs", b.batchSize)
	}

	if err = demultiplexBatch(responseBufPair, b.outToInPositions, bufPairs); err != nil {
		return err
	}

	return
}

func multiplexBatch(out *fastbuffer.FastBuffer, inputs ...[]byte) (outToInPositions map[int][]int, err error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	var variables [][]byte
	var currOutPosition int

	outToInPositions = make(map[int][]int, len(inputs))
	hashToOutPositions := make(map[uint64]int, len(inputs))

	hash64 := pool.Hash64.Get().(hash.Hash64)
	defer pool.Hash64.Put(hash64)

	for i := range inputs {
		inputVariables, _, _, err := jsonparser.Get(inputs[i], representationPath...)
		if err != nil {
			return nil, err
		}

		if _, err = hash64.Write(inputVariables); err != nil {
			return nil, err
		}
		// deduplicate inputs, do not send the same representation inputVariables
		inputHash := hash64.Sum64()
		hash64.Reset()

		if outPosition, ok := hashToOutPositions[inputHash]; ok {
			outToInPositions[outPosition] = append(outToInPositions[outPosition], i)
			continue
		}

		hashToOutPositions[inputHash] = currOutPosition
		outToInPositions[currOutPosition] = []int{i}
		currOutPosition++

		_, err = jsonparser.ArrayEach(inputVariables, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
			variables = append(variables, value)
		})
		if err != nil {
			return nil, err
		}
	}

	representationJson := append([]byte("["), append(bytes.Join(variables, []byte(",")), []byte("]")...)...)

	mergedInput, err := jsonparser.Set(inputs[0], representationJson, representationPath...)
	if err != nil {
		return nil, err
	}

	out.WriteBytes(mergedInput)

	return outToInPositions, nil
}

func demultiplexBatch(responsePair *resolve.BufPair, outToInPositions map[int][]int, resultBufPairs []*resolve.BufPair) (err error) {
	var outPosition int

	if responsePair.HasData() {
		_, err = jsonparser.ArrayEach(responsePair.Data.Bytes(), func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
			inputPositions := outToInPositions[outPosition]

			for _, pos := range inputPositions {
				resultBufPairs[pos].Data.WriteBytes(value)
			}

			outPosition++
		})
		if err != nil {
			return err
		}
	}

	if responsePair.HasErrors() {
		resultBufPairs[0].Errors.WriteBytes(responsePair.Errors.Bytes())
	}

	return
}
