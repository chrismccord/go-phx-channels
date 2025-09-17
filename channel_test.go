package gophxchannels

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewChannel(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	params := map[string]interface{}{
		"user_id": 123,
	}

	channel := newChannel("room:lobby", params, socket)

	assert.Equal(t, "room:lobby", channel.topic)
	assert.Equal(t, params, channel.params)
	assert.Equal(t, socket, channel.socket)
	assert.Equal(t, ChannelClosed, channel.state)
	assert.False(t, channel.joinedOnce)
	assert.NotNil(t, channel.joinPush)
	assert.NotNil(t, channel.rejoinTimer)
	assert.Empty(t, channel.bindings)
	assert.Empty(t, channel.pushBuffer)
}

func TestChannelStateQueries(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	// Test initial state
	assert.True(t, channel.IsClosed())
	assert.False(t, channel.IsErrored())
	assert.False(t, channel.IsJoined())
	assert.False(t, channel.IsJoining())
	assert.False(t, channel.IsLeaving())
	assert.Equal(t, ChannelClosed, channel.GetState())

	// Test state changes
	testStates := []struct {
		state    ChannelState
		isClosed bool
		isErrored bool
		isJoined bool
		isJoining bool
		isLeaving bool
	}{
		{ChannelClosed, true, false, false, false, false},
		{ChannelErrored, false, true, false, false, false},
		{ChannelJoined, false, false, true, false, false},
		{ChannelJoining, false, false, false, true, false},
		{ChannelLeaving, false, false, false, false, true},
	}

	for _, test := range testStates {
		channel.mu.Lock()
		channel.state = test.state
		channel.mu.Unlock()

		assert.Equal(t, test.isClosed, channel.IsClosed(), "state: %v", test.state)
		assert.Equal(t, test.isErrored, channel.IsErrored(), "state: %v", test.state)
		assert.Equal(t, test.isJoined, channel.IsJoined(), "state: %v", test.state)
		assert.Equal(t, test.isJoining, channel.IsJoining(), "state: %v", test.state)
		assert.Equal(t, test.isLeaving, channel.IsLeaving(), "state: %v", test.state)
		assert.Equal(t, test.state, channel.GetState())
	}
}

func TestChannelStates(t *testing.T) {
	tests := []struct {
		state    ChannelState
		expected string
	}{
		{ChannelClosed, "closed"},
		{ChannelErrored, "errored"},
		{ChannelJoined, "joined"},
		{ChannelJoining, "joining"},
		{ChannelLeaving, "leaving"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.state.String())
	}
}

func TestChannelTopic(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:lobby", nil, socket)

	assert.Equal(t, "room:lobby", channel.Topic())
}

func TestChannelJoinRef(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	// Initially empty
	assert.Equal(t, "", channel.JoinRef())

	// Set join ref
	channel.mu.Lock()
	channel.joinRef = "test_ref"
	channel.mu.Unlock()

	assert.Equal(t, "test_ref", channel.JoinRef())
}

func TestChannelJoin(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	// Mock socket as connected
	socket.mu.Lock()
	socket.connected = true
	socket.state = StateOpen
	socket.mu.Unlock()

	push := channel.Join()

	assert.NotNil(t, push)
	assert.True(t, channel.joinedOnce)
	assert.Equal(t, ChannelJoining, channel.GetState())
}

func TestChannelJoinMultipleTimes(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	channel.Join()

	// Second join should panic
	assert.Panics(t, func() {
		channel.Join()
	})
}

func TestChannelJoinWithTimeout(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	customTimeout := 5 * time.Second
	push := channel.Join(customTimeout)

	assert.Equal(t, customTimeout, channel.timeout)
	assert.Equal(t, customTimeout, push.timeout)
}

func TestChannelPush(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	// Join first
	channel.mu.Lock()
	channel.joinedOnce = true
	channel.state = ChannelJoined
	channel.mu.Unlock()

	payload := map[string]interface{}{
		"message": "hello",
	}

	push := channel.Push("new_message", payload)

	assert.NotNil(t, push)
	assert.Equal(t, channel, push.channel)
	assert.Equal(t, "new_message", push.event)
}

func TestChannelPushBeforeJoin(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	// Should panic when pushing before join
	assert.Panics(t, func() {
		channel.Push("test_event", nil)
	})
}

func TestChannelPushBuffering(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	// Set up channel as joined but socket not connected
	channel.mu.Lock()
	channel.joinedOnce = true
	channel.state = ChannelJoined
	channel.mu.Unlock()

	socket.mu.Lock()
	socket.connected = false
	socket.mu.Unlock()

	push := channel.Push("test_event", map[string]interface{}{})

	// Should be buffered
	assert.Len(t, channel.pushBuffer, 1)
	assert.True(t, push.IsSent()) // timeout started but not actually sent
}

func TestChannelLeave(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	// Set up as joined
	channel.mu.Lock()
	channel.state = ChannelJoined
	channel.mu.Unlock()

	push := channel.Leave()

	assert.NotNil(t, push)
	assert.Equal(t, ChannelLeaving, channel.GetState())
}

func TestChannelLeaveWithTimeout(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	customTimeout := 3 * time.Second
	push := channel.Leave(customTimeout)

	assert.Equal(t, customTimeout, push.timeout)
}

func TestChannelEventHandlers(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	var eventReceived bool
	var receivedPayload interface{}

	ref := channel.On("test_event", func(payload interface{}) {
		eventReceived = true
		receivedPayload = payload
	})

	assert.Greater(t, ref, 0)
	assert.Len(t, channel.bindings, 1)

	// Trigger the event
	testPayload := map[string]interface{}{"test": true}
	channel.trigger("test_event", testPayload, "", "")

	assert.True(t, eventReceived)
	assert.Equal(t, testPayload, receivedPayload)
}

func TestChannelOffEvent(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	ref1 := channel.On("test_event", func(payload interface{}) {})
	_ = channel.On("test_event", func(payload interface{}) {})
	channel.On("other_event", func(payload interface{}) {})

	assert.Len(t, channel.bindings, 3)

	// Remove specific handler
	channel.Off("test_event", ref1)
	assert.Len(t, channel.bindings, 2)

	// Remove all handlers for event
	channel.Off("test_event")
	assert.Len(t, channel.bindings, 1)

	// Should only have "other_event" binding left
	assert.Equal(t, "other_event", channel.bindings[0].event)
}

func TestChannelIsMember(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	channel.mu.Lock()
	channel.joinRef = "join123"
	channel.mu.Unlock()

	tests := []struct {
		name     string
		msg      *Message
		expected bool
	}{
		{
			name: "matching topic and join ref",
			msg: &Message{
				Topic:   "room:test",
				JoinRef: "join123",
			},
			expected: true,
		},
		{
			name: "matching topic, no join ref",
			msg: &Message{
				Topic:   "room:test",
				JoinRef: "",
			},
			expected: true,
		},
		{
			name: "different topic",
			msg: &Message{
				Topic:   "room:other",
				JoinRef: "join123",
			},
			expected: false,
		},
		{
			name: "matching topic, different join ref",
			msg: &Message{
				Topic:   "room:test",
				JoinRef: "join456",
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := channel.isMember(test.msg)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestChannelCanPush(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	// Initially can't push
	assert.False(t, channel.canPush())

	// Set socket as connected
	socket.mu.Lock()
	socket.connected = true
	socket.mu.Unlock()

	// Still can't push - channel not joined
	assert.False(t, channel.canPush())

	// Set channel as joined
	channel.mu.Lock()
	channel.state = ChannelJoined
	channel.mu.Unlock()

	// Now can push
	assert.True(t, channel.canPush())

	// Disconnect socket
	socket.mu.Lock()
	socket.connected = false
	socket.mu.Unlock()

	// Can't push anymore
	assert.False(t, channel.canPush())
}

func TestRejoinTimer(t *testing.T) {
	var rejoinCalled bool
	afterMs := func(tries int) time.Duration {
		return 10 * time.Millisecond
	}

	timer := NewRejoinTimer(func() {
		rejoinCalled = true
	}, afterMs)

	timer.ScheduleTimeout()

	// Wait for timer to fire
	time.Sleep(20 * time.Millisecond)
	assert.True(t, rejoinCalled)

	// Reset should stop timer
	rejoinCalled = false
	timer.ScheduleTimeout()
	timer.Reset()

	// Wait and verify it didn't fire
	time.Sleep(20 * time.Millisecond)
	assert.False(t, rejoinCalled)
}

func TestChannelReply(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	var replyReceived bool
	var replyPayload interface{}

	// Set up a reply event handler
	ref := channel.On("chan_reply_123", func(payload interface{}) {
		replyReceived = true
		replyPayload = payload
	})

	assert.Greater(t, ref, 0)

	// Simulate a reply
	replyData := map[string]interface{}{
		"status":   "ok",
		"response": map[string]interface{}{"result": "success"},
	}

	channel.handleReply(replyData, "123")

	assert.True(t, replyReceived)
	assert.Equal(t, replyData, replyPayload)
}

func TestChannelOnMessage(t *testing.T) {
	socket := NewSocket("ws://localhost:4000/socket/websocket", nil)
	channel := newChannel("room:test", nil, socket)

	// Test default onMessage implementation
	payload := map[string]interface{}{"test": "data"}
	result := channel.onMessage("test_event", payload, "ref1", "joinref1")

	assert.Equal(t, payload, result)
}