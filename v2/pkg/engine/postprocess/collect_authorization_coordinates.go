package postprocess

import (
	"sort"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// collectAuthorizationCoordinates is a post-processing step that records, on the response's
// GraphQLResponseInfo, every field coordinate that carries an authorization rule
// (@requiresScopes / @authenticated) together with the data source that resolves it. It runs right
// after createFetchTree while the fetch tree is still flat — so the fetch side is a plain loop over
// the root's children (plus RawFetches, which still hold the fetches when extraction is disabled).
// The result is request-independent and cached with the plan; when pre-fetch field authorization is
// enabled the resolver asks the BatchAuthorizer to decide all of these coordinates up front, before
// any fetch executes.
//
// Coordinates are deduplicated by {DataSourceID, TypeName, FieldName} and sorted for determinism.
// When the operation selects no protected field the list is left empty, which makes the enabled mode
// a no-op.
type collectAuthorizationCoordinates struct {
	disable bool
}

type authorizationCoordinateKey struct {
	dataSourceID string
	typeName     string
	fieldName    string
}

func (c *collectAuthorizationCoordinates) Process(response *resolve.GraphQLResponse) {
	if c.disable {
		return
	}
	if response == nil || response.Info == nil {
		return
	}

	coordinates := make(map[authorizationCoordinateKey]resolve.AuthorizationCoordinate)
	for i := range response.RawFetches {
		c.collectFetchItem(response.RawFetches[i], coordinates)
	}
	if response.Fetches != nil {
		c.collectFetchItem(response.Fetches.Item, coordinates)
		for _, child := range response.Fetches.ChildNodes {
			if child == nil {
				continue
			}
			c.collectFetchItem(child.Item, coordinates)
		}
	}
	c.collectNode(response.Data, coordinates)
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

func (c *collectAuthorizationCoordinates) collectFetchItem(item *resolve.FetchItem, coordinates map[authorizationCoordinateKey]resolve.AuthorizationCoordinate) {
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
		c.addCoordinate(coordinates, info.DataSourceID, info.RootFields[i])
	}
}

func (c *collectAuthorizationCoordinates) collectNode(node resolve.Node, coordinates map[authorizationCoordinateKey]resolve.AuthorizationCoordinate) {
	switch n := node.(type) {
	case *resolve.Object:
		if n == nil {
			return
		}
		for i := range n.Fields {
			field := n.Fields[i]
			if field.Info != nil && field.Info.HasAuthorizationRule {
				// A merged (e.g. @shareable) field can be resolved by multiple data sources; seed a
				// coordinate for each so every source that could serve it gets a pre-fetch decision.
				for _, dataSourceID := range field.Info.Source.IDs {
					c.addCoordinate(coordinates, dataSourceID, resolve.GraphCoordinate{
						TypeName:  field.Info.ExactParentTypeName,
						FieldName: field.Info.Name,
					})
				}
			}
			c.collectNode(field.Value, coordinates)
		}
	case *resolve.Array:
		if n == nil {
			return
		}
		c.collectNode(n.Item, coordinates)
	}
}

func (c *collectAuthorizationCoordinates) addCoordinate(coordinates map[authorizationCoordinateKey]resolve.AuthorizationCoordinate, dataSourceID string, coordinate resolve.GraphCoordinate) {
	key := authorizationCoordinateKey{
		dataSourceID: dataSourceID,
		typeName:     coordinate.TypeName,
		fieldName:    coordinate.FieldName,
	}
	coordinates[key] = resolve.AuthorizationCoordinate{
		DataSourceID: dataSourceID,
		Coordinate: resolve.GraphCoordinate{
			TypeName:  coordinate.TypeName,
			FieldName: coordinate.FieldName,
		},
	}
}
