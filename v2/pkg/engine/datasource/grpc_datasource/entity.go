package grpcdatasource

import (
	"errors"
	"fmt"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

const representationsKey = "representations"

// entityIndexMap maps positions in the typed gRPC response back to positions
// in the original representations array. The slice index is the response
// position; the value is the representation index. It is built per call by
// recording the position of every representation whose __typename matches
// the requested entity type.
type entityIndexMap []int

// newEntityIndexMap builds the index map for a single entity call by collecting
// the positions of representations whose __typename matches the requested type.
// A single pass over representations populates the slice.
func newEntityIndexMap(requestedEntityType string, representations []*astjson.Value) entityIndexMap {
	indexMap := make(entityIndexMap, 0, len(representations))
	for i, representation := range representations {
		if string(representation.Get(typenameFieldName).GetStringBytes()) == requestedEntityType {
			indexMap = append(indexMap, i)
		}
	}
	return indexMap
}

// getRepresentations returns the representations from the variables, or nil if there are none.
func getRepresentations(variables *astjson.Value) []*astjson.Value {
	return variables.GetArray(representationsKey)
}

// filterRepresentations filters the representations to only include the ones of the requested entity type.
func filterRepresentations(arena arena.Arena, variables *astjson.Value, requestedEntityType string) *astjson.Value {
	representations := getRepresentations(variables)
	if len(representations) == 0 {
		return nil
	}

	// Required calls might have arguments, which are provided as additional variables
	// along with the representations. Therefore we need to copy all other elements into the new variables object.
	ov := astjson.ObjectValue(arena)
	variables.GetObject().Visit(func(key []byte, v *astjson.Value) {
		if string(key) != representationsKey {
			ov.Set(arena, string(key), v)
		}
	})

	representationsArr := astjson.ArrayValue(arena)

	for _, representation := range representations {
		if string(representation.Get(typenameFieldName).GetStringBytes()) == requestedEntityType {
			representationsArr.SetArrayItem(arena, len(representationsArr.GetArray()), representation)
		}
	}

	ov.Set(arena, representationsKey, representationsArr)
	return ov
}

// validateEntityResponse verifies that the number of entities returned by the
// subgraph matches the number of representations of the requested type.
// Callers should subsequently build an entityIndexMap via newEntityIndexMap to
// merge the response — mergeEntities relies on the invariant that
// len(response entities) == len(indexMap), which this function establishes.
func validateEntityResponse(data *astjson.Value, requestedEntityType string, representations []*astjson.Value) error {
	if data == nil {
		return errors.New("validateEntityResponse: subgraph response data is nil")
	}

	if requestedEntityType == "" {
		return errors.New("validateEntityResponse: requested entity type is empty; the entity RPC plan is missing a RequestedEntityType")
	}

	if len(representations) == 0 {
		return errors.New("validateEntityResponse: no entity representations provided in the request variables")
	}

	expected := 0
	for _, representation := range representations {
		if string(representation.Get(typenameFieldName).GetStringBytes()) == requestedEntityType {
			expected++
		}
	}

	entities := data.Get(entityPath).GetArray()
	if len(entities) != expected {
		return fmt.Errorf("entity type %s received %d entities in the subgraph response, but %d are expected", requestedEntityType, len(entities), expected)
	}

	return nil
}
