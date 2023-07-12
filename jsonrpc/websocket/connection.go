package websocket

import (
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

// ConnectionManager holds the websocket connection and corresponding subscription id
type ConnectionManager struct {
	connection     *websocket.Conn
	writeLock      sync.Mutex // writer lock
	subscriptionID string
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

// SetSubscriptionID updates the subscription id to the connection manager
func (cm *ConnectionManager) SetSubscriptionID(subscriptionID string) {
	cm.subscriptionID = subscriptionID
}

// GetSubscriptionID returns the subscription id, created while subscribing
func (cm *ConnectionManager) GetSubscriptionID() string {
	return cm.subscriptionID
}
