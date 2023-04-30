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
)

var (
	headerData  = []byte("data:")
	headerEvent = []byte("event:")

	eventTypeComplete = []byte("complete")
	eventTypeNext     = []byte("next")
)

type gqlSSEConnectionHandler struct {
	conn    *http.Client
	ctx     context.Context
	log     log.Logger
	options GraphQLSubscriptionOptions
}

func newSSEConnectionHandler(ctx context.Context, conn *http.Client, opts GraphQLSubscriptionOptions, l log.Logger) *gqlSSEConnectionHandler {
	return &gqlSSEConnectionHandler{
		conn:    conn,
		ctx:     ctx,
		log:     l,
		options: opts,
	}
}

func (h *gqlSSEConnectionHandler) StartBlocking(sub Subscription) {
	reqCtx := sub.ctx

	dataCh := make(chan []byte)
	errCh := make(chan []byte)
	defer func() {
		close(dataCh)
		close(errCh)
		close(sub.next)
	}()

	go h.subscribe(reqCtx, sub, dataCh, errCh)

	for {
		select {
		case data := <-dataCh:
			sub.next <- data
		case err := <-errCh:
			sub.next <- err
			return
		case <-reqCtx.Done():
			return
		}
	}
}

func (h *gqlSSEConnectionHandler) subscribe(ctx context.Context, sub Subscription, dataCh, errCh chan []byte) {
	resp, err := h.performSubscriptionRequest(ctx)
	if err != nil {
		h.log.Error("failed to perform subscription request", log.Error(err))

		if ctx.Err() != nil {
			// request context was canceled do not send an error as channel will be closed
			return
		}

		sub.next <- []byte(internalError)

		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	reader := sse.NewEventStreamReader(resp.Body, math.MaxInt)

	for {
		if ctx.Err() != nil {
			return
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

				if ctx.Err() != nil {
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

func (h *gqlSSEConnectionHandler) performSubscriptionRequest(ctx context.Context) (*http.Response, error) {

	var req *http.Request
	var err error

	// default to GET requests when SSEMethodPost is not enabled in the SubscriptionConfiguration
	if h.options.SSEMethodPost {
		req, err = h.buildPOSTRequest(ctx)
	} else {
		req, err = h.buildGETRequest(ctx)
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

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache-Control", "no-cache")

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

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache-Control", "no-cache")

	return req, nil
}
