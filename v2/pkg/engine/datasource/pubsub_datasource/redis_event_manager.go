package pubsub_datasource

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/argument_templates"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type RedisSubscriptionEventConfiguration struct {
	ProviderID string   `json:"providerId"`
	Channels   []string `json:"channels"`
}

type RedisPublishEventConfiguration struct {
	ProviderID string          `json:"providerId"`
	Channel    string          `json:"channel"`
	Data       json.RawMessage `json:"data"`
}

func (s *RedisPublishEventConfiguration) MarshalJSONTemplate() string {
	return fmt.Sprintf(`{"channel":"%s", "data": %s, "providerId":"%s"}`, s.Channel, s.Data, s.ProviderID)
}

type RedisEventManager struct {
	visitor                        *plan.Visitor
	variables                      *resolve.Variables
	eventMetadata                  EventMetadata
	eventConfiguration             *RedisEventConfiguration
	publishEventConfiguration      *RedisPublishEventConfiguration
	subscriptionEventConfiguration *RedisSubscriptionEventConfiguration
}

func isValidRedisChannel(channel string) bool {
	if channel == "" {
		return false
	}

	if strings.ContainsAny(channel, "\t\n\f\r ") {
		return false
	}

	return true
}

func (p *RedisEventManager) addContextVariableByArgumentRef(argumentRef int, argumentPath []string) (string, error) {
	variablePath, err := p.visitor.Operation.VariablePathByArgumentRefAndArgumentPath(argumentRef, argumentPath, p.visitor.Walker.Ancestors[0].Ref)
	if err != nil {
		return "", err
	}
	/* The definition is passed as both definition and operation below because getJSONRootType resolves the type
	 * from the first argument, but finalInputValueTypeRef comes from the definition
	 */
	contextVariable := &resolve.ContextVariable{
		Path:     variablePath,
		Renderer: resolve.NewPlainVariableRenderer(),
	}
	variablePlaceHolder, _ := p.variables.AddVariable(contextVariable)
	return variablePlaceHolder, nil
}

func (p *RedisEventManager) extractEventChannel(fieldRef int, channel string) (string, error) {
	matches := argument_templates.ArgumentTemplateRegex.FindAllStringSubmatch(channel, -1)
	// If no argument templates are defined, there are only static values
	if len(matches) < 1 {
		if isValidRedisChannel(channel) {
			return channel, nil
		}
		return "", fmt.Errorf(`channel "%s" is not a valid RedisPubSub PubSub channel`, channel)
	}
	fieldNameBytes := p.visitor.Operation.FieldNameBytes(fieldRef)
	// TODO: handling for interfaces and unions
	fieldDefinitionRef, ok := p.visitor.Definition.ObjectTypeDefinitionFieldWithName(p.visitor.Walker.EnclosingTypeDefinition.Ref, fieldNameBytes)
	if !ok {
		return "", fmt.Errorf(`expected field definition to exist for field "%s"`, fieldNameBytes)
	}
	channelWithVariableTemplateReplacements := channel
	for templateNumber, groups := range matches {
		// The first group is the whole template; the second is the period delimited argument path
		if len(groups) != 2 {
			return "", fmt.Errorf(`argument template #%d defined on field "%s" is invalid: expected 2 matching groups but received %d`, templateNumber+1, fieldNameBytes, len(groups)-1)
		}
		validationResult, err := argument_templates.ValidateArgumentPath(p.visitor.Definition, groups[1], fieldDefinitionRef)
		if err != nil {
			return "", fmt.Errorf(`argument template #%d defined on field "%s" is invalid: %w`, templateNumber+1, fieldNameBytes, err)
		}
		argumentNameBytes := []byte(validationResult.ArgumentPath[0])
		argumentRef, ok := p.visitor.Operation.FieldArgument(fieldRef, argumentNameBytes)
		if !ok {
			return "", fmt.Errorf(`operation field "%s" does not define argument "%s"`, fieldNameBytes, argumentNameBytes)
		}
		// variablePlaceholder has the form $$0$$, $$1$$, etc.
		variablePlaceholder, err := p.addContextVariableByArgumentRef(argumentRef, validationResult.ArgumentPath)
		if err != nil {
			return "", fmt.Errorf(`failed to retrieve variable placeholder for argument ""%s" defined on operation field "%s": %w`, argumentNameBytes, fieldNameBytes, err)
		}
		// Replace the template literal with the variable placeholder (and reuse the variable if it already exists)
		channelWithVariableTemplateReplacements = strings.ReplaceAll(channelWithVariableTemplateReplacements, groups[0], variablePlaceholder)
	}
	// Substitute the variable templates for dummy values to check naïvely that the string is a valid RedisPubSub PubSub channel
	if isValidRedisChannel(variableTemplateRegex.ReplaceAllLiteralString(channelWithVariableTemplateReplacements, "a")) {
		return channelWithVariableTemplateReplacements, nil
	}
	return "", fmt.Errorf(`channel "%s" is not a valid RedisPubSub PubSub channel`, channel)
}

func (p *RedisEventManager) eventDataBytes(ref int) ([]byte, error) {
	return buildEventDataBytes(ref, p.visitor, p.variables)
}

func (p *RedisEventManager) handlePublishEvent(ref int) {
	if len(p.eventConfiguration.Channels) != 1 {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("publish events should define one channel but received %d", len(p.eventConfiguration.Channels)))
		return
	}
	rawChannel := p.eventConfiguration.Channels[0]
	extractedChannel, err := p.extractEventChannel(ref, rawChannel)
	if err != nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("could not extract event channel: %w", err))
		return
	}
	dataBytes, err := p.eventDataBytes(ref)
	if err != nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to write event data bytes: %w", err))
		return
	}

	p.publishEventConfiguration = &RedisPublishEventConfiguration{
		ProviderID: p.eventMetadata.ProviderID,
		Channel:    extractedChannel,
		Data:       dataBytes,
	}
}

func (p *RedisEventManager) handleSubscriptionEvent(ref int) {

	if len(p.eventConfiguration.Channels) == 0 {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("expected at least one subscription channel but received %d", len(p.eventConfiguration.Channels)))
		return
	}
	extractedChannels := make([]string, 0, len(p.eventConfiguration.Channels))
	for _, rawChannel := range p.eventConfiguration.Channels {
		extractedChannel, err := p.extractEventChannel(ref, rawChannel)
		if err != nil {
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("could not extract subscription event subjects: %w", err))
			return
		}
		extractedChannels = append(extractedChannels, extractedChannel)
	}

	slices.Sort(extractedChannels)

	p.subscriptionEventConfiguration = &RedisSubscriptionEventConfiguration{
		ProviderID: p.eventMetadata.ProviderID,
		Channels:   extractedChannels,
	}
}
