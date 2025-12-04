package main

import (
	"bufio"
	"fmt"
	"log"
	"net/rpc"
	"os"
	"strings"
	"time"
)

func main() {
	// Connect to server
	client, err := rpc.Dial("tcp", "127.0.0.1:1234")
	if err != nil {
		log.Fatal("Error connecting to server:", err)
	}
	defer client.Close()

	reader := bufio.NewReader(os.Stdin)

	// Get username
	fmt.Print("Enter your username (or press Enter for random): ")
	requestedName, _ := reader.ReadString('\n')
	requestedName = strings.TrimSpace(requestedName)

	// Join chat room
	var joinReply struct {
		Success      bool
		AssignedName string
		Message      string
	}

	err = client.Call("ChatRoom.Join",
		struct{ RequestedName string }{RequestedName: requestedName},
		&joinReply)

	if err != nil || !joinReply.Success {
		log.Fatal("Failed to join:", err, joinReply.Message)
	}

	username := joinReply.AssignedName

	fmt.Printf("\n=== %s ===\n", joinReply.Message)
	fmt.Println("Type your messages and press Enter.")
	fmt.Println("Type 'exit' to leave the chat.")
	fmt.Println("=============================")

	// Channel to control message receiver
	stopReceiver := make(chan bool)
	lastMsgID := 0

	// Goroutine to receive updates
	go func() {
		for {
			select {
			case <-stopReceiver:
				return
			case <-time.After(300 * time.Millisecond): // Poll every 300ms
				var updatesReply struct {
					Messages []struct {
						ID      int
						Sender  string
						Content string
					}
					NewMsgID int
				}

				err := client.Call("ChatRoom.GetUpdates",
					struct {
						ID        string
						LastMsgID int
					}{
						ID:        username,
						LastMsgID: lastMsgID,
					},
					&updatesReply)

				if err != nil {
					// Connection lost
					fmt.Println("\n[ERROR] Connection to server lost")
					stopReceiver <- true
					return
				}

				// Display new messages
				if len(updatesReply.Messages) > 0 {
					for _, msg := range updatesReply.Messages {
						if msg.Sender == "System" {
							fmt.Printf("\n[SYSTEM] %s\n", msg.Content)
						} else {
							fmt.Printf("\n%s: %s\n", msg.Sender, msg.Content)
						}
						// Update prompt
						fmt.Print("> ")
					}
					lastMsgID = updatesReply.NewMsgID
				}
			}
		}
	}()

	// Cleanup function
	defer func() {
		stopReceiver <- true
		// Notify server we're leaving
		var leaveReply struct {
			Success bool
			Message string
		}
		client.Call("ChatRoom.Leave", struct{ ID string }{ID: username}, &leaveReply)
	}()

	// Main input loop
	for {
		fmt.Print("> ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if strings.ToLower(input) == "exit" {
			fmt.Println("Goodbye!")
			break
		}

		if input == "" {
			continue
		}

		// Send message
		var sendReply struct{ Success bool }
		err = client.Call("ChatRoom.Send", struct {
			ID      string
			Message string
		}{ID: username, Message: input}, &sendReply)

		if err != nil {
			fmt.Printf("\n[ERROR] Failed to send message: %v\n", err)
			break
		}

		// Immediately show our own message (not from broadcast)
		fmt.Printf("\n[You] %s\n", input)
	}
}
