package gophxchannels

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// SocketState represents the connection state
type SocketState int

const (
	StateClosed SocketState = iota
	StateConnecting
	StateOpen
	StateClosing
)

// String returns the string representation of the socket state
func (s SocketState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateConnecting:
		return "connecting"
	case StateOpen:
		return "open"
	case StateClosing:
		return "closing"
	default:
		return "unknown"
	}
}

// SocketOptions configures the socket behavior
type SocketOptions struct {
	// Timeout for push operations (default: 10 seconds)
	Timeout time.Duration

	// HeartbeatInterval for sending heartbeats (default: 30 seconds)
	HeartbeatInterval time.Duration

	// ReconnectAfterMs function that returns the reconnect interval
	ReconnectAfterMs func(tries int) time.Duration

	// Logger for debug output
	Logger *log.Logger

	// Parameters to send on connect
	Params map[string]interface{}

	// VSN is the protocol version (default: "2.0.0")
	VSN string

	// AuthToken for authentication
	AuthToken string

	// ReconnectEnabled controls automatic reconnection (default: true)
	ReconnectEnabled bool

	// MaxReconnectAttempts limits reconnection attempts (0 = unlimited)
	MaxReconnectAttempts int
}

// DefaultSocketOptions returns default socket options
func DefaultSocketOptions() *SocketOptions {
	return &SocketOptions{
		Timeout:           10 * time.Second,
		HeartbeatInterval: 30 * time.Second,
		ReconnectAfterMs:  DefaultReconnectAfterMs,
		VSN:               "2.0.0",
		ReconnectEnabled:  true,
	}
}

// DefaultReconnectAfterMs returns the default reconnect backoff strategy
func DefaultReconnectAfterMs(tries int) time.Duration {
	intervals := []time.Duration{
		10 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		150 * time.Millisecond,
		200 * time.Millisecond,
		250 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
	}

	if tries-1 < len(intervals) {
		return intervals[tries-1]
	}
	return 5 * time.Second
}

// EventCallback is called when events are received
type EventCallback func(payload interface{})

// StateChangeCallback is called when socket state changes
type StateChangeCallback func()

// ErrorCallback is called when errors occur
type ErrorCallback func(error)

// Socket manages the WebSocket connection to Phoenix
type Socket struct {
	endpoint   string
	options    *SocketOptions
	serializer *Serializer

	// Connection state
	mu          sync.RWMutex
	conn        *websocket.Conn
	state       SocketState
	connected   bool
	reconnectTries int

	// Channels and references
	channels   []*Channel
	ref        int64
	sendBuffer []func()

	// Event callbacks
	stateChangeCallbacks struct {
		open    []StateChangeCallback
		close   []StateChangeCallback
		error   []ErrorCallback
		message []func(*Message)
	}

	// Heartbeat management
	heartbeatTimer    *time.Timer
	heartbeatTimeout  *time.Timer
	pendingHeartbeat  string
	heartbeatCancel   context.CancelFunc

	// Reconnection management
	reconnectTimer  *time.Timer
	reconnectCancel context.CancelFunc
	reconnectCtx    context.Context

	// Shutdown management
	shutdownCh chan struct{}
	done       chan struct{}
}

// NewSocket creates a new Phoenix socket client
func NewSocket(endpoint string, options *SocketOptions) *Socket {
	if options == nil {
		options = DefaultSocketOptions()
	}

	// Ensure endpoint ends with /websocket
	if endpoint[len(endpoint)-10:] != "/websocket" {
		endpoint += "/websocket"
	}

	s := &Socket{
		endpoint:   endpoint,
		options:    options,
		serializer: NewSerializer(),
		state:      StateClosed,
		channels:   make([]*Channel, 0),
		shutdownCh: make(chan struct{}),
		done:       make(chan struct{}),
	}

	return s
}

// Connect establishes the WebSocket connection
func (s *Socket) Connect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateClosed {
		return fmt.Errorf("socket is not closed")
	}

	return s.connect()
}

// connect performs the actual connection (must be called with lock held)
func (s *Socket) connect() error {
	s.state = StateConnecting

	// Build URL with parameters
	u, err := url.Parse(s.endpoint)
	if err != nil {
		s.state = StateClosed
		return fmt.Errorf("invalid endpoint: %w", err)
	}

	// Add VSN parameter
	q := u.Query()
	q.Set("vsn", s.options.VSN)

	// Add custom parameters
	if s.options.Params != nil {
		for key, value := range s.options.Params {
			q.Set(key, fmt.Sprintf("%v", value))
		}
	}

	u.RawQuery = q.Encode()

	// Set up WebSocket headers
	headers := make(map[string][]string)
	if s.options.AuthToken != "" {
		// Phoenix uses Sec-WebSocket-Protocol for token authentication
		headers["Sec-WebSocket-Protocol"] = []string{"phoenix", "phoenix.token." + s.options.AuthToken}
	}

	// Establish WebSocket connection
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), headers)
	if err != nil {
		s.state = StateClosed
		s.triggerError(fmt.Errorf("connection failed: %w", err))
		s.scheduleReconnect()
		return err
	}

	s.conn = conn
	s.state = StateOpen
	s.connected = true
	s.reconnectTries = 0

	// Start goroutines for handling connection
	go s.readPump()
	go s.heartbeatLoop()

	// Flush send buffer
	s.flushSendBuffer()

	// Trigger open callbacks
	s.triggerOpen()

	s.log("Connected to %s", u.String())

	return nil
}

// Disconnect closes the WebSocket connection
func (s *Socket) Disconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateClosed {
		return
	}

	s.disconnect(false)
}

// disconnect performs the actual disconnection (must be called with lock held)
func (s *Socket) disconnect(reconnect bool) {
	if s.state == StateClosed {
		return
	}

	s.state = StateClosing
	s.connected = false

	// Cancel heartbeat
	if s.heartbeatCancel != nil {
		s.heartbeatCancel()
	}

	// Cancel reconnection if not reconnecting
	if !reconnect && s.reconnectCancel != nil {
		s.reconnectCancel()
	}

	// Close connection
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}

	s.state = StateClosed

	// Trigger close callbacks
	s.triggerClose()

	s.log("Disconnected")

	// Schedule reconnection if enabled and not manually disconnected
	if reconnect && s.options.ReconnectEnabled {
		s.scheduleReconnect()
	}
}

// IsConnected returns true if the socket is connected
func (s *Socket) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected
}

// ConnectionState returns the current connection state
func (s *Socket) ConnectionState() SocketState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// Channel creates a new channel for the given topic
func (s *Socket) Channel(topic string, params map[string]interface{}) *Channel {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if channel already exists
	for _, ch := range s.channels {
		if ch.topic == topic {
			return ch
		}
	}

	// Create new channel
	ch := newChannel(topic, params, s)
	s.channels = append(s.channels, ch)

	return ch
}

// removeChannel removes a channel from the socket
func (s *Socket) removeChannel(channel *Channel) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, ch := range s.channels {
		if ch == channel {
			s.channels = append(s.channels[:i], s.channels[i+1:]...)
			break
		}
	}
}

// Push sends a message to the server
func (s *Socket) Push(msg *Message) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.options.Logger != nil {
		s.options.Logger.Printf("Push: %s %s (%s, %s)", msg.Topic, msg.Event, msg.JoinRef, msg.Ref)
	}

	if s.connected && s.conn != nil {
		s.sendMessage(msg)
	} else {
		// Buffer the message for later sending
		s.sendBuffer = append(s.sendBuffer, func() {
			s.sendMessage(msg)
		})
	}
}

// sendMessage sends a message immediately (must be called with lock held)
func (s *Socket) sendMessage(msg *Message) {
	data, err := s.serializer.Encode(msg)
	if err != nil {
		s.triggerError(fmt.Errorf("failed to encode message: %w", err))
		return
	}

	messageType := websocket.TextMessage
	if msg.IsBinary() {
		messageType = websocket.BinaryMessage
	}

	if err := s.conn.WriteMessage(messageType, data); err != nil {
		s.triggerError(fmt.Errorf("failed to send message: %w", err))
	}
}

// MakeRef generates a unique reference
func (s *Socket) MakeRef() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ref++
	if s.ref == 0 { // overflow protection
		s.ref = 1
	}

	return fmt.Sprintf("%d", s.ref)
}

// flushSendBuffer sends all buffered messages (must be called with lock held)
func (s *Socket) flushSendBuffer() {
	if s.connected && len(s.sendBuffer) > 0 {
		for _, sendFunc := range s.sendBuffer {
			sendFunc()
		}
		s.sendBuffer = s.sendBuffer[:0] // clear buffer
	}
}

// readPump handles incoming messages
func (s *Socket) readPump() {
	defer func() {
		s.mu.Lock()
		s.disconnect(true)
		s.mu.Unlock()
	}()

	for {
		_, data, err := s.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				s.triggerError(fmt.Errorf("websocket error: %w", err))
			}
			return
		}

		msg, err := s.serializer.Decode(data)
		if err != nil {
			s.triggerError(fmt.Errorf("failed to decode message: %w", err))
			continue
		}

		s.handleMessage(msg)
	}
}

// handleMessage processes incoming messages
func (s *Socket) handleMessage(msg *Message) {
	// Handle heartbeat responses
	if msg.Topic == "phoenix" && msg.Event == "phx_reply" && msg.Ref == s.pendingHeartbeat {
		s.mu.Lock()
		s.pendingHeartbeat = ""
		if s.heartbeatTimeout != nil {
			s.heartbeatTimeout.Stop()
		}
		s.mu.Unlock()
		return
	}

	if s.options.Logger != nil {
		status := ""
		if replyPayload, err := GetReplyPayload(msg); err == nil {
			status = replyPayload.Status + " "
		}
		s.options.Logger.Printf("Receive: %s%s %s %s", status, msg.Topic, msg.Event, msg.Ref)
	}

	// Route message to appropriate channel
	s.mu.RLock()
	for _, ch := range s.channels {
		if ch.isMember(msg) {
			ch.trigger(msg.Event, msg.Payload, msg.Ref, msg.JoinRef)
		}
	}
	s.mu.RUnlock()

	// Trigger message callbacks
	for _, callback := range s.stateChangeCallbacks.message {
		callback(msg)
	}
}

// heartbeatLoop manages heartbeat messages
func (s *Socket) heartbeatLoop() {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.heartbeatCancel = cancel
	s.mu.Unlock()

	ticker := time.NewTicker(s.options.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendHeartbeat()
		}
	}
}

// sendHeartbeat sends a heartbeat message
func (s *Socket) sendHeartbeat() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return
	}

	// Check if previous heartbeat is still pending
	if s.pendingHeartbeat != "" {
		s.log("Heartbeat timeout - disconnecting")
		s.disconnect(true)
		return
	}

	ref := s.makeRef()
	s.pendingHeartbeat = ref

	msg := &Message{
		Topic:   "phoenix",
		Event:   "heartbeat",
		Payload: map[string]interface{}{},
		Ref:     ref,
	}

	s.sendMessage(msg)

	// Set heartbeat timeout
	s.heartbeatTimeout = time.AfterFunc(s.options.HeartbeatInterval, func() {
		s.mu.Lock()
		if s.pendingHeartbeat == ref {
			s.log("Heartbeat timeout - disconnecting")
			s.disconnect(true)
		}
		s.mu.Unlock()
	})
}

// makeRef generates a reference without locking (must be called with lock held)
func (s *Socket) makeRef() string {
	s.ref++
	if s.ref == 0 {
		s.ref = 1
	}
	return fmt.Sprintf("%d", s.ref)
}

// scheduleReconnect schedules a reconnection attempt
func (s *Socket) scheduleReconnect() {
	if !s.options.ReconnectEnabled {
		return
	}

	if s.options.MaxReconnectAttempts > 0 && s.reconnectTries >= s.options.MaxReconnectAttempts {
		s.log("Max reconnect attempts reached")
		return
	}

	s.reconnectTries++
	delay := s.options.ReconnectAfterMs(s.reconnectTries)

	s.log("Scheduling reconnect attempt %d in %v", s.reconnectTries, delay)

	ctx, cancel := context.WithCancel(context.Background())
	s.reconnectCtx = ctx
	s.reconnectCancel = cancel

	s.reconnectTimer = time.AfterFunc(delay, func() {
		select {
		case <-ctx.Done():
			return
		default:
			s.mu.Lock()
			if s.state == StateClosed {
				s.log("Attempting reconnect...")
				s.connect()
			}
			s.mu.Unlock()
		}
	})
}

// Event handler registration methods

// OnOpen registers a callback for connection open events
func (s *Socket) OnOpen(callback StateChangeCallback) {
	s.stateChangeCallbacks.open = append(s.stateChangeCallbacks.open, callback)
}

// OnClose registers a callback for connection close events
func (s *Socket) OnClose(callback StateChangeCallback) {
	s.stateChangeCallbacks.close = append(s.stateChangeCallbacks.close, callback)
}

// OnError registers a callback for error events
func (s *Socket) OnError(callback ErrorCallback) {
	s.stateChangeCallbacks.error = append(s.stateChangeCallbacks.error, callback)
}

// OnMessage registers a callback for all messages
func (s *Socket) OnMessage(callback func(*Message)) {
	s.stateChangeCallbacks.message = append(s.stateChangeCallbacks.message, callback)
}

// Event triggering methods

func (s *Socket) triggerOpen() {
	for _, callback := range s.stateChangeCallbacks.open {
		callback()
	}
}

func (s *Socket) triggerClose() {
	for _, callback := range s.stateChangeCallbacks.close {
		callback()
	}
}

func (s *Socket) triggerError(err error) {
	for _, callback := range s.stateChangeCallbacks.error {
		callback(err)
	}
}

// log writes a log message if logger is configured
func (s *Socket) log(format string, args ...interface{}) {
	if s.options.Logger != nil {
		s.options.Logger.Printf(format, args...)
	}
}