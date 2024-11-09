package websocket

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type LoggerService struct {
	clients   map[*websocket.Conn]bool
	broadcast chan string
	mutex     sync.Mutex
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for testing
		},
	}
	Logger = NewLoggerService()
)

func NewLoggerService() *LoggerService {
	ls := &LoggerService{
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan string),
	}
	go ls.handleMessages()
	return ls
}

func (ls *LoggerService) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	ls.mutex.Lock()
	ls.clients[conn] = true
	ls.mutex.Unlock()

	// Remove client when connection closes
	defer func() {
		ls.mutex.Lock()
		delete(ls.clients, conn)
		ls.mutex.Unlock()
		conn.Close()
	}()

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (ls *LoggerService) SendLog(message string) {
	ls.broadcast <- message
}

func (ls *LoggerService) handleMessages() {
	for message := range ls.broadcast {
		ls.mutex.Lock()
		for client := range ls.clients {
			err := client.WriteMessage(websocket.TextMessage, []byte(message))
			if err != nil {
				client.Close()
				delete(ls.clients, client)
			}
		}
		ls.mutex.Unlock()
	}
}
