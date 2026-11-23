package grpcdatasource

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/events"
)

// eventKey is the federation key of the Event entity.
var eventKey = plan.FederationFieldConfiguration{TypeName: "Event", SelectionSet: "id"}

// eventsClients holds a gRPC and a ConnectRPC transport to the events service.
// The gRPC transport uses createProtoMessage. The ConnectRPC transport uses createProtoWire.
// All loads share one compiler, because the ConnectRPC transport keeps the response descriptors of the first call.
type eventsClients struct {
	grpc     RPCTransport
	connect  RPCTransport
	compiler *RPCCompiler
}

func newEventsClients(t *testing.T) eventsClients {
	t.Helper()

	compiler, err := NewProtoCompiler(events.MustProtoSchema(t), eventsTestMapping())
	require.NoError(t, err)

	return eventsClients{
		grpc:     NewGRPCTransport(events.NewClientConn(t)),
		connect:  NewConnectTransport(ConnectTransportConfig{BaseURL: events.NewConnectServer(t)}),
		compiler: compiler,
	}
}

// load runs the operation over both transports and returns the response.
// It fails the test when the two responses are not equal.
func (c eventsClients) load(t *testing.T, operation, variables string, federationConfigs ...plan.FederationFieldConfiguration) string {
	t.Helper()

	got := c.loadWith(t, c.grpc, operation, variables, federationConfigs)
	gotConnect := c.loadWith(t, c.connect, operation, variables, federationConfigs)
	require.JSONEq(t, got, gotConnect, "the ConnectRPC response is different from the gRPC response")

	return got
}

// loadWith runs the operation over the transport and returns the response.
func (c eventsClients) loadWith(t *testing.T, transport RPCTransport, operation, variables string, federationConfigs plan.FederationFieldConfigurations) string {
	t.Helper()

	schemaDoc := events.MustGraphQLSchema(t)
	operationDoc, report := astparser.ParseGraphqlDocumentString(operation)
	require.False(t, report.HasErrors(), report.Error())

	ds, err := NewDataSource(transport, DataSourceConfig{
		Operation:         &operationDoc,
		Definition:        &schemaDoc,
		SubgraphName:      "Events",
		Mapping:           eventsTestMapping(),
		Compiler:          c.compiler,
		FederationConfigs: federationConfigs,
	})
	require.NoError(t, err)

	output, err := ds.Load(t.Context(), nil, fmt.Appendf(nil, `{"query":%q,"body":{"variables":%s}}`, operation, variables))
	require.NoError(t, err)

	return string(output)
}

func TestDataSourceLoadOptionalFieldsInTypes(t *testing.T) {
	clients := newEventsClients(t)
	operation := `query($id: ID!) { event(id: $id) { id description capacity price online organizerId status } }`

	t.Run("returns the values of set fields", func(t *testing.T) {
		got := clients.load(t, operation, `{"id":"event-1"}`)
		require.JSONEq(t, `{"data":{"event":{
			"id": "event-1",
			"description": "Write your first Go service",
			"capacity": 30,
			"price": 49.5,
			"online": true,
			"organizerId": "organizer-1",
			"status": "SCHEDULED"
		}}}`, got)
	})

	t.Run("returns null for unset fields", func(t *testing.T) {
		got := clients.load(t, operation, `{"id":"event-2"}`)
		require.JSONEq(t, `{"data":{"event":{
			"id": "event-2",
			"description": null,
			"capacity": null,
			"price": null,
			"online": null,
			"organizerId": null,
			"status": null
		}}}`, got)
	})

	t.Run("returns zero values for fields set to zero", func(t *testing.T) {
		got := clients.load(t, operation, `{"id":"event-3"}`)
		require.JSONEq(t, `{"data":{"event":{
			"id": "event-3",
			"description": "",
			"capacity": 0,
			"price": 0,
			"online": false,
			"organizerId": "",
			"status": null
		}}}`, got)
	})
}

func TestDataSourceLoadOptionalFieldsInInputs(t *testing.T) {
	clients := newEventsClients(t)
	operation := `mutation($input: EventInput!) { createEvent(input: $input) { id title description capacity price online organizerId status } }`

	t.Run("sends the values of set fields", func(t *testing.T) {
		got := clients.load(t, operation, `{"input":{
			"title": "Go Workshop",
			"description": "Write your first Go service",
			"capacity": 30,
			"price": 49.5,
			"online": true,
			"organizerId": "organizer-1",
			"status": "POSTPONED"
		}}`)
		require.JSONEq(t, `{"data":{"createEvent":{
			"id": "event-created",
			"title": "Go Workshop",
			"description": "Write your first Go service",
			"capacity": 30,
			"price": 49.5,
			"online": true,
			"organizerId": "organizer-1",
			"status": "POSTPONED"
		}}}`, got)
	})

	t.Run("sends null fields as unset fields", func(t *testing.T) {
		got := clients.load(t, operation, `{"input":{
			"title": "Community Meetup",
			"description": null,
			"capacity": null,
			"price": null,
			"online": null,
			"organizerId": null,
			"status": null
		}}`)
		require.JSONEq(t, `{"data":{"createEvent":{
			"id": "event-created",
			"title": "Community Meetup",
			"description": null,
			"capacity": null,
			"price": null,
			"online": null,
			"organizerId": null,
			"status": null
		}}}`, got)
	})

	t.Run("sends absent fields as unset fields", func(t *testing.T) {
		got := clients.load(t, operation, `{"input":{"title":"Community Meetup"}}`)
		require.JSONEq(t, `{"data":{"createEvent":{
			"id": "event-created",
			"title": "Community Meetup",
			"description": null,
			"capacity": null,
			"price": null,
			"online": null,
			"organizerId": null,
			"status": null
		}}}`, got)
	})

	t.Run("sends zero values as set fields", func(t *testing.T) {
		got := clients.load(t, operation, `{"input":{
			"title": "Free Lightning Talks",
			"description": "",
			"capacity": 0,
			"price": 0,
			"online": false,
			"organizerId": ""
		}}`)
		require.JSONEq(t, `{"data":{"createEvent":{
			"id": "event-created",
			"title": "Free Lightning Talks",
			"description": "",
			"capacity": 0,
			"price": 0,
			"online": false,
			"organizerId": "",
			"status": null
		}}}`, got)
	})

	t.Run("does not match null values with a set filter field", func(t *testing.T) {
		got := clients.load(t, `query($filter: EventFilter) { events(filter: $filter) { id } }`, `{"filter":{"online":false}}`)
		require.JSONEq(t, `{"data":{"events":[{"id":"event-3"}]}}`, got)
	})

	t.Run("does not filter with null filter fields", func(t *testing.T) {
		got := clients.load(t, `query($filter: EventFilter) { events(filter: $filter) { id } }`, `{"filter":{
			"titleContains": null,
			"minCapacity": null,
			"maxPrice": null,
			"online": null,
			"organizerId": null,
			"status": null
		}}`)
		require.JSONEq(t, `{"data":{"events":[{"id":"event-1"},{"id":"event-2"},{"id":"event-3"}]}}`, got)
	})
}

func TestDataSourceLoadOptionalFieldsInArguments(t *testing.T) {
	clients := newEventsClients(t)
	operation := `query($limit: Int) { events(limit: $limit) { id } }`

	t.Run("sends a null argument as an unset field", func(t *testing.T) {
		got := clients.load(t, operation, `{"limit":null}`)
		require.JSONEq(t, `{"data":{"events":[{"id":"event-1"},{"id":"event-2"},{"id":"event-3"}]}}`, got)
	})

	t.Run("sends a zero argument as a set field", func(t *testing.T) {
		got := clients.load(t, operation, `{"limit":0}`)
		require.JSONEq(t, `{"data":{"events":[]}}`, got)
	})

	t.Run("sends the value of a set argument", func(t *testing.T) {
		got := clients.load(t, operation, `{"limit":1}`)
		require.JSONEq(t, `{"data":{"events":[{"id":"event-1"}]}}`, got)
	})
}

func TestDataSourceLoadOptionalFieldsInEntityLookups(t *testing.T) {
	clients := newEventsClients(t)

	t.Run("returns values, null and zero values for the entities", func(t *testing.T) {
		got := clients.load(t,
			`query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Event { id capacity online } } }`,
			`{"representations":[
				{"__typename":"Event","id":"event-1"},
				{"__typename":"Event","id":"event-2"},
				{"__typename":"Event","id":"event-3"}
			]}`,
			eventKey,
		)
		require.JSONEq(t, `{"data":{"_entities":[
			{"__typename":"Event","id":"event-1","capacity":30,"online":true},
			{"__typename":"Event","id":"event-2","capacity":null,"online":null},
			{"__typename":"Event","id":"event-3","capacity":0,"online":false}
		]}}`, got)
	})
}

func TestDataSourceLoadOptionalFieldsInRequiredFields(t *testing.T) {
	clients := newEventsClients(t)
	operation := `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Event { id availableSeats } } }`
	availableSeats := plan.FederationFieldConfiguration{TypeName: "Event", FieldName: "availableSeats", SelectionSet: "ticketsSold"}

	t.Run("sends the value of a set required field", func(t *testing.T) {
		got := clients.load(t, operation, `{"representations":[{"__typename":"Event","id":"event-1","ticketsSold":10}]}`, eventKey, availableSeats)
		require.JSONEq(t, `{"data":{"_entities":[{"__typename":"Event","id":"event-1","availableSeats":20}]}}`, got)
	})

	t.Run("sends a null required field as an unset field", func(t *testing.T) {
		got := clients.load(t, operation, `{"representations":[{"__typename":"Event","id":"event-1","ticketsSold":null}]}`, eventKey, availableSeats)
		require.JSONEq(t, `{"data":{"_entities":[{"__typename":"Event","id":"event-1","availableSeats":null}]}}`, got)
	})

	t.Run("sends a zero required field as a set field", func(t *testing.T) {
		got := clients.load(t, operation, `{"representations":[{"__typename":"Event","id":"event-1","ticketsSold":0}]}`, eventKey, availableSeats)
		require.JSONEq(t, `{"data":{"_entities":[{"__typename":"Event","id":"event-1","availableSeats":30}]}}`, got)
	})

	t.Run("returns null and zero values for the results", func(t *testing.T) {
		got := clients.load(t, operation, `{"representations":[
			{"__typename":"Event","id":"event-2","ticketsSold":5},
			{"__typename":"Event","id":"event-3","ticketsSold":0}
		]}`, eventKey, availableSeats)
		require.JSONEq(t, `{"data":{"_entities":[
			{"__typename":"Event","id":"event-2","availableSeats":null},
			{"__typename":"Event","id":"event-3","availableSeats":0}
		]}}`, got)
	})
}

func TestDataSourceLoadOptionalFieldsInFieldResolvers(t *testing.T) {
	clients := newEventsClients(t)
	operation := `query($discount: Float) { events { id ticketPrice(discount: $discount) } }`

	t.Run("sends the value of a set argument", func(t *testing.T) {
		got := clients.load(t, operation, `{"discount":0.5}`)
		require.JSONEq(t, `{"data":{"events":[
			{"id":"event-1","ticketPrice":24.75},
			{"id":"event-2","ticketPrice":null},
			{"id":"event-3","ticketPrice":0}
		]}}`, got)
	})

	t.Run("sends a null argument as an unset field", func(t *testing.T) {
		got := clients.load(t, operation, `{"discount":null}`)
		require.JSONEq(t, `{"data":{"events":[
			{"id":"event-1","ticketPrice":49.5},
			{"id":"event-2","ticketPrice":null},
			{"id":"event-3","ticketPrice":0}
		]}}`, got)
	})
}
