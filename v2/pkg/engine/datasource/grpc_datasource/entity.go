package grpcdatasource

import (
	"errors"
	"fmt"

	"github.com/tidwall/gjson"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// entityIndex represents the mapping between representation order and result order
// for GraphQL federation entities. This is crucial for maintaining correct entity
// order when multiple subgraphs return entities in different orders.
type entityIndex struct {
	representationIndex int // Position within the representations array grouped by typename
	resultIndex         int // Index where this entity should appear in the final result
}

// typeLinker is a map of type names to interfaces.
type typeLinker map[string]string

// add adds a linked type to the type linker.
// We assume that each type has only one interface for simplicity.
func (t typeLinker) add(interfaceName string, linkedTypes ...string) {
	for _, linkedType := range linkedTypes {
		if _, ok := t[linkedType]; !ok {
			t[linkedType] = interfaceName
		}
	}
}

// getLinkedTypes returns the possible interface names for a given type
func (t typeLinker) getLinkedType(memberTypeName string) (string, bool) {
	if len(t) == 0 {
		return "", false
	}

	if interfaceName, ok := t[memberTypeName]; ok {
		return interfaceName, true
	}

	return "", false
}

type entityIndexMap struct {
	typeLinker typeLinker
	entities   map[string][]entityIndex
}

// createEntityIndexMap builds an index mapping for GraphQL federation entities
// from the variables containing entity representations. This map is used to ensure
// that entities are returned in the correct order when merging responses from multiple
// subgraphs, which is critical for GraphQL federation correctness.
func createEntityIndexMap(definition *ast.Document, representations []gjson.Result) *entityIndexMap {
	if len(representations) == 0 {
		// No variables present, so no index map is needed
		return nil
	}

	im := &entityIndexMap{
		typeLinker: make(typeLinker),
	}

	im.parseRepresentations(representations, definition)

	return im
}

// parseRepresentations parses the representations and builds the entity index map.
func (im *entityIndexMap) parseRepresentations(representations []gjson.Result, definition *ast.Document) {
	im.entities = make(map[string][]entityIndex)
	indexSet := make(map[string]int) // Track count per type name

	for i, representation := range representations {
		typeName := representation.Get(typenameFieldName).String()

		memberTypes := getMemberTypesForInterface(definition, typeName)
		if len(memberTypes) > 0 {
			im.typeLinker.add(typeName, memberTypes...)
		}

		// Initialize counter for new type names
		if _, ok := indexSet[typeName]; !ok {
			indexSet[typeName] = -1
		}

		// Increment index for this type
		indexSet[typeName]++

		im.entities[typeName] = append(im.entities[typeName], entityIndex{
			representationIndex: indexSet[typeName], // Position within entities of this type
			resultIndex:         i,                  // Position in the overall result array
		})
	}
}

// getResultIndex returns the correct result index for an entity based on its type
// and representation index. This ensures federated entities maintain proper ordering
// across multiple subgraph responses.
func (im *entityIndexMap) getResultIndex(val *astjson.Value, representationIndex int) int {
	if im == nil || im.entities == nil {
		return representationIndex
	}

	if val == nil {
		return representationIndex
	}

	typeName := val.Get(typenameFieldName).GetStringBytes()
	entities, ok := im.entities[string(typeName)]
	if ok {
		for _, entityIndex := range entities {
			if entityIndex.representationIndex == representationIndex {
				return entityIndex.resultIndex
			}
		}
	}

	entities, found := im.findEntitiesFromLinkedType(string(typeName))
	if found {
		for _, entityIndex := range entities {
			if entityIndex.representationIndex == representationIndex {
				return entityIndex.resultIndex
			}
		}
	}

	return representationIndex
}

func (im *entityIndexMap) findEntitiesFromLinkedType(typeName string) ([]entityIndex, bool) {
	linkedType, found := im.typeLinker.getLinkedType(typeName)
	if !found {
		return nil, false
	}

	return im.entities[linkedType], true
}

// getMemberTypesForInterface gets the member types for an interface.
// If the interface is not found, it returns nil.
func getMemberTypesForInterface(definition *ast.Document, typeName string) []string {
	node, found := definition.NodeByNameStr(typeName)
	if !found {
		return nil
	}

	if node.Kind != ast.NodeKindInterfaceTypeDefinition {
		return nil
	}

	memberTypes, _ := definition.InterfaceTypeDefinitionImplementedByObjectWithNames(node.Ref)
	return memberTypes
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

	entities, err := data.Get(entityPath).Array()
	if err != nil {
		return err
	}

	filteredRepresentationCount := filterRepresentations(repesentations, requestedEntityType)

	if len(entities) != filteredRepresentationCount {
		return fmt.Errorf("entity type %s received %d entities in the subgraph response, but %d are expected", requestedEntityType, len(entities), filteredRepresentationCount)
	}

	return nil
}

// filterRepresentations filters the representations for a given entity type.
// It returns the number of representations for the given entity type.
func filterRepresentations(repesentations []gjson.Result, requestedEntityType string) int {
	count := 0
	for _, representation := range repesentations {
		if representation.Get(typenameFieldName).String() == requestedEntityType {
			count++
		}
	}
	return count

}
