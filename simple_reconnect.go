package main

import (
	"fmt"
	"log"
	"os"
	"time"

	phx "github.com/go-phx-channels"
)

func main() {
	fmt.Println("🧪 Simple Reconnection Test")
	fmt.Println("This test will NOT call Disconnect() - let's see if reconnection works")

	reconnectEnabled := true
	socket := phx.NewSocket("ws://localhost:4000/socket/websocket", &phx.SocketOptions{
		Logger:               log.New(os.Stdout, "SimpleTest: ", log.LstdFlags),
		ReconnectEnabled:     &reconnectEnabled,
		MaxReconnectAttempts: 5,
	})

	connectionCount := 0

	socket.OnOpen(func() {
		connectionCount++
		fmt.Printf("🟢 CONNECTION #%d\n", connectionCount)
	})

	fmt.Println("Connecting...")
	if err := socket.Connect(); err != nil {
		log.Fatal("Failed to connect:", err)
	}

	fmt.Println("✅ Connected! Now kill the Phoenix server and watch for reconnection attempts...")
	fmt.Println("This program will run for 60 seconds without calling Disconnect()")

	// Just wait and watch - don't call Disconnect()
	for i := 0; i < 60; i++ {
		time.Sleep(1 * time.Second)

		if i % 5 == 0 {
			status := "DISCONNECTED"
			if socket.IsConnected() {
				status = "CONNECTED"
			}
			fmt.Printf("Status: %s, Connections: %d\n", status, connectionCount)
		}
	}

	fmt.Printf("Final: %d total connections made\n", connectionCount)
}