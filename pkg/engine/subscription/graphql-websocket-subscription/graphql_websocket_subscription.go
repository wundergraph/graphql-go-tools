package graphql_websocket_subscription

import (
	"encoding/json"
	"net/http"
	"sync"
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
	var (
		config Config
	)
	err := json.Unmarshal(input, &config)
	if err != nil {
		return
	}

	key := config.Scheme + config.Host + config.Path
	g.wsClientsMux.Lock()
	client, ok := g.wsClients[key]
	if !ok {
		client = &WebsocketClient{}
		err = client.Open(config.Scheme, config.Host, config.Path, config.Header)
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

	subscription, ok := client.Subscribe(config.Body)
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
			select {
			case next <- data:
			case <-stop:
				return
			}
		}
	}
}

func (g *GraphQLWebsocketSubscriptionStream) UniqueIdentifier() []byte {
	return uniqueIdentifier
}
