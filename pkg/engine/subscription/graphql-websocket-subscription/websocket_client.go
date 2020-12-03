package graphql_websocket_subscription

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"

	"github.com/buger/jsonparser"
	"github.com/gorilla/websocket"
	byte_template "github.com/jensneuse/byte-template"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/jensneuse/graphql-go-tools/pkg/pool"
)

var (
	defaultHeader = http.Header{
		"Sec-WebSocket-Protocol": []string{"graphql-ws"},
		"Sec-WebSocket-Version":  []string{"13"},
	}

	connectionInitMessage = []byte(`{"type":"connection_init"}`)
	startMessage          = []byte(`{"type":"start","id":"{{ .id }}","payload":{{ .payload }}}`)
	stopMessage           = []byte(`{"type":"stop","id":"{{ .id }}"}`)
)

type WebsocketClient struct {
	conn                   *websocket.Conn
	done                   chan struct{}
	tmpl                   *byte_template.Template
	addSubscription        chan addSubscriptionCmd
	removeSubscription     chan removeSubscriptionCmd
	subscriptions          map[uint64]Subscription
	closeIfNoSubscriptions chan chan bool
}

func (w *WebsocketClient) Open(scheme, host, path string, header http.Header) (err error) {
	w.tmpl = byte_template.New()
	w.addSubscription = make(chan addSubscriptionCmd)
	w.removeSubscription = make(chan removeSubscriptionCmd)
	w.subscriptions = map[uint64]Subscription{}
	w.closeIfNoSubscriptions = make(chan chan bool)
	w.done = make(chan struct{})

	u := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   path,
	}

	if header != nil && len(header) != 0 {
		for key := range defaultHeader {
			header[key] = defaultHeader[key]
		}
	} else {
		header = defaultHeader
	}

	w.conn, _, err = websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		return err
	}

	err = w.connectionInit()
	if err != nil {
		return err
	}

	go w.run()

	return
}

func (w *WebsocketClient) Close() {
	close(w.done)
	_ = w.conn.Close()
}

func (w *WebsocketClient) CloseIfNoSubscriptions() (closed bool) {
	closedChan := make(chan bool)
	w.closeIfNoSubscriptions <- closedChan
	return <-closedChan
}

func (w *WebsocketClient) connectionInit() (err error) {
	err = w.conn.WriteMessage(websocket.TextMessage, connectionInitMessage)
	if err != nil {
		return err
	}

	_, connectionAckMessage, err := w.conn.ReadMessage()
	if err != nil {
		return err
	}

	connectionAck, err := jsonparser.GetString(connectionAckMessage, "type")
	if err != nil {
		return err
	}

	if connectionAck != "connection_ack" {
		return fmt.Errorf("ws connection_init not acked")
	}

	return nil
}

func (w *WebsocketClient) run() {
	// run message handling separately from the control flow
	go func() {
		for {
			select {
			case <-w.done:
				return
			default:
				w.handleNextMessage()
			}
		}
	}()

	for {
		select {
		case <-w.done:
			return
		case add := <-w.addSubscription:
			w.handleAdd(add)
		case remove := <-w.removeSubscription:
			w.handleRemove(remove)
		case closed := <-w.closeIfNoSubscriptions:
			shouldClose := len(w.subscriptions) == 0
			if shouldClose {
				w.Close()
			}
			closed <- shouldClose
		}
	}
}

func (w *WebsocketClient) handleNextMessage() {
	_, data, err := w.conn.ReadMessage()
	if err != nil {
		return
	}

	payload, _, _, err := jsonparser.Get(data, "payload")
	if err != nil {
		return
	}

	id, err := jsonparser.GetString(data, "id")
	if err != nil {
		return
	}

	messageType, err := jsonparser.GetString(data, "type")
	if err != nil {
		return
	}

	switch messageType {
	case "data":
	case "complete":
		return
	default:
		return
	}

	sub, ok := w.subscriptions[uint64(unsafebytes.BytesToInt64(unsafebytes.StringToBytes(id)))]
	if !ok {
		return
	}

	select {
	case sub.next <- payload:
	case <-sub.unsubscribe:
	}
}

func (w *WebsocketClient) handleAdd(add addSubscriptionCmd) {
	var (
		err error
		id  uint64
	)

	defer func() {
		if err != nil {
			close(add.getSubscription)
		}
	}()

	id, err = w.nextSubscriptionID()
	if err != nil {
		return
	}

	idStr := strconv.FormatUint(id, 10)

	buf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(buf)

	_, err = w.tmpl.Execute(buf, startMessage, func(w io.Writer, path []byte) (n int, err error) {
		if bytes.Equal(path, []byte(".id")) {
			return w.Write(unsafebytes.StringToBytes(idStr))
		}
		if bytes.Equal(path, []byte(".payload")) {
			return w.Write(add.payload)
		}
		return 0, nil
	})
	if err != nil {
		return
	}

	message := buf.Bytes()

	err = w.conn.WriteMessage(websocket.TextMessage, message)
	if err != nil {
		return
	}

	subscription := Subscription{
		id:          id,
		next:        make(chan []byte),
		stop:        make(chan struct{}),
		unsubscribe: make(chan struct{}),
	}

	w.subscriptions[id] = subscription
	add.getSubscription <- subscription
}

func (w *WebsocketClient) handleRemove(remove removeSubscriptionCmd) {
	close(w.subscriptions[remove.id].stop)
	delete(w.subscriptions, remove.id)

	buf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(buf)

	idStr := strconv.FormatUint(remove.id, 10)

	_, err := w.tmpl.Execute(buf, stopMessage, func(w io.Writer, path []byte) (n int, err error) {
		if bytes.Equal(path, []byte(".id")) {
			return w.Write(unsafebytes.StringToBytes(idStr))
		}
		return 0, nil
	})
	if err != nil {
		return
	}

	message := buf.Bytes()

	err = w.conn.WriteMessage(websocket.TextMessage, message)
	if err != nil {
		return
	}
}

func (w *WebsocketClient) nextSubscriptionID() (uint64, error) {
	for i := uint64(1); i < math.MaxInt64; i++ {
		_, exists := w.subscriptions[i]
		if exists {
			continue
		}
		return i, nil
	}
	return 0, fmt.Errorf("too many open subscriptions")
}

type addSubscriptionCmd struct {
	payload         []byte
	getSubscription chan Subscription
}

func (w *WebsocketClient) Subscribe(payload []byte) (subscription Subscription, ok bool) {
	getSubscription := make(chan Subscription)

	cmd := addSubscriptionCmd{
		payload:         payload,
		getSubscription: getSubscription,
	}

	select {
	case <-w.done:
		close(getSubscription)
	case w.addSubscription <- cmd:
	}

	subscription, ok = <-getSubscription
	return
}

type removeSubscriptionCmd struct {
	id uint64
}

func (w *WebsocketClient) Unsubscribe(subscription Subscription) {
	close(subscription.unsubscribe)
	cmd := removeSubscriptionCmd{
		id: subscription.id,
	}
	w.removeSubscription <- cmd
	<-subscription.stop
}

type Subscription struct {
	next        chan []byte
	id          uint64
	stop        chan struct{}
	unsubscribe chan struct{}
}

func (s *Subscription) Next(done <-chan struct{}) (data []byte, ok bool) {
	select {
	case <-s.stop:
		return nil, false
	case <-done:
		return nil, false
	case data = <-s.next:
		return data, true
	}
}
