package transport

import (
	"errors"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestHandleNextReaderError(t *testing.T) {
	tests := []struct {
		name   string
		input  error
		expect error
	}{
		{"Normal Closure", &websocket.CloseError{Code: websocket.CloseNormalClosure}, errWsConnClosed},
		{"No Status Received", &websocket.CloseError{Code: websocket.CloseNoStatusReceived}, errWsConnClosed},
		{"Other Error", errors.New("some other error"), errors.New("some other error")},
		{"Random WebSocket Error", &websocket.CloseError{Code: websocket.CloseAbnormalClosure}, &websocket.CloseError{Code: websocket.CloseAbnormalClosure}},
	}

	for _, test := range tests {
		result := handleNextReaderError(test.input)
		if ce, ok := test.expect.(*websocket.CloseError); ok {
			if resCe, ok := result.(*websocket.CloseError); ok {
				// Check close error codes directly for websocket errors
				assert.Equal(t, ce.Code, resCe.Code, test.name)
			} else {
				assert.Fail(t, "Expected a websocket close error", test.name)
			}
		} else {
			assert.Equal(t, test.expect, result, test.name)
		}
	}
}
