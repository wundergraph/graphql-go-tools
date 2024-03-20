# Chat Demo

## Getting started
1. Install go modules & npm dependencies
```shell
go mod download
npm i
```
2. Start server and start react client
```
chmod +x start-server.sh
./start-server.sh
npm run start
```

Example is forked from: [gqlgen](https://github.com/99designs/gqlgen/tree/master/example/chat)

## Example(s)
```graphql
mutation SendMessage {
  post(roomName: "#test", username: "me", text: "hello!") {
    ...MessageData
  }
}

query GetMessages {
  room(name:"#test") {
    name
    messages {
      ...MessageData
    }
  }
}

subscription LiveMessages {
  messageAdded(roomName: "#test") {
    ...MessageData
  }
}

fragment MessageData on Message{
  id
  text
  createdBy
  createdAt
}
```