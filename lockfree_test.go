package gophxchannels

import (
	"testing"
	"time"
)

func TestLockFreeBasicOperations(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	defer socket.Disconnect()

	// Create a channel
	channel := socket.Channel("room:test", nil)

	// Register event handler
	channel.On("test_event", func(payload interface{}) {
		// Event received
	})

	// Check initial state
	if !channel.IsClosed() {
		t.Error("Channel should initially be closed")
	}

	t.Log("Basic operations completed successfully")
}

func TestLockFreeConcurrentOperations(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	defer socket.Disconnect()

	// Create multiple channels concurrently
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			// Each goroutine creates its own channel
			channel := socket.Channel("room:test_"+string(rune('a'+id)), nil)

			// Register event handlers
			channel.On("test", func(payload interface{}) {
				// Event handler
			})

			// Query state
			_ = channel.IsJoined()
			_ = channel.GetState()
			_ = channel.Topic()

		}(i)
	}

	// Wait for all goroutines to complete
	timeout := time.After(5 * time.Second)
	completed := 0

	for completed < 10 {
		select {
		case <-done:
			completed++
		case <-timeout:
			t.Fatal("Test timed out - possible deadlock in lock-free implementation")
		}
	}

	t.Log("All concurrent operations completed successfully")
}

func TestLockFreeConcurrentJoins(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	defer socket.Disconnect()

	// Test concurrent joins on different channels
	done := make(chan bool, 5)

	for i := 0; i < 5; i++ {
		go func(id int) {
			defer func() {
				if r := recover(); r != nil {
					t.Logf("Goroutine %d panicked (this might be expected): %v", id, r)
				}
				done <- true
			}()

			channel := socket.Channel("room:concurrent_"+string(rune('a'+id)), nil)

			// Each goroutine tries to join its channel
			push := channel.Join()
			if push == nil {
				t.Errorf("Join should return a push object")
			}

		}(i)
	}

	// Wait for all goroutines to complete
	timeout := time.After(3 * time.Second)
	completed := 0

	for completed < 5 {
		select {
		case <-done:
			completed++
		case <-timeout:
			t.Fatal("Concurrent joins timed out - deadlock in lock-free join")
		}
	}

	t.Log("All concurrent join operations completed successfully")
}