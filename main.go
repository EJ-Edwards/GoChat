// main.go
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// Configurable parameters
const (
	// Time allowed to write a message to the client.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the client.
	pongWait = 60 * time.Second

	// Send pings to client with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from client (bytes).
	maxMessageSize = 1024 * 8 // 8 KiB
)

// upgrader with sane defaults (rejects cross-origin by default).
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// You can configure CheckOrigin here for production (e.g. allow specific origins).
	CheckOrigin: func(r *http.Request) bool {
		// TODO: tighten this to allowed origins
		return true
	},
}

// Message is an example of structured messages (could be JSON).
type Message struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

// Client represents a single WebSocket connection.
type Client struct {
	conn *websocket.Conn
	send chan []byte // outbound messages to the client

	// metadata
	id   string
	hub  *Hub
	once sync.Once
}

// Hub maintains active clients and broadcasts.
type Hub struct {
	clients    map[*Client]struct{}
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte

	// mutex for reading clients slice/map outside run loop if necessary
	mu sync.RWMutex
}

// NewHub initializes a Hub.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte, 256),
	}
}

// Run starts the hub event loop.
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// Shutdown: close all clients
			h.mu.Lock()
			for c := range h.clients {
				c.close(websocket.CloseGoingAway, "server shutting down")
			}
			h.mu.Unlock()
			return
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = struct{}{}
			h.mu.Unlock()
			log.Printf("client registered: %s (total=%d)", c.id, len(h.clients))
		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send) // signal writePump to exit
				log.Printf("client unregistered: %s (total=%d)", c.id, len(h.clients))
			}
			h.mu.Unlock()
		case msg := <-h.broadcast:
			h.mu.RLock()
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					// If client's send channel is full, consider it stuck: remove it.
					h.mu.RUnlock()
					h.unregister <- c
					h.mu.RLock()
				}
			}
			h.mu.RUnlock()
		}
	}
}

// close is safe to call multiple times.
func (c *Client) close(code int, text string) {
	c.once.Do(func() {
		// Try to send a close message; ignore errors.
		_ = c.conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(code, text),
			time.Now().Add(writeWait))
		_ = c.conn.Close()
	})
}

// readPump pumps messages from the websocket connection to the hub.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.close(websocket.CloseNormalClosure, "readPump exiting")
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(appData string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		// We read raw messages; in a real app decode and validate JSON here.
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			// Distinguish between normal closures and unexpected errors.
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("client %s closed connection: %v", c.id, err)
			} else if ne, ok := err.(*net.OpError); ok && ne.Timeout() {
				log.Printf("client %s read timeout: %v", c.id, err)
			} else {
				log.Printf("unexpected read error from client %s: %v", c.id, err)
			}
			return
		}

		// Here you would unmarshal/validate the message and act accordingly.
		// For demonstration, we broadcast the raw message.
		select {
		case c.hub.broadcast <- message:
		default:
			// If hub is overloaded, optionally drop or disconnect client.
			log.Printf("hub busy. dropping message from client %s", c.id)
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close(websocket.CloseNormalClosure, "writePump exiting")
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Use NextWriter for performance and to support sending multiple messages in one frame if needed.
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				log.Printf("client %s writer error: %v", c.id, err)
				return
			}
			if _, err := w.Write(message); err != nil {
				log.Printf("client %s write error: %v", c.id, err)
				_ = w.Close()
				return
			}

			// Append queued messages to current websocket message (reduce frames).
			n := len(c.send)
			for i := 0; i < n; i++ {
				if b := <-c.send; b != nil {
					if _, err := w.Write(b); err != nil {
						log.Printf("client %s write (batch) error: %v", c.id, err)
						_ = w.Close()
						return
					}
				}
			}

			if err := w.Close(); err != nil {
				log.Printf("client %s writer close error: %v", c.id, err)
				return
			}

		case <-ticker.C:
			// Send a ping
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("client %s ping error: %v", c.id, err)
				return
			}
		}
	}
}

// serveWs handles websocket requests from the peer.
func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	// Upgrade connection
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Detailed error handling, but avoid exposing internals to clients.
		var statusErr *websocket.CloseError
		if errors.As(err, &statusErr) {
			http.Error(w, "websocket upgrade failed", http.StatusBadRequest)
			log.Printf("upgrade rejected: %v", err)
		} else {
			log.Printf("failed to upgrade websocket: %v", err)
		}
		return
	}

	// Derive a client ID (in prod, use UUIDs or authenticated user ID)
	clientID := fmt.Sprintf("%s->%s", r.RemoteAddr, time.Now().Format("20060102T150405"))

	client := &Client{
		conn: ws,
		send: make(chan []byte, 256), // buffered to reduce blocking
		id:   clientID,
		hub:  hub,
	}

	// Register and start pumps
	hub.register <- client

	go client.writePump()
	go client.readPump()
}

// http middleware to add context with request id, timeout etc (simple example).
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func main() {
	var addr string
	var shutdownTimeout time.Duration
	flag.StringVar(&addr, "addr", ":8080", "http service address")
	flag.DurationVar(&shutdownTimeout, "shutdown-timeout", 15*time.Second, "graceful shutdown timeout")
	flag.Parse()

	logger := log.New(os.Stdout, "", log.LstdFlags)
	_ = logger // use if you want to replace log.* calls

	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})

	// Optional simple health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := loggingMiddleware(mux)

	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// Start server
	go func() {
		log.Printf("starting server on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen failed: %v", err)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutdown signal received")

	// stop hub and server
	cancel() // signals hub to close clients
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	} else {
		log.Println("server shutdown complete")
	}
}
