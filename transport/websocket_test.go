package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockMessageExchanger struct {
	mock.Mock
}

func (m *MockMessageExchanger) NextMessage() (message, error) {
	args := m.Called()
	return args.Get(0).(message), args.Error(1)
}

func (m *MockMessageExchanger) Send(msg *message) error {
	args := m.Called(msg)
	return args.Error(0)
}

func TestWebsocketUpgrade(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	wsHandler := Websocket{
		Upgrader: upgrader,
		InitFunc: func(ctx context.Context, payload InitPayload) (context.Context, error) {
			// Assume initialization is always successful
			return ctx, nil
		},
		InitTimeout: 10 * time.Second,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsHandler.Do(w, r, nil) // Assuming nil for GraphQLService for simplicity
	}))
	defer server.Close()

	// Simulate a client connecting to the server
	dialer := websocket.Dialer{
		Subprotocols: []string{graphqlwsSubprotocol},
	}
	header := http.Header{"Origin": []string{"http://localhost"}}
	conn, resp, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), header)
	if err != nil {
		t.Fatalf("Dialing error: %v", err)
	}
	defer conn.Close()

	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode, "Expected successful websocket upgrade")
}
