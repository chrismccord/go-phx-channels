package gophxchannels

import (
	"context"
	"sync"
	"time"
)

// ReceiveHook represents a callback for handling responses
type ReceiveHook struct {
	status   string
	callback func(interface{})
}

// PushV2 represents a push operation using goroutines and channels
type PushV2 struct {
	channel      *ChannelV2
	event        string
	payload      func() interface{}
	timeout      time.Duration

	// Communication
	commands chan ChannelCommand

	// State (only accessed by push manager goroutine)
	receivedResp *ReplyPayload
	recHooks     []ReceiveHook
	sent         bool
	ref          string
	refEvent     string
	refEventRef  int

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewPushV2 creates a new lock-free push instance
func NewPushV2(channel *ChannelV2, event string, payload func() interface{}, timeout time.Duration) *PushV2 {
	ctx, cancel := context.WithCancel(context.Background())

	p := &PushV2{
		channel:  channel,
		event:    event,
		payload:  payload,
		timeout:  timeout,
		commands: make(chan ChannelCommand, 10),
		recHooks: make([]ReceiveHook, 0),
		ctx:      ctx,
		cancel:   cancel,
	}

	// Start the push manager goroutine
	p.wg.Add(1)
	go p.pushManager()

	return p
}

// pushManager is the main goroutine that owns push state
func (p *PushV2) pushManager() {
	defer p.wg.Done()

	var timeoutTimer *time.Timer

	for {
		select {
		case <-p.ctx.Done():
			if timeoutTimer != nil {
				timeoutTimer.Stop()
			}
			return

		case cmd := <-p.commands:
			p.handlePushCommand(cmd)

		case <-func() <-chan time.Time {
			if timeoutTimer != nil {
				return timeoutTimer.C
			}
			return nil
		}():
			// Timeout occurred
			p.triggerTimeout()
		}
	}
}

func (p *PushV2) handlePushCommand(cmd ChannelCommand) {
	switch cmd.Type {
	case "send":
		p.doSend()
		if cmd.Response != nil {
			cmd.Response <- nil
		}

	case "resend":
		timeout := cmd.Data.(time.Duration)
		p.doResend(timeout)
		if cmd.Response != nil {
			cmd.Response <- nil
		}

	case "receive":
		data := cmd.Data.(map[string]interface{})
		status := data["status"].(string)
		callback := data["callback"].(func(interface{}))

		p.doReceive(status, callback)
		if cmd.Response != nil {
			cmd.Response <- p // Return self for chaining
		}

	case "cancel_timeout":
		p.doCancelTimeout()
		if cmd.Response != nil {
			cmd.Response <- nil
		}

	case "start_timeout":
		p.doStartTimeout()
		if cmd.Response != nil {
			cmd.Response <- nil
		}

	case "is_sent":
		if cmd.Response != nil {
			cmd.Response <- p.sent
		}

	case "get_response":
		if cmd.Response != nil {
			cmd.Response <- p.receivedResp
		}

	case "has_received":
		status := cmd.Data.(string)
		if cmd.Response != nil {
			cmd.Response <- p.hasReceived(status)
		}

	case "get_ref":
		if cmd.Response != nil {
			cmd.Response <- p.ref
		}

	case "reply_received":
		replyPayload := cmd.Data.(*ReplyPayload)
		p.handleReplyReceived(replyPayload)
	}
}

// Public API methods

// Send sends the push message
func (p *PushV2) Send() {
	p.commands <- ChannelCommand{
		Type: "send",
	}
}

// Resend resets and resends the push with a new timeout
func (p *PushV2) Resend(timeout time.Duration) {
	p.commands <- ChannelCommand{
		Type: "resend",
		Data: timeout,
	}
}

// Receive registers a callback for a specific response status
func (p *PushV2) Receive(status string, callback func(interface{})) *PushV2 {
	response := make(chan interface{}, 1)
	p.commands <- ChannelCommand{
		Type: "receive",
		Data: map[string]interface{}{
			"status":   status,
			"callback": callback,
		},
		Response: response,
	}

	result := <-response
	return result.(*PushV2)
}

// CancelTimeout cancels the timeout timer
func (p *PushV2) CancelTimeout() {
	p.commands <- ChannelCommand{
		Type: "cancel_timeout",
	}
}

// StartTimeout starts the timeout timer
func (p *PushV2) StartTimeout() {
	p.commands <- ChannelCommand{
		Type: "start_timeout",
	}
}

// IsSent returns true if the push has been sent
func (p *PushV2) IsSent() bool {
	response := make(chan interface{}, 1)
	p.commands <- ChannelCommand{
		Type:     "is_sent",
		Response: response,
	}
	return (<-response).(bool)
}

// GetResponse returns the received response if available
func (p *PushV2) GetResponse() *ReplyPayload {
	response := make(chan interface{}, 1)
	p.commands <- ChannelCommand{
		Type:     "get_response",
		Response: response,
	}
	result := <-response
	if result == nil {
		return nil
	}
	return result.(*ReplyPayload)
}

// HasReceived returns true if a response with the given status was received
func (p *PushV2) HasReceived(status string) bool {
	response := make(chan interface{}, 1)
	p.commands <- ChannelCommand{
		Type:     "has_received",
		Data:     status,
		Response: response,
	}
	return (<-response).(bool)
}

// GetRef returns the push reference
func (p *PushV2) GetRef() string {
	response := make(chan interface{}, 1)
	p.commands <- ChannelCommand{
		Type:     "get_ref",
		Response: response,
	}
	return (<-response).(string)
}

// Private methods (only called by push manager goroutine)

func (p *PushV2) doSend() {
	if p.hasReceived("timeout") {
		return
	}

	p.doStartTimeout()
	p.sent = true

	msg := &Message{
		Topic:   p.channel.topic,
		Event:   p.event,
		Payload: p.payload(),
		Ref:     p.ref,
		JoinRef: p.channel.JoinRef(),
	}

	p.channel.socket.Push(msg)
}

func (p *PushV2) doResend(timeout time.Duration) {
	p.timeout = timeout
	p.doReset()
	p.doSend()
}

func (p *PushV2) doReceive(status string, callback func(interface{})) {
	// If we already received this status, call the callback immediately
	if p.hasReceived(status) {
		go callback(p.receivedResp.Response)
	}

	// Add the hook for future responses
	p.recHooks = append(p.recHooks, ReceiveHook{
		status:   status,
		callback: callback,
	})
}

func (p *PushV2) doStartTimeout() {
	p.sent = true
	p.ref = p.channel.socket.MakeRef()
	p.refEvent = p.replyEventName(p.ref)

	// Register reply event handler with the channel
	p.refEventRef = p.channel.On(p.refEvent, func(payload interface{}) {
		// Parse the reply payload
		replyPayload, err := GetReplyPayload(&Message{Event: "phx_reply", Payload: payload})
		if err != nil {
			return
		}

		// Send to push manager
		p.commands <- ChannelCommand{
			Type: "reply_received",
			Data: replyPayload,
		}
	})

	// Note: Timeout timer will be started by the pushManager goroutine
}

func (p *PushV2) doCancelTimeout() {
	if p.refEvent != "" && p.refEventRef != 0 {
		p.channel.Off(p.refEvent, p.refEventRef)
		p.refEventRef = 0
	}
}

func (p *PushV2) doReset() {
	p.doCancelTimeout()
	p.ref = ""
	p.refEvent = ""
	p.receivedResp = nil
	p.sent = false
}

func (p *PushV2) hasReceived(status string) bool {
	return p.receivedResp != nil && p.receivedResp.Status == status
}

func (p *PushV2) handleReplyReceived(replyPayload *ReplyPayload) {
	p.doCancelTimeout()
	p.receivedResp = replyPayload

	// Call matching receive hooks
	for _, hook := range p.recHooks {
		if hook.status == replyPayload.Status {
			go hook.callback(replyPayload.Response)
		}
	}
}

func (p *PushV2) triggerTimeout() {
	// Create timeout response
	timeoutPayload := &ReplyPayload{
		Status:   "timeout",
		Response: map[string]interface{}{},
	}

	p.handleReplyReceived(timeoutPayload)
}

func (p *PushV2) replyEventName(ref string) string {
	return "chan_reply_" + ref
}

// trigger triggers a response status (for testing and internal use)
func (p *PushV2) trigger(status string, response interface{}) {
	payload := &ReplyPayload{
		Status:   status,
		Response: response,
	}

	p.commands <- ChannelCommand{
		Type: "reply_received",
		Data: payload,
	}
}