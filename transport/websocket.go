package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

type (
	Websocket struct {
		Upgrader              websocket.Upgrader
		InitFunc              WebsocketInitFunc
		InitTimeout           time.Duration
		ErrorFunc             WebsocketErrorFunc
		KeepAlivePingInterval time.Duration
		PingPongInterval      time.Duration

		didInjectSubprotocols bool
	}
	wsConnection struct {
		Websocket
		ctx             context.Context
		conn            *websocket.Conn
		me              messageExchanger
		active          map[string]context.CancelFunc
		mu              sync.Mutex
		keepAliveTicker *time.Ticker
		pingPongTicker  *time.Ticker
		service         GraphQLService

		initPayload InitPayload
	}

	WebsocketInitFunc  func(ctx context.Context, initPayload InitPayload) (context.Context, error)
	WebsocketErrorFunc func(ctx context.Context, err error)

	startMessagePayload struct {
		OperationName string                 `json:"operationName"`
		Query         string                 `json:"query"`
		Variables     map[string]interface{} `json:"variables"`
	}
)

var errReadTimeout = errors.New("read timeout")

var _ error = WebsocketError{}

type WebsocketError struct {
	Err error

	// IsReadError flags whether the error occurred on read or write to the websocket
	IsReadError bool
}

func (e WebsocketError) Error() string {
	if e.IsReadError {
		return fmt.Sprintf("websocket read: %v", e.Err)
	}
	return fmt.Sprintf("websocket write: %v", e.Err)
}

func (t Websocket) Supports(r *http.Request) bool {
	return r.Header.Get("Upgrade") != ""
}

func (t Websocket) Do(w http.ResponseWriter, r *http.Request, service GraphQLService) {
	t.injectGraphQLWSSubprotocols()
	ws, err := t.Upgrader.Upgrade(w, r, http.Header{})
	if err != nil {
		log.Printf("unable to upgrade %T to websocket %s: ", w, err.Error())
		SendErrorf(w, http.StatusBadRequest, "unable to upgrade")
		return
	}

	var me messageExchanger
	switch ws.Subprotocol() {
	default:
		msg := websocket.FormatCloseMessage(websocket.CloseProtocolError, fmt.Sprintf("unsupported negotiated subprotocol %s", ws.Subprotocol()))
		_ = ws.WriteMessage(websocket.CloseMessage, msg)
		return
	case graphqlwsSubprotocol, "":
		// clients are required to send a subprotocol, to be backward compatible with the previous implementation we select
		// "graphql-ws" by default
		me = graphqlwsMessageExchanger{c: ws}
	case graphqltransportwsSubprotocol:
		me = graphqltransportwsMessageExchanger{c: ws}
	}

	conn := wsConnection{
		active:    map[string]context.CancelFunc{},
		conn:      ws,
		ctx:       r.Context(),
		service:   service,
		me:        me,
		Websocket: t,
	}

	if !conn.init() {
		return
	}

	conn.run()
}

func (c *wsConnection) handlePossibleError(err error, isReadError bool) {
	if c.ErrorFunc != nil && err != nil {
		c.ErrorFunc(c.ctx, WebsocketError{
			Err:         err,
			IsReadError: isReadError,
		})
	}
}

func (c *wsConnection) nextMessageWithTimeout(timeout time.Duration) (message, error) {
	messages, errs := make(chan message, 1), make(chan error, 1)

	go func() {
		if m, err := c.me.NextMessage(); err != nil {
			errs <- err
		} else {
			messages <- m
		}
	}()

	select {
	case m := <-messages:
		return m, nil
	case err := <-errs:
		return message{}, err
	case <-time.After(timeout):
		return message{}, errReadTimeout
	}
}

func (c *wsConnection) init() bool {
	var m message
	var err error

	if c.InitTimeout != 0 {
		m, err = c.nextMessageWithTimeout(c.InitTimeout)
	} else {
		m, err = c.me.NextMessage()
	}

	if err != nil {
		if err == errReadTimeout {
			c.close(websocket.CloseProtocolError, "connection initialisation timeout")
			return false
		}

		if err == errInvalidMsg {
			c.sendConnectionError("invalid json")
		}

		c.close(websocket.CloseProtocolError, "decoding error")
		return false
	}

	switch m.t {
	case initMessageType:
		if len(m.payload) > 0 {
			c.initPayload = make(InitPayload)
			err := jsonDecode(m.payload, &c.initPayload)
			if err != nil {
				return false
			}
		}

		if c.InitFunc != nil {
			ctx, err := c.InitFunc(c.ctx, c.initPayload)
			if err != nil {
				c.sendConnectionError(err.Error())
				c.close(websocket.CloseNormalClosure, "terminated")
				return false
			}
			c.ctx = ctx
		}

		c.write(&message{t: connectionAckMessageType})
		c.write(&message{t: keepAliveMessageType})
	case connectionCloseMessageType:
		c.close(websocket.CloseNormalClosure, "terminated")
		return false
	default:
		c.sendConnectionError("unexpected message %s", m.t)
		c.close(websocket.CloseProtocolError, "unexpected message")
		return false
	}

	return true
}

func (c *wsConnection) write(msg *message) {
	c.mu.Lock()
	c.handlePossibleError(c.me.Send(msg), false)
	c.mu.Unlock()
}

func (c *wsConnection) run() {
	// We create a cancellation that will shutdown the keep-alive when we leave
	// this function.
	ctx, cancel := context.WithCancel(c.ctx)
	defer func() {
		cancel()
		c.close(websocket.CloseAbnormalClosure, "unexpected closure")
	}()

	// If we're running in graphql-ws mode, create a timer that will trigger a
	// keep alive message every interval
	if (c.conn.Subprotocol() == "" || c.conn.Subprotocol() == graphqlwsSubprotocol) && c.KeepAlivePingInterval != 0 {
		c.mu.Lock()
		c.keepAliveTicker = time.NewTicker(c.KeepAlivePingInterval)
		c.mu.Unlock()

		go c.keepAlive(ctx)
	}

	// If we're running in graphql-transport-ws mode, create a timer that will
	// trigger a ping message every interval
	if c.conn.Subprotocol() == graphqltransportwsSubprotocol && c.PingPongInterval != 0 {
		c.mu.Lock()
		c.pingPongTicker = time.NewTicker(c.PingPongInterval)
		c.mu.Unlock()

		// Note: when the connection is closed by this deadline, the client
		// will receive an "invalid close code"
		_ = c.conn.SetReadDeadline(time.Now().UTC().Add(2 * c.PingPongInterval))
		go c.ping(ctx)
	}

	// Close the connection when the context is cancelled.
	// Will optionally send a "close reason" that is retrieved from the context.
	go c.closeOnCancel(ctx)

	for {
		m, err := c.me.NextMessage()
		if err != nil {
			// If the connection got closed by us, don't report the error
			if !errors.Is(err, net.ErrClosed) {
				c.handlePossibleError(err, true)
			}
			return
		}

		switch m.t {
		case startMessageType:
			c.subscribe(c.ctx, &m)
		case stopMessageType:
			c.mu.Lock()
			closer := c.active[m.id]
			c.mu.Unlock()
			if closer != nil {
				closer()
			}
		case connectionCloseMessageType:
			c.close(websocket.CloseNormalClosure, "terminated")
			return
		case pingMessageType:
			c.write(&message{t: pongMessageType, payload: m.payload})
		case pongMessageType:
			_ = c.conn.SetReadDeadline(time.Now().UTC().Add(2 * c.PingPongInterval))
		default:
			c.sendConnectionError("unexpected message %s", m.t)
			c.close(websocket.CloseProtocolError, "unexpected message")
			return
		}
	}
}

func (c *wsConnection) keepAlive(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.keepAliveTicker.Stop()
			return
		case <-c.keepAliveTicker.C:
			c.write(&message{t: keepAliveMessageType})
		}
	}
}

func (c *wsConnection) ping(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.pingPongTicker.Stop()
			return
		case <-c.pingPongTicker.C:
			c.write(&message{t: pingMessageType, payload: json.RawMessage{}})
		}
	}
}

func (c *wsConnection) closeOnCancel(ctx context.Context) {
	<-ctx.Done()

	if r := closeReasonForContext(ctx); r != "" {
		c.sendConnectionError(r)
	}
	c.close(websocket.CloseNormalClosure, "terminated")
}

func (c *wsConnection) subscribe(ctx context.Context, msg *message) {
	var params startMessagePayload
	if err := jsonDecode(msg.payload, &params); err != nil {
		c.sendError(msg.id, &gqlerror.Error{Message: "invalid json"})
		c.complete(msg.id)
		return
	}

	ctx, cancel := context.WithCancel(ctx)

	payloads, err := c.service.Subscribe(ctx, params.Query, params.OperationName, params.Variables)
	if err != nil {
		c.sendError(msg.id, toGQLError(err))
		c.complete(msg.id)
		cancel()
		return
	}

	if c.initPayload != nil {
		ctx = withInitPayload(ctx, c.initPayload)
	}

	c.mu.Lock()
	c.active[msg.id] = cancel
	c.mu.Unlock()

	go func() {
		ctx = withSubscriptionErrorContext(ctx)
		defer func() {
			if errs := getSubscriptionError(ctx); len(errs) != 0 {
				c.sendError(msg.id, errs...)
			} else {
				c.complete(msg.id)
			}
			c.mu.Lock()
			delete(c.active, msg.id)
			c.mu.Unlock()
			cancel()
			for range payloads { // drain input channel
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case payload, more := <-payloads:
				if !more {
					return
				}
				jsonPayload, err := json.Marshal(payload)
				if err != nil {
					c.sendError(msg.id, toGQLError(err))
					continue
				}
				c.sendResponse(msg.id, jsonPayload)
			}
		}

	}()
}

func (c *wsConnection) sendResponse(id string, response []byte) {
	b, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	c.write(&message{
		payload: b,
		id:      id,
		t:       dataMessageType,
	})
}

func (c *wsConnection) complete(id string) {
	c.write(&message{id: id, t: completeMessageType})
}

func (c *wsConnection) sendError(id string, errors ...*gqlerror.Error) {
	errs := make([]error, len(errors))
	for i, err := range errors {
		errs[i] = err
	}
	b, err := json.Marshal(errs)
	if err != nil {
		panic(err)
	}
	c.write(&message{t: errorMessageType, id: id, payload: b})
}

func (c *wsConnection) sendConnectionError(format string, args ...interface{}) {
	b, err := json.Marshal(&gqlerror.Error{Message: fmt.Sprintf(format, args...)})
	if err != nil {
		panic(err)
	}

	c.write(&message{t: connectionErrorMessageType, payload: b})
}

func (c *wsConnection) close(closeCode int, message string) {
	c.mu.Lock()
	_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(closeCode, message))
	for _, closer := range c.active {
		closer()
	}
	c.mu.Unlock()
	_ = c.conn.Close()
}
