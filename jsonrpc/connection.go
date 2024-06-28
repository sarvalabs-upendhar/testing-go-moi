package jsonrpc

import (
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

// ConnectionManager holds the websocket connection and corresponding filter id
type ConnectionManager struct {
	connection *websocket.Conn
	writeLock  sync.Mutex // writer lock
	filterID   string
}

// NewConnectionManager returns a new connection manager with the websocket connection
func NewConnectionManager(connection *websocket.Conn) *ConnectionManager {
	return &ConnectionManager{
		connection: connection,
	}
}

// HasConn checks if the websocket connection is alive or not
func (cm *ConnectionManager) HasConn() bool {
	return cm.connection != nil
}

// WriteMessage writes the message to the websocket peer
func (cm *ConnectionManager) WriteMessage(messageType int, data []byte) error {
	cm.writeLock.Lock()
	defer cm.writeLock.Unlock()
	writeErr := cm.connection.WriteMessage(messageType, data)

	if writeErr != nil {
		fmt.Printf("Failed to write websocket message, %s", writeErr.Error())
	}

	return writeErr
}

// ReadMessage reads the message from the websocket peer
func (cm *ConnectionManager) ReadMessage() (messageType int, p []byte, err error) {
	return cm.connection.ReadMessage()
}

// SetFilterID updates the filter id to the connection manager
func (cm *ConnectionManager) SetFilterID(filterID string) {
	cm.filterID = filterID
}

// GetFilterID returns the filter id
func (cm *ConnectionManager) GetFilterID() string {
	return cm.filterID
}
