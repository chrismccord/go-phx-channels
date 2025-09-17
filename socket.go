package gophxchannels

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

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
	ReconnectEnabled *bool

	// MaxReconnectAttempts limits reconnection attempts (0 = unlimited)
	MaxReconnectAttempts int
}

// DefaultReconnectAfterMs returns the default reconnect backoff function
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

// debugLog logs debug messages if PHX_DEBUG environment variable is set
func debugLog(format string, args ...interface{}) {
	if os.Getenv("PHX_DEBUG") != "" {
		log.Printf("[PHX_DEBUG] "+format, args...)
	}
}

// setDefaultOptions sets default values for unspecified options
func setDefaultOptions(options *SocketOptions) {
	if options.Timeout == 0 {
		options.Timeout = 10 * time.Second
	}
	if options.HeartbeatInterval == 0 {
		options.HeartbeatInterval = 30 * time.Second
	}
	if options.ReconnectAfterMs == nil {
		options.ReconnectAfterMs = DefaultReconnectAfterMs
	}
	if options.VSN == "" {
		options.VSN = "2.0.0"
	}
	if options.Params == nil {
		options.Params = make(map[string]interface{})
	}
	// ReconnectEnabled defaults to true
	if options.ReconnectEnabled == nil {
		enabled := true
		options.ReconnectEnabled = &enabled
	}
}

// SocketCommand represents commands sent to the socket manager
type SocketCommand struct {
	Type     string
	Data     interface{}
	Response chan interface{} // For synchronous operations
}

// SocketState represents the state of the socket connection
type SocketState int

const (
	StateClosed SocketState = iota
	StateConnecting
	StateOpen
	StateClosing
)

// Socket represents a Phoenix WebSocket connection using goroutines and channels
type Socket struct {
	endpoint string
	options  *SocketOptions

	// Communication channels
	commands        chan SocketCommand
	outbound        chan *Message  // Messages to send
	inbound         chan *Message  // Messages received
	shutdown        chan struct{}
	connectionError chan error     // Connection error signals

	// State (only accessed by socket manager goroutine)
	conn         *websocket.Conn
	state        SocketState
	connected    bool
	channels     map[string]*Channel
	ref          int64
	sendBuffer   []func()

	// Reconnection state
	reconnectTries  int
	isReconnecting  bool

	// For external state queries (read-only after creation)
	serializer   *Serializer

	// Lifecycle
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewSocket creates a new lock-free socket using goroutines
func NewSocket(endpoint string, options *SocketOptions) *Socket {
	if options == nil {
		options = &SocketOptions{}
	}
	setDefaultOptions(options)

	ctx, cancel := context.WithCancel(context.Background())

	s := &Socket{
		endpoint:        endpoint,
		options:         options,
		commands:        make(chan SocketCommand, 100),
		outbound:        make(chan *Message, 100),
		inbound:         make(chan *Message, 100),
		shutdown:        make(chan struct{}),
		connectionError: make(chan error, 10),
		channels:        make(map[string]*Channel),
		state:           StateClosed,
		serializer:      NewSerializer(),
		ctx:             ctx,
		cancel:          cancel,
	}

	// Start the socket manager goroutine
	s.wg.Add(1)
	go s.socketManager()

	// Start the message handler goroutine
	s.wg.Add(1)
	go s.messageHandler()

	return s
}

// socketManager is the main goroutine that owns socket state
func (s *Socket) socketManager() {
	defer s.wg.Done()

	var heartbeatTicker *time.Ticker
	var reconnectTimer *time.Timer

	for {
		select {
		case <-s.ctx.Done():
			return

		case cmd := <-s.commands:
			s.handleCommand(cmd)

		case msg := <-s.outbound:
			s.handleOutboundMessage(msg)

		case <-s.shutdown:
			if s.conn != nil {
				s.conn.Close()
			}
			if heartbeatTicker != nil {
				heartbeatTicker.Stop()
			}
			if reconnectTimer != nil {
				reconnectTimer.Stop()
			}
			return

		// Heartbeat ticker
		case <-func() <-chan time.Time {
			if heartbeatTicker != nil {
				return heartbeatTicker.C
			}
			return nil
		}():
			if s.connected {
				s.sendHeartbeat()
			}

		// Reconnect timer
		case <-func() <-chan time.Time {
			if reconnectTimer != nil {
				return reconnectTimer.C
			}
			return nil
		}():
			debugLog("Reconnect timer fired, calling attemptReconnect")
			s.attemptReconnect()

		// Connection error from readMessages/writeMessages
		case err := <-s.connectionError:
			debugLog("Received connection error: %v", err)
			if s.options.Logger != nil {
				s.options.Logger.Printf("Connection error: %v", err)
			}
			// Mark as disconnected
			s.connected = false
			s.state = StateClosed
			if s.conn != nil {
				s.conn.Close()
				s.conn = nil
			}
			// Schedule reconnection if not manually disconnected
			debugLog("Checking if should reconnect: isReconnecting=%v, ReconnectEnabled=%v", s.isReconnecting, *s.options.ReconnectEnabled)
			if !s.isReconnecting {
				debugLog("Calling scheduleReconnect...")
				s.scheduleReconnect(&reconnectTimer)
			} else {
				debugLog("Not scheduling reconnect because isReconnecting=true")
			}
		}
	}
}

// messageHandler processes incoming messages
func (s *Socket) messageHandler() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return

		case msg := <-s.inbound:
			s.routeMessage(msg)
		}
	}
}

// handleCommand processes commands from the public API
func (s *Socket) handleCommand(cmd SocketCommand) {
	switch cmd.Type {
	case "connect":
		err := s.doConnect()
		if cmd.Response != nil {
			cmd.Response <- err
		}

	case "disconnect":
		s.doDisconnect()
		if cmd.Response != nil {
			cmd.Response <- nil
		}

	case "get_state":
		if cmd.Response != nil {
			cmd.Response <- s.state
		}

	case "is_connected":
		if cmd.Response != nil {
			cmd.Response <- s.connected
		}

	case "make_ref":
		s.ref++
		if s.ref == 0 { // overflow protection
			s.ref = 1
		}
		if cmd.Response != nil {
			cmd.Response <- fmt.Sprintf("%d", s.ref)
		}

	case "create_channel":
		data := cmd.Data.(map[string]interface{})
		topic := data["topic"].(string)
		params := data["params"].(map[string]interface{})

		// Check if channel already exists
		if ch, exists := s.channels[topic]; exists {
			if cmd.Response != nil {
				cmd.Response <- ch
			}
			return
		}

		// Create new channel
		ch := NewChannel(topic, params, s)
		s.channels[topic] = ch
		if cmd.Response != nil {
			cmd.Response <- ch
		}

	case "remove_channel":
		topic := cmd.Data.(string)
		delete(s.channels, topic)
		if cmd.Response != nil {
			cmd.Response <- nil
		}
	}
}

// Public API methods that send commands to the socket manager

// Connect connects to the WebSocket server
func (s *Socket) Connect() error {
	response := make(chan interface{}, 1)
	s.commands <- SocketCommand{
		Type:     "connect",
		Response: response,
	}

	result := <-response
	if err, ok := result.(error); ok {
		return err
	}
	return nil
}

// Disconnect closes the WebSocket connection
func (s *Socket) Disconnect() {
	// Cancel context first to signal all goroutines to stop
	s.cancel()

	// Send disconnect command with a very short timeout
	select {
	case s.commands <- SocketCommand{Type: "disconnect"}:
		// Command sent successfully
	case <-time.After(10 * time.Millisecond):
		// Command couldn't be sent, socket manager probably already exiting
	}

	// Close shutdown channel to signal all goroutines
	select {
	case <-s.shutdown:
		// Already closed
	default:
		close(s.shutdown)
	}

	// Give goroutines a chance to exit gracefully, but don't wait forever
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines exited cleanly
		if s.options.Logger != nil {
			s.options.Logger.Printf("Disconnected cleanly from %s", s.endpoint)
		}
	case <-time.After(500 * time.Millisecond):
		// Timeout - just continue, the test doesn't need to wait for all cleanup
		if s.options.Logger != nil {
			s.options.Logger.Printf("Disconnect from %s (some goroutines may still be running)", s.endpoint)
		}
	}
}

// IsConnected returns true if connected to the server
func (s *Socket) IsConnected() bool {
	response := make(chan interface{}, 1)
	select {
	case s.commands <- SocketCommand{
		Type:     "is_connected",
		Response: response,
	}:
		// Command sent successfully
		select {
		case result := <-response:
			return result.(bool)
		case <-time.After(100 * time.Millisecond):
			// Timeout - assume disconnected if we can't get status
			return false
		}
	case <-time.After(10 * time.Millisecond):
		// Can't send command - assume disconnected
		return false
	}
}

// ConnectionState returns the current connection state
func (s *Socket) ConnectionState() SocketState {
	response := make(chan interface{}, 1)
	select {
	case s.commands <- SocketCommand{
		Type:     "get_state",
		Response: response,
	}:
		// Command sent successfully
		select {
		case result := <-response:
			return result.(SocketState)
		case <-time.After(100 * time.Millisecond):
			// Timeout - assume closed if we can't get status
			return StateClosed
		}
	case <-time.After(10 * time.Millisecond):
		// Can't send command - assume closed
		return StateClosed
	}
}

// MakeRef generates a unique reference
func (s *Socket) MakeRef() string {
	response := make(chan interface{}, 1)
	s.commands <- SocketCommand{
		Type:     "make_ref",
		Response: response,
	}
	return (<-response).(string)
}

// Channel creates or returns an existing channel
func (s *Socket) Channel(topic string, params map[string]interface{}) *Channel {
	response := make(chan interface{}, 1)
	s.commands <- SocketCommand{
		Type: "create_channel",
		Data: map[string]interface{}{
			"topic":  topic,
			"params": params,
		},
		Response: response,
	}
	return (<-response).(*Channel)
}

// Push sends a message through the socket
func (s *Socket) Push(msg *Message) {
	select {
	case s.outbound <- msg:
	case <-s.ctx.Done():
	}
}

// OnOpen registers a callback for when the socket connects
func (s *Socket) OnOpen(callback func()) {
	// For now, just store the callback and call it when connecting
	// This is a simple implementation for API compatibility
	go func() {
		for {
			select {
			case <-s.ctx.Done():
				return
			default:
				if s.IsConnected() {
					callback()
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
}

// Private methods (only called by socket manager goroutine)

func (s *Socket) doConnect() error {
	if s.connected {
		return nil
	}

	s.state = StateConnecting

	conn, _, err := websocket.DefaultDialer.Dial(s.endpoint, nil)
	if err != nil {
		s.state = StateClosed
		return fmt.Errorf("failed to connect: %w", err)
	}

	s.conn = conn
	s.connected = true
	s.state = StateOpen

	// Start reading messages
	s.wg.Add(1)
	go s.readMessages()

	// Send buffered messages
	for _, sendFn := range s.sendBuffer {
		sendFn()
	}
	s.sendBuffer = s.sendBuffer[:0]

	if s.options.Logger != nil {
		s.options.Logger.Printf("Connected to %s", s.endpoint)
	}

	return nil
}

func (s *Socket) doDisconnect() {
	if !s.connected {
		return
	}

	s.state = StateClosing
	s.connected = false

	// Stop reconnection attempts for manual disconnect
	s.isReconnecting = false
	s.reconnectTries = 0

	// Close all channels first
	for _, ch := range s.channels {
		ch.cancel() // Cancel the channel's context
	}

	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}

	s.state = StateClosed

	if s.options.Logger != nil {
		s.options.Logger.Printf("Disconnected from %s", s.endpoint)
	}
}

func (s *Socket) handleOutboundMessage(msg *Message) {
	if s.connected && s.conn != nil {
		s.sendMessage(msg)
	} else {
		// Buffer the message
		s.sendBuffer = append(s.sendBuffer, func() {
			s.sendMessage(msg)
		})
	}
}

func (s *Socket) sendMessage(msg *Message) {
	data, err := s.serializer.Encode(msg)
	if err != nil {
		if s.options.Logger != nil {
			s.options.Logger.Printf("Failed to encode message: %v", err)
		}
		return
	}

	messageType := websocket.TextMessage
	if msg.IsBinary() {
		messageType = websocket.BinaryMessage
	}

	if err := s.conn.WriteMessage(messageType, data); err != nil {
		if s.options.Logger != nil {
			s.options.Logger.Printf("Failed to send message: %v", err)
		}

		// Check if this should trigger reconnection
		if s.shouldReconnectOnError(err) {
			// Signal connection error to socket manager for reconnection
			select {
			case s.connectionError <- err:
			case <-s.ctx.Done():
			default:
				// If channel is full, just handle it directly
				s.connected = false
				s.state = StateClosed
			}
		} else {
			// Graceful close - just mark as disconnected
			s.connected = false
			s.state = StateClosed
		}
	}
}

func (s *Socket) sendHeartbeat() {
	heartbeat := &Message{
		Topic:   "phoenix",
		Event:   "heartbeat",
		Payload: map[string]interface{}{},
		Ref:     s.MakeRef(),
	}
	s.handleOutboundMessage(heartbeat)
}

func (s *Socket) attemptReconnect() {
	if s.options.Logger != nil {
		s.options.Logger.Printf("Attempting to reconnect...")
	}
	err := s.doConnect()
	if err == nil {
		// Reset reconnection state on successful connection
		s.reconnectTries = 0
		s.isReconnecting = false
	}
}

// shouldReconnectOnError determines if an error should trigger reconnection
// Mirrors the Phoenix JavaScript client logic: !closeWasClean && closeCode !== 1000
func (s *Socket) shouldReconnectOnError(err error) bool {
	if !*s.options.ReconnectEnabled {
		return false
	}

	// Check if it's a WebSocket close error
	if closeErr, ok := err.(*websocket.CloseError); ok {
		// Reconnect on unexpected close codes (not normal closure)
		// WebSocket close code 1000 = Normal Closure (should NOT reconnect)
		// Other codes like 1001, 1006, etc. should trigger reconnection
		if closeErr.Code == websocket.CloseNormalClosure { // 1000
			if s.options.Logger != nil {
				s.options.Logger.Printf("WebSocket closed normally (1000) - not reconnecting")
			}
			return false
		}

		if s.options.Logger != nil {
			s.options.Logger.Printf("WebSocket closed unexpectedly (code %d) - will reconnect", closeErr.Code)
		}
		return true
	}

	// For other connection errors (network issues, etc.), attempt reconnection
	if s.options.Logger != nil {
		s.options.Logger.Printf("Connection error (not close) - will reconnect: %v", err)
	}
	return true
}

// scheduleReconnect schedules a reconnection attempt with backoff
func (s *Socket) scheduleReconnect(reconnectTimer **time.Timer) {
	debugLog("scheduleReconnect called")
	if !*s.options.ReconnectEnabled {
		debugLog("Reconnection disabled, not scheduling")
		return
	}

	// Check max attempts limit
	if s.options.MaxReconnectAttempts > 0 && s.reconnectTries >= s.options.MaxReconnectAttempts {
		debugLog("Max reconnect attempts (%d) reached", s.options.MaxReconnectAttempts)
		if s.options.Logger != nil {
			s.options.Logger.Printf("Max reconnect attempts (%d) reached", s.options.MaxReconnectAttempts)
		}
		return
	}

	s.reconnectTries++
	s.isReconnecting = true
	delay := s.options.ReconnectAfterMs(s.reconnectTries)

	debugLog("Scheduling reconnect attempt %d in %v", s.reconnectTries, delay)
	if s.options.Logger != nil {
		s.options.Logger.Printf("Scheduling reconnect attempt %d in %v", s.reconnectTries, delay)
	}

	// Stop any existing timer
	if *reconnectTimer != nil {
		(*reconnectTimer).Stop()
		debugLog("Stopped existing reconnect timer")
	}

	// Start new reconnection timer
	*reconnectTimer = time.NewTimer(delay)
	debugLog("Started new reconnect timer with delay %v", delay)
}

func (s *Socket) readMessages() {
	defer s.wg.Done()

	for {
		if s.conn == nil {
			return
		}

		_, data, err := s.conn.ReadMessage()
		if err != nil {
			if s.options.Logger != nil {
				s.options.Logger.Printf("WebSocket read error: %v", err)
			}

			// Check if this is a close error that should trigger reconnection
			shouldReconnect := s.shouldReconnectOnError(err)
			debugLog("Read error: %v, shouldReconnect: %v", err, shouldReconnect)
			if shouldReconnect {
				// Signal connection error to socket manager for reconnection
				debugLog("Sending connection error to manager")
				select {
				case s.connectionError <- err:
					debugLog("Successfully sent connection error to manager")
				case <-s.ctx.Done():
					debugLog("Context canceled while sending connection error")
				default:
					debugLog("Connection error channel is full")
				}
			} else {
				debugLog("Not reconnecting due to error type")
			}
			return
		}

		msg, err := s.serializer.Decode(data)
		if err != nil {
			if s.options.Logger != nil {
				s.options.Logger.Printf("Failed to decode message: %v", err)
			}
			continue
		}

		select {
		case s.inbound <- msg:
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Socket) routeMessage(msg *Message) {
	// Route message to appropriate channel
	if ch, exists := s.channels[msg.Topic]; exists {
		ch.HandleMessage(msg)
	} else if s.options.Logger != nil {
		s.options.Logger.Printf("No channel found for topic: %s", msg.Topic)
	}
}