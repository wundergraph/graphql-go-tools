package graphql_datasource

import (
	"bytes"
	"hash"

	"github.com/buger/jsonparser"

	"github.com/jensneuse/graphql-go-tools/pkg/fastbuffer"
	"github.com/jensneuse/graphql-go-tools/pkg/pool"
)

var representationPath = []string{"body", "variables", "representations"}

func prepareBatch(out *fastbuffer.FastBuffer, inputs ...[]byte) (outToInPositions map[int][]int, err error) {
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
