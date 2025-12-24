package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type Kind int

const (
	SynchronousResponseKind Kind = iota + 1
	SubscriptionResponseKind
)

type Plan interface {
	PlanKind() Kind
	SetFlushInterval(interval int64)
	GetStaticCost() int
	SetStaticCost(cost int)
}

type SynchronousResponsePlan struct {
	Response      *resolve.GraphQLResponse
	FlushInterval int64
	StaticCost    int
}

func (s *SynchronousResponsePlan) GetStaticCost() int {
	return s.StaticCost
}

func (s *SynchronousResponsePlan) SetStaticCost(cost int) {
	s.StaticCost = cost
}

func (s *SynchronousResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (*SynchronousResponsePlan) PlanKind() Kind {
	return SynchronousResponseKind
}

type SubscriptionResponsePlan struct {
	Response      *resolve.GraphQLSubscription
	FlushInterval int64
	StaticCost    int
}

func (s *SubscriptionResponsePlan) GetStaticCost() int {
	return s.StaticCost
}

func (s *SubscriptionResponsePlan) SetStaticCost(cost int) {
	s.StaticCost = cost
}

func (s *SubscriptionResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (*SubscriptionResponsePlan) PlanKind() Kind {
	return SubscriptionResponseKind
}
