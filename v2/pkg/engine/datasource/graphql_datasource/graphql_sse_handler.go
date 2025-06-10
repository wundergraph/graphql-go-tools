package graphql_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"

	"github.com/buger/jsonparser"
	log "github.com/jensneuse/abstractlogger"
	"github.com/r3labs/sse/v2"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

var (
	headerData  = []byte("data:")
	headerEvent = []byte("event:")

	eventTypeComplete = []byte("complete")
	eventTypeNext     = []byte("next")
)

type gqlSSEConnectionHandler struct {
	conn                          *http.Client
	requestContext, engineContext context.Context
	log                           log.Logger
	options                       GraphQLSubscriptionOptions
	updater                       resolve.SubscriptionUpdater
}

func newSSEConnectionHandler(requestContext, engineContext context.Context, conn *http.Client, updater resolve.SubscriptionUpdater, options GraphQLSubscriptionOptions, l log.Logger) *gqlSSEConnectionHandler {
	return &gqlSSEConnectionHandler{
		conn:           conn,
		requestContext: requestContext,
		engineContext:  engineContext,
		log:            l,
		updater:        updater,
		options:        options,
	}
}

func (h *gqlSSEConnectionHandler) StartBlocking() {
	dataCh := make(chan []byte)
	errCh := make(chan []byte)
	defer func() {
		close(dataCh)
		close(errCh)
		h.updater.Complete()
	}()

	go h.subscribe(dataCh, errCh)

	for {
		select {
		case data := <-dataCh:
			h.updater.Update(data)
		case data := <-errCh:
			h.updater.Update(data)
			return
		case <-h.requestContext.Done():
			return
		case <-h.engineContext.Done():
			return
		}
	}
}

func (h *gqlSSEConnectionHandler) subscribe(dataCh, errCh chan []byte) {
	resp, err := h.performSubscriptionRequest()
	if err != nil {
		h.log.Error("failed to perform subscription request", log.Error(err))

		if h.requestContext.Err() != nil {
			// request context was canceled do not send an error as channel will be closed
			return
		}

		h.updater.Update([]byte(internalError))

		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	reader := sse.NewEventStreamReader(resp.Body, math.MaxInt)

	for {
		select {
		case <-h.requestContext.Done():
			return
		case <-h.engineContext.Done():
			return
		default:
		}

		msg, err := reader.ReadEvent()
		if err != nil {
			if err == io.EOF {
				return
			}

			h.log.Error("failed to read event", log.Error(err))

			errCh <- []byte(internalError)
			return
		}

		if len(msg) == 0 {
			continue
		}

		// normalize the crlf to lf to make it easier to split the lines.
		// split the line by "\n" or "\r", per the spec.
		lines := bytes.FieldsFunc(msg, func(r rune) bool { return r == '\n' || r == '\r' })
		for _, line := range lines {
			switch {
			case bytes.HasPrefix(line, headerData):
				data := trim(line[len(headerData):])

				if len(data) == 0 {
					continue
				}

				if h.requestContext.Err() != nil {
					// request context was canceled do not send an error as channel will be closed
					return
				}

				dataCh <- data
			case bytes.HasPrefix(line, headerEvent):
				event := trim(line[len(headerEvent):])

				switch {
				case bytes.Equal(event, eventTypeComplete):
					return
				case bytes.Equal(event, eventTypeNext):
					continue
				}
			case bytes.HasPrefix(msg, []byte(":")):
				// according to the spec, we ignore messages starting with a colon
				continue
			default:
				// ideally we should not get here, or if we do, we should ignore it
				// but some providers send a json object with the error messages, without the event header

				// check for errors which came without event header
				data := trim(line)

				val, valueType, _, err := jsonparser.Get(data, "errors")
				switch err {
				case jsonparser.KeyPathNotFoundError:
					continue
				case jsonparser.MalformedJsonError:
					// ignore garbage
					continue
				case nil:
					if valueType == jsonparser.Array {
						response := []byte(`{}`)
						response, err = jsonparser.Set(response, val, "errors")
						if err != nil {
							h.log.Error("failed to set errors", log.Error(err))

							errCh <- []byte(internalError)
							return
						}

						errCh <- response
						return
					} else if valueType == jsonparser.Object {
						response := []byte(`{"errors":[]}`)
						response, err = jsonparser.Set(response, val, "errors", "[0]")
						if err != nil {
							h.log.Error("failed to set errors", log.Error(err))

							errCh <- []byte(internalError)
							return
						}

						errCh <- response
						return
					}

				default:
					h.log.Error("failed to parse errors", log.Error(err))
					errCh <- []byte(internalError)
					return
				}
			}
		}
	}
}

func trim(data []byte) []byte {
	// remove the leading space
	data = bytes.TrimLeft(data, " \t")

	// remove the trailing new line
	data = bytes.TrimRight(data, "\n")

	return data
}

func (h *gqlSSEConnectionHandler) performSubscriptionRequest() (*http.Response, error) {

	var req *http.Request
	var err error

	// default to GET requests when SSEMethodPost is not enabled in the SubscriptionConfiguration
	if h.options.SSEMethodPost {
		req, err = h.buildPOSTRequest(h.requestContext)
	} else {
		req, err = h.buildGETRequest(h.requestContext)
	}

	if err != nil {
		return nil, err
	}

	resp, err := h.conn.Do(req)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return resp, nil
	default:
		return nil, fmt.Errorf("failed to connect to stream unexpected resp status code: %d", resp.StatusCode)
	}
}

func (h *gqlSSEConnectionHandler) buildGETRequest(ctx context.Context) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", h.options.URL, nil)
	if err != nil {
		return nil, err
	}

	if h.options.Header != nil {
		req.Header = h.options.Header
	}

	query := req.URL.Query()
	query.Add("query", h.options.Body.Query)

	if h.options.Body.Variables != nil {
		variables, _ := h.options.Body.Variables.MarshalJSON()

		query.Add("variables", string(variables))
	}

	if h.options.Body.OperationName != "" {
		query.Add("operationName", h.options.Body.OperationName)
	}

	if h.options.Body.Extensions != nil {
		extensions, _ := h.options.Body.Extensions.MarshalJSON()

		query.Add("extensions", string(extensions))
	}

	req.URL.RawQuery = query.Encode()
	h.setSSEHeaders(req)

	return req, nil
}

func (h *gqlSSEConnectionHandler) buildPOSTRequest(ctx context.Context) (*http.Request, error) {
	body, err := json.Marshal(h.options.Body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.options.URL, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	if h.options.Header != nil {
		req.Header = h.options.Header
	}

	req.Header.Set("Content-Type", "application/json")
	h.setSSEHeaders(req)
	return req, nil
}

// setSSEHeaders sets the headers required for SSE for both GET and POST requests
func (h *gqlSSEConnectionHandler) setSSEHeaders(req *http.Request) {
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache-Control", "no-cache")
}
