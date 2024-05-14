package pubsub_datasource

import (
	"encoding/json"
	"fmt"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"slices"
)

type KafkaSubscriptionEventConfiguration struct {
	ProviderID string   `json:"providerId"`
	Topics     []string `json:"topics"`
}

type KafkaPublishEventConfiguration struct {
	ProviderID string          `json:"providerId"`
	Topic      string          `json:"topic"`
	Data       json.RawMessage `json:"data"`
}

func (s *KafkaPublishEventConfiguration) MarshalJSONTemplate() string {
	return fmt.Sprintf(`{"topic":"%s", "data": %s, "providerId":"%s"}`, s.Topic, s.Data, s.ProviderID)
}

type KafkaEventManager struct {
	visitor                        *plan.Visitor
	variables                      *resolve.Variables
	eventMetadata                  EventMetadata
	eventConfiguration             *KafkaEventConfiguration
	publishEventConfiguration      *KafkaPublishEventConfiguration
	subscriptionEventConfiguration *KafkaSubscriptionEventConfiguration
}

func (p *KafkaEventManager) eventDataBytes(ref int) ([]byte, error) {
	return buildEventDataBytes(ref, p.visitor, p.variables)
}

func (p *KafkaEventManager) handlePublishEvent(ref int) {
	if len(p.eventConfiguration.Topics) != 1 {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("publish and request events should define one subject but received %d", len(p.eventConfiguration.Topics)))
		return
	}
	topic := p.eventConfiguration.Topics[0]
	dataBytes, err := p.eventDataBytes(ref)
	if err != nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to write event data bytes: %w", err))
		return
	}

	p.publishEventConfiguration = &KafkaPublishEventConfiguration{
		ProviderID: p.eventMetadata.ProviderID,
		Topic:      topic,
		Data:       dataBytes,
	}
}

func (p *KafkaEventManager) handleSubscriptionEvent(ref int) {

	if len(p.eventConfiguration.Topics) == 0 {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("expected at least one subscription topic but received %d", len(p.eventConfiguration.Topics)))
		return
	}

	slices.Sort(p.eventConfiguration.Topics)

	p.subscriptionEventConfiguration = &KafkaSubscriptionEventConfiguration{
		ProviderID: p.eventMetadata.ProviderID,
		Topics:     p.eventConfiguration.Topics,
	}
}
