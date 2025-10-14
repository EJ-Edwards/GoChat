package main

import (
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/websocket"
)

const (
	socketBufferSize  = 1024
	messageBufferSize = 256
)

// --- Client ---
type client struct {
	conn *websocket.Conn
	send chan []byte
	room *room
}

func (c *client) read() {
	defer func() {
		c.room.leave <- c
		c.conn.Close()
	}()
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		c.room.forward <- msg
	}
}

func (c *client) write() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}

// --- Room ---
type room struct {
	clients map[*client]bool
	join    chan *client
	leave   chan *client
	forward chan []byte
	pin     string
}

func createRoom(pin string) *room {
	r := &room{
		clients: make(map[*client]bool),
		join:    make(chan *client),
		leave:   make(chan *client),
		forward: make(chan []byte),
		pin:     pin,
	}
	go r.run()
	return r
}

func (r *room) run() {
	for {
		select {
		case c := <-r.join:
			r.clients[c] = true
			log.Println("Client joined room", r.pin)
		case c := <-r.leave:
			if _, ok := r.clients[c]; ok {
				delete(r.clients, c)
				close(c.send)
				log.Println("Client left room", r.pin)
				if len(r.clients) == 0 {
					manager.deleteRoom(r.pin)
					return
				}
			}
		case msg := <-r.forward:
			for c := range r.clients {
				select {
				case c.send <- msg:
				default:
					delete(r.clients, c)
					close(c.send)
					if len(r.clients) == 0 {
						manager.deleteRoom(r.pin)
						return
					}
				}
			}
		}
	}
}

func (r *room) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	socket, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	c := &client{conn: socket, send: make(chan []byte, messageBufferSize), room: r}
	r.join <- c
	go c.write()
	go c.read()
}

// --- Room Manager ---
type roomManager struct {
	mu    sync.RWMutex
	rooms map[string]*room
}

func (m *roomManager) GetOrCreate(pin string) *room {
	m.mu.RLock()
	r, ok := m.rooms[pin]
	m.mu.RUnlock()
	if ok {
		return r
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok = m.rooms[pin]; ok {
		return r
	}
	r = createRoom(pin)
	m.rooms[pin] = r
	log.Println("Created new room:", pin)
	return r
}

func (m *roomManager) deleteRoom(pin string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rooms, pin)
	log.Println("Deleted empty room:", pin)
}

var manager = &roomManager{rooms: make(map[string]*room)}

// --- WebSocket upgrader ---
var upgrader = &websocket.Upgrader{
	ReadBufferSize:  socketBufferSize,
	WriteBufferSize: socketBufferSize,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// --- HTTP Handlers ---
func handleRoom(w http.ResponseWriter, r *http.Request) {
	pin := r.URL.Query().Get("pin")
	if pin == "" {
		http.Error(w, "pin required", http.StatusBadRequest)
		return
	}
	room := manager.GetOrCreate(pin)
	room.ServeHTTP(w, r)
}

func main() {
	http.HandleFunc("/room", handleRoom)

	// Serve static files (chat.html, connect.js, etc.)
	fs := http.FileServer(http.Dir("./"))
	http.Handle("/", fs)

	port := os.Getenv("PORT")
	if port == "" {
		port = "5500" // local dev default
	}
	log.Println("Listening on :" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
