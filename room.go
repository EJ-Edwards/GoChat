package main

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type room struct {
	clients map[*client]bool
	join    chan *client
	leave   chan *client
	forward chan []byte
}

type roomManager struct {
	mu    sync.RWMutex
	rooms map[string]*room
}

var manager = &roomManager{rooms: make(map[string]*room)}

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
	r = createRoom()
	m.rooms[pin] = r
	return r
}

func createRoom() *room {
	r := &room{
		clients: make(map[*client]bool),
		join:    make(chan *client),
		leave:   make(chan *client),
		forward: make(chan []byte),
	}

	go r.run()

	return r
}

func (r *room) run() {
	for {
		select {
		case client := <-r.join:
			r.clients[client] = true
		case client := <-r.leave:
			if _, ok := r.clients[client]; ok {
				delete(r.clients, client)
				close(client.send)
			}
		case msg := <-r.forward:
			for c := range r.clients {
				select {
				case c.send <- msg:
				default:
					delete(r.clients, c)
					close(c.send)
				}
			}
		}
	}
}

const (
	socketBufferSize  = 1024
	messageBufferSize = 256
)

var upgrader = &websocket.Upgrader{
	ReadBufferSize:  socketBufferSize,
	WriteBufferSize: socketBufferSize,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func (r *room) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	socket, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return
	}
	client := &client{conn: socket, send: make(chan []byte, messageBufferSize), room: r}
	r.join <- client
	go client.write()
	go client.read()
}
