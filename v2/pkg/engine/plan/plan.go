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
	GetStaticCostCalculator() *CostCalculator
}

type SynchronousResponsePlan struct {
	Response             *resolve.GraphQLResponse
	FlushInterval        int64
	StaticCostCalculator *CostCalculator
}

func (s *SynchronousResponsePlan) GetStaticCost() int {
	return s.StaticCostCalculator.GetTotalCost()
}

func (s *SynchronousResponsePlan) GetStaticCostCalculator() *CostCalculator {
	return s.StaticCostCalculator
}

func (s *SynchronousResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (*SynchronousResponsePlan) PlanKind() Kind {
	return SynchronousResponseKind
}

type SubscriptionResponsePlan struct {
	Response             *resolve.GraphQLSubscription
	FlushInterval        int64
	StaticCostCalculator *CostCalculator
}

func (s *SubscriptionResponsePlan) GetStaticCostCalculator() *CostCalculator {
	return s.StaticCostCalculator
}

func (s *SubscriptionResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (*SubscriptionResponsePlan) PlanKind() Kind {
	return SubscriptionResponseKind
}
