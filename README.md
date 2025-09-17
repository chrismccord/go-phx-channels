# Go Phoenix Channels Client

A complete Go client library for Phoenix Framework Channels, providing real-time bidirectional communication with Phoenix servers over WebSockets. This library closely mirrors the JavaScript Phoenix client API while leveraging Go's type safety and performance.

## Features

- **Real-time WebSocket communication** with Phoenix servers
- **Familiar API** similar to the JavaScript Phoenix client
- **Binary message support** with Phoenix's binary protocol
- **Automatic reconnection** with configurable backoff strategies
- **Channel management** with join/leave/push operations
- **Event handling** with callback registration
- **Request/response patterns** with receive callbacks
- **Thread-safe** operations for concurrent use
- **Comprehensive error handling** and timeout support
- **Configurable logging** and debugging support

## Installation

```bash
go get github.com/go-phx-channels
```

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    "os"
    "time"

    phx "github.com/go-phx-channels"
)

func main() {
    // Create socket with options
    socket := phx.NewSocket("ws://localhost:4000/socket/websocket", &phx.SocketOptions{
        Logger:  log.New(os.Stdout, "Phoenix: ", log.LstdFlags),
        Timeout: 10 * time.Second,
    })

    // Connect to the server
    if err := socket.Connect(); err != nil {
        log.Fatal("Failed to connect:", err)
    }
    defer socket.Disconnect()

    // Create and join a channel
    channel := socket.Channel("room:lobby", map[string]interface{}{
        "user_id": "123",
    })

    // Set up event handlers
    channel.On("new_message", func(payload interface{}) {
        fmt.Printf("Received message: %v\n", payload)
    })

    // Join the channel
    join := channel.Join()
    join.Receive("ok", func(resp interface{}) {
        fmt.Println("Successfully joined the lobby")

        // Send a message after joining
        push := channel.Push("new_message", map[string]interface{}{
            "body": "Hello from Go!",
            "user": "go_client",
        })

        push.Receive("ok", func(resp interface{}) {
            fmt.Println("Message sent successfully")
        }).Receive("error", func(reason interface{}) {
            fmt.Printf("Failed to send message: %v\n", reason)
        })

    }).Receive("error", func(reason interface{}) {
        fmt.Printf("Failed to join channel: %v\n", reason)
    })

    // Keep the program running
    select {}
}
```

## API Reference

### Socket

#### Creating a Socket

```go
socket := phx.NewSocket(endpoint, options)
```

**Parameters:**
- `endpoint` (string): WebSocket URL (e.g., "ws://localhost:4000/socket/websocket")
- `options` (*SocketOptions): Configuration options (can be nil for defaults)

#### Socket Options

```go
type SocketOptions struct {
    Timeout           time.Duration // Push timeout (default: 10s)
    HeartbeatInterval time.Duration // Heartbeat interval (default: 30s)
    Logger            *log.Logger   // Custom logger
    ReconnectEnabled  bool          // Enable auto-reconnection (default: true)
    MaxReconnectAttempts int        // Max reconnection attempts (0 = unlimited)
    ReconnectAfterMs  func(int) time.Duration // Custom backoff strategy
    Params            map[string]interface{}  // Connection parameters
    VSN               string        // Protocol version (default: "2.0.0")
    AuthToken         string        // Authentication token
}
```

#### Socket Methods

**Connection Management:**
```go
err := socket.Connect()           // Connect to server
socket.Disconnect()               // Disconnect from server
connected := socket.IsConnected() // Check connection status
state := socket.ConnectionState() // Get connection state
```

**Channel Management:**
```go
channel := socket.Channel(topic, params) // Create/get channel
ref := socket.MakeRef()                  // Generate unique reference
```

**Event Handlers:**
```go
socket.OnOpen(func() { /* connected */ })
socket.OnClose(func() { /* disconnected */ })
socket.OnError(func(err error) { /* error occurred */ })
socket.OnMessage(func(msg *Message) { /* message received */ })
```

### Channel

#### Creating Channels

```go
channel := socket.Channel("room:lobby", map[string]interface{}{
    "user_id": 123,
})
```

#### Channel Methods

**Lifecycle:**
```go
push := channel.Join()                    // Join channel
push := channel.Leave()                   // Leave channel
state := channel.GetState()               // Get channel state
topic := channel.Topic()                  // Get channel topic
joinRef := channel.JoinRef()              // Get join reference
```

**State Queries:**
```go
joined := channel.IsJoined()   // Check if joined
joining := channel.IsJoining() // Check if joining
leaving := channel.IsLeaving() // Check if leaving
closed := channel.IsClosed()   // Check if closed
errored := channel.IsErrored() // Check if errored
```

**Messaging:**
```go
push := channel.Push(event, payload)      // Send message
push := channel.Push(event, payload, timeout) // Send with custom timeout
```

**Event Handling:**
```go
ref := channel.On(event, callback)        // Register event handler
channel.Off(event)                       // Remove all handlers for event
channel.Off(event, ref)                  // Remove specific handler
```

### Push (Request/Response)

```go
push := channel.Push("event_name", payload)

push.Receive("ok", func(response interface{}) {
    // Handle successful response
}).Receive("error", func(reason interface{}) {
    // Handle error response
}).Receive("timeout", func(resp interface{}) {
    // Handle timeout
})
```

#### Push Methods

```go
push.Receive(status, callback)  // Register response handler
sent := push.IsSent()          // Check if sent
resp := push.GetResponse()     // Get received response
hasResp := push.HasReceived(status) // Check for specific response
```

### Binary Data

#### Creating Binary Payloads

```go
data := []byte{0x01, 0x02, 0x03, 0x04}
payload := phx.NewBinaryPayload(data)

push := channel.Push("binary_event", payload)
```

#### Handling Binary Messages

```go
channel.On("binary_message", func(payload interface{}) {
    if binaryPayload, ok := payload.(phx.BinaryPayload); ok {
        fmt.Printf("Received binary data: %v\n", binaryPayload.Data)
    }
})
```

## Examples

### Basic Chat Application

```go
package main

import (
    "bufio"
    "fmt"
    "log"
    "os"
    "strings"

    phx "github.com/go-phx-channels"
)

func main() {
    socket := phx.NewSocket("ws://localhost:4000/socket/websocket", nil)

    if err := socket.Connect(); err != nil {
        log.Fatal(err)
    }
    defer socket.Disconnect()

    channel := socket.Channel("room:general", nil)

    // Handle incoming messages
    channel.On("new_message", func(payload interface{}) {
        if msg, ok := payload.(map[string]interface{}); ok {
            fmt.Printf("%s: %s\n", msg["user"], msg["body"])
        }
    })

    // Join channel
    join := channel.Join()
    join.Receive("ok", func(resp interface{}) {
        fmt.Println("Connected to chat room")
    })

    // Read and send messages
    scanner := bufio.NewScanner(os.Stdin)
    for scanner.Scan() {
        text := strings.TrimSpace(scanner.Text())
        if text == "" {
            continue
        }

        push := channel.Push("new_message", map[string]interface{}{
            "body": text,
            "user": "go_user",
        })
        push.Receive("error", func(reason interface{}) {
            fmt.Printf("Error: %v\n", reason)
        })
    }
}
```

### Binary Data Handling

```go
package main

import (
    "fmt"
    "log"

    phx "github.com/go-phx-channels"
)

func main() {
    socket := phx.NewSocket("ws://localhost:4000/socket/websocket", nil)

    if err := socket.Connect(); err != nil {
        log.Fatal(err)
    }
    defer socket.Disconnect()

    channel := socket.Channel("binary:data", nil)

    // Handle binary messages
    channel.On("binary_response", func(payload interface{}) {
        if binary, ok := payload.(phx.BinaryPayload); ok {
            fmt.Printf("Received %d bytes: %v\n", len(binary.Data), binary.Data)
        }
    })

    // Join and send binary data
    join := channel.Join()
    join.Receive("ok", func(resp interface{}) {
        // Send binary data
        data := []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f} // "Hello"
        binaryPayload := phx.NewBinaryPayload(data)

        push := channel.Push("process_binary", binaryPayload)
        push.Receive("ok", func(resp interface{}) {
            fmt.Println("Binary data processed")
        })
    })

    select {} // Keep running
}
```

### Error Handling and Reconnection

```go
package main

import (
    "fmt"
    "log"
    "os"
    "time"

    phx "github.com/go-phx-channels"
)

func main() {
    options := &phx.SocketOptions{
        Logger:               log.New(os.Stdout, "Phoenix: ", log.LstdFlags),
        ReconnectEnabled:     true,
        MaxReconnectAttempts: 5,
        ReconnectAfterMs: func(tries int) time.Duration {
            // Custom backoff: 1s, 2s, 4s, 8s, 16s
            delay := time.Duration(1<<uint(tries-1)) * time.Second
            if delay > 16*time.Second {
                delay = 16 * time.Second
            }
            return delay
        },
    }

    socket := phx.NewSocket("ws://localhost:4000/socket/websocket", options)

    // Connection event handlers
    socket.OnOpen(func() {
        fmt.Println("Connected to Phoenix server")
    })

    socket.OnClose(func() {
        fmt.Println("Disconnected from Phoenix server")
    })

    socket.OnError(func(err error) {
        fmt.Printf("Socket error: %v\n", err)
    })

    if err := socket.Connect(); err != nil {
        log.Fatal(err)
    }
    defer socket.Disconnect()

    channel := socket.Channel("room:lobby", nil)

    // Channel error handling
    channel.On("phx_error", func(payload interface{}) {
        fmt.Printf("Channel error: %v\n", payload)
    })

    join := channel.Join()
    join.Receive("ok", func(resp interface{}) {
        fmt.Println("Joined channel successfully")
    }).Receive("error", func(reason interface{}) {
        fmt.Printf("Failed to join: %v\n", reason)
    }).Receive("timeout", func(resp interface{}) {
        fmt.Println("Join timed out")
    })

    select {}
}
```

## Phoenix Server Setup

To test the client, you'll need a Phoenix server with channels configured:

### Socket Configuration (endpoint.ex)

```elixir
socket "/socket", MyAppWeb.UserSocket,
  websocket: true,
  longpoll: false
```

### User Socket (user_socket.ex)

```elixir
defmodule MyAppWeb.UserSocket do
  use Phoenix.Socket

  channel "room:*", MyAppWeb.RoomChannel

  def connect(_params, socket, _connect_info) do
    {:ok, socket}
  end

  def id(_socket), do: nil
end
```

### Channel Implementation (room_channel.ex)

```elixir
defmodule MyAppWeb.RoomChannel do
  use Phoenix.Channel

  def join("room:" <> _room_id, _payload, socket) do
    {:ok, socket}
  end

  def handle_in("new_message", payload, socket) do
    broadcast!(socket, "new_message", payload)
    {:reply, {:ok, %{status: "sent"}}, socket}
  end

  def handle_in("binary_event", {:binary, data}, socket) do
    # Echo binary data back
    {:reply, {:ok, {:binary, data}}, socket}
  end
end
```

## Testing

This project includes comprehensive unit and integration tests.

### Unit Tests Only

Run unit tests without requiring a Phoenix server:

```bash
go test -short
```

### Integration Tests

Integration tests require a running Phoenix server on `localhost:4000`. The project includes a complete Phoenix test server for this purpose.

#### Quick Start - All Tests

1. **Start the test server** (in one terminal):
   ```bash
   cd test_server
   mix deps.get    # Install dependencies (first time only)
   mix phx.server  # Start server on localhost:4000
   ```

2. **Run all tests** (in another terminal):
   ```bash
   go test -v
   ```

#### Integration Tests Only

Run only the integration tests:

```bash
go test -v -run TestIntegration
```

#### Individual Test Categories

```bash
# Test specific functionality
go test -v -run TestIntegrationTimeout
go test -v -run TestIntegrationBinaryMessages
go test -v -run TestIntegrationReconnection
```

### Test Coverage

The test suite covers:
- Socket connection/disconnection
- Channel join/leave operations
- Push/receive message patterns
- Binary message handling (base64 encoded over JSON)
- Automatic reconnection
- Error handling and timeouts
- Multi-client broadcast scenarios

## Development

### Building

```bash
go build ./...
```

### Running Examples

```bash
# Basic usage
cd examples/basic_usage && go run main.go

# Binary data
cd examples/binary_data && go run main.go

# Chat room (interactive)
cd examples/chat_room && go run main.go <username>
```

### Using Make

The project includes a Makefile for common tasks:

```bash
make help          # Show available commands
make test          # Run all tests
make test-unit     # Run unit tests only
make server        # Start Phoenix test server
make examples      # Run example applications
make lint          # Run linting
make clean         # Clean build artifacts
```

## Protocol Compatibility

This client implements the Phoenix WebSocket protocol and is compatible with:

- Phoenix Framework 1.7+
- Phoenix Channels protocol versions 1.0.0 and 2.0.0
- Binary message format (Phoenix.Socket.Serializer)
- JSON message format

## Message Format

### JSON Messages

```json
[join_ref, ref, topic, event, payload]
```

### Binary Messages

Binary messages use Phoenix's binary protocol with the following structure:

```
[kind][meta_length][join_ref_length][ref_length][topic_length][event_length][join_ref][ref][topic][event][payload]
```

Supported message kinds:
- `0`: Push
- `1`: Reply
- `2`: Broadcast

## Contributing

Please read [CONTRIBUTING.md](CONTRIBUTING.md) for details on our code of conduct and the process for submitting pull requests.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Inspired by the Phoenix Framework JavaScript client
- Built for the Go community's real-time application needs
- Thanks to the Phoenix Framework team for the excellent protocol design