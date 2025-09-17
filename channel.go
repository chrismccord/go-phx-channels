package gophxchannels

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ChannelState represents the channel state
type ChannelState int

const (
	ChannelClosed ChannelState = iota
	ChannelErrored
	ChannelJoined
	ChannelJoining
	ChannelLeaving
)

// String returns the string representation of the channel state
func (cs ChannelState) String() string {
	switch cs {
	case ChannelClosed:
		return "closed"
	case ChannelErrored:
		return "errored"
	case ChannelJoined:
		return "joined"
	case ChannelJoining:
		return "joining"
	case ChannelLeaving:
		return "leaving"
	default:
		return "unknown"
	}
}

// EventBinding represents an event callback binding
type EventBinding struct {
	event    string
	ref      int
	callback EventCallback
}

// RejoinTimer manages automatic rejoining
type RejoinTimer struct {
	timer     *time.Timer
	cancel    context.CancelFunc
	enabled   bool
	tries     int
	afterMs   func(int) time.Duration
	rejoinFn  func()
}

// NewRejoinTimer creates a new rejoin timer
func NewRejoinTimer(rejoinFn func(), afterMs func(int) time.Duration) *RejoinTimer {
	return &RejoinTimer{
		rejoinFn: rejoinFn,
		afterMs:  afterMs,
		enabled:  true,
	}
}

// Reset stops the timer and resets tries
func (rt *RejoinTimer) Reset() {
	if rt.timer != nil {
		rt.timer.Stop()
		rt.timer = nil
	}
	if rt.cancel != nil {
		rt.cancel()
		rt.cancel = nil
	}
	rt.tries = 0
}

// ScheduleTimeout schedules a rejoin attempt
func (rt *RejoinTimer) ScheduleTimeout() {
	if !rt.enabled {
		return
	}

	rt.Reset()
	rt.tries++

	delay := rt.afterMs(rt.tries)
	ctx, cancel := context.WithCancel(context.Background())
	rt.cancel = cancel

	rt.timer = time.AfterFunc(delay, func() {
		select {
		case <-ctx.Done():
			return
		default:
			rt.rejoinFn()
		}
	})
}

// Channel represents a Phoenix channel
type Channel struct {
	mu            sync.RWMutex
	topic         string
	params        map[string]interface{}
	socket        *Socket
	state         ChannelState
	bindings      []EventBinding
	bindingRef    int
	timeout       time.Duration
	joinedOnce    bool
	joinPush      *Push
	pushBuffer    []func()
	joinRef       string
	rejoinTimer   *RejoinTimer
}

// newChannel creates a new channel instance
func newChannel(topic string, params map[string]interface{}, socket *Socket) *Channel {
	if params == nil {
		params = make(map[string]interface{})
	}

	ch := &Channel{
		topic:       topic,
		params:      params,
		socket:      socket,
		state:       ChannelClosed,
		bindings:    make([]EventBinding, 0),
		timeout:     socket.options.Timeout,
		pushBuffer:  make([]func(), 0),
	}

	// Default rejoin strategy
	rejoinAfterMs := func(tries int) time.Duration {
		intervals := []time.Duration{
			1 * time.Second,
			2 * time.Second,
			5 * time.Second,
		}
		if tries-1 < len(intervals) {
			return intervals[tries-1]
		}
		return 10 * time.Second
	}

	ch.rejoinTimer = NewRejoinTimer(func() {
		if ch.socket.IsConnected() {
			ch.rejoin()
		}
	}, rejoinAfterMs)

	// Set up socket event handlers
	socket.OnError(func(err error) {
		ch.rejoinTimer.Reset()
	})

	socket.OnOpen(func() {
		ch.rejoinTimer.Reset()
		if ch.IsErrored() {
			ch.rejoin()
		}
	})

	// Set up join push callbacks
	ch.setupJoinPush()

	// Set up built-in event handlers
	ch.On("phx_reply", func(payload interface{}) {
		_, err := GetReplyPayload(&Message{Event: "phx_reply", Payload: payload})
		if err != nil {
			return
		}
		// Extract the ref from the context - this is handled in trigger()
	})

	ch.On("phx_close", func(payload interface{}) {
		ch.rejoinTimer.Reset()
		if ch.socket.options.Logger != nil {
			ch.socket.options.Logger.Printf("Channel close %s %s", ch.topic, ch.joinRef)
		}
		ch.mu.Lock()
		ch.state = ChannelClosed
		ch.mu.Unlock()
		ch.socket.removeChannel(ch)
	})

	ch.On("phx_error", func(payload interface{}) {
		if ch.socket.options.Logger != nil {
			ch.socket.options.Logger.Printf("Channel error %s: %v", ch.topic, payload)
		}
		ch.mu.Lock()
		if ch.IsJoining() && ch.joinPush != nil {
			ch.joinPush.Reset()
		}
		ch.state = ChannelErrored
		ch.mu.Unlock()
		if ch.socket.IsConnected() {
			ch.rejoinTimer.ScheduleTimeout()
		}
	})

	return ch
}

// setupJoinPush configures the join push with callbacks
func (ch *Channel) setupJoinPush() {
	ch.joinPush = NewPush(ch, "phx_join", func() interface{} {
		return ch.params
	}, ch.timeout)

	ch.joinPush.Receive("ok", func(resp interface{}) {
		ch.mu.Lock()
		ch.state = ChannelJoined
		ch.rejoinTimer.Reset()
		// Send all buffered pushes
		for _, pushFn := range ch.pushBuffer {
			pushFn()
		}
		ch.pushBuffer = ch.pushBuffer[:0] // clear buffer
		ch.mu.Unlock()
	})

	ch.joinPush.Receive("error", func(reason interface{}) {
		ch.mu.Lock()
		ch.state = ChannelErrored
		ch.mu.Unlock()
		if ch.socket.IsConnected() {
			ch.rejoinTimer.ScheduleTimeout()
		}
	})

	ch.joinPush.Receive("timeout", func(resp interface{}) {
		if ch.socket.options.Logger != nil {
			ch.socket.options.Logger.Printf("Channel timeout %s (%s)", ch.topic, ch.joinRef)
		}

		// Send leave message on timeout
		leavePush := NewPush(ch, "phx_leave", func() interface{} { return map[string]interface{}{} }, ch.timeout)
		leavePush.Send()

		ch.mu.Lock()
		ch.state = ChannelErrored
		ch.joinPush.Reset()
		ch.mu.Unlock()

		if ch.socket.IsConnected() {
			ch.rejoinTimer.ScheduleTimeout()
		}
	})
}

// Join joins the channel
func (ch *Channel) Join(timeout ...time.Duration) *Push {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if ch.joinedOnce {
		panic("tried to join multiple times. 'Join' can only be called a single time per channel instance")
	}

	if len(timeout) > 0 {
		ch.timeout = timeout[0]
		ch.joinPush.timeout = timeout[0]
	}

	ch.joinedOnce = true
	ch.rejoin()
	return ch.joinPush
}

// rejoin performs the actual rejoin (must be called with lock held)
func (ch *Channel) rejoin() {
	if ch.IsLeaving() {
		return
	}

	ch.socket.leaveOpenTopic(ch.topic)
	ch.state = ChannelJoining
	ch.joinRef = ch.socket.MakeRef()
	ch.joinPush.Resend(ch.timeout)
}

// Leave leaves the channel
func (ch *Channel) Leave(timeout ...time.Duration) *Push {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	leaveTimeout := ch.timeout
	if len(timeout) > 0 {
		leaveTimeout = timeout[0]
	}

	ch.rejoinTimer.Reset()
	if ch.joinPush != nil {
		ch.joinPush.CancelTimeout()
	}

	ch.state = ChannelLeaving

	onClose := func(resp interface{}) {
		if ch.socket.options.Logger != nil {
			ch.socket.options.Logger.Printf("Channel leave %s", ch.topic)
		}
		ch.trigger("phx_close", "leave", "", "")
	}

	leavePush := NewPush(ch, "phx_leave", func() interface{} { return map[string]interface{}{} }, leaveTimeout)
	leavePush.Receive("ok", onClose)
	leavePush.Receive("timeout", onClose)
	leavePush.Send()

	if !ch.canPush() {
		leavePush.trigger("ok", map[string]interface{}{})
	}

	return leavePush
}

// Push sends an event to the channel
func (ch *Channel) Push(event string, payload interface{}, timeout ...time.Duration) *Push {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if payload == nil {
		payload = map[string]interface{}{}
	}

	if !ch.joinedOnce {
		panic(fmt.Sprintf("tried to push '%s' to '%s' before joining. Use channel.Join() before pushing events", event, ch.topic))
	}

	pushTimeout := ch.timeout
	if len(timeout) > 0 {
		pushTimeout = timeout[0]
	}

	pushEvent := NewPush(ch, event, func() interface{} { return payload }, pushTimeout)

	if ch.canPush() {
		pushEvent.Send()
	} else {
		pushEvent.StartTimeout()
		ch.pushBuffer = append(ch.pushBuffer, func() {
			pushEvent.Send()
		})
	}

	return pushEvent
}

// On registers an event handler
func (ch *Channel) On(event string, callback EventCallback) int {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.bindingRef++
	ref := ch.bindingRef

	ch.bindings = append(ch.bindings, EventBinding{
		event:    event,
		ref:      ref,
		callback: callback,
	})

	return ref
}

// Off removes event handlers
func (ch *Channel) Off(event string, ref ...int) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if len(ref) == 0 {
		// Remove all handlers for the event
		newBindings := make([]EventBinding, 0)
		for _, binding := range ch.bindings {
			if binding.event != event {
				newBindings = append(newBindings, binding)
			}
		}
		ch.bindings = newBindings
	} else {
		// Remove specific handler
		targetRef := ref[0]
		newBindings := make([]EventBinding, 0)
		for _, binding := range ch.bindings {
			if !(binding.event == event && binding.ref == targetRef) {
				newBindings = append(newBindings, binding)
			}
		}
		ch.bindings = newBindings
	}
}

// canPush returns true if the channel can send messages
func (ch *Channel) canPush() bool {
	return ch.socket.IsConnected() && ch.IsJoined()
}

// State query methods

// IsClosed returns true if the channel is closed
func (ch *Channel) IsClosed() bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.state == ChannelClosed
}

// IsErrored returns true if the channel is in error state
func (ch *Channel) IsErrored() bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.state == ChannelErrored
}

// IsJoined returns true if the channel is joined
func (ch *Channel) IsJoined() bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.state == ChannelJoined
}

// IsJoining returns true if the channel is joining
func (ch *Channel) IsJoining() bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.state == ChannelJoining
}

// IsLeaving returns true if the channel is leaving
func (ch *Channel) IsLeaving() bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.state == ChannelLeaving
}

// GetState returns the current channel state
func (ch *Channel) GetState() ChannelState {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.state
}

// Topic returns the channel topic
func (ch *Channel) Topic() string {
	return ch.topic
}

// JoinRef returns the current join reference
func (ch *Channel) JoinRef() string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.joinRef
}

// isMember checks if a message belongs to this channel
func (ch *Channel) isMember(msg *Message) bool {
	if ch.topic != msg.Topic {
		return false
	}

	// Check join reference for message filtering
	if msg.JoinRef != "" && msg.JoinRef != ch.joinRef {
		if ch.socket.options.Logger != nil {
			ch.socket.options.Logger.Printf("Dropping outdated message: topic=%s, event=%s, joinRef=%s", msg.Topic, msg.Event, msg.JoinRef)
		}
		return false
	}

	return true
}

// trigger calls event handlers for the given event
func (ch *Channel) trigger(event string, payload interface{}, ref string, joinRef string) {
	// Apply onMessage hook if needed (for future extensibility)
	handledPayload := ch.onMessage(event, payload, ref, joinRef)

	ch.mu.RLock()
	eventBindings := make([]EventBinding, 0)
	for _, binding := range ch.bindings {
		if binding.event == event {
			eventBindings = append(eventBindings, binding)
		}
	}
	ch.mu.RUnlock()

	// Call all matching event handlers
	for _, binding := range eventBindings {
		binding.callback(handledPayload)
	}

	// Handle reply events specifically
	if event == "phx_reply" {
		ch.handleReply(payload, ref)
	}
}

// onMessage is a hook for message processing (can be overridden)
func (ch *Channel) onMessage(event string, payload interface{}, ref string, joinRef string) interface{} {
	return payload
}

// handleReply processes reply messages for pending pushes
func (ch *Channel) handleReply(payload interface{}, ref string) {
	if ref == "" {
		return
	}

	replyEvent := fmt.Sprintf("chan_reply_%s", ref)
	ch.trigger(replyEvent, payload, ref, "")
}

// Socket method for leaving open topics
func (s *Socket) leaveOpenTopic(topic string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, ch := range s.channels {
		if ch.topic == topic && (ch.IsJoined() || ch.IsJoining()) {
			if s.options.Logger != nil {
				s.options.Logger.Printf("Leaving duplicate topic \"%s\"", topic)
			}
			ch.Leave()
		}
	}
}