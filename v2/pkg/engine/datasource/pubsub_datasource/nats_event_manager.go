package pubsub_datasource

import (
	"encoding/json"
	"fmt"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/argument_templates"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"regexp"
	"slices"
	"strings"
)

// A variable template has form $$number$$ where the number can range from one to multiple digits
var variableTemplateRegex = regexp.MustCompile(`\$\$\d+\$\$`)

type NatsSubscriptionEventConfiguration struct {
	ProviderID          string                   `json:"providerId"`
	Subjects            []string                 `json:"subjects"`
	StreamConfiguration *NatsStreamConfiguration `json:"streamConfiguration,omitempty"`
}

type NatsPublishAndRequestEventConfiguration struct {
	ProviderID string          `json:"providerId"`
	Subject    string          `json:"subject"`
	Data       json.RawMessage `json:"data"`
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

func (p *NatsEventManager) extractEventSubject(fieldRef int, subject string) (string, error) {
	matches := argument_templates.ArgumentTemplateRegex.FindAllStringSubmatch(subject, -1)
	// If no argument templates are defined, there are only static values
	if len(matches) < 1 {
		if natsserver.IsValidSubject(subject) {
			return subject, nil
		}
		return "", fmt.Errorf(`subject "%s" is not a valid NATS subject`, subject)
	}
	fieldNameBytes := p.visitor.Operation.FieldNameBytes(fieldRef)
	// TODO: handling for interfaces and unions
	fieldDefinitionRef, ok := p.visitor.Definition.ObjectTypeDefinitionFieldWithName(p.visitor.Walker.EnclosingTypeDefinition.Ref, fieldNameBytes)
	if !ok {
		return "", fmt.Errorf(`expected field definition to exist for field "%s"`, fieldNameBytes)
	}
	subjectWithVariableTemplateReplacements := subject
	for templateNumber, groups := range matches {
		validationResult, err := argument_templates.ValidateArgumentPath(p.visitor.Definition, groups, fieldDefinitionRef)
		if err != nil {
			return "", fmt.Errorf(`argument template #%d defined on field "%s" is invalid: %w`, templateNumber+1, fieldNameBytes, err)
		}
		argumentNameBytes := []byte(validationResult.ArgumentPath[0])
		argumentRef, ok := p.visitor.Operation.FieldArgument(fieldRef, argumentNameBytes)
		if !ok {
			return "", fmt.Errorf(`operation field "%s" does not define argument "%s"`, fieldNameBytes, argumentNameBytes)
		}
		// variablePlaceholder has the form $$0$$, $$1$$, etc.
		variablePlaceholder, err := p.variables.AddContextVariableByArgumentRef(
			p.visitor.Operation, p.visitor.Definition, p.visitor.Walker.Ancestors[0].Ref, argumentRef, validationResult.ArgumentPath, validationResult.FinalInputValueTypeRef,
		)
		if err != nil {
			return "", fmt.Errorf(`failed to retrieve variable placeholder for argument ""%s" defined on operation field "%s": %w`, argumentNameBytes, fieldNameBytes, err)
		}
		// Replace the template literal with the variable placeholder (and reuse the variable if it already exists)
		subjectWithVariableTemplateReplacements = strings.ReplaceAll(subjectWithVariableTemplateReplacements, groups[0], variablePlaceholder)
	}
	// Substitute the variable templates for dummy values to check naïvely that the string is a valid NATS subject
	if natsserver.IsValidSubject(variableTemplateRegex.ReplaceAllLiteralString(subjectWithVariableTemplateReplacements, "a")) {
		return subjectWithVariableTemplateReplacements, nil
	}
	return "", fmt.Errorf(`subject "%s" is not a valid NATS subject`, subject)
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

	slices.Sort(extractedSubjects)

	p.subscriptionEventConfiguration = &NatsSubscriptionEventConfiguration{
		ProviderID:          p.eventMetadata.ProviderID,
		Subjects:            extractedSubjects,
		StreamConfiguration: p.eventConfiguration.StreamConfiguration,
	}
}
