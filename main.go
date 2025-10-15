// File: main.go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocket upgrader with Render-friendly origin handling
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		// Allow empty origin (e.g., non-browser or same-origin), local dev, and Render subdomains
		if origin == "" {
			return true
		}
		if origin == "http://localhost:8080" || origin == "http://127.0.0.1:8080" {
			return true
		}
		// allow any https://*.onrender.com
		if strings.HasPrefix(origin, "https://") && strings.Contains(origin, ".onrender.com") {
			return true
		}
		// adjust as needed for custom domains
		return false
	},
}

// Client represents a single websocket connection
type Client struct {
	conn *websocket.Conn
	send chan []byte
	hub  *Hub
}

// Hub holds clients for a single PIN.
// Note: no mutex here because hub.run serializes access to `clients`.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	pin        string
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

// run processes register/unregister/broadcast messages.
// When no clients remain and unregister path closes last client, run returns to allow cleanup.
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
				// if no clients remain, exit so manager can clean up this hub
				if len(h.clients) == 0 {
					return
				}
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// slow client — drop it
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

// HubManager manages hubs per-PIN. It protects the hubs map with a mutex.
type HubManager struct {
	hubs map[string]*Hub
	mu   sync.Mutex
}

func newHubManager() *HubManager {
	return &HubManager{
		hubs: make(map[string]*Hub),
	}
}

// getHub returns the hub for a pin, creating it if needed.
// It starts hub.run in a goroutine and ensures the hub is removed from the map when run returns.
func (m *HubManager) getHub(pin string) *Hub {
	m.mu.Lock()
	hub, exists := m.hubs[pin]
	if !exists {
		hub = newHub(pin)
		m.hubs[pin] = hub
		// start hub.run and cleanup when it returns
		ctx, cancel := context.WithCancel(context.Background())
		go func(p string, h *Hub) {
			h.run(ctx)
			m.mu.Lock()
			delete(m.hubs, p)
			m.mu.Unlock()
			cancel()
		}(pin, hub)
	}
	m.mu.Unlock()
	return hub
}

// serveWs upgrades HTTP -> WebSocket and registers client with the hub.
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
		// unregister client and close connection
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
			// read error or client closed; defer handles unregister
			break
		}
		// broadcast to the hub
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
				// hub closed the channel
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(message)
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

func main() {
	// Render sets PORT; default to 8080 locally
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	manager := newHubManager()
	mux := http.NewServeMux()

	// serve static files from ./static
	mux.Handle("/", http.FileServer(http.Dir("./static")))

	// websocket endpoint
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(manager, w, r)
	})

	// health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
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
