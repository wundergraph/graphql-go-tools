package graphql_websocket_subscription

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/buger/jsonparser"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
)

var (
	uniqueIdentifier = []byte("graphql_websocket_subscription")
)

type Config struct {
	Scheme string
	Host   string
	Path   string
	Body   json.RawMessage
	Header http.Header
}

type GraphQLWebsocketSubscriptionStream struct {
	wsClients    map[string]*WebsocketClient
	wsClientsMux sync.Mutex
}

func New() *GraphQLWebsocketSubscriptionStream {
	return &GraphQLWebsocketSubscriptionStream{
		wsClients: map[string]*WebsocketClient{},
	}
}

func (g *GraphQLWebsocketSubscriptionStream) Start(input []byte, next chan<- []byte, stop <-chan struct{}) {

	scheme, host, path, body, headers := httpclient.GetSubscriptionInput(input)

	var (
		header http.Header
	)

	if len(headers) != 0 {
		header = map[string][]string{}
		_ = jsonparser.ObjectEach(headers, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
			header[string(key)] = []string{string(value)}
			return nil
		})
	}

	key := string(scheme) + string(host) + string(path)
	g.wsClientsMux.Lock()
	client, ok := g.wsClients[key]
	if !ok {
		client = &WebsocketClient{}
		err := client.Open(string(scheme), string(host), string(path), header)
		if err != nil {
			g.wsClientsMux.Unlock()
			return
		}
		g.wsClients[key] = client
	}
	g.wsClientsMux.Unlock()

	defer func() {
		g.wsClientsMux.Lock()
		client, ok := g.wsClients[key]
		if ok {
			closed := client.CloseIfNoSubscriptions()
			if closed {
				delete(g.wsClients, key)
			}
		}
		g.wsClientsMux.Unlock()
	}()

	subscription, ok := client.Subscribe(body)
	if !ok {
		return
	}

	defer func() {
		client.Unsubscribe(subscription)
	}()

	for {
		select {
		case <-stop:
			return
		default:
			data, ok := subscription.Next(stop)
			if !ok {
				return
			}
			content, _, _, err := jsonparser.Get(data, "data")
			if err != nil || len(content) == 0 {
				continue
			}
			select {
			case next <- content:
			case <-stop:
				return
			}
		}
	}
}

func (g *GraphQLWebsocketSubscriptionStream) UniqueIdentifier() []byte {
	return uniqueIdentifier
}
