# Go Phoenix Channels - Examples

This directory contains example applications demonstrating various features of the Go Phoenix Channels client library.

## Running the Examples

First, make sure you have a Phoenix server running. You can use the test server included in this project:

```bash
cd test_server
mix deps.get
mix phx.server
```

The server will start on `http://localhost:4000`.

## Available Examples

### 1. Basic Usage (`basic_usage/`)

Demonstrates the fundamental concepts of connecting to a Phoenix server, joining channels, and sending/receiving messages.

```bash
cd examples/basic_usage
go run main.go
```

**Features demonstrated:**
- Socket connection with options
- Event handlers for connection lifecycle
- Channel joining with success/error handling
- Sending messages with response callbacks

### 2. Binary Data (`binary_data/`)

Shows how to work with binary payloads using Phoenix's binary message protocol.

```bash
cd examples/binary_data
go run main.go
```

**Features demonstrated:**
- Creating and sending binary payloads
- Receiving binary messages from the server
- Handling large binary data transfers
- Binary message encoding/decoding

### 3. Chat Room (`chat_room/`)

A complete interactive chat application demonstrating real-time messaging.

```bash
cd examples/chat_room
go run main.go <username>
```

**Features demonstrated:**
- Real-time messaging between multiple clients
- User join/leave notifications
- Interactive command-line interface
- Graceful disconnect handling

Example usage:
```bash
# Terminal 1
go run main.go Alice

# Terminal 2
go run main.go Bob
```

## Phoenix Server Channels

The examples work with the following Phoenix channels defined in the test server:

### `room:*` - General purpose room channel
- **Events:**
  - `ping` - Echo server with pong response
  - `echo` - Push echo_reply event and respond with success
  - `broadcast` - Broadcast message to all channel subscribers
  - `new_message` - Chat message handling
  - `binary_echo` - Echo binary data back to sender
  - `push_binary` - Server pushes binary data to client

### `binary:*` - Binary data specialized channel
- **Events:**
  - `binary_data` - Process and return modified binary data
  - `large_binary` - Return large binary payload
  - `binary_broadcast` - Broadcast binary data to all subscribers

## Error Handling Examples

The examples include comprehensive error handling patterns:

- Connection errors and automatic reconnection
- Channel join failures
- Message sending failures
- Server-side errors and timeouts
- Network disconnections

## Advanced Features

### Custom Socket Options

```go
socket := phx.NewSocket("ws://localhost:4000/socket/websocket", &phx.SocketOptions{
    Timeout:           5 * time.Second,
    HeartbeatInterval: 30 * time.Second,
    Logger:            logger,
    ReconnectEnabled:  true,
    MaxReconnectAttempts: 5,
    Params: map[string]interface{}{
        "token": "user_auth_token",
    },
})
```

### Event Handler Management

```go
// Add event handler
ref := channel.On("user_message", handleMessage)

// Remove specific handler
channel.Off("user_message", ref)

// Remove all handlers for event
channel.Off("user_message")
```

### Binary Payload Handling

```go
// Create binary payload
data := []byte{0x01, 0x02, 0x03, 0x04}
payload := phx.NewBinaryPayload(data)

// Send binary message
push := channel.Push("binary_event", payload)

// Handle binary response
push.Receive("ok", func(resp interface{}) {
    if respMap, ok := resp.(map[string]interface{}); ok {
        if binaryResp, ok := respMap["response"].(phx.BinaryPayload); ok {
            fmt.Printf("Received binary: %v\n", binaryResp.Data)
        }
    }
})
```

## Testing

Run the examples against the test server to verify functionality:

```bash
# Start the Phoenix test server
cd test_server && mix phx.server

# In another terminal, run examples
cd examples/basic_usage && go run main.go
```

## Troubleshooting

### Connection Issues
- Ensure Phoenix server is running on the correct port
- Check WebSocket endpoint URL format
- Verify firewall/network settings

### Channel Join Failures
- Check channel route definition in Phoenix router
- Verify join payload format
- Review server logs for authorization issues

### Message Sending Issues
- Ensure channel is joined before sending messages
- Check message payload structure
- Verify server channel handles the event type

## Additional Resources

- [Phoenix Channels Guide](https://hexdocs.pm/phoenix/channels.html)
- [Phoenix WebSocket Protocol](https://hexdocs.pm/phoenix/Phoenix.Socket.Transport.html)
- [Go Phoenix Channels Documentation](../README.md)