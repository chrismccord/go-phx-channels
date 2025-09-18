package gophxchannels

import (
	"context"
	"sync"
	"time"
)

// ChannelCommand represents commands sent to channel manager
type ChannelCommand struct {
	Type     string
	Data     interface{}
	Response chan interface{}
}

// ChannelState represents the channel state
type ChannelState int

const (
	ChannelClosed ChannelState = iota
	ChannelErrored
	ChannelJoined
	ChannelJoining
	ChannelLeaving
)

// EventCallback represents an event handler function
type EventCallback func(interface{})

// EventBinding represents an event callback binding
type EventBinding struct {
	event    string
	ref      int
	callback EventCallback
}

// Channel represents a Phoenix channel using goroutines and channels
type Channel struct {
	topic    string
	params   map[string]interface{}
	socket   *Socket

	// Communication
	commands chan ChannelCommand
	messages chan *Message

	// State (only accessed by channel manager goroutine)
	state       ChannelState
	bindings    []EventBinding
	bindingRef  int
	joinedOnce  bool
	pushBuffer  []func()
	joinRef     string
	timeout     time.Duration

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewChannel creates a new lock-free channel
func NewChannel(topic string, params map[string]interface{}, socket *Socket) *Channel {
	if params == nil {
		params = make(map[string]interface{})
	}

	ctx, cancel := context.WithCancel(context.Background())

	ch := &Channel{
		topic:    topic,
		params:   params,
		socket:   socket,
		commands: make(chan ChannelCommand, 100),
		messages: make(chan *Message, 100),
		state:    ChannelClosed,
		bindings: make([]EventBinding, 0),
		timeout:  socket.options.Timeout,
		ctx:      ctx,
		cancel:   cancel,
	}

	// Set up built-in event handlers
	ch.setupBuiltinHandlers()

	// Start the channel manager goroutine
	ch.wg.Add(1)
	go ch.channelManager()

	return ch
}

// channelManager is the main goroutine that owns channel state
func (ch *Channel) channelManager() {
	defer ch.wg.Done()

	for {
		select {
		case <-ch.ctx.Done():
			return

		case cmd := <-ch.commands:
			ch.handleCommand(cmd)

		case msg := <-ch.messages:
			ch.handleMessage(msg)
		}
	}
}

// handleCommand processes commands from the public API
func (ch *Channel) handleCommand(cmd ChannelCommand) {
	switch cmd.Type {
	case "join":
		result := ch.doJoin(cmd.Data)
		if cmd.Response != nil {
			cmd.Response <- result
		}

	case "leave":
		result := ch.doLeave(cmd.Data)
		if cmd.Response != nil {
			cmd.Response <- result
		}

	case "push":
		result := ch.doPush(cmd.Data)
		if cmd.Response != nil {
			cmd.Response <- result
		}

	case "on":
		result := ch.doOn(cmd.Data)
		if cmd.Response != nil {
			cmd.Response <- result
		}

	case "off":
		ch.doOff(cmd.Data)
		if cmd.Response != nil {
			cmd.Response <- nil
		}

	case "get_state":
		if cmd.Response != nil {
			cmd.Response <- ch.state
		}

	case "is_joined":
		if cmd.Response != nil {
			cmd.Response <- (ch.state == ChannelJoined)
		}

	case "is_joining":
		if cmd.Response != nil {
			cmd.Response <- (ch.state == ChannelJoining)
		}

	case "is_leaving":
		if cmd.Response != nil {
			cmd.Response <- (ch.state == ChannelLeaving)
		}

	case "is_closed":
		if cmd.Response != nil {
			cmd.Response <- (ch.state == ChannelClosed)
		}

	case "is_errored":
		if cmd.Response != nil {
			cmd.Response <- (ch.state == ChannelErrored)
		}

	case "get_join_ref":
		if cmd.Response != nil {
			cmd.Response <- ch.joinRef
		}

	case "join_success":
		ch.state = ChannelJoined

	case "join_error":
		ch.state = ChannelErrored

	case "rejoin":
		result := ch.doRejoin(cmd.Data)
		if cmd.Response != nil {
			cmd.Response <- result
		}
	}
}

// Public API methods

// Join joins the channel
func (ch *Channel) Join(timeout ...time.Duration) *Push {
	var joinTimeout time.Duration = ch.timeout
	if len(timeout) > 0 {
		joinTimeout = timeout[0]
	}

	response := make(chan interface{}, 1)
	ch.commands <- ChannelCommand{
		Type: "join",
		Data: map[string]interface{}{
			"timeout": joinTimeout,
		},
		Response: response,
	}

	result := <-response
	return result.(*Push)
}

// Leave leaves the channel
func (ch *Channel) Leave(timeout ...time.Duration) *Push {
	var leaveTimeout time.Duration = ch.timeout
	if len(timeout) > 0 {
		leaveTimeout = timeout[0]
	}

	response := make(chan interface{}, 1)
	ch.commands <- ChannelCommand{
		Type: "leave",
		Data: map[string]interface{}{
			"timeout": leaveTimeout,
		},
		Response: response,
	}

	result := <-response
	return result.(*Push)
}

// Push sends an event to the channel
func (ch *Channel) Push(event string, payload interface{}, timeout ...time.Duration) *Push {
	var pushTimeout time.Duration = ch.timeout
	if len(timeout) > 0 {
		pushTimeout = timeout[0]
	}

	if payload == nil {
		payload = map[string]interface{}{}
	}

	response := make(chan interface{}, 1)
	ch.commands <- ChannelCommand{
		Type: "push",
		Data: map[string]interface{}{
			"event":   event,
			"payload": payload,
			"timeout": pushTimeout,
		},
		Response: response,
	}

	result := <-response
	return result.(*Push)
}

// On registers an event handler
func (ch *Channel) On(event string, callback EventCallback) int {
	response := make(chan interface{}, 1)
	ch.commands <- ChannelCommand{
		Type: "on",
		Data: map[string]interface{}{
			"event":    event,
			"callback": callback,
		},
		Response: response,
	}

	result := <-response
	return result.(int)
}

// Off removes event handlers
func (ch *Channel) Off(event string, ref ...int) {
	data := map[string]interface{}{
		"event": event,
	}
	if len(ref) > 0 {
		data["ref"] = ref[0]
	}

	ch.commands <- ChannelCommand{
		Type: "off",
		Data: data,
	}
}

// State query methods
func (ch *Channel) GetState() ChannelState {
	response := make(chan interface{}, 1)
	ch.commands <- ChannelCommand{
		Type:     "get_state",
		Response: response,
	}
	return (<-response).(ChannelState)
}

func (ch *Channel) IsJoined() bool {
	response := make(chan interface{}, 1)
	ch.commands <- ChannelCommand{
		Type:     "is_joined",
		Response: response,
	}
	return (<-response).(bool)
}

func (ch *Channel) IsJoining() bool {
	response := make(chan interface{}, 1)
	ch.commands <- ChannelCommand{
		Type:     "is_joining",
		Response: response,
	}
	return (<-response).(bool)
}

func (ch *Channel) IsLeaving() bool {
	response := make(chan interface{}, 1)
	ch.commands <- ChannelCommand{
		Type:     "is_leaving",
		Response: response,
	}
	return (<-response).(bool)
}

func (ch *Channel) IsClosed() bool {
	response := make(chan interface{}, 1)
	ch.commands <- ChannelCommand{
		Type:     "is_closed",
		Response: response,
	}
	return (<-response).(bool)
}

func (ch *Channel) IsErrored() bool {
	response := make(chan interface{}, 1)
	ch.commands <- ChannelCommand{
		Type:     "is_errored",
		Response: response,
	}
	return (<-response).(bool)
}

func (ch *Channel) Topic() string {
	return ch.topic
}

func (ch *Channel) JoinRef() string {
	response := make(chan interface{}, 1)
	ch.commands <- ChannelCommand{
		Type:     "get_join_ref",
		Response: response,
	}
	return (<-response).(string)
}

// Rejoin rejoins the channel after reconnection, bypassing the joinedOnce check
func (ch *Channel) Rejoin(timeout ...time.Duration) *Push {
	var rejoinTimeout time.Duration = ch.timeout
	if len(timeout) > 0 {
		rejoinTimeout = timeout[0]
	}

	response := make(chan interface{}, 1)
	ch.commands <- ChannelCommand{
		Type: "rejoin",
		Data: map[string]interface{}{
			"timeout": rejoinTimeout,
		},
		Response: response,
	}

	result := <-response
	return result.(*Push)
}

// HandleMessage is called by the socket to deliver messages to this channel
func (ch *Channel) HandleMessage(msg *Message) {
	select {
	case ch.messages <- msg:
	case <-ch.ctx.Done():
	}
}

// Private methods (only called by channel manager goroutine)

func (ch *Channel) setupBuiltinHandlers() {
	// Add built-in Phoenix event handlers during construction
	ch.bindingRef++
	ch.bindings = append(ch.bindings, EventBinding{
		event: "phx_reply",
		ref:   ch.bindingRef,
		callback: func(payload interface{}) {
			// Handle reply - this will be implemented in Push
		},
	})

	ch.bindingRef++
	ch.bindings = append(ch.bindings, EventBinding{
		event: "phx_close",
		ref:   ch.bindingRef,
		callback: func(payload interface{}) {
			ch.state = ChannelClosed
			// Remove from socket's channel list
			ch.socket.commands <- SocketCommand{
				Type: "remove_channel",
				Data: ch.topic,
			}
		},
	})

	ch.bindingRef++
	ch.bindings = append(ch.bindings, EventBinding{
		event: "phx_error",
		ref:   ch.bindingRef,
		callback: func(payload interface{}) {
			ch.state = ChannelErrored
			// Could trigger reconnection logic here
		},
	})
}

func (ch *Channel) doJoin(data interface{}) *Push {
	params := data.(map[string]interface{})
	timeout := params["timeout"].(time.Duration)

	if ch.joinedOnce {
		panic("tried to join multiple times. 'Join' can only be called a single time per channel instance")
	}

	ch.joinedOnce = true
	ch.state = ChannelJoining
	ch.joinRef = ch.socket.MakeRef()

	// Create join push
	joinPush := NewPush(ch, "phx_join", func() interface{} {
		return ch.params
	}, timeout)

	// Set up join push callbacks
	joinPush.Receive("ok", func(resp interface{}) {
		ch.commands <- ChannelCommand{
			Type: "join_success",
		}
	})

	joinPush.Receive("error", func(reason interface{}) {
		ch.commands <- ChannelCommand{
			Type: "join_error",
			Data: reason,
		}
	})

	// Send the join message
	joinPush.Send()

	return joinPush
}

func (ch *Channel) doRejoin(data interface{}) *Push {
	params := data.(map[string]interface{})
	timeout := params["timeout"].(time.Duration)

	// Reset state for rejoin - this allows rejoining after disconnection
	ch.state = ChannelJoining
	ch.joinRef = ch.socket.MakeRef()

	// Create rejoin push (same as join, but bypasses joinedOnce check)
	rejoinPush := NewPush(ch, "phx_join", func() interface{} {
		return ch.params
	}, timeout)

	// Set up rejoin push callbacks
	rejoinPush.Receive("ok", func(resp interface{}) {
		ch.commands <- ChannelCommand{
			Type: "join_success",
		}
	})

	rejoinPush.Receive("error", func(reason interface{}) {
		ch.commands <- ChannelCommand{
			Type: "join_error",
			Data: reason,
		}
	})

	// Send the rejoin message
	rejoinPush.Send()

	return rejoinPush
}

func (ch *Channel) doLeave(data interface{}) *Push {
	params := data.(map[string]interface{})
	timeout := params["timeout"].(time.Duration)

	ch.state = ChannelLeaving

	leavePush := NewPush(ch, "phx_leave", func() interface{} {
		return map[string]interface{}{}
	}, timeout)

	onClose := func(resp interface{}) {
		ch.state = ChannelClosed
	}

	leavePush.Receive("ok", onClose)
	leavePush.Receive("timeout", onClose)
	leavePush.Send()

	return leavePush
}

func (ch *Channel) doPush(data interface{}) *Push {
	params := data.(map[string]interface{})
	event := params["event"].(string)
	payload := params["payload"]
	timeout := params["timeout"].(time.Duration)

	if !ch.joinedOnce {
		panic("tried to push before joining. Use channel.Join() before pushing events")
	}

	push := NewPush(ch, event, func() interface{} {
		return payload
	}, timeout)

	if ch.canPush() {
		push.Send()
	} else {
		// Buffer the push
		ch.pushBuffer = append(ch.pushBuffer, func() {
			push.Send()
		})
		push.StartTimeout()
	}

	return push
}

func (ch *Channel) doOn(data interface{}) int {
	params := data.(map[string]interface{})
	event := params["event"].(string)
	callback := params["callback"].(EventCallback)

	ch.bindingRef++
	ref := ch.bindingRef

	ch.bindings = append(ch.bindings, EventBinding{
		event:    event,
		ref:      ref,
		callback: callback,
	})

	return ref
}

func (ch *Channel) doOff(data interface{}) {
	params := data.(map[string]interface{})
	event := params["event"].(string)

	if ref, hasRef := params["ref"]; hasRef {
		// Remove specific handler
		targetRef := ref.(int)
		newBindings := make([]EventBinding, 0)
		for _, binding := range ch.bindings {
			if !(binding.event == event && binding.ref == targetRef) {
				newBindings = append(newBindings, binding)
			}
		}
		ch.bindings = newBindings
	} else {
		// Remove all handlers for event
		newBindings := make([]EventBinding, 0)
		for _, binding := range ch.bindings {
			if binding.event != event {
				newBindings = append(newBindings, binding)
			}
		}
		ch.bindings = newBindings
	}
}

func (ch *Channel) canPush() bool {
	// Channel can push if it's joined and socket is connected
	return ch.state == ChannelJoined && ch.socket.IsConnected()
}

func (ch *Channel) handleMessage(msg *Message) {
	// Check if message belongs to this channel
	if !ch.isMember(msg) {
		return
	}

	// Trigger event handlers
	ch.trigger(msg.Event, msg.Payload, msg.Ref, msg.JoinRef)
}

func (ch *Channel) isMember(msg *Message) bool {
	if ch.topic != msg.Topic {
		return false
	}

	// Check join reference for message filtering
	if msg.JoinRef != "" && msg.JoinRef != ch.joinRef {
		return false
	}

	return true
}

func (ch *Channel) trigger(event string, payload interface{}, ref string, joinRef string) {
	// Find matching event handlers
	var matchingBindings []EventBinding
	for _, binding := range ch.bindings {
		if binding.event == event {
			matchingBindings = append(matchingBindings, binding)
		}
	}

	// Call handlers in separate goroutines to avoid blocking
	for _, binding := range matchingBindings {
		go binding.callback(payload)
	}

	// Handle reply events specifically
	if event == "phx_reply" {
		ch.handleReply(payload, ref)
	}
}

func (ch *Channel) handleReply(payload interface{}, ref string) {
	if ref == "" {
		return
	}

	replyEvent := "chan_reply_" + ref
	ch.trigger(replyEvent, payload, ref, "")
}