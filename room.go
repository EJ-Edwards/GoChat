package main 

type room struct {
	clients map[*client]bool
	join    chan *client
	leave   chan *client
	forward chan []byte
}


func createRoom() *room {
	return &room{
		clients: make(map[*client]bool),
		join:    make(chan *client),
		leave:   make(chan *client),
		forward: make(chan []byte),
	}
}