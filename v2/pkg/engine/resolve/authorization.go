package resolve

import (
	"sort"
)

func CollectAuthorizationCoordinates(response *GraphQLResponse) {
	if response == nil || response.Info == nil {
		return
	}

	coordinates := make(map[authorizationCoordinateKey]AuthorizationCoordinate)
	// Collect from both the raw fetch list and the fetch tree. This function runs at plan build,
	// before the post-processor moves RawFetches into the Fetches tree, so in normal engine
	// execution the fetches only exist in RawFetches at this point; test fixtures build the tree
	// directly. Deduplication makes covering both harmless.
	for i := range response.RawFetches {
		collectFetchItemAuthorizationCoordinates(response.RawFetches[i], coordinates)
	}
	collectFetchAuthorizationCoordinates(response.Fetches, coordinates)
	collectNodeAuthorizationCoordinates(response.Data, coordinates)
	if len(coordinates) == 0 {
		response.Info.AuthorizationCoordinates = nil
		return
	}

	response.Info.AuthorizationCoordinates = response.Info.AuthorizationCoordinates[:0]
	for _, coordinate := range coordinates {
		response.Info.AuthorizationCoordinates = append(response.Info.AuthorizationCoordinates, coordinate)
	}
	sort.Slice(response.Info.AuthorizationCoordinates, func(i, j int) bool {
		left := response.Info.AuthorizationCoordinates[i]
		right := response.Info.AuthorizationCoordinates[j]
		if left.DataSourceID != right.DataSourceID {
			return left.DataSourceID < right.DataSourceID
		}
		if left.Coordinate.TypeName != right.Coordinate.TypeName {
			return left.Coordinate.TypeName < right.Coordinate.TypeName
		}
		return left.Coordinate.FieldName < right.Coordinate.FieldName
	})
}

type authorizationCoordinateKey struct {
	dataSourceID string
	typeName     string
	fieldName    string
}

func collectFetchAuthorizationCoordinates(node *FetchTreeNode, coordinates map[authorizationCoordinateKey]AuthorizationCoordinate) {
	if node == nil {
		return
	}
	collectFetchItemAuthorizationCoordinates(node.Item, coordinates)
	for i := range node.ChildNodes {
		collectFetchAuthorizationCoordinates(node.ChildNodes[i], coordinates)
	}
	if node.Trigger != nil {
		collectFetchAuthorizationCoordinates(node.Trigger, coordinates)
	}
}

func collectFetchItemAuthorizationCoordinates(item *FetchItem, coordinates map[authorizationCoordinateKey]AuthorizationCoordinate) {
	if item == nil || item.Fetch == nil {
		return
	}
	info := item.Fetch.FetchInfo()
	if info == nil {
		return
	}
	for i := range info.RootFields {
		if !info.RootFields[i].HasAuthorizationRule {
			continue
		}
		addAuthorizationCoordinate(coordinates, info.DataSourceID, info.RootFields[i])
	}
}

func collectNodeAuthorizationCoordinates(node Node, coordinates map[authorizationCoordinateKey]AuthorizationCoordinate) {
	switch n := node.(type) {
	case *Object:
		if n == nil {
			return
		}
		for i := range n.Fields {
			field := n.Fields[i]
			if field.Info != nil && field.Info.HasAuthorizationRule {
				// A merged (e.g. @shareable) field can be resolved by multiple data sources; seed a
				// coordinate for each so every source that could serve it gets a pre-fetch decision.
				for _, dataSourceID := range field.Info.Source.IDs {
					addAuthorizationCoordinate(coordinates, dataSourceID, GraphCoordinate{
						TypeName:  field.Info.ExactParentTypeName,
						FieldName: field.Info.Name,
					})
				}
			}
			collectNodeAuthorizationCoordinates(field.Value, coordinates)
		}
	case *Array:
		if n == nil {
			return
		}
		collectNodeAuthorizationCoordinates(n.Item, coordinates)
	}
}

func addAuthorizationCoordinate(coordinates map[authorizationCoordinateKey]AuthorizationCoordinate, dataSourceID string, coordinate GraphCoordinate) {
	key := authorizationCoordinateKey{
		dataSourceID: dataSourceID,
		typeName:     coordinate.TypeName,
		fieldName:    coordinate.FieldName,
	}
	coordinates[key] = AuthorizationCoordinate{
		DataSourceID: dataSourceID,
		Coordinate: GraphCoordinate{
			TypeName:  coordinate.TypeName,
			FieldName: coordinate.FieldName,
		},
	}
}
