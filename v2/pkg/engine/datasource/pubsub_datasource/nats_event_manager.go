package pubsub_datasource

import (
	"fmt"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type NatsSubscriptionEventConfiguration struct {
	ProviderID          string                   `json:"providerId"`
	Subjects            []string                 `json:"subjects"`
	StreamConfiguration *NatsStreamConfiguration `json:"streamConfiguration,omitempty"`
}

type NatsPublishAndRequestEventConfiguration struct {
	ProviderID string `json:"providerId"`
	Subject    string `json:"subject"`
	Data       []byte `json:"data"`
}

func (s *NatsPublishAndRequestEventConfiguration) MarshalJSONTemplate() string {
	return fmt.Sprintf(`{"subject":"%s", "data": %s, "providerId":"%s"}`, s.Subject, s.Data, s.ProviderID)
}

type NatsEventManager struct {
	visitor                             *plan.Visitor
	variables                           *resolve.Variables
	eventMetadata                       EventMetadata
	eventConfiguration                  *NatsEventConfiguration
	publishAndRequestEventConfiguration *NatsPublishAndRequestEventConfiguration
	subscriptionEventConfiguration      *NatsSubscriptionEventConfiguration
}

func (p *NatsEventManager) extractEventSubject(ref int, subject string) (string, error) {
	matches := eventSubjectRegex.FindAllStringSubmatch(subject, -1)
	if len(matches) != 1 || len(matches[0]) != 2 {
		return "", fmt.Errorf("expected subject to match regex")
	}
	argumentName := matches[0][1]
	// We need to find the argument in the operation
	argumentRef, ok := p.visitor.Operation.FieldArgument(ref, []byte(argumentName))
	if !ok {
		return "", fmt.Errorf("argument \"%s\" is not defined", argumentName)
	}
	argumentValue := p.visitor.Operation.ArgumentValue(argumentRef)
	if argumentValue.Kind != ast.ValueKindVariable {
		return "", fmt.Errorf("expected argument \"%s\" kind to be \"ValueKindVariable\" but received \"%s\"", argumentName, argumentValue.Kind)
	}
	variableName := p.visitor.Operation.VariableValueNameBytes(argumentValue.Ref)
	variableDefinition, ok := p.visitor.Operation.VariableDefinitionByNameAndOperation(p.visitor.Walker.Ancestors[0].Ref, variableName)
	if !ok {
		return "", fmt.Errorf("expected definition to exist for variable \"%s\"", variableName)
	}
	variableTypeRef := p.visitor.Operation.VariableDefinitions[variableDefinition].Type
	renderer, err := resolve.NewPlainVariableRendererWithValidationFromTypeRef(p.visitor.Operation, p.visitor.Definition, variableTypeRef, string(variableName))
	if err != nil {
		return "", err
	}
	contextVariable := &resolve.ContextVariable{
		Path:     []string{string(variableName)},
		Renderer: renderer,
	}
	// We need to replace the template literal with the variable placeholder (and reuse if it already exists)
	variablePlaceHolder, _ := p.variables.AddVariable(contextVariable) // $$0$$
	return eventSubjectRegex.ReplaceAllLiteralString(subject, variablePlaceHolder), nil
}

func (p *NatsEventManager) eventDataBytes(ref int) ([]byte, error) {
	return buildEventDataBytes(ref, p.visitor, p.variables)
}

func (p *NatsEventManager) handlePublishAndRequestEvent(ref int) {
	if len(p.eventConfiguration.Subjects) != 1 {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("publish and request events should define one subject but received %d", len(p.eventConfiguration.Subjects)))
		return
	}
	rawSubject := p.eventConfiguration.Subjects[0]
	extractedSubject, err := p.extractEventSubject(ref, rawSubject)
	if err != nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("could not extract event subject: %w", err))
		return
	}
	dataBytes, err := p.eventDataBytes(ref)
	if err != nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to write event data bytes: %w", err))
		return
	}

	p.publishAndRequestEventConfiguration = &NatsPublishAndRequestEventConfiguration{
		ProviderID: p.eventMetadata.ProviderID,
		Subject:    extractedSubject,
		Data:       dataBytes,
	}
}

func (p *NatsEventManager) handleSubscriptionEvent(ref int) {

	if len(p.eventConfiguration.Subjects) == 0 {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("expected at least one subscription subject but received %d", len(p.eventConfiguration.Subjects)))
		return
	}
	extractedSubjects := make([]string, 0, len(p.eventConfiguration.Subjects))
	for _, rawSubject := range p.eventConfiguration.Subjects {
		extractedSubject, err := p.extractEventSubject(ref, rawSubject)
		if err != nil {
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("could not extract subscription event subjects: %w", err))
			return
		}
		extractedSubjects = append(extractedSubjects, extractedSubject)
	}

	p.subscriptionEventConfiguration = &NatsSubscriptionEventConfiguration{
		ProviderID:          p.eventMetadata.ProviderID,
		Subjects:            extractedSubjects,
		StreamConfiguration: p.eventConfiguration.StreamConfiguration,
	}
}
