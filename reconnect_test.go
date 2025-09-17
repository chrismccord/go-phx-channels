package gophxchannels_test

import (
	"log"
	"os"
	"testing"
	"time"

	phx "github.com/go-phx-channels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconnectionAfterServerRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping reconnection test in short mode")
	}

	socket := phx.NewSocket("ws://localhost:4000/socket/websocket", &phx.SocketOptions{
		Logger: log.New(os.Stdout, "Reconnect: ", log.LstdFlags),
	})

	// First connection
	err := socket.Connect()
	require.NoError(t, err, "Should connect initially")

	// Wait to ensure connection is stable
	time.Sleep(100 * time.Millisecond)
	assert.True(t, socket.IsConnected(), "Should be connected")

	// Join a channel
	channel := socket.Channel("room:reconnect_test", nil)
	join := channel.Join()

	var joinSucceeded bool
	join.Receive("ok", func(resp interface{}) {
		joinSucceeded = true
	})

	time.Sleep(300 * time.Millisecond)
	assert.True(t, joinSucceeded, "Should join channel")

	// Now simulate server restart by manually disconnecting
	// In a real test, you would kill and restart the Phoenix server
	socket.Disconnect()
	time.Sleep(100 * time.Millisecond)
	assert.False(t, socket.IsConnected(), "Should be disconnected")

	// Reconnect
	err = socket.Connect()
	require.NoError(t, err, "Should reconnect successfully")
	time.Sleep(100 * time.Millisecond)
	assert.True(t, socket.IsConnected(), "Should be connected after reconnect")

	socket.Disconnect()
}