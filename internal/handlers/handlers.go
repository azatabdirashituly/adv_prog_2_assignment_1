package handlers

import (
	"fmt"
	"strings"
	"tcp-chat/internal/chat"
	"tcp-chat/internal/core"
	"time"
)

type ChatMessageHandler struct{}

func (h *ChatMessageHandler) HandleMessage(client *core.Client, message string) {
    cleanMessage := strings.TrimSpace(message)
    cleanMessage = strings.ToLower(cleanMessage)  // Convert message to lowercase for case-insensitive comparison

    if client.ChatRoom == nil || strings.HasPrefix(cleanMessage, "/") {
        command := strings.Fields(cleanMessage) // Split message into fields to handle commands with arguments
        if len(command) == 0 {
            fmt.Fprintf(client.Conn, "Unknown command. Type /help for command list.\n")
            return
        }

        switch command[0] {
        case "/help":
            h.help(client)
        case "/create":
            if len(command) < 2 {
                fmt.Fprintf(client.Conn, "Usage: /create [room_name]\n")
                return
            }
            chatName := strings.Join(command[1:], " ") // Support chat names with spaces
            h.createChatRoom(client, chatName)
        case "/join":
            if len(command) < 2 {
                fmt.Fprintf(client.Conn, "Usage: /join [room_name]\n")
                return
            }
            chatName := strings.Join(command[1:], " ")
            h.joinChatRoom(client, chatName)
		case "/users":
			if client.ChatRoom == nil {
				fmt.Fprintf(client.Conn, "You are not in a chat room.\n")
			} else {
				h.listUsers(client)
			}
        case "/kick":
            if len(command) < 2 {
                fmt.Fprintf(client.Conn, "Usage: /kick [username]\n")
                return
            }
            username := strings.Join(command[1:], " ")
            h.kick(client, username)
        case "/ban":
            if len(command) < 2 {
                fmt.Fprintf(client.Conn, "Usage: /ban [username]\n")
                return
            }
            username := strings.Join(command[1:], " ")
            h.ban(client, username)
        default:
            fmt.Fprintf(client.Conn, "Unknown command. Type /help for command list.\n")
        }
    } else {
        // Normal chat message handling
        h.broadcast(client.ChatRoom, fmt.Sprintf("%s %s: %s", client.Name, time.Now().Format("15:04")), cleanMessage)
    }
}


func (h *ChatMessageHandler) help(client *core.Client) {
    helpText := `
Available commands:
/help - Shows help information.
/create [room_name] - Creates a new chat room.
/join [room_name] - Joins an existing chat room.
/kick [username] - Kicks a user from the chat room. (Only for room creators)
/ban [username] - Bans a user from the chat room. (Only for room creators)
`
    fmt.Fprintf(client.Conn, helpText)
}

func (h *ChatMessageHandler) kick(client *core.Client, username string) {
    if client.ChatRoom == nil || client.ChatRoom.Creator != client {
        fmt.Fprintf(client.Conn, "You do not have permissions to kick users from this room.\n")
        return
    }

    normalizedUsername := strings.ToLower(username)  // Normalize input for comparison
    found := false

    client.ChatRoom.Lock.Lock()
    for i, c := range client.ChatRoom.Clients {
        if c.Name == normalizedUsername {
            client.ChatRoom.Clients = append(client.ChatRoom.Clients[:i], client.ChatRoom.Clients[i+1:]...)
            fmt.Fprintf(c.Conn, "You have been kicked from the room.\n")
            c.Conn.Close()  // Force disconnect
            found = true
            break
        }
    }
    client.ChatRoom.Lock.Unlock()

    if found {
        h.broadcast(client.ChatRoom, "Server", fmt.Sprintf("%s has been kicked from the room.", username))
        fmt.Fprintf(client.Conn, "%s has been kicked from the room.\n", username)
    } else {
        fmt.Fprintf(client.Conn, "User '%s' not found in the chat room.\n", username)
    }
}


func (h *ChatMessageHandler) ban(client *core.Client, username string) {
    if client.ChatRoom == nil || client.ChatRoom.Creator != client {
        fmt.Fprintf(client.Conn, "You do not have permissions to ban users from this room.\n")
        return
    }
    // Logic to ban the user (simplified example)
    for i, c := range client.ChatRoom.Clients {
        if c.Name == username {
            client.ChatRoom.Clients = append(client.ChatRoom.Clients[:i], client.ChatRoom.Clients[i+1:]...)
            fmt.Fprintf(c.Conn, "You have been banned from the room.\n")
            break
        }
    }
}

func (h *ChatMessageHandler) broadcast(chatRoom *core.ChatRoom, username, message string) {
    chatRoom.Lock.Lock()
    defer chatRoom.Lock.Unlock()

    // Format the message with the time, username, and message content
    currentTime := time.Now().Format("15:04")
    formattedMessage := fmt.Sprintf("%s: %s %s\n", currentTime, username, message)

    fmt.Printf("Broadcasting message: %s\n", formattedMessage)  // Debug output to see the broadcasting action
    for _, client := range chatRoom.Clients {
        if _, err := client.Conn.Write([]byte(formattedMessage)); err != nil {
            fmt.Println("Error writing to client:", err)
        }
    }
}


func (h *ChatMessageHandler) createChatRoom(client *core.Client, name string) {
    chat.ChatRoomsLock.Lock()
    defer chat.ChatRoomsLock.Unlock()

    if _, exists := chat.ChatRooms[name]; exists {
        fmt.Fprintf(client.Conn, "Chat room '%s' already exists.\n", name)
        return
    }

    chat.ChatRooms[name] = core.NewChatRoom(name, client)  // Assign the creator here
    fmt.Fprintf(client.Conn, "Chat room '%s' created. You can join now using '/join %s'.\n", name, name)
}


func (h *ChatMessageHandler) joinChatRoom(client *core.Client, name string) {
    chat.ChatRoomsLock.Lock()
    chatRoom, exists := chat.ChatRooms[name]
    if !exists {
        fmt.Fprintf(client.Conn, "Chat room '%s' does not exist. Create it using '/create %s'.\n", name, name)
        chat.ChatRoomsLock.Unlock()
        return
    }
    defer chat.ChatRoomsLock.Unlock()

    if client.ChatRoom != nil {
        h.leaveChatRoom(client)
    }

    chatRoom.Lock.Lock()
    client.Name = strings.ToLower(client.Name)  // Normalize the username upon joining
    chatRoom.Clients = append(chatRoom.Clients, client)
    client.ChatRoom = chatRoom
    chatRoom.Lock.Unlock()

    // Notify other members in the chat room
    h.broadcast(chatRoom, client.Name, fmt.Sprintf("%s has joined the chat room.", client.Name))

    fmt.Fprintf(client.Conn, "You joined chat room '%s'.\n", name)
}



func (h *ChatMessageHandler) leaveChatRoom(client *core.Client) {
    if client.ChatRoom == nil {
        return
    }

    client.ChatRoom.Lock.Lock()
    defer client.ChatRoom.Lock.Unlock()

    clients := client.ChatRoom.Clients
    for i, c := range clients {
        if c == client {
            client.ChatRoom.Clients = append(clients[:i], clients[i+1:]...)
            break
        }
    }
    client.ChatRoom = nil
}

func (h *ChatMessageHandler) listUsers(client *core.Client) {
    if client.ChatRoom == nil {
        fmt.Fprintf(client.Conn, "You must be in a chat room to see the list of users.\n")
        return
    }

    client.ChatRoom.Lock.Lock()
    defer client.ChatRoom.Lock.Unlock()

    fmt.Fprintf(client.Conn, "Users in '%s':\n", client.ChatRoom.Name)
    for _, user := range client.ChatRoom.Clients {
        fmt.Fprintf(client.Conn, "- %s\n", user.Name)  // Display names as stored
    }
}

