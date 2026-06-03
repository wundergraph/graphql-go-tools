package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVisitor_ForceHashAnalyticsKeys verifies that Configuration.ForceHashAnalyticsKeys
// ORs onto the per-entity HashAnalyticsKeys setting and cannot be downgraded by the
// per-entity flag. This is the core invariant the global override exists to provide:
// router operators need to guarantee no raw entity key leaves the engine via analytics
// (PII / GDPR), regardless of what each subgraph SDL declares.
func TestVisitor_ForceHashAnalyticsKeys(t *testing.T) {
	cases := []struct {
		name          string
		perEntityHash bool
		forceHash     bool
		wantHashKeys  bool
	}{
		{"both off — raw keys", false, false, false},
		{"per-entity on, force off — per-entity wins (true)", true, false, true},
		{"per-entity off, force on — force overrides", false, true, true},
		{"both on — true", true, true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metadata := &DataSourceMetadata{
				FederationMetaData: FederationMetaData{
					Keys: FederationFieldConfigurations{
						{TypeName: "User", SelectionSet: "id"},
					},
					EntityCaching: EntityCacheConfigurations{
						{
							TypeName:          "User",
							CacheName:         "default",
							HashAnalyticsKeys: tc.perEntityHash,
						},
					},
				},
			}
			ds, err := NewDataSourceConfiguration[any]("ds-1", nil, metadata, nil)
			require.NoError(t, err)

			v := &Visitor{
				Config: Configuration{
					DataSources:            []DataSource{ds},
					ForceHashAnalyticsKeys: tc.forceHash,
				},
			}
			caching := newCachingPlannerState(v)

			analytics := caching.entityCacheAnalytics("User")
			require.NotNil(t, analytics, "User is an entity, analytics must be non-nil")
			assert.Equal(t, tc.wantHashKeys, analytics.HashKeys,
				"HashKeys = perEntity(%v) || force(%v)", tc.perEntityHash, tc.forceHash)
		})
	}

	t.Run("non-entity returns nil regardless of force flag", func(t *testing.T) {
		// ForceHashAnalyticsKeys must not cause non-entities to spuriously
		// report as entities — it only ORs the HashKeys field on entities
		// that already exist in the federation config.
		v := &Visitor{
			Config: Configuration{
				DataSources:            nil,
				ForceHashAnalyticsKeys: true,
			},
		}
		caching := newCachingPlannerState(v)
		assert.Nil(t, caching.entityCacheAnalytics("NotAnEntity"))
	})
}
