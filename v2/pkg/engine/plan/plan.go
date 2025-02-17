package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type Kind int

const (
	SynchronousResponseKind Kind = iota + 1
	SubscriptionResponseKind
	IncrementalResponseKind
)

type Plan interface {
	PlanKind() Kind
	SetFlushInterval(interval int64)
}

type SynchronousResponsePlan struct {
	Response      *resolve.GraphQLResponse
	FlushInterval int64
}

func (s *SynchronousResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (_ *SynchronousResponsePlan) PlanKind() Kind {
	return SynchronousResponseKind
}

type IncrementalResponsePlan struct {
	Response      *resolve.GraphQLIncrementalResponse
	FlushInterval int64
}

func (s *IncrementalResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (_ *IncrementalResponsePlan) PlanKind() Kind {
	return IncrementalResponseKind
}

type SubscriptionResponsePlan struct {
	Response      *resolve.GraphQLSubscription
	FlushInterval int64
}

func (s *SubscriptionResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (_ *SubscriptionResponsePlan) PlanKind() Kind {
	return SubscriptionResponseKind
}
