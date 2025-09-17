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

// Push represents a push operation to a channel
type Push struct {
	mu           sync.RWMutex
	channel      *Channel
	event        string
	payload      func() interface{}
	receivedResp *ReplyPayload
	timeout      time.Duration
	timeoutTimer *time.Timer
	recHooks     []ReceiveHook
	sent         bool
	ref          string
	refEvent     string
	refEventRef  int
	timeoutCtx   context.Context
	timeoutCancel context.CancelFunc
}

// NewPush creates a new push instance
func NewPush(channel *Channel, event string, payload func() interface{}, timeout time.Duration) *Push {
	return &Push{
		channel:  channel,
		event:    event,
		payload:  payload,
		timeout:  timeout,
		recHooks: make([]ReceiveHook, 0),
	}
}

// Resend resets and resends the push with a new timeout
func (p *Push) Resend(timeout time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.timeout = timeout
	p.reset()
	p.send()
}

// Send sends the push message
func (p *Push) Send() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.send()
}

// send performs the actual send (must be called with lock held)
func (p *Push) send() {
	if p.hasReceived("timeout") {
		return
	}

	p.startTimeout()
	p.sent = true

	msg := &Message{
		Topic:   p.channel.topic,
		Event:   p.event,
		Payload: p.payload(),
		Ref:     p.ref,
		JoinRef: p.channel.joinRef,
	}

	p.channel.socket.Push(msg)
}

// Receive registers a callback for a specific response status
func (p *Push) Receive(status string, callback func(interface{})) *Push {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.hasReceived(status) {
		callback(p.receivedResp.Response)
	}

	p.recHooks = append(p.recHooks, ReceiveHook{
		status:   status,
		callback: callback,
	})

	return p
}

// Reset resets the push state
func (p *Push) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reset()
}

// reset performs the actual reset (must be called with lock held)
func (p *Push) reset() {
	p.cancelRefEvent()
	p.ref = ""
	p.refEvent = ""
	p.receivedResp = nil
	p.sent = false
}

// matchReceive calls matching receive hooks
func (p *Push) matchReceive(replyPayload *ReplyPayload) {
	p.mu.RLock()
	hooks := make([]ReceiveHook, len(p.recHooks))
	copy(hooks, p.recHooks)
	p.mu.RUnlock()

	for _, hook := range hooks {
		if hook.status == replyPayload.Status {
			hook.callback(replyPayload.Response)
		}
	}
}

// cancelRefEvent removes the reply event listener
func (p *Push) cancelRefEvent() {
	if p.refEvent != "" && p.refEventRef != 0 {
		p.channel.Off(p.refEvent, p.refEventRef)
		p.refEventRef = 0
	}
}

// CancelTimeout cancels the timeout timer
func (p *Push) CancelTimeout() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelTimeout()
}

// cancelTimeout performs the actual timeout cancellation (must be called with lock held)
func (p *Push) cancelTimeout() {
	if p.timeoutTimer != nil {
		p.timeoutTimer.Stop()
		p.timeoutTimer = nil
	}
	if p.timeoutCancel != nil {
		p.timeoutCancel()
		p.timeoutCancel = nil
	}
}

// StartTimeout starts the timeout timer
func (p *Push) StartTimeout() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.startTimeout()
}

// startTimeout performs the actual timeout start (must be called with lock held)
func (p *Push) startTimeout() {
	if p.timeoutTimer != nil {
		p.cancelTimeout()
	}

	p.ref = p.channel.socket.MakeRef()
	p.refEvent = p.replyEventName(p.ref)

	// Register reply event handler
	p.refEventRef = p.channel.On(p.refEvent, func(payload interface{}) {
		p.mu.Lock()
		p.cancelRefEvent()
		p.cancelTimeout()

		// Parse the reply payload
		replyPayload, err := GetReplyPayload(&Message{Event: "phx_reply", Payload: payload})
		if err != nil {
			// Handle error - maybe trigger error hooks
			return
		}

		p.receivedResp = replyPayload
		p.mu.Unlock()

		p.matchReceive(replyPayload)
	})

	// Set up timeout
	ctx, cancel := context.WithCancel(context.Background())
	p.timeoutCtx = ctx
	p.timeoutCancel = cancel

	p.timeoutTimer = time.AfterFunc(p.timeout, func() {
		select {
		case <-ctx.Done():
			return
		default:
			p.trigger("timeout", map[string]interface{}{})
		}
	})
}

// hasReceived checks if a response with the given status was received (must be called with lock held)
func (p *Push) hasReceived(status string) bool {
	return p.receivedResp != nil && p.receivedResp.Status == status
}

// trigger triggers a response status
func (p *Push) trigger(status string, response interface{}) {
	// Create the reply payload structure that Phoenix expects
	payload := map[string]interface{}{
		"status":   status,
		"response": response,
	}

	p.channel.trigger(p.refEvent, payload, p.ref, "")
}

// replyEventName generates the reply event name for a reference
func (p *Push) replyEventName(ref string) string {
	return "chan_reply_" + ref
}

// HasReceived returns true if a response with the given status was received
func (p *Push) HasReceived(status string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.hasReceived(status)
}

// GetResponse returns the received response if available
func (p *Push) GetResponse() *ReplyPayload {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.receivedResp
}

// IsSent returns true if the push has been sent
func (p *Push) IsSent() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sent
}

// GetRef returns the push reference
func (p *Push) GetRef() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ref
}