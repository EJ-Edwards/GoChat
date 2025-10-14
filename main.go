package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// --- WebSocket upgrader ---
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow from your Render domain and local dev
		origin := r.Header.Get("Origin")
		return origin == "https://gochat-tz6u.onrender.com" || origin == "http://localhost:8080"
	},
}

// --- Client struct ---
type Client struct {
	conn *websocket.Conn
	send chan []byte
	hub  *Hub
}

// --- Hub for each PIN ---
// --- Hub for each PIN ---
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	pin        string
	mu         sync.Mutex
}

func newHub(pin string) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		pin:        pin,
	}
}

func (h *Hub) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

// --- Hub Manager ---
type HubManager struct {
	hubs map[string]*Hub
	mu   sync.Mutex
}

func newHubManager() *HubManager {
	return &HubManager{hubs: make(map[string]*Hub)}
}

func (m *HubManager) getHub(pin string) *Hub {
	m.mu.Lock()
	defer m.mu.Unlock()
	hub, exists := m.hubs[pin]
	if !exists {
		hub = newHub(pin)
		m.hubs[pin] = hub
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			hub.run(ctx)
			cancel()
		}()
	}
	return hub
}

// --- WebSocket handler ---
func serveWs(manager *HubManager, w http.ResponseWriter, r *http.Request) {
	pin := r.URL.Query().Get("pin")
	if pin == "" {
		http.Error(w, "PIN required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	hub := manager.getHub(pin)
	client := &Client{conn: conn, send: make(chan []byte, 256), hub: hub}
	hub.register <- client

	go client.writePump()
	client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		c.hub.broadcast <- message
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// --- Main function ---
func main() {
	// Render-compatible port handling
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	manager := newHubManager()
	mux := http.NewServeMux()

	// Serve frontend from ./static
	mux.Handle("/", http.FileServer(http.Dir("./static")))

	// WebSocket endpoint
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(manager, w, r)
	})

	// Health check for Render
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Server running on %s", addr)
	log.Fatal(server.ListenAndServe())
}
