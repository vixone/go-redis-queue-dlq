package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

var ctx = context.Background()

type Queue struct {
	client     *redis.Client
	queueName  string
	retryQueue string
	dlqQueue   string
	maxRetries int
	clients    map[*websocket.Conn]bool
	mu         sync.Mutex
}

type Message struct {
	Event     string `json:"event"`
	Data      string `json:"data"`
	Retry     int    `json:"retry"`
	Timestamp int64  `json:"timestamp"`
}

// WebSocket upgrader
// TODO: limit connectios to specific client on the network
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Create new queue
func NewQueue(redisAddr, queueName string, maxRetries int) *Queue {
	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	return &Queue{
		client:     client,
		queueName:  queueName,
		retryQueue: queueName + "_retry",
		dlqQueue:   queueName + "_dlq",
		maxRetries: maxRetries,
		clients:    make(map[*websocket.Conn]bool),
	}
}

// Handle WebSocket connections
func (q *Queue) HandleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer ws.Close()

	q.mu.Lock()
	q.clients[ws] = true
	q.mu.Unlock()

	log.Println("New client connected!")

	// Listen for close events
	for {
		_, _, err := ws.ReadMessage()
		if err != nil {
			q.mu.Lock()
			delete(q.clients, ws)
			q.mu.Unlock()
			log.Println("Client disconnected")
			break
		}
	}
}

// Push message to queue (Laravel calls this)
func (q *Queue) Publish(c *gin.Context) {
	var msg Message
	if err := c.ShouldBindJSON(&msg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	msg.Retry = 0
	msg.Timestamp = time.Now().Unix()
	msgJSON, _ := json.Marshal(msg)

	err := q.client.LPush(ctx, q.queueName, msgJSON).Err()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enqueue message"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Message published"})
}

// Consume messages from Redis and push to WebSocket subscribers
func (q *Queue) Consume() {
	for {
		data, err := q.client.BRPop(ctx, 5*time.Second, q.queueName).Result()
		if err != nil {
			continue
		}

		var msg Message
		json.Unmarshal([]byte(data[1]), &msg)

		log.Println("Processing:", msg.Event, msg.Data)

		// Notify all WebSocket clients
		q.mu.Lock()
		for client := range q.clients {
			err := client.WriteJSON(msg)
			if err != nil {
				client.Close()
				delete(q.clients, client)
			}
		}
		q.mu.Unlock()
	}
}

func main() {
	queue := NewQueue("redis:6379", "my_queue", 3)

	// Start consuming messages
	go queue.Consume()

	// Start HTTP Server
	router := gin.Default()
	router.POST("/publish", queue.Publish)
	router.GET("/ws", func(c *gin.Context) {
		queue.HandleConnections(c.Writer, c.Request)
	})
	log.Println("Go Queue Server running on :8080")
	router.Run(":8080")
}

