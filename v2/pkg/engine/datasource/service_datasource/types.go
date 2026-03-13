package service_datasource

// Service represents the GraphQL service capabilities exposed via __service query.
type Service struct {
	Capabilities []Capability `json:"capabilities"`
}

// Capability represents a single service capability.
// This follows the pattern proposed in GraphQL spec PR #1163 for service introspection.
type Capability struct {
	// Identifier is the unique identifier for this capability (e.g., "graphql.onError")
	Identifier string `json:"identifier"`
	// Value is an optional value associated with the capability (e.g., "PROPAGATE" for default error behavior)
	Value *string `json:"value,omitempty"`
	// Description provides human-readable documentation for the capability
	Description *string `json:"description,omitempty"`
}

// ServiceOptions configures the service capabilities to expose.
type ServiceOptions struct {
	// DefaultErrorBehavior is the default error behavior when onError is not specified.
	// This is exposed as the "graphql.defaultErrorBehavior" capability.
	DefaultErrorBehavior string
}

// NewService creates a Service with the configured capabilities.
func NewService(opts ServiceOptions) *Service {
	capabilities := []Capability{
		{
			Identifier:  "graphql.onError",
			Description: ptr("Supports the onError request extension for controlling error propagation behavior"),
		},
	}

	if opts.DefaultErrorBehavior != "" {
		capabilities = append(capabilities, Capability{
			Identifier:  "graphql.defaultErrorBehavior",
			Value:       ptr(opts.DefaultErrorBehavior),
			Description: ptr("The default error behavior when onError is not specified in the request"),
		})
	}

	return &Service{
		Capabilities: capabilities,
	}
}

func ptr(s string) *string {
	return &s
}
