package main

import (
	"log"

	"github.com/gorilla/websocket"
)

type Hub struct {
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	broadcast  chan []byte
	clients    map[*websocket.Conn]bool
}

func NewHub() *Hub {
	return &Hub{
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
		broadcast:  make(chan []byte),
		clients:    map[*websocket.Conn]bool{},
	}
}

func (h *Hub) Run() {
	for {
		select {
		case conn := <-h.register:
			h.clients[conn] = true
		case conn := <-h.unregister:
			if _, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
				conn.Close()
			}
		case message := <-h.broadcast:
			for conn := range h.clients {
				if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
					log.Printf("websocket send failed: %v", err)
					conn.Close()
					delete(h.clients, conn)
				}
			}
		}
	}
}
