package plan

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// splitDSSnapshot captures the essential state of a split datasource for full assertion.
// Uses plain types instead of plan types to avoid unexported field comparison issues
// (e.g. FederationFieldConfiguration.parsedSelectionSet).
type splitDSSnapshot struct {
	ID               string
	Name             string
	RootNodes        []snapshotTypeField
	ChildNodes       []snapshotTypeField
	Keys             []snapshotKey
	EntityCaching    []snapshotEntityCache
	RootFieldCaching []snapshotRootFieldCache
}

type snapshotTypeField struct {
	TypeName   string
	FieldNames []string
}

type snapshotKey struct {
	TypeName     string
	SelectionSet string
}

type snapshotEntityCache struct {
	TypeName  string
	CacheName string
	TTL       time.Duration
}

type snapshotRootFieldCache struct {
	TypeName  string
	FieldName string
	CacheName string
	TTL       time.Duration
}

// snapshotDS extracts a splitDSSnapshot from a datasource for comparison.
func snapshotDS(t *testing.T, ds DataSource) splitDSSnapshot {
	t.Helper()
	na, ok := ds.(NodesAccess)
	require.True(t, ok)
	fed := ds.FederationConfiguration()

	var rootNodes []snapshotTypeField
	for _, n := range na.ListRootNodes() {
		rootNodes = append(rootNodes, snapshotTypeField{TypeName: n.TypeName, FieldNames: n.FieldNames})
	}
	var childNodes []snapshotTypeField
	for _, n := range na.ListChildNodes() {
		childNodes = append(childNodes, snapshotTypeField{TypeName: n.TypeName, FieldNames: n.FieldNames})
	}
	var keys []snapshotKey
	for _, k := range fed.Keys {
		keys = append(keys, snapshotKey{TypeName: k.TypeName, SelectionSet: k.SelectionSet})
	}
	var entityCaching []snapshotEntityCache
	for _, e := range fed.EntityCaching {
		entityCaching = append(entityCaching, snapshotEntityCache{TypeName: e.TypeName, CacheName: e.CacheName, TTL: e.TTL})
	}
	var rootFieldCaching []snapshotRootFieldCache
	for _, r := range fed.RootFieldCaching {
		rootFieldCaching = append(rootFieldCaching, snapshotRootFieldCache{TypeName: r.TypeName, FieldName: r.FieldName, CacheName: r.CacheName, TTL: r.TTL})
	}

	return splitDSSnapshot{
		ID:               ds.Id(),
		Name:             ds.Name(),
		RootNodes:        rootNodes,
		ChildNodes:       childNodes,
		Keys:             keys,
		EntityCaching:    entityCaching,
		RootFieldCaching: rootFieldCaching,
	}
}

// snapshotAll extracts snapshots from all datasources, keyed by ID.
func snapshotAll(t *testing.T, dss []DataSource) map[string]splitDSSnapshot {
	t.Helper()
	result := make(map[string]splitDSSnapshot, len(dss))
	for _, ds := range dss {
		result[ds.Id()] = snapshotDS(t, ds)
	}
	return result
}

// splitTestDS creates a datasource for split testing using the dsBuilder.
// Schema is required so that cloneForSplit has a valid factory.
func splitTestDS(id, name string) *dsBuilder {
	return dsb().Id(id).Name(name).Schema("type Query { placeholder: String }")
}

func TestSplitDataSourcesByRootFieldCaching(t *testing.T) {
	t.Run("no RootFieldCaching - no split", func(t *testing.T) {
		ds := splitTestDS("0", "accounts").
			RootNode("Query", "me", "cat").
			DS()

		result, err := SplitDataSourcesByRootFieldCaching([]DataSource{ds})
		require.NoError(t, err)
		require.Equal(t, 1, len(result))
		assert.Equal(t, "0", result[0].Id(), "original DS returned unchanged")
	})

	t.Run("two cached root fields - 2 separate datasources", func(t *testing.T) {
		ds := splitTestDS("0", "accounts").
			RootNode("Query", "me", "cat").
			RootNode("User", "id", "username").
			ChildNode("Cat", "name").
			KeysMetadata(FederationFieldConfigurations{
				{TypeName: "User", SelectionSet: "id"},
			}).
			WithMetadata(func(data *FederationMetaData) {
				data.RootFieldCaching = RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 30 * time.Second},
					{TypeName: "Query", FieldName: "cat", CacheName: "default", TTL: 60 * time.Second},
				}
			}).
			DS()

		result, err := SplitDataSourcesByRootFieldCaching([]DataSource{ds})
		require.NoError(t, err)
		require.Equal(t, 2, len(result))

		snapshots := snapshotAll(t, result)
		assert.Equal(t, splitDSSnapshot{
			ID:   "0_rf_me",
			Name: "accounts",
			RootNodes: []snapshotTypeField{
				{TypeName: "Query", FieldNames: []string{"me"}},
				{TypeName: "User", FieldNames: []string{"id", "username"}},
			},
			ChildNodes: []snapshotTypeField{
				{TypeName: "Cat", FieldNames: []string{"name"}},
			},
			Keys: []snapshotKey{
				{TypeName: "User", SelectionSet: "id"},
			},
			RootFieldCaching: []snapshotRootFieldCache{
				{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 30 * time.Second},
			},
		}, snapshots["0_rf_me"])

		assert.Equal(t, splitDSSnapshot{
			ID:   "0_rf_cat",
			Name: "accounts",
			RootNodes: []snapshotTypeField{
				{TypeName: "Query", FieldNames: []string{"cat"}},
				{TypeName: "User", FieldNames: []string{"id", "username"}},
			},
			ChildNodes: []snapshotTypeField{
				{TypeName: "Cat", FieldNames: []string{"name"}},
			},
			Keys: []snapshotKey{
				{TypeName: "User", SelectionSet: "id"},
			},
			RootFieldCaching: []snapshotRootFieldCache{
				{TypeName: "Query", FieldName: "cat", CacheName: "default", TTL: 60 * time.Second},
			},
		}, snapshots["0_rf_cat"])
	})

	t.Run("3 root fields, 2 cached, 1 uncached - 3 datasources", func(t *testing.T) {
		ds := splitTestDS("0", "accounts").
			RootNode("Query", "me", "cat", "user").
			WithMetadata(func(data *FederationMetaData) {
				data.RootFieldCaching = RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 10 * time.Second},
					{TypeName: "Query", FieldName: "cat", CacheName: "default", TTL: 60 * time.Second},
				}
			}).
			DS()

		result, err := SplitDataSourcesByRootFieldCaching([]DataSource{ds})
		require.NoError(t, err)
		require.Equal(t, 3, len(result))

		snapshots := snapshotAll(t, result)
		assert.Equal(t, splitDSSnapshot{
			ID:   "0_rf_me",
			Name: "accounts",
			RootNodes: []snapshotTypeField{
				{TypeName: "Query", FieldNames: []string{"me"}},
			},
			RootFieldCaching: []snapshotRootFieldCache{
				{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 10 * time.Second},
			},
		}, snapshots["0_rf_me"])

		assert.Equal(t, splitDSSnapshot{
			ID:   "0_rf_cat",
			Name: "accounts",
			RootNodes: []snapshotTypeField{
				{TypeName: "Query", FieldNames: []string{"cat"}},
			},
			RootFieldCaching: []snapshotRootFieldCache{
				{TypeName: "Query", FieldName: "cat", CacheName: "default", TTL: 60 * time.Second},
			},
		}, snapshots["0_rf_cat"])

		assert.Equal(t, splitDSSnapshot{
			ID:   "0",
			Name: "accounts",
			RootNodes: []snapshotTypeField{
				{TypeName: "Query", FieldNames: []string{"user"}},
			},
		}, snapshots["0"])
	})

	t.Run("entity-only caching, no root field caching - no split", func(t *testing.T) {
		ds := splitTestDS("0", "accounts").
			RootNode("Query", "me", "cat").
			RootNode("User", "id", "username").
			KeysMetadata(FederationFieldConfigurations{
				{TypeName: "User", SelectionSet: "id"},
			}).
			WithMetadata(func(data *FederationMetaData) {
				data.EntityCaching = EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				}
			}).
			DS()

		result, err := SplitDataSourcesByRootFieldCaching([]DataSource{ds})
		require.NoError(t, err)
		require.Equal(t, 1, len(result))
		assert.Equal(t, "0", result[0].Id(), "original DS returned unchanged")
	})

	t.Run("single cached root field, no uncached - no split", func(t *testing.T) {
		ds := splitTestDS("0", "accounts").
			RootNode("Query", "me").
			WithMetadata(func(data *FederationMetaData) {
				data.RootFieldCaching = RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 30 * time.Second},
				}
			}).
			DS()

		result, err := SplitDataSourcesByRootFieldCaching([]DataSource{ds})
		require.NoError(t, err)
		require.Equal(t, 1, len(result))
		assert.Equal(t, "0", result[0].Id(), "original DS returned unchanged")
	})

	t.Run("single cached + uncached fields - 2 datasources", func(t *testing.T) {
		ds := splitTestDS("0", "accounts").
			RootNode("Query", "me", "cat").
			WithMetadata(func(data *FederationMetaData) {
				data.RootFieldCaching = RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 30 * time.Second},
				}
			}).
			DS()

		result, err := SplitDataSourcesByRootFieldCaching([]DataSource{ds})
		require.NoError(t, err)
		require.Equal(t, 2, len(result))

		snapshots := snapshotAll(t, result)
		assert.Equal(t, splitDSSnapshot{
			ID:   "0_rf_me",
			Name: "accounts",
			RootNodes: []snapshotTypeField{
				{TypeName: "Query", FieldNames: []string{"me"}},
			},
			RootFieldCaching: []snapshotRootFieldCache{
				{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 30 * time.Second},
			},
		}, snapshots["0_rf_me"])

		assert.Equal(t, splitDSSnapshot{
			ID:   "0",
			Name: "accounts",
			RootNodes: []snapshotTypeField{
				{TypeName: "Query", FieldNames: []string{"cat"}},
			},
		}, snapshots["0"])
	})

	t.Run("entity caching preserved on all split datasources", func(t *testing.T) {
		ds := splitTestDS("0", "accounts").
			RootNode("Query", "me", "cat").
			RootNode("User", "id", "username").
			KeysMetadata(FederationFieldConfigurations{
				{TypeName: "User", SelectionSet: "id"},
			}).
			WithMetadata(func(data *FederationMetaData) {
				data.EntityCaching = EntityCacheConfigurations{
					{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
				}
				data.RootFieldCaching = RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 10 * time.Second},
					{TypeName: "Query", FieldName: "cat", CacheName: "default", TTL: 60 * time.Second},
				}
			}).
			DS()

		result, err := SplitDataSourcesByRootFieldCaching([]DataSource{ds})
		require.NoError(t, err)
		require.Equal(t, 2, len(result))

		snapshots := snapshotAll(t, result)
		assert.Equal(t, splitDSSnapshot{
			ID:   "0_rf_me",
			Name: "accounts",
			RootNodes: []snapshotTypeField{
				{TypeName: "Query", FieldNames: []string{"me"}},
				{TypeName: "User", FieldNames: []string{"id", "username"}},
			},
			Keys: []snapshotKey{
				{TypeName: "User", SelectionSet: "id"},
			},
			EntityCaching: []snapshotEntityCache{
				{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
			},
			RootFieldCaching: []snapshotRootFieldCache{
				{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 10 * time.Second},
			},
		}, snapshots["0_rf_me"])

		assert.Equal(t, splitDSSnapshot{
			ID:   "0_rf_cat",
			Name: "accounts",
			RootNodes: []snapshotTypeField{
				{TypeName: "Query", FieldNames: []string{"cat"}},
				{TypeName: "User", FieldNames: []string{"id", "username"}},
			},
			Keys: []snapshotKey{
				{TypeName: "User", SelectionSet: "id"},
			},
			EntityCaching: []snapshotEntityCache{
				{TypeName: "User", CacheName: "default", TTL: 30 * time.Second},
			},
			RootFieldCaching: []snapshotRootFieldCache{
				{TypeName: "Query", FieldName: "cat", CacheName: "default", TTL: 60 * time.Second},
			},
		}, snapshots["0_rf_cat"])
	})

	t.Run("non-Query root nodes preserved on all split datasources", func(t *testing.T) {
		ds := splitTestDS("0", "accounts").
			RootNode("Query", "me", "cat").
			RootNode("User", "id", "username").
			RootNode("Mutation", "updateUser").
			KeysMetadata(FederationFieldConfigurations{
				{TypeName: "User", SelectionSet: "id"},
			}).
			WithMetadata(func(data *FederationMetaData) {
				data.RootFieldCaching = RootFieldCacheConfigurations{
					{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 30 * time.Second},
					{TypeName: "Query", FieldName: "cat", CacheName: "default", TTL: 30 * time.Second},
				}
			}).
			DS()

		result, err := SplitDataSourcesByRootFieldCaching([]DataSource{ds})
		require.NoError(t, err)
		require.Equal(t, 2, len(result))

		snapshots := snapshotAll(t, result)
		assert.Equal(t, splitDSSnapshot{
			ID:   "0_rf_me",
			Name: "accounts",
			RootNodes: []snapshotTypeField{
				{TypeName: "Query", FieldNames: []string{"me"}},
				{TypeName: "User", FieldNames: []string{"id", "username"}},
				{TypeName: "Mutation", FieldNames: []string{"updateUser"}},
			},
			Keys: []snapshotKey{
				{TypeName: "User", SelectionSet: "id"},
			},
			RootFieldCaching: []snapshotRootFieldCache{
				{TypeName: "Query", FieldName: "me", CacheName: "default", TTL: 30 * time.Second},
			},
		}, snapshots["0_rf_me"])

		assert.Equal(t, splitDSSnapshot{
			ID:   "0_rf_cat",
			Name: "accounts",
			RootNodes: []snapshotTypeField{
				{TypeName: "Query", FieldNames: []string{"cat"}},
				{TypeName: "User", FieldNames: []string{"id", "username"}},
				{TypeName: "Mutation", FieldNames: []string{"updateUser"}},
			},
			Keys: []snapshotKey{
				{TypeName: "User", SelectionSet: "id"},
			},
			RootFieldCaching: []snapshotRootFieldCache{
				{TypeName: "Query", FieldName: "cat", CacheName: "default", TTL: 30 * time.Second},
			},
		}, snapshots["0_rf_cat"])
	})
}
