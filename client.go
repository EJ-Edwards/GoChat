package main

import (
	"sync"

	"github.com/gorilla/websocket"
)

type client struct {
	conn     *websocket.Conn
	send     chan []byte
	room     *room
	leftOnce sync.Once
}

func (c *client) read() {
	defer func() {
		if c.room != nil {
			c.leftOnce.Do(func() { c.room.leave <- c })
		}
		c.conn.Close()
	}()

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if c.room != nil {
			c.room.forward <- msg
		}
	}
}

func (c *client) write() {

	defer func() {
		if c.room != nil {
			c.leftOnce.Do(func() { c.room.leave <- c })
		}
		c.conn.Close()
	}()

	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}
