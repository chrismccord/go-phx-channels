package gophxchannels

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewSocket(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket", nil)

	// Should append /websocket
	assert.Equal(t, "ws://localhost:4000/socket/websocket", socket.endpoint)
	assert.Equal(t, StateClosed, socket.state)
	assert.NotNil(t, socket.options)
	assert.NotNil(t, socket.serializer)
	assert.Empty(t, socket.channels)
}

func TestNewSocketWithOptions(t *testing.T) {
	logger := log.New(os.Stdout, "test: ", log.LstdFlags)
	options := &SocketOptions{
		Timeout:           5 * time.Second,
		HeartbeatInterval: 15 * time.Second,
		Logger:            logger,
		VSN:               "1.0.0",
		Params: map[string]interface{}{
			"token": "abc123",
		},
	}

	socket := NewSocket("ws://localhost:4000/socket/websocket", options)

	assert.Equal(t, options, socket.options)
	assert.Equal(t, "1.0.0", socket.options.VSN)
	assert.Equal(t, logger, socket.options.Logger)
}

func TestSocketDefaultOptions(t *testing.T) {
	opts := DefaultSocketOptions()

	assert.Equal(t, 10*time.Second, opts.Timeout)
	assert.Equal(t, 30*time.Second, opts.HeartbeatInterval)
	assert.Equal(t, "2.0.0", opts.VSN)
	assert.True(t, opts.ReconnectEnabled)
	assert.NotNil(t, opts.ReconnectAfterMs)
}

func TestDefaultReconnectAfterMs(t *testing.T) {
	tests := []struct {
		tries    int
		expected time.Duration
	}{
		{1, 10 * time.Millisecond},
		{2, 50 * time.Millisecond},
		{5, 200 * time.Millisecond},
		{9, 2 * time.Second},
		{10, 5 * time.Second}, // fallback
		{100, 5 * time.Second}, // fallback
	}

	for _, test := range tests {
		result := DefaultReconnectAfterMs(test.tries)
		assert.Equal(t, test.expected, result, "tries: %d", test.tries)
	}
}

func TestSocketStateQueries(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)

	// Initial state
	assert.Equal(t, StateClosed, socket.ConnectionState())
	assert.False(t, socket.IsConnected())

	// Test state changes
	socket.mu.Lock()
	socket.state = StateOpen
	socket.connected = true
	socket.mu.Unlock()

	assert.Equal(t, StateOpen, socket.ConnectionState())
	assert.True(t, socket.IsConnected())
}

func TestSocketMakeRef(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)

	ref1 := socket.MakeRef()
	ref2 := socket.MakeRef()

	assert.Equal(t, "1", ref1)
	assert.Equal(t, "2", ref2)
	assert.NotEqual(t, ref1, ref2)
}

func TestSocketMakeRefOverflow(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)

	// Set ref to max value to test overflow
	socket.mu.Lock()
	socket.ref = 9223372036854775807 // max int64
	socket.mu.Unlock()

	ref := socket.MakeRef()
	assert.Equal(t, "1", ref) // should reset to 1 after overflow
}

func TestSocketChannel(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)

	params := map[string]interface{}{
		"user_id": 123,
	}

	channel := socket.Channel("room:lobby", params)

	assert.NotNil(t, channel)
	assert.Equal(t, "room:lobby", channel.topic)
	assert.Equal(t, params, channel.params)
	assert.Equal(t, socket, channel.socket)
	assert.Len(t, socket.channels, 1)

	// Test getting the same channel again
	channel2 := socket.Channel("room:lobby", nil)
	assert.Equal(t, channel, channel2)
	assert.Len(t, socket.channels, 1) // should not create duplicate
}

func TestSocketRemoveChannel(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)

	channel1 := socket.Channel("room:1", nil)
	channel2 := socket.Channel("room:2", nil)

	assert.Len(t, socket.channels, 2)

	socket.removeChannel(channel1)
	assert.Len(t, socket.channels, 1)
	assert.Equal(t, channel2, socket.channels[0])

	socket.removeChannel(channel2)
	assert.Empty(t, socket.channels)
}

func TestSocketEventCallbacks(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)

	var openCalled bool
	var closeCalled bool
	var errorCalled bool
	var messageCalled bool

	socket.OnOpen(func() {
		openCalled = true
	})

	socket.OnClose(func() {
		closeCalled = true
	})

	socket.OnError(func(err error) {
		errorCalled = true
	})

	socket.OnMessage(func(msg *Message) {
		messageCalled = true
	})

	// Trigger callbacks
	socket.triggerOpen()
	socket.triggerClose()
	socket.triggerError(assert.AnError)

	testMsg := &Message{Topic: "test", Event: "test"}
	for _, callback := range socket.stateChangeCallbacks.message {
		callback(testMsg)
	}

	assert.True(t, openCalled)
	assert.True(t, closeCalled)
	assert.True(t, errorCalled)
	assert.True(t, messageCalled)
}

func TestSocketPushBuffering(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)

	msg := &Message{
		Topic:   "room:test",
		Event:   "test_event",
		Payload: map[string]interface{}{"test": true},
		Ref:     "1",
	}

	// Push when not connected - should buffer
	socket.Push(msg)
	assert.Len(t, socket.sendBuffer, 1)

	// Simulate connection
	socket.mu.Lock()
	socket.connected = true
	socket.mu.Unlock()

	// Flush buffer
	socket.mu.Lock()
	socket.flushSendBuffer()
	socket.mu.Unlock()

	assert.Empty(t, socket.sendBuffer)
}

func TestSocketStates(t *testing.T) {
	tests := []struct {
		state    SocketState
		expected string
	}{
		{StateClosed, "closed"},
		{StateConnecting, "connecting"},
		{StateOpen, "open"},
		{StateClosing, "closing"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.state.String())
	}
}

func TestSocketLog(t *testing.T) {
	logger := log.New(os.Stdout, "test: ", log.LstdFlags)
	options := &SocketOptions{
		Logger: logger,
	}

	socket := NewSocket("ws://localhost:4000/socket/websocket", options)

	// Should not panic
	socket.log("test message: %s", "hello")

	// Test with no logger
	socket.options.Logger = nil
	socket.log("test message: %s", "hello") // should not panic
}

func TestSocketDisconnectWhenNotConnected(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)

	// Should not panic when disconnecting a closed socket
	socket.Disconnect()
	assert.Equal(t, StateClosed, socket.ConnectionState())
}

func TestSocketConnectError(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)

	// Set state to connecting to test error condition
	socket.mu.Lock()
	socket.state = StateConnecting
	socket.mu.Unlock()

	err := socket.Connect()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "socket is not closed")
}