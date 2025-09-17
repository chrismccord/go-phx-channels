package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	phx "github.com/go-phx-channels"
)

func main() {
	username := "debug"
	if len(os.Args) >= 2 {
		username = os.Args[1]
	}

	fmt.Printf("🔧 Debug Reconnection Test - User: %s\n", username)
	fmt.Println("📋 This tool will help debug automatic reconnection behavior")
	fmt.Println()

	// Enable detailed logging and reconnection
	reconnectEnabled := true
	socket := phx.NewSocket("ws://localhost:4000/socket/websocket", &phx.SocketOptions{
		Logger:               log.New(os.Stdout, fmt.Sprintf("[%s] ", username), log.LstdFlags),
		ReconnectEnabled:     &reconnectEnabled,
		MaxReconnectAttempts: 0, // Unlimited
	})

	connectionCount := 0

	socket.OnOpen(func() {
		connectionCount++
		fmt.Printf("🟢 CONNECTION #%d - Socket connected!\n", connectionCount)
	})

	fmt.Println("🔌 Attempting initial connection...")
	if err := socket.Connect(); err != nil {
		log.Fatal("❌ Failed to connect:", err)
	}
	defer socket.Disconnect()

	channel := socket.Channel("room:debug", nil)

	channel.On("new_message", func(payload interface{}) {
		if msg, ok := payload.(map[string]interface{}); ok {
			fmt.Printf("💬 [%s]: %s\n", msg["user"], msg["body"])
		}
	})

	join := channel.Join()
	join.Receive("ok", func(resp interface{}) {
		fmt.Printf("✅ Joined room as %s\n", username)
		fmt.Println()
		fmt.Println("🧪 TESTING INSTRUCTIONS:")
		fmt.Println("1. Type messages normally")
		fmt.Println("2. Kill the Phoenix server (Ctrl+C in server terminal)")
		fmt.Println("3. Watch the logs for reconnection attempts")
		fmt.Println("4. Restart the Phoenix server")
		fmt.Println("5. See if it reconnects automatically")
		fmt.Println()
		fmt.Println("💡 Type 'status' to check connection status")
		fmt.Println("💡 Type 'quit' to exit")
		fmt.Println()
	})

	// Connection status monitor
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			status := "🔴 DISCONNECTED"
			if socket.IsConnected() {
				status = "🟢 CONNECTED"
			}
			fmt.Printf("📊 Status: %s (Total connections: %d)\n", status, connectionCount)
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")

	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())

		if text == "quit" || text == "exit" {
			fmt.Println("👋 Goodbye!")
			break
		}

		if text == "status" {
			status := "DISCONNECTED"
			if socket.IsConnected() {
				status = "CONNECTED"
			}
			fmt.Printf("📊 Connection Status: %s\n", status)
			fmt.Printf("📈 Total Connections Made: %d\n", connectionCount)
			fmt.Print("> ")
			continue
		}

		if text != "" {
			if !socket.IsConnected() {
				fmt.Printf("⚠️  Cannot send message - not connected: %s\n", text)
			} else {
				channel.Push("new_message", map[string]interface{}{
					"body": text,
					"user": username,
				})
			}
		}
		fmt.Print("> ")
	}
}