package entitycaching

import (
	"net/http"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
)

func TTL(headers http.Header, defaultTTL time.Duration) (time.Duration, bool) {
	cc, err := cache.ParseCacheControlResponse(headers)
	if err != nil {
		return 0, false
	}

	if cc.NoStore {
		return 0, false
	}

	// We for now consider no cache and private as dont cache even
	if cc.NoCache != nil || cc.Private != nil {
		return 0, false
	}
	if !cc.Public {
		return 0, false
	}

	switch {
	case cc.SMaxAge != nil:
		if *cc.SMaxAge <= 0 {
			return 0, false
		}
		return cc.SMaxAge.AsDuration(), true

	case cc.MaxAge != nil:
		if *cc.MaxAge <= 0 {
			return 0, false
		}
		return cc.MaxAge.AsDuration(), true
	}

	if defaultTTL <= 0 {
		return 0, false
	}
	return defaultTTL, true
}
