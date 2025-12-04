package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
	"time"
)

// Message represents a chat message
type Message struct {
	ID      int
	Sender  string
	Content string
	Time    time.Time
}

// ClientInfo tracks connected clients
type ClientInfo struct {
	ID        string
	LastMsgID int
	JoinedAt  time.Time
	RealName  string // Original requested name
}

// ChatRoom is the RPC service with broadcasting
type ChatRoom struct {
	mu          sync.RWMutex
	clients     map[string]*ClientInfo
	messages    []Message
	nextMsgID   int
	nameCounter map[string]int // Track name usage
}

// NewChatRoom creates a new chat room
func NewChatRoom() *ChatRoom {
	return &ChatRoom{
		clients:     make(map[string]*ClientInfo),
		messages:    make([]Message, 0),
		nextMsgID:   1,
		nameCounter: make(map[string]int),
	}
}

// JoinArgs for joining chat
type JoinArgs struct {
	RequestedName string
}

// JoinReply response
type JoinReply struct {
	Success      bool
	AssignedName string
	Message      string
}

// SendArgs for sending messages
type SendArgs struct {
	ID      string
	Message string
}

// SendReply response
type SendReply struct {
	Success bool
}

// GetUpdatesArgs for getting updates
type GetUpdatesArgs struct {
	ID        string
	LastMsgID int
}

// GetUpdatesReply response
type GetUpdatesReply struct {
	Messages []Message
	NewMsgID int
}

// generateUniqueName creates a unique username if requested name is taken
func (cr *ChatRoom) generateUniqueName(requestedName string) string {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	// If name doesn't exist, use it as-is
	if _, exists := cr.clients[requestedName]; !exists {
		cr.nameCounter[requestedName] = 1
		return requestedName
	}

	// Find available variation
	for i := 1; i <= 100; i++ {
		candidate := fmt.Sprintf("%s%d", requestedName, i)
		if _, exists := cr.clients[candidate]; !exists {
			cr.nameCounter[requestedName] = i + 1
			return candidate
		}
	}

	// Fallback with timestamp
	return fmt.Sprintf("%s_%d", requestedName, time.Now().UnixNano()%10000)
}

// Join adds a client to the chat room
func (cr *ChatRoom) Join(args JoinArgs, reply *JoinReply) error {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	// Clean requested name
	requestedName := args.RequestedName
	if requestedName == "" {
		requestedName = "Guest"
	}

	// Generate unique name
	assignedName := requestedName
	if _, exists := cr.clients[requestedName]; exists {
		// Name exists, generate unique one
		for i := 1; i <= 100; i++ {
			candidate := fmt.Sprintf("%s%d", requestedName, i)
			if _, exists := cr.clients[candidate]; !exists {
				assignedName = candidate
				break
			}
		}
		// Last resort
		if assignedName == requestedName {
			assignedName = fmt.Sprintf("%s_%d", requestedName, time.Now().UnixNano()%10000)
		}
	}

	// Add client
	cr.clients[assignedName] = &ClientInfo{
		ID:        assignedName,
		LastMsgID: cr.nextMsgID - 1,
		JoinedAt:  time.Now(),
		RealName:  args.RequestedName,
	}

	// Create system join message
	joinMsg := Message{
		ID:      cr.nextMsgID,
		Sender:  "System",
		Content: fmt.Sprintf("User %s joined the chat", assignedName),
		Time:    time.Now(),
	}
	cr.nextMsgID++
	cr.messages = append(cr.messages, joinMsg)

	// Notify if name was changed
	notification := ""
	if assignedName != args.RequestedName && args.RequestedName != "" {
		notification = fmt.Sprintf(" (Name '%s' was taken)", args.RequestedName)
	}

	fmt.Printf("[SYSTEM] %s joined%s\n", assignedName, notification)

	reply.Success = true
	reply.AssignedName = assignedName
	reply.Message = fmt.Sprintf("Joined as %s%s", assignedName, notification)
	return nil
}

// Send sends a message and stores it
func (cr *ChatRoom) Send(args SendArgs, reply *SendReply) error {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	// Check if client exists
	if _, exists := cr.clients[args.ID]; !exists {
		reply.Success = false
		return fmt.Errorf("user not found")
	}

	// Create message
	msg := Message{
		ID:      cr.nextMsgID,
		Sender:  args.ID,
		Content: args.Message,
		Time:    time.Now(),
	}
	cr.nextMsgID++
	cr.messages = append(cr.messages, msg)

	// Update client's last message ID
	cr.clients[args.ID].LastMsgID = msg.ID

	fmt.Printf("[MESSAGE] %s: %s\n", args.ID, args.Message)
	reply.Success = true
	return nil
}

// GetUpdates gets new messages since lastMsgID for a client
func (cr *ChatRoom) GetUpdates(args GetUpdatesArgs, reply *GetUpdatesReply) error {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	// Check if client exists
	if _, exists := cr.clients[args.ID]; !exists {
		return fmt.Errorf("user not found")
	}

	reply.Messages = make([]Message, 0)
	reply.NewMsgID = args.LastMsgID

	// Find messages after lastMsgID, filter out client's own messages
	for _, msg := range cr.messages {
		if msg.ID > args.LastMsgID {
			// Don't send system join messages about yourself
			if msg.Sender == "System" && args.LastMsgID > 0 {
				if msg.Content == fmt.Sprintf("User %s joined the chat", args.ID) {
					continue
				}
			}
			// Don't send user's own messages (no self-echo)
			if msg.Sender == args.ID {
				continue
			}
			reply.Messages = append(reply.Messages, msg)
			if msg.ID > reply.NewMsgID {
				reply.NewMsgID = msg.ID
			}
		}
	}

	return nil
}

// Leave removes a client from the chat room
func (cr *ChatRoom) Leave(args struct{ ID string }, reply *JoinReply) error {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if client, exists := cr.clients[args.ID]; exists {
		delete(cr.clients, args.ID)

		// Create leave message
		leaveMsg := Message{
			ID:      cr.nextMsgID,
			Sender:  "System",
			Content: fmt.Sprintf("User %s left the chat", args.ID),
			Time:    time.Now(),
		}
		cr.nextMsgID++
		cr.messages = append(cr.messages, leaveMsg)

		// Clean up name counter if needed
		if client.RealName != "" {
			// We could decrement counter here, but simpler to leave it
		}

		fmt.Printf("[SYSTEM] %s left\n", args.ID)
		reply.Success = true
		reply.Message = "Left successfully"
	}
	return nil
}

func main() {
	chatRoom := NewChatRoom()

	// Register RPC service
	err := rpc.Register(chatRoom)
	if err != nil {
		log.Fatal("Error registering RPC service:", err)
	}

	// Listen on port 1234
	listener, err := net.Listen("tcp", "127.0.0.1:1234")
	if err != nil {
		log.Fatal("Error starting server:", err)
	}

	fmt.Println("Server running on port 1234...")
	fmt.Println("----- Real-time Chat Room -----")
	fmt.Println("Duplicate usernames will get numbers added automatically")
	fmt.Println("Waiting for clients...")

	// Accept connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}
