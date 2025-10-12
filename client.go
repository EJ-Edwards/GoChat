package main 

import (
	"sync"

	"github.com/gorilla/websocket"
)

type client struct{
	conn *websocket.Conn
	send chan []byte
	room *room
	leftOnce sync.Once
}


func (c *client) read() {
	// Ensure we cleanup on exit: notify room and close the connection.
	defer func() {
		// best-effort: notify room that this client left exactly once
		if c.room != nil {
			c.leftOnce.Do(func() { c.room.leave <- c })
		}
		c.conn.Close()
	}()

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			// read error (client disconnected or network issue)
			return
		}
		// forward received message to the room
		if c.room != nil {
			c.room.forward <- msg
		}
	}
}

func (c *client) write() {
	// Close connection when write loop exits
	defer c.conn.Close()

	// Range over the send channel. When the channel is closed, the loop exits
	// instead of repeatedly receiving zero values which would create a busy-loop.
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
	// Notify the room (only once) that this client is leaving if send channel was closed.
	if c.room != nil {
		c.leftOnce.Do(func() { c.room.leave <- c })
	}
}