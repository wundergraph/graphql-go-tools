package events

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/grpc/status"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/events/eventv1"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/events/eventv1/eventv1connect"
)

// NewConnectServer starts the mock service as a ConnectRPC server.
// It returns the base URL of the server. The test cleanup stops the server.
func NewConnectServer(t testing.TB) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle(eventv1connect.NewEventServiceHandler(&connectService{inner: &MockService{}}))

	server := httptest.NewUnstartedServer(mux)
	server.Config.Protocols = new(http.Protocols)
	server.Config.Protocols.SetHTTP1(true)
	server.Config.Protocols.SetUnencryptedHTTP2(true)
	server.Start()
	t.Cleanup(server.Close)

	return server.URL
}

// connectService forwards the ConnectRPC calls to the MockService.
type connectService struct {
	inner *MockService
}

var _ eventv1connect.EventServiceHandler = (*connectService)(nil)

// forward calls the MockService method with the request message.
// It converts a gRPC status error to a ConnectRPC error with the same code.
func forward[Req, Res any](ctx context.Context, req *connect.Request[Req], call func(context.Context, *Req) (*Res, error)) (*connect.Response[Res], error) {
	res, err := call(ctx, req.Msg)
	if err != nil {
		if st, ok := status.FromError(err); ok {
			return nil, connect.NewError(connect.Code(st.Code()), errors.New(st.Message()))
		}
		return nil, err
	}

	return connect.NewResponse(res), nil
}

func (s *connectService) LookupEventById(ctx context.Context, req *connect.Request[eventv1.LookupEventByIdRequest]) (*connect.Response[eventv1.LookupEventByIdResponse], error) {
	return forward(ctx, req, s.inner.LookupEventById)
}

func (s *connectService) MutationCreateEvent(ctx context.Context, req *connect.Request[eventv1.MutationCreateEventRequest]) (*connect.Response[eventv1.MutationCreateEventResponse], error) {
	return forward(ctx, req, s.inner.MutationCreateEvent)
}

func (s *connectService) QueryEvent(ctx context.Context, req *connect.Request[eventv1.QueryEventRequest]) (*connect.Response[eventv1.QueryEventResponse], error) {
	return forward(ctx, req, s.inner.QueryEvent)
}

func (s *connectService) QueryEvents(ctx context.Context, req *connect.Request[eventv1.QueryEventsRequest]) (*connect.Response[eventv1.QueryEventsResponse], error) {
	return forward(ctx, req, s.inner.QueryEvents)
}

func (s *connectService) RequireEventAvailableSeatsById(ctx context.Context, req *connect.Request[eventv1.RequireEventAvailableSeatsByIdRequest]) (*connect.Response[eventv1.RequireEventAvailableSeatsByIdResponse], error) {
	return forward(ctx, req, s.inner.RequireEventAvailableSeatsById)
}

func (s *connectService) ResolveEventTicketPrice(ctx context.Context, req *connect.Request[eventv1.ResolveEventTicketPriceRequest]) (*connect.Response[eventv1.ResolveEventTicketPriceResponse], error) {
	return forward(ctx, req, s.inner.ResolveEventTicketPrice)
}
