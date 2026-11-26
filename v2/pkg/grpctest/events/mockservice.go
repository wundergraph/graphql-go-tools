package events

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/events/eventv1"
)

// CreatedEventID is the ID of the event that MutationCreateEvent returns.
const CreatedEventID = "event-created"

// mockEvents holds the events of the mock service.
// Each event has a different state for the nullable fields.
var mockEvents = []*eventv1.Event{
	// All nullable fields have a value.
	{
		Id:          "event-1",
		Title:       "Go Workshop",
		Description: new("Write your first Go service"),
		Capacity:    new(int32(30)),
		Price:       new(49.5),
		Online:      new(true),
		OrganizerId: new("organizer-1"),
		Status:      eventv1.EventStatus_EVENT_STATUS_SCHEDULED,
	},
	// All nullable fields are null.
	{
		Id:    "event-2",
		Title: "Community Meetup",
	},
	// All nullable fields have the zero value of their type.
	{
		Id:          "event-3",
		Title:       "Free Lightning Talks",
		Description: new(""),
		Capacity:    new(int32(0)),
		Price:       new(0.0),
		Online:      new(false),
		OrganizerId: new(""),
	},
}

// MockService is the mock implementation of the EventService.
type MockService struct {
	eventv1.UnimplementedEventServiceServer
}

func findEvent(id string) *eventv1.Event {
	for _, event := range mockEvents {
		if event.GetId() == id {
			return event
		}
	}

	return nil
}

// LookupEventById returns the events for the keys.
// It returns an event with only the ID for an unknown key.
func (s *MockService) LookupEventById(_ context.Context, req *eventv1.LookupEventByIdRequest) (*eventv1.LookupEventByIdResponse, error) {
	result := make([]*eventv1.Event, 0, len(req.GetKeys()))
	for _, key := range req.GetKeys() {
		event := findEvent(key.GetId())
		if event == nil {
			event = &eventv1.Event{Id: key.GetId()}
		}

		result = append(result, event)
	}

	return &eventv1.LookupEventByIdResponse{Result: result}, nil
}

// QueryEvent returns the event with the ID, or null for an unknown ID.
func (s *MockService) QueryEvent(_ context.Context, req *eventv1.QueryEventRequest) (*eventv1.QueryEventResponse, error) {
	return &eventv1.QueryEventResponse{Event: findEvent(req.GetId())}, nil
}

// QueryEvents returns the events that match the filter.
// A null limit does not limit the events. A limit of 0 returns no events.
// A negative limit returns an InvalidArgument error.
func (s *MockService) QueryEvents(_ context.Context, req *eventv1.QueryEventsRequest) (*eventv1.QueryEventsResponse, error) {
	if req.GetLimit() < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "limit must not be negative: %d", req.GetLimit())
	}

	result := make([]*eventv1.Event, 0, len(mockEvents))
	for _, event := range mockEvents {
		if matchesFilter(event, req.GetFilter()) {
			result = append(result, event)
		}
	}

	if req.Limit != nil && int(req.GetLimit()) < len(result) {
		result = result[:req.GetLimit()]
	}

	return &eventv1.QueryEventsResponse{Events: result}, nil
}

// matchesFilter reports whether the event matches all filter fields that are set.
// An event with a null value does not match a set filter field.
func matchesFilter(event *eventv1.Event, filter *eventv1.EventFilter) bool {
	if filter == nil {
		return true
	}

	if filter.TitleContains != nil && !strings.Contains(event.GetTitle(), filter.GetTitleContains()) {
		return false
	}

	if filter.MinCapacity != nil && (event.Capacity == nil || event.GetCapacity() < filter.GetMinCapacity()) {
		return false
	}

	if filter.MaxPrice != nil && (event.Price == nil || event.GetPrice() > filter.GetMaxPrice()) {
		return false
	}

	if filter.Online != nil && (event.Online == nil || event.GetOnline() != filter.GetOnline()) {
		return false
	}

	if filter.OrganizerId != nil && (event.OrganizerId == nil || event.GetOrganizerId() != filter.GetOrganizerId()) {
		return false
	}

	if filter.GetStatus() != eventv1.EventStatus_EVENT_STATUS_UNSPECIFIED && event.GetStatus() != filter.GetStatus() {
		return false
	}

	return true
}

// MutationCreateEvent returns an event with the values of the input.
func (s *MockService) MutationCreateEvent(_ context.Context, req *eventv1.MutationCreateEventRequest) (*eventv1.MutationCreateEventResponse, error) {
	input := req.GetInput()

	return &eventv1.MutationCreateEventResponse{
		CreateEvent: &eventv1.Event{
			Id:          CreatedEventID,
			Title:       input.GetTitle(),
			Description: input.Description,
			Capacity:    input.Capacity,
			Price:       input.Price,
			Online:      input.Online,
			OrganizerId: input.OrganizerId,
			Status:      input.GetStatus(),
		},
	}, nil
}

// RequireEventAvailableSeatsById returns the capacity minus the sold tickets.
// The result is null when the capacity or the sold tickets are null.
func (s *MockService) RequireEventAvailableSeatsById(_ context.Context, req *eventv1.RequireEventAvailableSeatsByIdRequest) (*eventv1.RequireEventAvailableSeatsByIdResponse, error) {
	result := make([]*eventv1.RequireEventAvailableSeatsByIdResult, 0, len(req.GetContext()))
	for _, rctx := range req.GetContext() {
		item := &eventv1.RequireEventAvailableSeatsByIdResult{}

		event := findEvent(rctx.GetKey().GetId())
		ticketsSold := rctx.GetFields().TicketsSold
		if event != nil && event.Capacity != nil && ticketsSold != nil {
			item.AvailableSeats = new(event.GetCapacity() - *ticketsSold)
		}

		result = append(result, item)
	}

	return &eventv1.RequireEventAvailableSeatsByIdResponse{Result: result}, nil
}

// ResolveEventTicketPrice returns the price with the discount.
// The result is null when the price is null. A null or absent discount gives the full price.
// A discount that is not between 0 and 1 returns an InvalidArgument error.
func (s *MockService) ResolveEventTicketPrice(_ context.Context, req *eventv1.ResolveEventTicketPriceRequest) (*eventv1.ResolveEventTicketPriceResponse, error) {
	// GetDiscount returns 0 for a null or absent discount. A discount of 0 gives the full price.
	discount := req.GetFieldArgs().GetDiscount()
	if discount < 0 || discount > 1 {
		return nil, status.Errorf(codes.InvalidArgument, "discount must be between 0 and 1: %g", discount)
	}

	result := make([]*eventv1.ResolveEventTicketPriceResult, 0, len(req.GetContext()))
	for _, rctx := range req.GetContext() {
		item := &eventv1.ResolveEventTicketPriceResult{}

		if rctx.Price != nil {
			item.TicketPrice = new(rctx.GetPrice() * (1 - discount))
		}

		result = append(result, item)
	}

	return &eventv1.ResolveEventTicketPriceResponse{Result: result}, nil
}
