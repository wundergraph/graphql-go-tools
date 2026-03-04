package plan

import "fmt"

// dataSourceSplitter is implemented by dataSourceConfiguration[T] to enable
// cloning a datasource with new ID and metadata during root field splitting.
type dataSourceSplitter interface {
	cloneForSplit(newID string, metadata *DataSourceMetadata) (DataSource, error)
}

// SplitDataSourcesByRootFieldCaching splits datasources that have root field caching
// configured into separate per-field datasources. This ensures each cacheable root field
// gets its own fetch, enabling independent L2 caching per field.
//
// Why split? The planner merges root fields from the same datasource into a single fetch.
// This means a query like { me { id } cat { name } } produces one request to the subgraph.
// However, configureFetchCaching requires all root fields in a fetch to have identical
// cache configs. By splitting each cached root field into its own datasource, the planner
// creates separate fetches, and each fetch can have its own TTL and cache key.
//
// The split produces up to N+1 datasources from the original:
//   - One datasource per cached root field (each with its own RootFieldCaching entry)
//   - One remainder datasource for all uncached root fields (no RootFieldCaching)
//
// All split datasources share the same non-Query root nodes (entity types, Mutation,
// Subscription), child nodes, entity caching config, and federation metadata (keys,
// requires, provides). This preserves entity resolution capability across all splits.
func SplitDataSourcesByRootFieldCaching(dataSources []DataSource) ([]DataSource, error) {
	var result []DataSource
	for _, ds := range dataSources {
		split, err := splitSingleDataSourceByRootFieldCaching(ds)
		if err != nil {
			return nil, fmt.Errorf("failed to split data source %s by root field caching: %w", ds.Id(), err)
		}
		result = append(result, split...)
	}
	return result, nil
}

func splitSingleDataSourceByRootFieldCaching(ds DataSource) ([]DataSource, error) {
	fedConfig := ds.FederationConfiguration()

	// No root field caching configured — nothing to split
	if len(fedConfig.RootFieldCaching) == 0 {
		return []DataSource{ds}, nil
	}

	// Check if the datasource supports cloning (all dataSourceConfiguration[T] do)
	splitter, ok := ds.(dataSourceSplitter)
	if !ok {
		return []DataSource{ds}, nil
	}

	nodesAccess, ok := ds.(NodesAccess)
	if !ok {
		return []DataSource{ds}, nil
	}

	// Find the Query root node — we only split Query fields, not Mutation/Subscription
	rootNodes := nodesAccess.ListRootNodes()
	queryNodeIdx := -1
	for i, node := range rootNodes {
		if node.TypeName == "Query" {
			queryNodeIdx = i
			break
		}
	}
	if queryNodeIdx == -1 {
		// No Query root node — nothing to split (entity-only datasource)
		return []DataSource{ds}, nil
	}

	// Partition Query fields into cached and uncached buckets
	queryNode := rootNodes[queryNodeIdx]
	var cachedFields, uncachedFields []string
	for _, fieldName := range queryNode.FieldNames {
		if fedConfig.RootFieldCaching.FindByTypeAndField("Query", fieldName) != nil {
			cachedFields = append(cachedFields, fieldName)
		} else {
			uncachedFields = append(uncachedFields, fieldName)
		}
	}

	// Skip splitting when there's only a single cached field and no uncached fields.
	// A single-field datasource already gets its own fetch — splitting adds no benefit.
	if len(cachedFields) <= 1 && len(uncachedFields) == 0 {
		return []DataSource{ds}, nil
	}

	childNodes := nodesAccess.ListChildNodes()

	// Collect non-Query root nodes (e.g. User entity, Mutation) — these are shared
	// across all split datasources so entity resolution continues to work
	var nonQueryRootNodes TypeFields
	for _, node := range rootNodes {
		if node.TypeName != "Query" {
			nonQueryRootNodes = append(nonQueryRootNodes, node)
		}
	}

	var result []DataSource

	// Create one datasource per cached Query root field.
	// Each gets a unique ID (original_rf_fieldName) and only its own cache config.
	for _, fieldName := range cachedFields {
		// Build root nodes: single Query field + all non-Query root nodes
		splitRootNodes := make(TypeFields, 0, len(nonQueryRootNodes)+1)
		splitRootNodes = append(splitRootNodes, TypeField{
			TypeName:           "Query",
			FieldNames:         []string{fieldName},
			ExternalFieldNames: queryNode.ExternalFieldNames,
			FetchReasonFields:  queryNode.FetchReasonFields,
		})
		splitRootNodes = append(splitRootNodes, nonQueryRootNodes...)

		// Attach only this field's cache config to the new datasource
		cacheConfig := fedConfig.RootFieldCaching.FindByTypeAndField("Query", fieldName)
		metadata := cloneMetadataForSplit(ds, splitRootNodes, childNodes)
		metadata.FederationMetaData.RootFieldCaching = RootFieldCacheConfigurations{*cacheConfig}

		splitID := fmt.Sprintf("%s_rf_%s", ds.Id(), fieldName)
		splitDS, err := splitter.cloneForSplit(splitID, metadata)
		if err != nil {
			return nil, err
		}
		result = append(result, splitDS)
	}

	// Create a remainder datasource for uncached fields (if any).
	// This keeps the original datasource ID so existing planner behavior is preserved.
	if len(uncachedFields) > 0 {
		remainderRootNodes := make(TypeFields, 0, len(nonQueryRootNodes)+1)
		remainderRootNodes = append(remainderRootNodes, TypeField{
			TypeName:           "Query",
			FieldNames:         uncachedFields,
			ExternalFieldNames: queryNode.ExternalFieldNames,
			FetchReasonFields:  queryNode.FetchReasonFields,
		})
		remainderRootNodes = append(remainderRootNodes, nonQueryRootNodes...)

		metadata := cloneMetadataForSplit(ds, remainderRootNodes, childNodes)
		// Explicitly clear root field caching — uncached fields should not inherit cache config
		metadata.FederationMetaData.RootFieldCaching = nil

		remainderDS, err := splitter.cloneForSplit(ds.Id(), metadata)
		if err != nil {
			return nil, err
		}
		result = append(result, remainderDS)
	}

	return result, nil
}

// cloneMetadataForSplit creates new DataSourceMetadata with the given root nodes
// while preserving all federation metadata, child nodes, and directives from the original.
func cloneMetadataForSplit(original DataSource, rootNodes, childNodes TypeFields) *DataSourceMetadata {
	origFed := original.FederationConfiguration()
	origDirectives := original.DirectiveConfigurations()

	return &DataSourceMetadata{
		RootNodes:  rootNodes,
		ChildNodes: childNodes,
		Directives: origDirectives,
		FederationMetaData: FederationMetaData{
			Keys:                         origFed.Keys,
			Requires:                     origFed.Requires,
			Provides:                     origFed.Provides,
			EntityInterfaces:             origFed.EntityInterfaces,
			InterfaceObjects:             origFed.InterfaceObjects,
			EntityCaching:                origFed.EntityCaching,
			RootFieldCaching:             origFed.RootFieldCaching,
			SubscriptionEntityPopulation: origFed.SubscriptionEntityPopulation,
		},
	}
}
