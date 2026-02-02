package graphql_datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/client"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// subscriptionClientV2 implements GraphQLSubscriptionClient using the new
// channel-based subscription client.
type subscriptionClientV2 struct {
	client *client.Client
}

// NewGraphQLSubscriptionClient creates a new subscription client
// httpClient is used for WebSocket upgrade requests.
// streamingClient is used for SSE requests (should have appropriate timeouts for long-lived connections).
func NewGraphQLSubscriptionClient(ctx context.Context, httpClient *http.Client, streamingClient *http.Client) GraphQLSubscriptionClient {
	return &subscriptionClientV2{
		client: client.New(ctx, httpClient, streamingClient),
	}
}

// Subscribe implements GraphQLSubscriptionClient.
// It bridges the channel-based new client API to the callback-based updater interface.
func (c *subscriptionClientV2) Subscribe(ctx *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	opts, req, err := convertToClientOptions(options)
	if err != nil {
		return err
	}

	msgCh, cancel, err := c.client.Subscribe(ctx.Context(), req, opts)
	if err != nil {
		fmt.Println("Error subscribing:", err)
		return err
	}

	go c.readLoop(ctx.Context(), msgCh, cancel, updater)

	return nil
}

// readLoop bridges the channel-based API to the callback-based updater.
func (c *subscriptionClientV2) readLoop(ctx context.Context, msgCh <-chan *common.Message, cancel func(), updater resolve.SubscriptionUpdater) {
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			updater.Complete()
			return

		case msg, ok := <-msgCh:
			if !ok {
				updater.Complete()
				return
			}

			if msg.Err != nil {
				updater.Update(formatSubscriptionError(msg.Err))
				updater.Close(resolve.SubscriptionCloseKindDownstreamServiceError)
				return
			}

			if msg.Payload != nil {
				data, err := json.Marshal(msg.Payload)
				if err != nil {
					updater.Update(formatSubscriptionError(err))
					updater.Close(resolve.SubscriptionCloseKindDownstreamServiceError)
					return
				}
				updater.Update(data)
			}

			if msg.Done {
				updater.Complete()
				return
			}
		}
	}
}

// convertToClientOptions converts GraphQLSubscriptionOptions to the new client's types.
func convertToClientOptions(options GraphQLSubscriptionOptions) (common.Options, *common.Request, error) {
	opts := common.Options{
		Endpoint: options.URL,
		Headers:  options.Header,
	}

	// Transport selection
	if options.UseSSE {
		opts.Transport = common.TransportSSE
		if options.SSEMethodPost {
			opts.SSEMethod = common.SSEMethodPOST
		} else {
			opts.SSEMethod = common.SSEMethodGET
		}
	} else {
		opts.Transport = common.TransportWS
		opts.WSSubprotocol = mapWSSubprotocol(options.WsSubProtocol)
	}

	// Convert InitialPayload from json.RawMessage to map[string]any
	if len(options.InitialPayload) > 0 {
		var initPayload map[string]any
		if err := json.Unmarshal(options.InitialPayload, &initPayload); err != nil {
			return common.Options{}, nil, fmt.Errorf("failed to unmarshal initial payload: %w", err)
		}
		opts.InitPayload = initPayload
	}

	// Build request
	req := &common.Request{
		Query:         options.Body.Query,
		OperationName: options.Body.OperationName,
	}

	// Convert Variables from json.RawMessage to map[string]any
	if len(options.Body.Variables) > 0 {
		var vars map[string]any
		if err := json.Unmarshal(options.Body.Variables, &vars); err != nil {
			return common.Options{}, nil, fmt.Errorf("failed to unmarshal variables: %w", err)
		}
		req.Variables = vars
	}

	// Convert Extensions from json.RawMessage to map[string]any
	if len(options.Body.Extensions) > 0 {
		var ext map[string]any
		if err := json.Unmarshal(options.Body.Extensions, &ext); err != nil {
			return common.Options{}, nil, fmt.Errorf("failed to unmarshal extensions: %w", err)
		}
		req.Extensions = ext
	}

	return opts, req, nil
}

// mapWSSubprotocol maps the string subprotocol to the common.WSSubprotocol type.
func mapWSSubprotocol(proto string) common.WSSubprotocol {
	switch proto {
	case ProtocolGraphQLWS:
		return common.SubprotocolGraphQLWS
	case ProtocolGraphQLTWS:
		return common.SubprotocolGraphQLTransportWS
	default:
		return common.SubprotocolAuto
	}
}

// formatSubscriptionError formats an error as a GraphQL error response.
func formatSubscriptionError(err error) []byte {
	errResponse := struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}{
		Errors: []struct {
			Message string `json:"message"`
		}{
			{Message: err.Error()},
		},
	}
	data, _ := json.Marshal(errResponse)
	return data
}
