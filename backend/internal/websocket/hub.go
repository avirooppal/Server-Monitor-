package websocket

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// WSMessage represents a message to be broadcasted
type WSMessage struct {
	Type     string      `json:"type"`
	Data     interface{} `json:"data"`
	ServerID uuid.UUID   `json:"-"` // Used for filtering
}

// WebSocket Hub
type WSClient struct {
	Conn     *websocket.Conn
	Send     chan *WSMessage
	ServerID *uuid.UUID
	Filters  map[string]string
}

type WSHub struct {
	Clients    map[*WSClient]bool
	Broadcast  chan *WSMessage
	Register   chan *WSClient
	Unregister chan *WSClient
	Mu         sync.RWMutex
}

func NewWSHub() *WSHub {
	return &WSHub{
		Clients:    make(map[*WSClient]bool),
		Broadcast:  make(chan *WSMessage, 256),
		Register:   make(chan *WSClient),
		Unregister: make(chan *WSClient),
	}
}

func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.Mu.Lock()
			h.Clients[client] = true
			h.Mu.Unlock()
		case client := <-h.Unregister:
			h.Mu.Lock()
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
			}
			h.Mu.Unlock()
		case msg := <-h.Broadcast:
			h.Mu.RLock()
			for client := range h.Clients {
				if client.Matches(msg) {
					select {
					case client.Send <- msg:
					default:
						close(client.Send)
						delete(h.Clients, client)
					}
				}
			}
			h.Mu.RUnlock()
		}
	}
}

func (c *WSClient) Matches(msg *WSMessage) bool {
	if c.ServerID != nil && *c.ServerID != msg.ServerID {
		return false
	}
	// Add more filters if needed, e.g. message type
	return true
}

func (c *WSClient) WritePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.Send:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.Conn.WriteJSON(msg)
		case <-ticker.C:
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *WSClient) ReadPump(hub *WSHub) {
	defer func() {
		hub.Unregister <- c
		c.Conn.Close()
	}()

	for {
		if _, _, err := c.Conn.ReadMessage(); err != nil {
			break
		}
	}
}
