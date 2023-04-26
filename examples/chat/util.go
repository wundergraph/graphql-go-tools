package chat

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

const (
	chatExampleDirectoryRelativePath = "examples/chat"

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

func LoadSchemaFromExamplesDirectoryWithinPkg() ([]byte, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	absolutePath := filepath.Join(strings.Split(wd, "pkg")[0], chatExampleDirectoryRelativePath, "schema.graphql")
	return ioutil.ReadFile(absolutePath)
}

func GraphQLRequestForOperation(operation string) ([]byte, error) {
	gqlRequest := graphqlRequest{
		OperationName: "",
		Query:         operation,
		Variables:     nil,
	}

	return json.Marshal(gqlRequest)
}
