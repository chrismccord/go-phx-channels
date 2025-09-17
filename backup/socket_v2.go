package gophxchannels

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

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
type SocketV2 struct {
	endpoint string
	options  *SocketOptions

	// Communication channels
	commands     chan SocketCommand
	outbound     chan *Message  // Messages to send
	inbound      chan *Message  // Messages received
	shutdown     chan struct{}

	// State (only accessed by socket manager goroutine)
	conn         *websocket.Conn
	state        SocketState
	connected    bool
	channels     map[string]*ChannelV2
	ref          int64
	sendBuffer   []func()

	// For external state queries (read-only after creation)
	serializer   *Serializer

	// Lifecycle
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewSocketV2 creates a new lock-free socket using goroutines
func NewSocketV2(endpoint string, options *SocketOptions) *SocketV2 {
	if options == nil {
		options = &SocketOptions{}
	}
	setDefaultOptions(options)

	ctx, cancel := context.WithCancel(context.Background())

	s := &SocketV2{
		endpoint:   endpoint,
		options:    options,
		commands:   make(chan SocketCommand, 100),
		outbound:   make(chan *Message, 100),
		inbound:    make(chan *Message, 100),
		shutdown:   make(chan struct{}),
		channels:   make(map[string]*ChannelV2),
		state:      StateClosed,
		serializer: NewSerializer(),
		ctx:        ctx,
		cancel:     cancel,
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
func (s *SocketV2) socketManager() {
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
			s.attemptReconnect()
		}
	}
}

// messageHandler processes incoming messages
func (s *SocketV2) messageHandler() {
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
func (s *SocketV2) handleCommand(cmd SocketCommand) {
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
		ch := NewChannelV2(topic, params, s)
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
func (s *SocketV2) Connect() error {
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
func (s *SocketV2) Disconnect() {
	response := make(chan interface{}, 1)
	s.commands <- SocketCommand{
		Type:     "disconnect",
		Response: response,
	}
	<-response

	// Shutdown everything
	close(s.shutdown)
	s.cancel()
	s.wg.Wait()
}

// IsConnected returns true if connected to the server
func (s *SocketV2) IsConnected() bool {
	response := make(chan interface{}, 1)
	s.commands <- SocketCommand{
		Type:     "is_connected",
		Response: response,
	}
	return (<-response).(bool)
}

// ConnectionState returns the current connection state
func (s *SocketV2) ConnectionState() SocketState {
	response := make(chan interface{}, 1)
	s.commands <- SocketCommand{
		Type:     "get_state",
		Response: response,
	}
	return (<-response).(SocketState)
}

// MakeRef generates a unique reference
func (s *SocketV2) MakeRef() string {
	response := make(chan interface{}, 1)
	s.commands <- SocketCommand{
		Type:     "make_ref",
		Response: response,
	}
	return (<-response).(string)
}

// Channel creates or returns an existing channel
func (s *SocketV2) Channel(topic string, params map[string]interface{}) *ChannelV2 {
	response := make(chan interface{}, 1)
	s.commands <- SocketCommand{
		Type: "create_channel",
		Data: map[string]interface{}{
			"topic":  topic,
			"params": params,
		},
		Response: response,
	}
	return (<-response).(*ChannelV2)
}

// Push sends a message through the socket
func (s *SocketV2) Push(msg *Message) {
	select {
	case s.outbound <- msg:
	case <-s.ctx.Done():
	}
}

// Private methods (only called by socket manager goroutine)

func (s *SocketV2) doConnect() error {
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

func (s *SocketV2) doDisconnect() {
	if !s.connected {
		return
	}

	s.state = StateClosing
	s.connected = false

	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}

	s.state = StateClosed

	if s.options.Logger != nil {
		s.options.Logger.Printf("Disconnected from %s", s.endpoint)
	}
}

func (s *SocketV2) handleOutboundMessage(msg *Message) {
	if s.connected && s.conn != nil {
		s.sendMessage(msg)
	} else {
		// Buffer the message
		s.sendBuffer = append(s.sendBuffer, func() {
			s.sendMessage(msg)
		})
	}
}

func (s *SocketV2) sendMessage(msg *Message) {
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
		// Handle connection error
		s.connected = false
		s.state = StateClosed
	}
}

func (s *SocketV2) sendHeartbeat() {
	heartbeat := &Message{
		Topic:   "phoenix",
		Event:   "heartbeat",
		Payload: map[string]interface{}{},
		Ref:     s.MakeRef(),
	}
	s.handleOutboundMessage(heartbeat)
}

func (s *SocketV2) attemptReconnect() {
	if s.options.Logger != nil {
		s.options.Logger.Printf("Attempting to reconnect...")
	}
	s.doConnect()
}

func (s *SocketV2) readMessages() {
	defer s.wg.Done()

	for {
		if s.conn == nil {
			return
		}

		messageType, data, err := s.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				if s.options.Logger != nil {
					s.options.Logger.Printf("WebSocket error: %v", err)
				}
			}
			return
		}

		msg, err := s.serializer.Decode(data, messageType == websocket.BinaryMessage)
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

func (s *SocketV2) routeMessage(msg *Message) {
	// Route message to appropriate channel
	if ch, exists := s.channels[msg.Topic]; exists {
		ch.HandleMessage(msg)
	} else if s.options.Logger != nil {
		s.options.Logger.Printf("No channel found for topic: %s", msg.Topic)
	}
}