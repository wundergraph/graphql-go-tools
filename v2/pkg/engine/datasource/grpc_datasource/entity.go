package grpcdatasource

import (
	"errors"
	"fmt"

	"github.com/tidwall/gjson"

	"github.com/wundergraph/astjson"
)

type entityIndexMap []int

func newEntityIndexMap(requestedEntityType string, representations []gjson.Result) entityIndexMap {
	indexMap := make(entityIndexMap, filteredRepresentationCount(representations, requestedEntityType))

	requestedTypeIndex := 0
	for i, representation := range representations {
		if representation.Get(typenameFieldName).String() == requestedEntityType {
			indexMap[requestedTypeIndex] = i
			requestedTypeIndex++
		}
	}

	return indexMap
}

// getRepesentations gets the representations from the variables.
// If no representations are found, it returns nil.
func getRepesentations(variables gjson.Result) []gjson.Result {
	r := variables.Get("representations")
	if !r.Exists() {
		return nil
	}

	return r.Array()
}

// validateEntityResponse validates that the entity response is valid
// by checking that the number of entities for a requested type matches the number of representations for that type.
func validateEntityResponse(data *astjson.Value, requestedEntityType string, repesentations []gjson.Result) error {
	if data == nil {
		return errors.New("unable to create entity validator: data is required")
	}

	if requestedEntityType == "" {
		return errors.New("unable to create entity validator: requested entity type is required")
	}

	if len(repesentations) == 0 {
		return errors.New("unable to create entity validator: representations are required")
	}

	entities := data.Get(entityPath).GetArray()
	filteredRepresentationCount := filteredRepresentationCount(repesentations, requestedEntityType)

	if len(entities) != filteredRepresentationCount {
		return fmt.Errorf("entity type %s received %d entities in the subgraph response, but %d are expected", requestedEntityType, len(entities), filteredRepresentationCount)
	}

	return nil
}

// filteredRepresentationCount filters the representations for a given entity type.
// It returns the number of representations for the given entity type.
func filteredRepresentationCount(repesentations []gjson.Result, requestedEntityType string) int {
	count := 0
	for _, representation := range repesentations {
		if representation.Get(typenameFieldName).String() == requestedEntityType {
			count++
		}
	}
	return count
}
