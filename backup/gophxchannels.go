// Package gophxchannels provides a Go client library for Phoenix Framework Channels.
// It offers a comprehensive API similar to the JavaScript Phoenix client, enabling
// real-time bidirectional communication with Phoenix servers over WebSockets.
//
// Basic usage:
//
//	socket := gophxchannels.NewSocket("ws://localhost:4000/socket/websocket", nil)
//	if err := socket.Connect(); err != nil {
//		log.Fatal(err)
//	}
//	defer socket.Disconnect()
//
//	channel := socket.Channel("room:lobby", nil)
//	join := channel.Join()
//	join.Receive("ok", func(resp interface{}) {
//		fmt.Println("Joined successfully")
//	})
//
package gophxchannels

// Version of the library
const Version = "1.0.0"

// Common Phoenix channel events
const (
	EventJoin      = "phx_join"
	EventReply     = "phx_reply"
	EventLeave     = "phx_leave"
	EventClose     = "phx_close"
	EventError     = "phx_error"
	EventHeartbeat = "heartbeat"
)

// Common reply statuses
const (
	StatusOK      = "ok"
	StatusError   = "error"
	StatusTimeout = "timeout"
)


// NewBinaryPayload creates a binary payload for sending binary data through channels.
// Binary payloads are automatically encoded using Phoenix's binary protocol.
//
// Example:
//	binaryData := []byte{0x01, 0x02, 0x03, 0x04}
//	payload := NewBinaryPayload(binaryData)
//	channel.Push("binary_event", payload)
func NewBinaryPayload(data []byte) BinaryPayload {
	return BinaryPayload{Data: data}
}

// Common helper functions

// IsConnected is a convenience function to check if a socket is connected
func IsConnected(socket *Socket) bool {
	return socket.IsConnected()
}

// IsJoined is a convenience function to check if a channel is joined
func IsJoined(channel *Channel) bool {
	return channel.IsJoined()
}

// GetChannelState returns the current state of a channel
func GetChannelState(channel *Channel) ChannelState {
	return channel.GetState()
}

// GetSocketState returns the current state of a socket
func GetSocketState(socket *Socket) SocketState {
	return socket.ConnectionState()
}