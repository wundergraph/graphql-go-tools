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
	GetCostCalculator() *CostCalculator
	SetCostCalculator(calc *CostCalculator)
}

type SynchronousResponsePlan struct {
	Response             *resolve.GraphQLResponse
	FlushInterval        int64
	StaticCostCalculator *CostCalculator
}

func (s *SynchronousResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (*SynchronousResponsePlan) PlanKind() Kind {
	return SynchronousResponseKind
}

func (s *SynchronousResponsePlan) GetCostCalculator() *CostCalculator {
	return s.StaticCostCalculator
}

func (s *SynchronousResponsePlan) SetCostCalculator(c *CostCalculator) {
	s.StaticCostCalculator = c
}

type SubscriptionResponsePlan struct {
	Response             *resolve.GraphQLSubscription
	FlushInterval        int64
	StaticCostCalculator *CostCalculator
}

func (s *SubscriptionResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (*SubscriptionResponsePlan) PlanKind() Kind {
	return SubscriptionResponseKind
}

func (s *SubscriptionResponsePlan) GetCostCalculator() *CostCalculator {
	return s.StaticCostCalculator
}

func (s *SubscriptionResponsePlan) SetCostCalculator(c *CostCalculator) {
	s.StaticCostCalculator = c
}
