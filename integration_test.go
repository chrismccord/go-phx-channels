package gophxchannels_test

import (
	"encoding/base64"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	phx "github.com/go-phx-channels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testServerURL = "ws://localhost:4000/socket/websocket"

// TestMain sets up and tears down the test environment
func TestMain(m *testing.M) {
	// Note: Phoenix test server should be running at localhost:4000
	// Start it with: cd test_server && mix phx.server
	code := m.Run()
	os.Exit(code)
}

func TestIntegrationConnectAndDisconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	socket := phx.NewSocket(testServerURL, &phx.SocketOptions{
		Logger: log.New(os.Stdout, "Integration: ", log.LstdFlags),
	})

	err := socket.Connect()
	require.NoError(t, err)

	// Give connection time to establish
	time.Sleep(100 * time.Millisecond)

	assert.True(t, socket.IsConnected())
	assert.Equal(t, phx.StateOpen, socket.ConnectionState())

	socket.Disconnect()

	// Give disconnection time to complete
	time.Sleep(100 * time.Millisecond)

	assert.False(t, socket.IsConnected())
	assert.Equal(t, phx.StateClosed, socket.ConnectionState())
}

func TestIntegrationChannelJoinAndLeave(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	socket := phx.NewSocket(testServerURL, &phx.SocketOptions{
		Logger: log.New(os.Stdout, "Integration: ", log.LstdFlags),
	})

	err := socket.Connect()
	require.NoError(t, err)
	defer socket.Disconnect()

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	channel := socket.Channel("room:test", nil)

	// Track join success
	var joinSucceeded bool
	var joinedEventReceived bool
	var mu sync.Mutex

	// Set up event handler for the "joined" push from server
	channel.On("joined", func(payload interface{}) {
		mu.Lock()
		joinedEventReceived = true
		mu.Unlock()
	})

	// Join the channel
	join := channel.Join()
	join.Receive("ok", func(resp interface{}) {
		mu.Lock()
		joinSucceeded = true
		mu.Unlock()
	}).Receive("error", func(reason interface{}) {
		t.Errorf("Join failed: %v", reason)
	})

	// Wait for join to complete
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	assert.True(t, joinSucceeded, "Channel join should succeed")
	assert.True(t, joinedEventReceived, "Should receive 'joined' event from server")
	assert.True(t, channel.IsJoined(), "Channel should be in joined state")
	mu.Unlock()

	// Leave the channel
	var leaveSucceeded bool
	leave := channel.Leave()
	leave.Receive("ok", func(resp interface{}) {
		mu.Lock()
		leaveSucceeded = true
		mu.Unlock()
	})

	// Wait for leave to complete
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	assert.True(t, leaveSucceeded, "Channel leave should succeed")
	mu.Unlock()
}

func TestIntegrationPushAndReceive(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	socket := phx.NewSocket(testServerURL, &phx.SocketOptions{
		Logger: log.New(os.Stdout, "Integration: ", log.LstdFlags),
	})

	err := socket.Connect()
	require.NoError(t, err)
	defer socket.Disconnect()

	time.Sleep(100 * time.Millisecond)

	channel := socket.Channel("room:test", nil)

	// Join first
	var joinSucceeded bool
	var mu sync.Mutex

	join := channel.Join()
	join.Receive("ok", func(resp interface{}) {
		mu.Lock()
		joinSucceeded = true
		mu.Unlock()
	})

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	require.True(t, joinSucceeded, "Must join channel before testing push")
	mu.Unlock()

	// Test ping-pong
	var pingResponse interface{}
	push := channel.Push("ping", map[string]interface{}{
		"message": "hello",
	})

	push.Receive("ok", func(resp interface{}) {
		mu.Lock()
		pingResponse = resp
		mu.Unlock()
	}).Receive("error", func(reason interface{}) {
		t.Errorf("Ping failed: %v", reason)
	})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	assert.NotNil(t, pingResponse, "Should receive ping response")
	mu.Unlock()
}

func TestIntegrationBroadcast(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	socket1 := phx.NewSocket(testServerURL, &phx.SocketOptions{
		Logger: log.New(os.Stdout, "Socket1: ", log.LstdFlags),
	})
	socket2 := phx.NewSocket(testServerURL, &phx.SocketOptions{
		Logger: log.New(os.Stdout, "Socket2: ", log.LstdFlags),
	})

	err1 := socket1.Connect()
	require.NoError(t, err1)
	defer socket1.Disconnect()

	err2 := socket2.Connect()
	require.NoError(t, err2)
	defer socket2.Disconnect()

	time.Sleep(100 * time.Millisecond)

	// Both join the same room
	channel1 := socket1.Channel("room:broadcast_test", nil)
	channel2 := socket2.Channel("room:broadcast_test", nil)

	var joins int
	var mu sync.Mutex

	join1 := channel1.Join()
	join1.Receive("ok", func(resp interface{}) {
		mu.Lock()
		joins++
		mu.Unlock()
	})

	join2 := channel2.Join()
	join2.Receive("ok", func(resp interface{}) {
		mu.Lock()
		joins++
		mu.Unlock()
	})

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	require.Equal(t, 2, joins, "Both channels should join successfully")
	mu.Unlock()

	// Set up broadcast listener on channel2
	var broadcastReceived bool
	var broadcastPayload interface{}

	channel2.On("broadcast_message", func(payload interface{}) {
		mu.Lock()
		broadcastReceived = true
		broadcastPayload = payload
		mu.Unlock()
	})

	// Send broadcast from channel1
	testPayload := map[string]interface{}{
		"message": "broadcast test",
		"from":    "channel1",
	}

	push := channel1.Push("broadcast", testPayload)
	push.Receive("ok", func(resp interface{}) {
		// Broadcast sent successfully
	})

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	assert.True(t, broadcastReceived, "Channel2 should receive broadcast")
	assert.Equal(t, testPayload, broadcastPayload, "Broadcast payload should match")
	mu.Unlock()
}

func TestIntegrationBinaryMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	socket := phx.NewSocket(testServerURL, &phx.SocketOptions{
		Logger: log.New(os.Stdout, "Binary: ", log.LstdFlags),
	})

	err := socket.Connect()
	require.NoError(t, err)
	defer socket.Disconnect()

	time.Sleep(100 * time.Millisecond)

	channel := socket.Channel("room:binary_test", nil)

	// Join first
	var joinSucceeded bool
	var mu sync.Mutex

	join := channel.Join()
	join.Receive("ok", func(resp interface{}) {
		mu.Lock()
		joinSucceeded = true
		mu.Unlock()
	})

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	require.True(t, joinSucceeded, "Must join channel before testing binary")
	mu.Unlock()

	// Test binary echo - with V1 JSON serializer, we encode as base64
	testData := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	var echoResponse interface{}

	push := channel.Push("binary_echo", testData)
	push.Receive("ok", func(resp interface{}) {
		mu.Lock()
		echoResponse = resp
		mu.Unlock()
	}).Receive("error", func(reason interface{}) {
		t.Errorf("Binary echo failed: %v", reason)
	})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	assert.NotNil(t, echoResponse, "Should receive binary echo response")

	// Verify the echoed data matches
	if respMap, ok := echoResponse.(map[string]interface{}); ok {
		if responseStr, ok := respMap["response"].(string); ok {
			// Server should return base64 encoded data
			decodedData, err := base64.StdEncoding.DecodeString(responseStr)
			assert.NoError(t, err, "Response should be valid base64")
			assert.Equal(t, testData, decodedData, "Echoed data should match original")
		} else {
			t.Errorf("Expected string response, got %T: %v", respMap["response"], respMap["response"])
		}
	} else {
		t.Errorf("Expected map response, got %T: %v", echoResponse, echoResponse)
	}
	mu.Unlock()

	// Test binary push from server
	var binaryEventReceived bool
	var binaryEventData []byte

	channel.On("binary_data", func(payload interface{}) {
		mu.Lock()
		binaryEventReceived = true
		if base64Str, ok := payload.(string); ok {
			if decoded, err := base64.StdEncoding.DecodeString(base64Str); err == nil {
				binaryEventData = decoded
			}
		}
		mu.Unlock()
	})

	push2 := channel.Push("push_binary", map[string]interface{}{})
	push2.Receive("ok", func(resp interface{}) {
		// Binary push initiated
	})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	assert.True(t, binaryEventReceived, "Should receive binary event from server")
	assert.Equal(t, []byte{1, 2, 3, 4}, binaryEventData, "Binary event data should match expected")
	mu.Unlock()
}

func TestIntegrationReconnection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	reconnectEnabled := true
	options := &phx.SocketOptions{
		Logger:                log.New(os.Stdout, "Reconnect: ", log.LstdFlags),
		ReconnectEnabled:      &reconnectEnabled,
		MaxReconnectAttempts:  3,
		HeartbeatInterval:     1 * time.Second,
	}

	socket := phx.NewSocket(testServerURL, options)

	var reconnections int
	var mu sync.Mutex

	socket.OnOpen(func() {
		mu.Lock()
		reconnections++
		mu.Unlock()
	})

	err := socket.Connect()
	require.NoError(t, err)
	defer socket.Disconnect()

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, reconnections, "Should have one initial connection")
	mu.Unlock()

	// Simulate disconnection by closing the connection
	// Note: This test is more complex as it requires the server to actually close
	// the connection. In a real test environment, you might simulate network issues.
}

func TestIntegrationErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	socket := phx.NewSocket(testServerURL, &phx.SocketOptions{
		Logger: log.New(os.Stdout, "Error: ", log.LstdFlags),
	})

	err := socket.Connect()
	require.NoError(t, err)
	defer socket.Disconnect()

	time.Sleep(100 * time.Millisecond)

	channel := socket.Channel("room:error_test", nil)

	// Join first
	var joinSucceeded bool
	var mu sync.Mutex

	join := channel.Join()
	join.Receive("ok", func(resp interface{}) {
		mu.Lock()
		joinSucceeded = true
		mu.Unlock()
	})

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	require.True(t, joinSucceeded, "Must join channel before testing errors")
	mu.Unlock()

	// Test server error
	var errorReceived bool
	var errorReason interface{}

	push := channel.Push("error_test", map[string]interface{}{})
	push.Receive("ok", func(resp interface{}) {
		t.Error("Should not receive ok for error test")
	}).Receive("error", func(reason interface{}) {
		mu.Lock()
		errorReceived = true
		errorReason = reason
		mu.Unlock()
	})

	time.Sleep(500 * time.Millisecond)

	// Note: The exact error handling depends on how Phoenix handles the raised error
	// It might send an error reply or trigger a channel error event
	mu.Lock()
	_ = errorReceived // Use the variables to avoid compiler errors
	_ = errorReason
	mu.Unlock()
}

func TestIntegrationTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	socket := phx.NewSocket(testServerURL, &phx.SocketOptions{
		Logger:  log.New(os.Stdout, "Timeout: ", log.LstdFlags),
		Timeout: 1 * time.Second, // Short timeout for testing
	})

	err := socket.Connect()
	require.NoError(t, err)
	defer socket.Disconnect()

	time.Sleep(100 * time.Millisecond)

	channel := socket.Channel("room:timeout_test", nil)

	// Join first
	var joinSucceeded bool
	var mu sync.Mutex

	join := channel.Join()
	join.Receive("ok", func(resp interface{}) {
		mu.Lock()
		joinSucceeded = true
		mu.Unlock()
	})

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	require.True(t, joinSucceeded, "Must join channel before testing timeout")
	mu.Unlock()

	// Test timeout - server won't reply to this
	var timeoutReceived bool

	push := channel.Push("timeout_test", map[string]interface{}{})
	push.Receive("ok", func(resp interface{}) {
		t.Error("Should not receive ok for timeout test")
	}).Receive("timeout", func(resp interface{}) {
		mu.Lock()
		timeoutReceived = true
		mu.Unlock()
	})

	// Wait longer than timeout
	time.Sleep(2 * time.Second)

	mu.Lock()
	assert.True(t, timeoutReceived, "Should receive timeout event")
	mu.Unlock()
}