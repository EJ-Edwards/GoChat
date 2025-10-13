package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {

	var addr = flag.String("addr", ":5500", "http service address")
	flag.Parse()
	// WebSocket endpoint will be handled by a small wrapper that picks the room based on ?pin=
	http.HandleFunc("/room", func(w http.ResponseWriter, req *http.Request) {
		pin := req.URL.Query().Get("pin")
		if pin == "" {
			http.Error(w, "pin required", http.StatusBadRequest)
			return
		}
		r := manager.GetOrCreate(pin)
		r.ServeHTTP(w, req)
	})

	// Serve static files (index.html, style.css, etc.) from the project root
	fs := http.FileServer(http.Dir("./"))
	http.Handle("/", fs)

	log.Println("Starting server on", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
