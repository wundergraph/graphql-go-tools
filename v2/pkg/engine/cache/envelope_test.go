package cache

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestEnvelopeEncodeDecode pins the stored form byte for byte and the decode
// contract: an envelope round-trips, and anything that is not one is a MISS
// (ok=false), never an error.
func TestEnvelopeEncodeDecode(t *testing.T) {
	t.Run("a value round-trips through the envelope", func(t *testing.T) {
		encoded := encodeEnvelope(
			astjson.MustParseBytes([]byte(`{"__typename":"Product","name":"Table","price":100}`)),
			cacheControl{TTL: time.Minute, Created: time.Unix(1785852117, 0), Scope: cacheScopePublic},
		)
		assert.Equal(t,
			`{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":60,"created":1785852117,"scope":"public"}}`,
			string(encoded))

		tx := testTx()
		defer tx.Commit()
		envelope, ok := decodeEnvelope(tx, encoded)
		require.True(t, ok)
		assert.Equal(t, `{"__typename":"Product","name":"Table","price":100}`, string(envelope.Data.MarshalTo(nil)))
	})

	t.Run("the negative sentinel is a null data member", func(t *testing.T) {
		encoded := encodeEnvelope(
			nil,
			cacheControl{TTL: 5 * time.Second, Created: time.Unix(1785852117, 0), Scope: cacheScopePublic},
		)
		assert.Equal(t, `{"data":null,"cc":{"ttl":5,"created":1785852117,"scope":"public"}}`, string(encoded))

		tx := testTx()
		defer tx.Commit()
		envelope, ok := decodeEnvelope(tx, encoded)
		require.True(t, ok)
		assert.Equal(t, astjson.TypeNull, envelope.Data.Type())
	})

	t.Run("a private writer records the private scope, and the reader gets it back", func(t *testing.T) {
		encoded := encodeEnvelope(
			astjson.MustParseBytes([]byte(`{"name":"Table"}`)),
			cacheControl{TTL: time.Minute, Created: time.Unix(1785852117, 0), Scope: cacheScopePrivate},
		)
		assert.Equal(t, `{"data":{"name":"Table"},"cc":{"ttl":60,"created":1785852117,"scope":"private"}}`, string(encoded))

		tx := testTx()
		defer tx.Commit()
		envelope, ok := decodeEnvelope(tx, encoded)
		require.True(t, ok)
		assert.Equal(t, cacheScopePrivate, envelope.Scope)
	})

	t.Run("an entry that records no scope reads as public", func(t *testing.T) {
		tx := testTx()
		defer tx.Commit()
		rows := []struct {
			name string
			raw  string
		}{
			{name: "an empty scope", raw: `{"data":{"name":"Table"},"cc":{"ttl":60,"created":1785852117,"scope":""}}`},
			{name: "a cc without a scope member", raw: `{"data":{"name":"Table"},"cc":{"ttl":60,"created":1785852117}}`},
			{name: "no cc at all", raw: `{"data":{"name":"Table"}}`},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				envelope, ok := decodeEnvelope(tx, []byte(row.raw))
				require.True(t, ok)
				// Public is the reading that can only ever cost a hit: a private
				// entry lives under a key no public read derives.
				assert.Equal(t, cacheScopePublic, envelope.Scope)
			})
		}
	})

	t.Run("bytes that are not an envelope decode as a miss", func(t *testing.T) {
		tx := testTx()
		defer tx.Commit()
		rows := []struct {
			name string
			raw  string
		}{
			{name: "a bare entity value (the pre-envelope format)", raw: `{"__typename":"Product","name":"Table"}`},
			{name: "the pre-envelope negative sentinel", raw: `null`},
			{name: "truncated bytes", raw: `{"data":{"name":"Tab`},
			{name: "an array", raw: `[{"data":{"name":"Table"}}]`},
			{name: "an object without a data member", raw: `{"cc":{"ttl":60,"created":1785852117,"scope":"public"}}`},
			{name: "empty bytes", raw: ``},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				envelope, ok := decodeEnvelope(tx, []byte(row.raw))
				assert.False(t, ok)
				assert.Equal(t, storedEnvelope{}, envelope)
			})
		}
	})
}

// TestControllerEnvelopeReadRows drives the envelope through the controller:
// an undecodable entry is a plain miss that the fetch's own write replaces.
func TestControllerEnvelopeReadRows(t *testing.T) {
	t.Run("an undecodable entry is a miss and is rewritten", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := newTestStore()
			cfg := entityConfig(t, time.Minute)
			cfg.ProvidesData = &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name:        []byte("name"),
						Value:       &resolve.Scalar{Nullable: false, Path: []string{"name"}},
						OnTypeNames: [][]byte{[]byte("Product")},
					},
					{
						Name:        []byte("price"),
						Value:       &resolve.Scalar{Nullable: true, Path: []string{"price"}},
						OnTypeNames: [][]byte{[]byte("Product")},
					},
				},
			}
			resolve.ComputeHasAliases(cfg.ProvidesData)
			key := writeThrough(t, NewController(store, nil).BeginRequest(nil), cfg, productItem(t, "1"),
				`{"__typename":"Product","name":"Table","price":100}`)
			store.data[key] = testStoreEntry{
				value:     []byte(`{"data":{"__typename":"Product","name":"Tab`),
				expiresAt: time.Now().Add(time.Minute),
			}
			store.ops = nil // the corrupted request's ops assert in isolation

			rc := NewController(store, nil).BeginRequest(nil)
			item := productItem(t, "1")
			decision, handle := prepare(t, rc, cfg, item)
			assert.Equal(t, resolve.DecisionFetch, decision)
			assert.Nil(t, handle.Items[0].FromCache)

			require.NoError(t, rc.OnFetchResult(handle, resolve.MergeInput{
				Items:        []*astjson.Value{item},
				ResponseData: astjson.MustParseBytes([]byte(`{"__typename":"Product","name":"Table","price":100}`)),
				Arena:        beginner(),
			}))
			rc.EndRequest()
			assert.Equal(t, []testStoreOp{
				{
					Kind: "GetMany",
					Keys: []string{key},
					Hits: []bool{true},
				},
				{
					Kind: "SetMany",
					Items: []testStoreItem{
						{
							Key:   key,
							Value: `{"data":{"__typename":"Product","name":"Table","price":100},"cc":{"ttl":60,"created":946684800,"scope":"public"}}`,
							TTL:   time.Minute,
							Tags: []string{
								"subgraph:products",
								"type:products:Product",
								"entity:products:Product:d3cc039c7a9789e7", // upc "1"
							},
						},
					},
				},
			}, store.ops)
		})
	})
}
