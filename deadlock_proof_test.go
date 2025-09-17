package gophxchannels

import (
	"fmt"
	"testing"
	"time"
)

// TestDeadlockProofUserAPI demonstrates that the public API is completely deadlock-free
// This is what actual users would write - no special lock handling required
func TestDeadlockProofUserAPI(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	defer socket.Disconnect()

	// Test the exact scenario that was deadlocking before
	numGoroutines := 20
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			// Each goroutine creates its own channel
			channelName := fmt.Sprintf("room:stress_%d", id)
			channel := socket.Channel(channelName, map[string]interface{}{
				"user_id": id,
			})

			// Register multiple event handlers concurrently
			for j := 0; j < 3; j++ {
				eventName := fmt.Sprintf("event_%d", j)
				channel.On(eventName, func(payload interface{}) {
					// Event handler
				})
			}

			// Perform the operations that were causing deadlocks
			if id%2 == 0 {
				// Some goroutines join
				push := channel.Join()
				if push != nil {
					// Set up receive handlers
					push.Receive("ok", func(resp interface{}) {
						t.Logf("Channel %s joined successfully", channelName)
					})
					push.Receive("error", func(resp interface{}) {
						t.Logf("Channel %s join failed: %v", channelName, resp)
					})
				}
			} else {
				// Others just query state
				_ = channel.IsJoined()
				_ = channel.GetState()
				_ = channel.Topic()
				_ = channel.JoinRef()
			}

			// Concurrent socket operations
			_ = socket.IsConnected()
			_ = socket.ConnectionState()
			ref := socket.MakeRef()
			if ref == "" {
				t.Errorf("MakeRef should return non-empty string")
			}

		}(i)
	}

	// Wait for all goroutines to complete with a reasonable timeout
	timeout := time.After(10 * time.Second)
	completed := 0

	for completed < numGoroutines {
		select {
		case <-done:
			completed++
		case <-timeout:
			t.Fatalf("Test timed out after 10 seconds - DEADLOCK DETECTED in user API")
		}
	}

	t.Logf("SUCCESS: All %d goroutines completed without deadlocks", numGoroutines)
}

// TestStressTestUserAPIWithManyChannels creates many channels concurrently
func TestStressTestUserAPIWithManyChannels(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	defer socket.Disconnect()

	numChannels := 50
	done := make(chan bool, numChannels)

	// Create many channels concurrently and perform operations on them
	for i := 0; i < numChannels; i++ {
		go func(id int) {
			defer func() { done <- true }()

			// Create channel
			channel := socket.Channel(fmt.Sprintf("room:stress_many_%d", id), nil)

			// Register handlers
			channel.On("message", func(payload interface{}) {})
			channel.On("ping", func(payload interface{}) {})

			// Query states multiple times
			for j := 0; j < 5; j++ {
				_ = channel.IsJoined()
				_ = channel.IsClosed()
				_ = channel.GetState()
				time.Sleep(time.Millisecond) // Small delay to mix operations
			}

		}(i)
	}

	// Wait for completion
	timeout := time.After(5 * time.Second)
	completed := 0

	for completed < numChannels {
		select {
		case <-done:
			completed++
		case <-timeout:
			t.Fatalf("Stress test timed out - possible deadlock with many channels")
		}
	}

	t.Logf("SUCCESS: Stress test with %d channels completed", numChannels)
}

// TestRapidSocketOperations performs rapid socket operations concurrently
func TestRapidSocketOperations(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	defer socket.Disconnect()

	numWorkers := 10
	done := make(chan bool, numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func(id int) {
			defer func() { done <- true }()

			// Rapidly perform socket operations
			for j := 0; j < 100; j++ {
				_ = socket.IsConnected()
				_ = socket.ConnectionState()
				_ = socket.MakeRef()

				// Create different channels
				channelName := fmt.Sprintf("room:rapid_%d_%d", id, j%5)
				channel := socket.Channel(channelName, nil)
				_ = channel.Topic()
			}
		}(i)
	}

	// Wait for completion
	timeout := time.After(3 * time.Second)
	completed := 0

	for completed < numWorkers {
		select {
		case <-done:
			completed++
		case <-timeout:
			t.Fatalf("Rapid operations test timed out - possible performance issue")
		}
	}

	t.Logf("SUCCESS: Rapid operations test completed")
}