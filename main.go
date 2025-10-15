package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 54 * time.Second // < pongWait
	maxMessageSize = 1024 * 8
)

// --- Origin check ---
func allowOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // same-origin or CLI
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}

	originHost := u.Host
	reqHost := r.Host

	if strings.Contains(originHost, "localhost") || strings.Contains(originHost, "127.0.0.1") {
		return true
	}

	if strings.EqualFold(originHost, reqHost) {
		return true
	}

	// Allow Render subdomains if needed
	if strings.HasSuffix(originHost, ".onrender.com") || originHost == "onrender.com" {
		return true
	}

	return false
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:    1024,
	WriteBufferSize:   1024,
	EnableCompression: true,
	CheckOrigin: func(r *http.Request) bool {
		ok := allowOrigin(r)
		log.Printf("Incoming WebSocket from Origin=%q Host=%q -> allow=%v", r.Header.Get("Origin"), r.Host, ok)
		return ok
	},
}

type Client struct {
	conn *websocket.Conn
	send chan []byte
	hub  *Hub
}

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

func (h *Hub) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case client := <-h.register:
			h.clients[client] = true
			client.send <- []byte(`{"type":"system","msg":"👋 Welcome to room ` + h.pin + `"}`)
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				if len(h.clients) == 0 {
					return
				}
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
		go func(p string, h *Hub) {
			h.run(ctx)
			m.mu.Lock()
			delete(m.hubs, p)
			m.mu.Unlock()
			cancel()
		}(pin, hub)
	}
	return hub
}

func serveWs(manager *HubManager, w http.ResponseWriter, r *http.Request) {
	pin := r.URL.Query().Get("pin")
	if pin == "" {
		http.Error(w, "PIN required", http.StatusBadRequest)
		return
	}

	log.Printf("New WebSocket connection for room PIN: %s", pin)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
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
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("readPump unexpected close: %v", err)
			}
			break
		}

		trim := strings.TrimSpace(string(message))
		if strings.Contains(trim, `"type":"ping"`) {
			c.send <- []byte(`{"type":"pong","ts":"` + time.Now().UTC().Format(time.RFC3339) + `"}`)
			continue
		}

		c.hub.broadcast <- message
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err := w.Write(message); err != nil {
				_ = w.Close()
				return
			}
			_ = w.Close()

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	manager := newHubManager()
	mux := http.NewServeMux()

	// --- Serve static files ---
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// --- Serve root & fallback routes ---
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := "static" + r.URL.Path
		if _, err := os.Stat(path); os.IsNotExist(err) || strings.HasSuffix(r.URL.Path, "/") {
			http.ServeFile(w, r, "static/index.html")
			return
		}
		http.ServeFile(w, r, path)
	})

	// --- WebSocket route ---
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(manager, w, r)
	})

	// --- Health check ---
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

	log.Printf("✅ Server running on %s", addr)
	log.Fatal(server.ListenAndServe())
}
