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
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <username>")
		os.Exit(1)
	}

	username := os.Args[1]
	roomName := "room:general"

	socket := phx.NewSocket("ws://localhost:4000/socket/websocket", &phx.SocketOptions{
		Logger: log.New(os.Stdout, "Chat: ", log.LstdFlags),
		Params: map[string]interface{}{
			"user": username,
		},
	})

	socket.OnOpen(func() {
		fmt.Printf("🔗 Connected as %s\n", username)
	})

	socket.OnClose(func() {
		fmt.Println("💔 Disconnected from chat server")
	})

	if err := socket.Connect(); err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer socket.Disconnect()

	// Join the chat room
	channel := socket.Channel(roomName, map[string]interface{}{
		"username": username,
	})

	// Handle incoming messages
	channel.On("new_message", func(payload interface{}) {
		if msgMap, ok := payload.(map[string]interface{}); ok {
			user := msgMap["user"]
			body := msgMap["body"]
			timestamp := time.Now().Format("15:04:05")
			fmt.Printf("[%s] %s: %s\n", timestamp, user, body)
		}
	})

	// Handle user join/leave events
	channel.On("user_joined", func(payload interface{}) {
		if userMap, ok := payload.(map[string]interface{}); ok {
			user := userMap["user"]
			fmt.Printf("👋 %s joined the chat\n", user)
		}
	})

	channel.On("user_left", func(payload interface{}) {
		if userMap, ok := payload.(map[string]interface{}); ok {
			user := userMap["user"]
			fmt.Printf("👋 %s left the chat\n", user)
		}
	})

	// Join the channel
	join := channel.Join()
	join.Receive("ok", func(resp interface{}) {
		fmt.Printf("✅ Joined %s\n", roomName)
		fmt.Println("Type messages and press Enter. Type 'quit' to exit.")
		fmt.Print("> ")
	}).Receive("error", func(reason interface{}) {
		fmt.Printf("❌ Failed to join %s: %v\n", roomName, reason)
		os.Exit(1)
	})

	// Read user input and send messages
	scanner := bufio.NewScanner(os.Stdin)
	go func() {
		for scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())

			if text == "quit" {
				fmt.Println("Goodbye!")
				channel.Leave()
				time.Sleep(100 * time.Millisecond)
				os.Exit(0)
			}

			if text == "" {
				fmt.Print("> ")
				continue
			}

			// Send message
			push := channel.Push("new_message", map[string]interface{}{
				"body": text,
				"user": username,
			})

			push.Receive("ok", func(resp interface{}) {
				// Message sent successfully
			}).Receive("error", func(reason interface{}) {
				fmt.Printf("❌ Failed to send message: %v\n", reason)
			})

			fmt.Print("> ")
		}
	}()

	// Keep the program running
	select {}
}