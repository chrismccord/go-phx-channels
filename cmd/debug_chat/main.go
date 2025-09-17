package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	phx "github.com/go-phx-channels"
)

func main() {
	var debug = flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	username := "user"
	if len(flag.Args()) >= 1 {
		username = flag.Args()[0]
	}

	var logger *log.Logger
	if *debug {
		logger = log.New(os.Stdout, fmt.Sprintf("[DEBUG-%s] ", username), log.LstdFlags|log.Lshortfile)
	}

	reconnectEnabled := true
	socket := phx.NewSocket("ws://localhost:4000/socket/websocket", &phx.SocketOptions{
		Logger:               logger,
		ReconnectEnabled:     &reconnectEnabled,
		MaxReconnectAttempts: 0, // Unlimited
	})

	connectionCount := 0

	socket.OnOpen(func() {
		connectionCount++
		fmt.Printf("🟢 Connected (#%d)\n", connectionCount)
	})

	if err := socket.Connect(); err != nil {
		log.Fatal("Failed to connect:", err)
	}

	channel := socket.Channel("room:lobby", nil)

	channel.On("new_message", func(payload interface{}) {
		if msg, ok := payload.(map[string]interface{}); ok {
			fmt.Printf("[%s]: %s\n", msg["user"], msg["body"])
		}
	})

	join := channel.Join()
	join.Receive("ok", func(resp interface{}) {
		fmt.Printf("✅ Joined as %s. Type messages:\n", username)
		if *debug {
			fmt.Println("🐛 Debug mode enabled - you'll see detailed connection logs")
			fmt.Println("💡 Kill server to test reconnection")
		}
	})

	// Monitor connection status if debug
	if *debug {
		go func() {
			for {
				time.Sleep(3 * time.Second)
				status := "🔴 DISCONNECTED"
				if socket.IsConnected() {
					status = "🟢 CONNECTED"
				}
				fmt.Printf("🔍 Status: %s (Connections: %d)\n", status, connectionCount)
			}
		}()
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "quit" || text == "exit" {
			break
		}
		if text != "" {
			channel.Push("new_message", map[string]interface{}{
				"body": text,
				"user": username,
			})
		}
	}

	socket.Disconnect()
}