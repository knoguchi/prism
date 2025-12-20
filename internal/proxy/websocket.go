package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"ai-proxy/pkg/models"
)

// WebSocketRelay handles WebSocket connection proxying and message capture
type WebSocketRelay struct {
	server    *Server
	requestID int64
	url       string
	clientWS  *websocket.Conn
	serverWS  *websocket.Conn
	mu        sync.Mutex
	seqNum    int
	closed    bool
}

// upgrader for server-side WebSocket connections
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for proxy
	},
}

// NewWebSocketRelay creates a new WebSocket relay
func (s *Server) NewWebSocketRelay(requestID int64, url string) *WebSocketRelay {
	return &WebSocketRelay{
		server:    s,
		requestID: requestID,
		url:       url,
		seqNum:    0,
	}
}

// HandleWebSocket handles a WebSocket upgrade request
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request, requestID int64) {
	// Connect to upstream WebSocket server
	targetURL := "ws" + r.URL.String()[4:] // Convert http:// to ws://
	if r.TLS != nil {
		targetURL = "wss" + r.URL.String()[5:] // Convert https:// to wss://
	}

	// Dial upstream
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// Forward headers
	header := http.Header{}
	for k, v := range r.Header {
		if k != "Upgrade" && k != "Connection" && k != "Sec-Websocket-Key" &&
			k != "Sec-Websocket-Version" && k != "Sec-Websocket-Extensions" {
			header[k] = v
		}
	}

	serverWS, resp, err := dialer.Dial(targetURL, header)
	if err != nil {
		s.logger.Error("Failed to connect to upstream WebSocket",
			zap.String("url", targetURL),
			zap.Error(err),
		)
		if resp != nil {
			w.WriteHeader(resp.StatusCode)
		} else {
			http.Error(w, "Failed to connect to upstream", http.StatusBadGateway)
		}
		return
	}
	defer serverWS.Close()

	// Upgrade client connection
	clientWS, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("Failed to upgrade client WebSocket", zap.Error(err))
		return
	}
	defer clientWS.Close()

	// Create relay
	relay := &WebSocketRelay{
		server:    s,
		requestID: requestID,
		url:       targetURL,
		clientWS:  clientWS,
		serverWS:  serverWS,
	}

	// Start relaying
	relay.Run()
}

// Run starts the bidirectional WebSocket relay
func (r *WebSocketRelay) Run() {
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Server
	go func() {
		defer wg.Done()
		r.relayMessages(r.clientWS, r.serverWS, models.WSDirectionClientToServer)
	}()

	// Server -> Client
	go func() {
		defer wg.Done()
		r.relayMessages(r.serverWS, r.clientWS, models.WSDirectionServerToClient)
	}()

	wg.Wait()
}

// relayMessages relays messages from src to dst, capturing each message
func (r *WebSocketRelay) relayMessages(src, dst *websocket.Conn, direction models.WSDirection) {
	for {
		messageType, data, err := src.ReadMessage()
		if err != nil {
			if !r.isClosed() {
				r.close()
				if !isNormalClose(err) {
					r.server.logger.Debug("WebSocket read error",
						zap.String("direction", string(direction)),
						zap.Error(err),
					)
				}
			}
			return
		}

		// Capture message
		r.captureMessage(direction, messageType, data)

		// Forward message
		if err := dst.WriteMessage(messageType, data); err != nil {
			if !r.isClosed() {
				r.close()
				r.server.logger.Debug("WebSocket write error",
					zap.String("direction", string(direction)),
					zap.Error(err),
				)
			}
			return
		}
	}
}

// captureMessage stores a WebSocket message
func (r *WebSocketRelay) captureMessage(direction models.WSDirection, messageType int, data []byte) {
	r.mu.Lock()
	r.seqNum++
	seqNum := r.seqNum
	r.mu.Unlock()

	// Determine message type
	var msgType models.WSMessageType
	switch messageType {
	case websocket.TextMessage:
		msgType = models.WSMessageTypeText
	case websocket.BinaryMessage:
		msgType = models.WSMessageTypeBinary
	case websocket.PingMessage:
		msgType = models.WSMessageTypePing
	case websocket.PongMessage:
		msgType = models.WSMessageTypePong
	case websocket.CloseMessage:
		msgType = models.WSMessageTypeClose
	default:
		msgType = models.WSMessageTypeBinary
	}

	// Truncate payload if too large
	payload := data
	if len(payload) > MaxBodyCapture {
		payload = payload[:MaxBodyCapture]
	}

	msg := &models.WebSocketMessage{
		UUID:        uuid.New().String(),
		RequestID:   r.requestID,
		Direction:   direction,
		MessageType: msgType,
		Payload:     payload,
		PayloadSize: int64(len(data)),
		SequenceNum: seqNum,
		CapturedAt:  time.Now(),
	}

	if err := r.server.db.SaveWebSocketMessage(context.Background(), msg); err != nil {
		r.server.logger.Error("Failed to save WebSocket message", zap.Error(err))
	} else {
		r.server.logger.Debug("Captured WebSocket message",
			zap.String("direction", string(direction)),
			zap.String("type", string(msgType)),
			zap.Int("size", len(data)),
			zap.Int("seq", seqNum),
		)
	}
}

// close marks the relay as closed
func (r *WebSocketRelay) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		r.closed = true
		r.clientWS.Close()
		r.serverWS.Close()
	}
}

// isClosed checks if the relay is closed
func (r *WebSocketRelay) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

// isNormalClose checks if the error is a normal WebSocket close
func isNormalClose(err error) bool {
	if err == nil {
		return true
	}
	if err == io.EOF {
		return true
	}
	if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		return true
	}
	if _, ok := err.(*net.OpError); ok {
		return true
	}
	return false
}
