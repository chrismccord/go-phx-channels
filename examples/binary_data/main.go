package main

import (
	"fmt"
	"log"
	"os"
	"time"

	phx "github.com/go-phx-channels"
)

func main() {
	socket := phx.NewSocket("ws://localhost:4000/socket/websocket", &phx.SocketOptions{
		Logger: log.New(os.Stdout, "Binary: ", log.LstdFlags),
	})

	if err := socket.Connect(); err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer socket.Disconnect()

	// Create a channel for binary data
	channel := socket.Channel("binary:data", nil)

	// Handle binary messages from server
	channel.On("binary_message", func(payload interface{}) {
		if binaryPayload, ok := payload.(phx.BinaryPayload); ok {
			fmt.Printf("📦 Received binary data: %v (length: %d)\n",
				binaryPayload.Data, len(binaryPayload.Data))
		}
	})

	// Join the channel
	join := channel.Join()
	join.Receive("ok", func(resp interface{}) {
		fmt.Println("✅ Joined binary channel")

		// Send binary data
		binaryData := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
		payload := phx.NewBinaryPayload(binaryData)

		push := channel.Push("binary_data", payload)
		push.Receive("ok", func(resp interface{}) {
			fmt.Println("📤 Binary data sent successfully")

			// Check if response contains binary data
			if respMap, ok := resp.(map[string]interface{}); ok {
				if binaryResp, ok := respMap["response"].(phx.BinaryPayload); ok {
					fmt.Printf("🔄 Received binary response: %v\n", binaryResp.Data)
				}
			}
		}).Receive("error", func(reason interface{}) {
			fmt.Printf("❌ Failed to send binary data: %v\n", reason)
		})

		// Send a request for large binary data
		time.Sleep(1 * time.Second)
		largePush := channel.Push("large_binary", phx.NewBinaryPayload([]byte("request")))
		largePush.Receive("ok", func(resp interface{}) {
			if respMap, ok := resp.(map[string]interface{}); ok {
				if binaryResp, ok := respMap["response"].(phx.BinaryPayload); ok {
					fmt.Printf("📦 Received large binary data (length: %d bytes)\n",
						len(binaryResp.Data))
				}
			}
		})

	}).Receive("error", func(reason interface{}) {
		fmt.Printf("❌ Failed to join binary channel: %v\n", reason)
	})

	// Keep the program running
	time.Sleep(5 * time.Second)
	fmt.Println("Demo completed")
}