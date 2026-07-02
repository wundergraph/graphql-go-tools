package resolve

import (
	"sort"
)

func CollectAuthorizationCoordinates(response *GraphQLResponse) {
	if response == nil || response.Info == nil {
		return
	}

	coordinates := make(map[authorizationCoordinateKey]AuthorizationCoordinate)
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
	if node.Item != nil && node.Item.Fetch != nil {
		info := node.Item.Fetch.FetchInfo()
		if info != nil {
			for i := range info.RootFields {
				if !info.RootFields[i].HasAuthorizationRule {
					continue
				}
				addAuthorizationCoordinate(coordinates, info.DataSourceID, info.RootFields[i])
			}
		}
	}
	for i := range node.ChildNodes {
		collectFetchAuthorizationCoordinates(node.ChildNodes[i], coordinates)
	}
	if node.Trigger != nil {
		collectFetchAuthorizationCoordinates(node.Trigger, coordinates)
	}
}

func collectNodeAuthorizationCoordinates(node Node, coordinates map[authorizationCoordinateKey]AuthorizationCoordinate) {
	switch n := node.(type) {
	case *Object:
		for i := range n.Fields {
			field := n.Fields[i]
			if field.Info != nil && field.Info.HasAuthorizationRule && len(field.Info.Source.IDs) > 0 {
				addAuthorizationCoordinate(coordinates, field.Info.Source.IDs[0], GraphCoordinate{
					TypeName:  field.Info.ExactParentTypeName,
					FieldName: field.Info.Name,
				})
			}
			collectNodeAuthorizationCoordinates(field.Value, coordinates)
		}
	case *Array:
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
