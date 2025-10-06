package api

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// EvaluationEvent describes websocket payloads emitted during evaluation runs.
type EvaluationEvent struct {
	Type       string          `json:"type"`
	JobID      string          `json:"job_id"`
	BatchID    uint            `json:"batch_id"`
	Total      int64           `json:"total,omitempty"`
	Processed  int             `json:"processed,omitempty"`
	Evaluation *EvaluationDTO  `json:"evaluation,omitempty"`
	Batch      []EvaluationDTO `json:"batch,omitempty"`
	Message    string          `json:"message,omitempty"`
	Reused     bool            `json:"reused,omitempty"`
	Timestamp  time.Time       `json:"timestamp"`
}

// wsClient wraps a websocket connection with write locking.
type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// EvaluationNotifier keeps track of active websocket clients and broadcasts evaluation events.
type EvaluationNotifier struct {
	mu         sync.Mutex
	clients    map[*wsClient]struct{}
	lastStatus *EvaluationEvent
}

// NewEvaluationNotifier constructs a notifier instance.
func NewEvaluationNotifier() *EvaluationNotifier {
	return &EvaluationNotifier{clients: make(map[*wsClient]struct{})}
}

// Register attaches a websocket connection and returns a client handle.
func (n *EvaluationNotifier) Register(conn *websocket.Conn) *wsClient {
	client := &wsClient{conn: conn}
	n.mu.Lock()
	n.clients[client] = struct{}{}
	status := n.lastStatus
	n.mu.Unlock()

	if status != nil {
		_ = client.writeJSON(*status)
	}
	return client
}

// Unregister removes the websocket client from the notifier and closes the socket.
func (n *EvaluationNotifier) Unregister(client *wsClient) {
	if client == nil {
		return
	}
	n.mu.Lock()
	delete(n.clients, client)
	n.mu.Unlock()
	_ = client.conn.Close()
}

// Broadcast sends the supplied event to all registered websocket clients.
func (n *EvaluationNotifier) Broadcast(event EvaluationEvent) {
	event.Timestamp = time.Now().UTC()

	n.mu.Lock()
	if event.Type == "progress" || event.Type == "evaluation" || event.Type == "started" {
		snapshot := event
		if snapshot.Evaluation != nil {
			snapshot.Evaluation = nil
		}
		n.lastStatus = &snapshot
	}

	for client := range n.clients {
		if err := client.writeJSON(event); err != nil {
			delete(n.clients, client)
			_ = client.conn.Close()
		}
	}
	n.mu.Unlock()
}

func (c *wsClient) writeJSON(payload interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteJSON(payload)
}

func (n *EvaluationNotifier) LastStatus() *EvaluationEvent {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.lastStatus == nil {
		return nil
	}
	copy := *n.lastStatus
	return &copy
}
