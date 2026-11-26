package grpcdatasource

// eventsTestMapping returns a hardcoded default mapping between GraphQL and Protobuf
func eventsTestMapping() *GRPCMapping {
	return &GRPCMapping{
		Service: "Events",
		QueryRPCs: RPCConfigMap[RPCConfig]{
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
		MutationRPCs: RPCConfigMap[RPCConfig]{
			"createEvent": {
				RPC:      "MutationCreateEvent",
				Request:  "MutationCreateEventRequest",
				Response: "MutationCreateEventResponse",
			},
		},
		SubscriptionRPCs: RPCConfigMap[RPCConfig]{},
		ResolveRPCs: RPCConfigMap[ResolveRPCMapping]{
			"Event": {
				"ticketPrice": {
					FieldMappingData: FieldMapData{
						TargetName: "ticket_price",
						ArgumentMappings: FieldArgumentMap{
							"discount": "discount",
						},
					},
					RPC:      "ResolveEventTicketPrice",
					Request:  "ResolveEventTicketPriceRequest",
					Response: "ResolveEventTicketPriceResponse",
				},
			},
		},
		EntityRPCs: map[string][]EntityRPCConfig{
			"Event": {
				{
					Key: "id",
					RPCConfig: RPCConfig{
						RPC:      "LookupEventById",
						Request:  "LookupEventByIdRequest",
						Response: "LookupEventByIdResponse",
					},
					RequiredFields: RequiredFieldsRPCMapping{
						"availableSeats": {
							TargetName: "available_seats",
							RPCConfig: RPCConfig{
								RPC:      "RequireEventAvailableSeatsById",
								Request:  "RequireEventAvailableSeatsByIdRequest",
								Response: "RequireEventAvailableSeatsByIdResponse",
							},
						},
					},
				},
			},
		},
		EnumValues: map[string][]EnumValueMapping{
			"EventStatus": {
				{Value: "SCHEDULED", TargetValue: "EVENT_STATUS_SCHEDULED"},
				{Value: "POSTPONED", TargetValue: "EVENT_STATUS_POSTPONED"},
				{Value: "CANCELLED", TargetValue: "EVENT_STATUS_CANCELLED"},
			},
		},
		Fields: map[string]FieldMap{
			"Query": {
				"event": {
					TargetName: "event",
					ArgumentMappings: FieldArgumentMap{
						"id": "id",
					},
				},
				"events": {
					TargetName: "events",
					ArgumentMappings: FieldArgumentMap{
						"filter": "filter",
						"limit":  "limit",
					},
				},
			},
			"Mutation": {
				"createEvent": {
					TargetName: "create_event",
					ArgumentMappings: FieldArgumentMap{
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
					ArgumentMappings: FieldArgumentMap{
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
