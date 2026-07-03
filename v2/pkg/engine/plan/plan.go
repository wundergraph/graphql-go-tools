package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type Kind int

const (
	SynchronousResponseKind Kind = iota + 1
	SubscriptionResponseKind
	DeferResponsePlanKind
)

type Plan interface {
	PlanKind() Kind
	SetFlushInterval(interval int64)
	GetCostCalculator() *CostCalculator
	SetCostCalculator(calc *CostCalculator)
	// CollectAuthorizationCoordinates populates the plan's response with the field coordinates that
	// require an authorization decision, so pre-fetch field authorization can resolve them up front.
	CollectAuthorizationCoordinates()
}

type SynchronousResponsePlan struct {
	Response       *resolve.GraphQLResponse
	FlushInterval  int64
	CostCalculator *CostCalculator
}

func (s *SynchronousResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (*SynchronousResponsePlan) PlanKind() Kind {
	return SynchronousResponseKind
}

func (s *SynchronousResponsePlan) GetCostCalculator() *CostCalculator {
	return s.CostCalculator
}

func (s *SynchronousResponsePlan) SetCostCalculator(c *CostCalculator) {
	s.CostCalculator = c
}

func (s *SynchronousResponsePlan) CollectAuthorizationCoordinates() {
	resolve.CollectAuthorizationCoordinates(s.Response)
}

type SubscriptionResponsePlan struct {
	Response       *resolve.GraphQLSubscription
	FlushInterval  int64
	CostCalculator *CostCalculator
}

func (s *SubscriptionResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (*SubscriptionResponsePlan) PlanKind() Kind {
	return SubscriptionResponseKind
}

func (s *SubscriptionResponsePlan) GetCostCalculator() *CostCalculator {
	return s.CostCalculator
}

func (s *SubscriptionResponsePlan) SetCostCalculator(c *CostCalculator) {
	s.CostCalculator = c
}

func (s *SubscriptionResponsePlan) CollectAuthorizationCoordinates() {
	if s.Response == nil {
		return
	}
	resolve.CollectAuthorizationCoordinates(s.Response.Response)
}

type DeferResponsePlan struct {
	Response       *resolve.GraphQLDeferResponse
	FlushInterval  int64
	CostCalculator *CostCalculator
}

func (d *DeferResponsePlan) PlanKind() Kind {
	return DeferResponsePlanKind
}

func (d *DeferResponsePlan) SetFlushInterval(interval int64) {
	d.FlushInterval = interval
}

func (d *DeferResponsePlan) GetCostCalculator() *CostCalculator {
	return d.CostCalculator
}

func (d *DeferResponsePlan) SetCostCalculator(c *CostCalculator) {
	d.CostCalculator = c
}

func (d *DeferResponsePlan) CollectAuthorizationCoordinates() {
	if d.Response == nil {
		return
	}
	resolve.CollectAuthorizationCoordinates(d.Response.Response)
}
