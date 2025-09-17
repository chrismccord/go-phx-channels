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

	// Set up event handlers
	socket.OnOpen(func() {
		fmt.Println("🔗 Connected to Phoenix server")
	})

	socket.OnClose(func() {
		fmt.Println("💔 Disconnected from Phoenix server")
	})

	socket.OnError(func(err error) {
		fmt.Printf("❌ Socket error: %v\n", err)
	})

	// Connect to the server
	if err := socket.Connect(); err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer socket.Disconnect()

	// Create a channel
	channel := socket.Channel("room:lobby", map[string]interface{}{
		"user_id": "123",
	})

	// Set up channel event handlers
	channel.On("user_joined", func(payload interface{}) {
		fmt.Printf("👋 User joined: %v\n", payload)
	})

	channel.On("new_message", func(payload interface{}) {
		fmt.Printf("💬 New message: %v\n", payload)
	})

	// Join the channel
	join := channel.Join()
	join.Receive("ok", func(resp interface{}) {
		fmt.Println("✅ Successfully joined the lobby")

		// Send a message after joining
		push := channel.Push("new_message", map[string]interface{}{
			"body": "Hello from Go!",
			"user": "go_client",
		})

		push.Receive("ok", func(resp interface{}) {
			fmt.Println("📤 Message sent successfully")
		}).Receive("error", func(reason interface{}) {
			fmt.Printf("❌ Failed to send message: %v\n", reason)
		})

	}).Receive("error", func(reason interface{}) {
		fmt.Printf("❌ Failed to join channel: %v\n", reason)
	})

	// Keep the program running
	fmt.Println("Press Ctrl+C to exit...")
	select {}
}