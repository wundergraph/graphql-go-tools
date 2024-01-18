package subscriptiontesting

import (
	_ "embed"
	"encoding/json"
)

const (
	chatExampleDirectoryRelativePath = "pkg/testing/subscriptiontesting"

	MutationSendMessage = `mutation SendMessage{
	post(roomName: "#test", username: "myuser", text: "Hello World!") {
		text
		createdBy
	}
}`

	QueryGetMessages = `query GetMessages {
	room(name:"#test") {
		name
		messages {
			text
			createdBy
		}
	}
}`

	SubscriptionLiveMessages = `subscription LiveMessages {
	messageAdded(roomName: "#test") {
		text
		createdBy
	}
}`

	InvalidSubscriptionLiveMessages = `subscription LiveMessages {
	messageAdded(roomName: "#test") {
		a: text
	}
	messageAdded(roomName: "#test") {
		a: createdBy
	}
}`

	InvalidOperation = `query InvalidOperation {
	serverName
}
`
)

type graphqlRequest struct {
	OperationName string          `json:"operationName"`
	Query         string          `json:"query"`
	Variables     json.RawMessage `json:"variables"`
}

//go:embed schema.graphql
var ChatSchema []byte

func GraphQLRequestForOperation(operation string) ([]byte, error) {
	gqlRequest := graphqlRequest{
		OperationName: "",
		Query:         operation,
		Variables:     nil,
	}

	return json.Marshal(gqlRequest)
}
