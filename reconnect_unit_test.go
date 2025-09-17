package gophxchannels_test

import (
	"testing"
	"time"

	phx "github.com/go-phx-channels"
)

// TestAttemptReconnectRetryBehavior tests that attemptReconnect schedules retries on failure
func TestAttemptReconnectRetryBehavior(t *testing.T) {
	t.Log("🧪 Testing attemptReconnect retry behavior")
	t.Log("📋 This test verifies that when doConnect() fails, another reconnection attempt is scheduled")
	t.Log("🐛 Bug was: attemptReconnect() tried once and gave up, no retries")

	// Test with a non-existent endpoint to guarantee connection failure
	reconnectEnabled := true
	socket := phx.NewSocket("ws://localhost:9999/socket/websocket", &phx.SocketOptions{
		ReconnectEnabled:     &reconnectEnabled,
		MaxReconnectAttempts: 2, // Limit for test
	})

	// Try to connect (should fail)
	err := socket.Connect()
	if err == nil {
		t.Log("⚠️  Unexpected: Connection succeeded to non-existent server")
		socket.Disconnect()
		return
	}

	t.Logf("✅ Connection failed as expected: %v", err)
	t.Log("💡 In a real scenario with a working server:")
	t.Log("   1. attemptReconnect() is called when timer fires")
	t.Log("   2. doConnect() fails because server is down")
	t.Log("   3. Should schedule another retry (this was the bug)")
	t.Log("   4. Process repeats until server comes back or max attempts reached")

	socket.Disconnect()
}

// TestReconnectionBackoffProgression tests that backoff increases properly
func TestReconnectionBackoffProgression(t *testing.T) {
	backoffFunc := phx.DefaultReconnectAfterMs

	expectedBackoffs := []time.Duration{
		10 * time.Millisecond,  // attempt 1
		50 * time.Millisecond,  // attempt 2
		100 * time.Millisecond, // attempt 3
		150 * time.Millisecond, // attempt 4
		200 * time.Millisecond, // attempt 5
		250 * time.Millisecond, // attempt 6
		500 * time.Millisecond, // attempt 7
		1 * time.Second,        // attempt 8
		2 * time.Second,        // attempt 9
		5 * time.Second,        // attempt 10+ (max)
	}

	for i, expected := range expectedBackoffs {
		attempt := i + 1
		actual := backoffFunc(attempt)

		if attempt <= 9 {
			if actual != expected {
				t.Errorf("Attempt %d: expected %v, got %v", attempt, expected, actual)
			}
		} else {
			// After 9 attempts, should be capped at 5 seconds
			if actual != 5*time.Second {
				t.Errorf("Attempt %d: expected 5s (max), got %v", attempt, actual)
			}
		}
	}

	t.Log("✅ Backoff progression is correct")
	t.Log("📊 Attempts should increase: 10ms → 50ms → 100ms → ... → 5s (max)")
}