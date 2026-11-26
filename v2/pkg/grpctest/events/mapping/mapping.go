package mapping

import (
	"testing"

	grpcdatasource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource"
)

// DefaultGRPCMapping returns a hardcoded default mapping between GraphQL and Protobuf
func DefaultGRPCMapping() *grpcdatasource.GRPCMapping {
	return &grpcdatasource.GRPCMapping{
		Service: "Events",
		QueryRPCs: grpcdatasource.RPCConfigMap[grpcdatasource.RPCConfig]{
			"event": {
				RPC:      "QueryEvent",
				Request:  "QueryEventRequest",
				Response: "QueryEventResponse",
			},
			"events": {
				RPC:      "QueryEvents",
				Request:  "QueryEventsRequest",
				Response: "QueryEventsResponse",
			},
		},
		MutationRPCs: grpcdatasource.RPCConfigMap[grpcdatasource.RPCConfig]{
			"createEvent": {
				RPC:      "MutationCreateEvent",
				Request:  "MutationCreateEventRequest",
				Response: "MutationCreateEventResponse",
			},
		},
		SubscriptionRPCs: grpcdatasource.RPCConfigMap[grpcdatasource.RPCConfig]{},
		ResolveRPCs: grpcdatasource.RPCConfigMap[grpcdatasource.ResolveRPCMapping]{
			"Event": {
				"ticketPrice": {
					FieldMappingData: grpcdatasource.FieldMapData{
						TargetName: "ticket_price",
						ArgumentMappings: grpcdatasource.FieldArgumentMap{
							"discount": "discount",
						},
					},
					RPC:      "ResolveEventTicketPrice",
					Request:  "ResolveEventTicketPriceRequest",
					Response: "ResolveEventTicketPriceResponse",
				},
			},
		},
		EntityRPCs: map[string][]grpcdatasource.EntityRPCConfig{
			"Event": {
				{
					Key: "id",
					RPCConfig: grpcdatasource.RPCConfig{
						RPC:      "LookupEventById",
						Request:  "LookupEventByIdRequest",
						Response: "LookupEventByIdResponse",
					},
					RequiredFields: grpcdatasource.RequiredFieldsRPCMapping{
						"availableSeats": {
							TargetName: "available_seats",
							RPCConfig: grpcdatasource.RPCConfig{
								RPC:      "RequireEventAvailableSeatsById",
								Request:  "RequireEventAvailableSeatsByIdRequest",
								Response: "RequireEventAvailableSeatsByIdResponse",
							},
						},
					},
				},
			},
		},
		EnumValues: map[string][]grpcdatasource.EnumValueMapping{
			"EventStatus": {
				{Value: "SCHEDULED", TargetValue: "EVENT_STATUS_SCHEDULED"},
				{Value: "POSTPONED", TargetValue: "EVENT_STATUS_POSTPONED"},
				{Value: "CANCELLED", TargetValue: "EVENT_STATUS_CANCELLED"},
			},
		},
		Fields: map[string]grpcdatasource.FieldMap{
			"Query": {
				"event": {
					TargetName: "event",
					ArgumentMappings: grpcdatasource.FieldArgumentMap{
						"id": "id",
					},
				},
				"events": {
					TargetName: "events",
					ArgumentMappings: grpcdatasource.FieldArgumentMap{
						"filter": "filter",
						"limit":  "limit",
					},
				},
			},
			"Mutation": {
				"createEvent": {
					TargetName: "create_event",
					ArgumentMappings: grpcdatasource.FieldArgumentMap{
						"input": "input",
					},
				},
			},
			"Event": {
				"id": {
					TargetName: "id",
				},
				"title": {
					TargetName: "title",
				},
				"description": {
					TargetName: "description",
				},
				"capacity": {
					TargetName: "capacity",
				},
				"price": {
					TargetName: "price",
				},
				"online": {
					TargetName: "online",
				},
				"organizerId": {
					TargetName: "organizer_id",
				},
				"status": {
					TargetName: "status",
				},
				"ticketsSold": {
					TargetName: "tickets_sold",
				},
				"availableSeats": {
					TargetName: "available_seats",
				},
				"ticketPrice": {
					TargetName: "ticket_price",
					ArgumentMappings: grpcdatasource.FieldArgumentMap{
						"discount": "discount",
					},
				},
			},
			"EventInput": {
				"title": {
					TargetName: "title",
				},
				"description": {
					TargetName: "description",
				},
				"capacity": {
					TargetName: "capacity",
				},
				"price": {
					TargetName: "price",
				},
				"online": {
					TargetName: "online",
				},
				"organizerId": {
					TargetName: "organizer_id",
				},
				"status": {
					TargetName: "status",
				},
			},
			"EventFilter": {
				"titleContains": {
					TargetName: "title_contains",
				},
				"minCapacity": {
					TargetName: "min_capacity",
				},
				"maxPrice": {
					TargetName: "max_price",
				},
				"online": {
					TargetName: "online",
				},
				"organizerId": {
					TargetName: "organizer_id",
				},
				"status": {
					TargetName: "status",
				},
			},
		},
	}
}

// MustDefaultGRPCMapping returns the default GRPC mapping
func MustDefaultGRPCMapping(t *testing.T) *grpcdatasource.GRPCMapping {
	mapping := DefaultGRPCMapping()
	return mapping
}
