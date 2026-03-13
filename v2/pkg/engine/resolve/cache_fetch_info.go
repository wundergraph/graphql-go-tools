package resolve

import "context"

// CacheFetchInfo describes which fetch triggered a cache operation.
// It is set on context.Context when Debug mode is enabled, allowing
// cache implementations to identify the source of each Get/Set/Delete call.
type CacheFetchInfo struct {
	DataSourceName string // e.g., "accounts"
	DataSourceID   string
	FetchType      string // "entity" or "rootField"
	TypeName       string // Entity type ("User") or root type ("Query")
	FieldName      string // Root field name ("topProducts"); empty for entity fetches
}

// String returns a concise fetch identifier like "accounts: entity(User)"
// or "products: rootField(Query.topProducts)".
func (c *CacheFetchInfo) String() string {
	if c == nil {
		return ""
	}
	if c.FetchType == "rootField" {
		return c.DataSourceName + ": rootField(" + c.TypeName + "." + c.FieldName + ")"
	}
	return c.DataSourceName + ": entity(" + c.TypeName + ")"
}

type cacheFetchInfoKeyType struct{}

// WithCacheFetchInfo returns a new context with CacheFetchInfo derived from the given FetchInfo and FetchCacheConfiguration.
func WithCacheFetchInfo(ctx context.Context, info *FetchInfo, cfg FetchCacheConfiguration) context.Context {
	if info == nil {
		return ctx
	}

	cfi := &CacheFetchInfo{
		DataSourceName: info.DataSourceName,
		DataSourceID:   info.DataSourceID,
	}

	switch cfg.CacheKeyTemplate.(type) {
	case *EntityQueryCacheKeyTemplate:
		cfi.FetchType = "entity"
		if len(info.RootFields) > 0 {
			cfi.TypeName = info.RootFields[0].TypeName
		}
	case *RootQueryCacheKeyTemplate:
		cfi.FetchType = "rootField"
		if len(info.RootFields) > 0 {
			cfi.TypeName = info.RootFields[0].TypeName
			cfi.FieldName = info.RootFields[0].FieldName
		}
	}

	return context.WithValue(ctx, cacheFetchInfoKeyType{}, cfi)
}

// GetCacheFetchInfo retrieves the CacheFetchInfo from a context, or nil if not set.
func GetCacheFetchInfo(ctx context.Context) *CacheFetchInfo {
	cfi, _ := ctx.Value(cacheFetchInfoKeyType{}).(*CacheFetchInfo)
	return cfi
}
