package resolve

import (
	"fmt"

	"github.com/cespare/xxhash/v2"

	"github.com/wundergraph/astjson"
)

// FieldAuthorization owns the per-request field-authorization decisions: it produces them
// (up-front batch in pre-fetch mode, memoized AuthorizeObjectField calls in the default mode)
// and answers lookups from the loader (fetch pruning) and the resolvable (field nulling).
// It is request-scoped: created next to the Resolvable in the resolver entry points, holding
// the request Context; it must never be stored on the Resolver.
type FieldAuthorization struct {
	ctx *Context

	// allow caches allowed authorization decision ids (keyed by authorizationDecisionID over
	// data source id + graph coordinate).
	allow map[uint64]struct{}

	// deny caches denied authorization decision ids mapped to their deny reason.
	deny map[uint64]string

	// marshalBuf is a scratch buffer for marshaling the object value passed to
	// AuthorizeObjectField (legacy mode). Owned here, not shared with Resolvable's render buffer.
	marshalBuf []byte
}

func NewFieldAuthorization(ctx *Context) *FieldAuthorization {
	return &FieldAuthorization{
		ctx:   ctx,
		allow: make(map[uint64]struct{}),
		deny:  make(map[uint64]string),
	}
}

// preFetchEnabled reports whether pre-fetch field authorization is active for this request.
func (a *FieldAuthorization) preFetchEnabled() bool {
	return a.ctx.preFetchFieldAuthorizer != nil
}

// authorizePreFetch resolves every protected field coordinate of the operation in one batch
// call and seeds the decision cache, before any fetch executes. It is a no-op when pre-fetch
// field authorization is disabled or the operation selects no protected field.
func (a *FieldAuthorization) authorizePreFetch(response *GraphQLResponse) error {
	if a.ctx.preFetchFieldAuthorizer == nil || response == nil || response.Info == nil || len(response.Info.AuthorizationCoordinates) == 0 {
		return nil
	}

	coordinateIndex := make(map[GraphCoordinate]int, len(response.Info.AuthorizationCoordinates))
	coordinates := make([]GraphCoordinate, 0, len(response.Info.AuthorizationCoordinates))
	for i := range response.Info.AuthorizationCoordinates {
		coordinate := response.Info.AuthorizationCoordinates[i].Coordinate
		if _, exists := coordinateIndex[coordinate]; exists {
			continue
		}
		coordinateIndex[coordinate] = len(coordinates)
		coordinates = append(coordinates, coordinate)
	}
	decisions, err := a.ctx.preFetchFieldAuthorizer.AuthorizeFields(a.ctx, coordinates)
	if err != nil {
		return err
	}
	if len(decisions) != len(coordinates) {
		return fmt.Errorf("batch authorizer returned %d decisions for %d coordinates", len(decisions), len(coordinates))
	}
	for i := range response.Info.AuthorizationCoordinates {
		authCoordinate := response.Info.AuthorizationCoordinates[i]
		decision := decisions[coordinateIndex[authCoordinate.Coordinate]]
		if decision.Allowed {
			a.seedAllow(authCoordinate.DataSourceID, authCoordinate.Coordinate)
		} else {
			a.seedDeny(authCoordinate.DataSourceID, authCoordinate.Coordinate, decision.Reason)
		}
	}
	return nil
}

// decide returns the allow/deny decision for a field coordinate during response resolution,
// memoizing the result in the allow/deny cache.
//
// This cache is also what prevents a field from being authorized twice under pre-fetch field
// authorization. When that mode is enabled the batch authorizer decides every selected protected
// coordinate up front and seeds the cache (see authorizePreFetch), so the lookups below always
// hit and AuthorizeObjectField — the data-aware, post-fetch authorizer call — is never reached.
// AuthorizeObjectField therefore runs only in the default (disabled) mode, where no decisions
// are seeded and each coordinate is decided here on first encounter.
func (a *FieldAuthorization) decide(value *astjson.Value, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
	decisionID := authorizationDecisionID(dataSourceID, coordinate)
	// Seeded (pre-fetch) or previously computed (post-fetch) decisions short-circuit here, so the
	// post-fetch AuthorizeObjectField call below is skipped whenever a decision already exists.
	if _, ok := a.allow[decisionID]; ok {
		return nil, nil
	}
	if reason, ok := a.deny[decisionID]; ok {
		return &AuthorizationDeny{Reason: reason}, nil
	}
	if a.ctx.authorizer == nil {
		// Pre-fetch field authorization without a post-fetch authorizer: the only decisions are those
		// seeded up front. A coordinate without a seeded decision is treated as authorized.
		return nil, nil
	}
	a.marshalBuf = value.MarshalTo(a.marshalBuf[:0])
	result, err = a.ctx.authorizer.AuthorizeObjectField(a.ctx, dataSourceID, a.marshalBuf, coordinate)
	if err != nil {
		return nil, err
	}
	if result == nil {
		a.allow[decisionID] = struct{}{}
	} else {
		a.deny[decisionID] = result.Reason
	}
	return result, nil
}

func authorizationDecisionID(dataSourceID string, coordinate GraphCoordinate) uint64 {
	// NUL delimiters keep the key unambiguous: without them distinct tuples like ("ab","c","d") and
	// ("a","bc","d") would hash the same input and could reuse a decision for the wrong coordinate.
	return xxhash.Sum64String(dataSourceID + "\x00" + coordinate.TypeName + "\x00" + coordinate.FieldName)
}

func (a *FieldAuthorization) seedAllow(dataSourceID string, coordinate GraphCoordinate) {
	a.allow[authorizationDecisionID(dataSourceID, coordinate)] = struct{}{}
}

func (a *FieldAuthorization) seedDeny(dataSourceID string, coordinate GraphCoordinate, reason string) {
	a.deny[authorizationDecisionID(dataSourceID, coordinate)] = reason
}

func (a *FieldAuthorization) denyReason(dataSourceID string, coordinate GraphCoordinate) (string, bool) {
	reason, ok := a.deny[authorizationDecisionID(dataSourceID, coordinate)]
	return reason, ok
}
