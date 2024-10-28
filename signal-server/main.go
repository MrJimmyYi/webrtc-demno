// signal_server.go
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

// Message 定义信令服务器与客户端之间的消息格式
type Message struct {
	Type    string          `json:"type"`    // "register", "offer", "answer", "candidate", "control_command"
	Payload json.RawMessage `json:"payload"` // SDP 信息、ICE 候选或控制指令
}

// Client 代表一个连接的客户端
type Client struct {
	conn        *websocket.Conn
	send        chan []byte
	role        string // "viewer" 或 "desktop"
	peerConn    *webrtc.PeerConnection
	dataChannel *webrtc.DataChannel
}

var (
	upgrader      = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	viewerClient  *Client
	desktopClient *Client
	mutex         = &sync.Mutex{}
)

func main() {
	http.HandleFunc("/ws", handleConnections)
	log.Println("Signaling server started on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe failed:", err)
	}
}

// handleConnections 升级 HTTP 连接为 WebSocket 并处理客户端
func handleConnections(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade failed:", err)
		return
	}
	client := &Client{conn: conn, send: make(chan []byte)}
	go handleClient(client)
}

// handleClient 处理单个客户端的消息
func handleClient(client *Client) {
	defer func() {
		client.conn.Close()
		mutex.Lock()
		defer mutex.Unlock()
		if client.role == "viewer" {
			log.Println("Viewer client disconnected.")
			viewerClient = nil
			// 通知 Desktop 连接已断开
			notifyDesktop("desktop_disconnected", nil)
		} else if client.role == "desktop" {
			log.Println("Desktop client disconnected.")
			desktopClient = nil
			// 通知 Viewer 连接已断开
			notifyViewer("desktop_disconnected", nil)
		}
	}()

	// 启动一个协程来发送消息
	go func() {
		for msg := range client.send {
			err := client.conn.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Println("Write message failed:", err)
				return
			}
		}
	}()

	for {
		_, msg, err := client.conn.ReadMessage()
		if err != nil {
			log.Println("Read message failed:", err)
			break
		}

		log.Printf("Received raw message: %s\n", string(msg))

		var message Message
		err = json.Unmarshal(msg, &message)
		if err != nil {
			log.Println("Unmarshal message failed:", err)
			continue
		}

		log.Printf("Parsed message type: %s\n", message.Type)

		switch message.Type {
		case "register":
			handleRegister(client, message.Payload)
		case "offer":
			handleOffer(client, message.Payload)
		case "answer":
			handleAnswer(client, message.Payload)
		case "candidate":
			handleCandidate(client, message.Payload)
		case "control_command":
			handleControlCommand(client, message.Payload)
		default:
			log.Println("Unknown message type:", message.Type)
		}
	}
}

// handleRegister 处理注册消息
func handleRegister(client *Client, payload json.RawMessage) {
	var data struct {
		Role string `json:"role"` // "viewer" 或 "desktop"
	}
	err := json.Unmarshal(payload, &data)
	if err != nil {
		log.Println("Unmarshal register payload failed:", err)
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	if data.Role == "viewer" {
		if viewerClient != nil {
			// 已有 Viewer 连接，拒绝注册
			response := Message{
				Type:    "register_failed",
				Payload: json.RawMessage(`{"reason": "viewer_already_exists"}`),
			}
			sendMessage(client, response)
			return
		}
		client.role = "viewer"
		viewerClient = client
		response := Message{
			Type:    "register_success",
			Payload: json.RawMessage(`{"role": "viewer"}`),
		}
		sendMessage(client, response)
		log.Println("Viewer client registered.")
	} else if data.Role == "desktop" {
		if desktopClient != nil {
			// 已有 Desktop 连接，拒绝注册
			response := Message{
				Type:    "register_failed",
				Payload: json.RawMessage(`{"reason": "desktop_already_exists"}`),
			}
			sendMessage(client, response)
			return
		}
		client.role = "desktop"
		desktopClient = client
		response := Message{
			Type:    "register_success",
			Payload: json.RawMessage(`{"role": "desktop"}`),
		}
		sendMessage(client, response)
		log.Println("Desktop client registered.")
	} else {
		// 无效角色
		response := Message{
			Type:    "register_failed",
			Payload: json.RawMessage(`{"reason": "invalid_role"}`),
		}
		sendMessage(client, response)
	}
}

// handleOffer 处理来自 Viewer 或 Desktop 的 Offer 并转发
func handleOffer(client *Client, payload json.RawMessage) {
	mutex.Lock()
	defer mutex.Unlock()

	if client.role != "viewer" && client.role != "desktop" {
		log.Println("Unknown role attempted to send offer, ignoring.")
		return
	}

	// 根据角色确定接收方
	var forwardClient *Client
	if client.role == "viewer" {
		if desktopClient == nil {
			log.Println("No desktop client connected, cannot forward offer.")
			return
		}
		forwardClient = desktopClient
	} else if client.role == "desktop" {
		if viewerClient == nil {
			log.Println("No viewer client connected, cannot forward offer.")
			return
		}
		forwardClient = viewerClient
	}

	log.Printf("Forwarding offer payload to %s: %s\n", forwardClient.role, string(payload))

	// 封装 payload 为新的 Message
	forwardMsg := Message{
		Type:    "offer",
		Payload: payload,
	}

	// 序列化新的 Message
	forwardBytes, err := json.Marshal(forwardMsg)
	if err != nil {
		log.Println("Failed to marshal offer message:", err)
		return
	}

	// 转发 Offer
	forwardClient.send <- forwardBytes
	log.Printf("Forwarded offer from %s to %s.\n", client.role, forwardClient.role)
}

// handleAnswer 处理来自 Viewer 或 Desktop 的 Answer 并转发
func handleAnswer(client *Client, payload json.RawMessage) {
	mutex.Lock()
	defer mutex.Unlock()

	if client.role != "viewer" && client.role != "desktop" {
		log.Println("Unknown role attempted to send answer, ignoring.")
		return
	}

	// 根据角色确定接收方
	var forwardClient *Client
	if client.role == "viewer" {
		if desktopClient == nil {
			log.Println("No desktop client connected, cannot forward answer.")
			return
		}
		forwardClient = desktopClient
	} else if client.role == "desktop" {
		if viewerClient == nil {
			log.Println("No viewer client connected, cannot forward answer.")
			return
		}
		forwardClient = viewerClient
	}

	log.Printf("Forwarding answer payload to %s: %s\n", forwardClient.role, string(payload))

	// 封装 payload 为新的 Message
	forwardMsg := Message{
		Type:    "answer",
		Payload: payload,
	}

	// 序列化新的 Message
	forwardBytes, err := json.Marshal(forwardMsg)
	if err != nil {
		log.Println("Failed to marshal answer message:", err)
		return
	}

	// 转发 Answer
	forwardClient.send <- forwardBytes
	log.Printf("Forwarded answer from %s to %s.\n", client.role, forwardClient.role)
}

// handleCandidate 处理来自 Viewer 或 Desktop 的 ICE Candidate 并转发
func handleCandidate(client *Client, payload json.RawMessage) {
	mutex.Lock()
	defer mutex.Unlock()

	if client.role != "viewer" && client.role != "desktop" {
		log.Println("Unknown role attempted to send ICE candidate, ignoring.")
		return
	}

	// 根据角色确定接收方
	var forwardClient *Client
	if client.role == "viewer" {
		if desktopClient == nil {
			log.Println("No desktop client connected, cannot forward ICE candidate.")
			return
		}
		forwardClient = desktopClient
	} else if client.role == "desktop" {
		if viewerClient == nil {
			log.Println("No viewer client connected, cannot forward ICE candidate.")
			return
		}
		forwardClient = viewerClient
	}

	log.Printf("Forwarding ICE candidate from %s to %s: %s\n", client.role, forwardClient.role, string(payload))

	// 封装payload为正确格式
	forwardMsg := Message{
		Type:    "candidate",
		Payload: payload,
	}

	// 序列化新的 Message
	forwardBytes, err := json.Marshal(forwardMsg)
	if err != nil {
		log.Println("Failed to marshal candidate message:", err)
		return
	}

	// 转发 ICE Candidate
	forwardClient.send <- forwardBytes
	log.Printf("Forwarded ICE candidate from %s to %s.\n", client.role, forwardClient.role)
}

// handleControlCommand 处理来自 Viewer 的控制指令并转发给 Desktop
func handleControlCommand(client *Client, payload json.RawMessage) {
	mutex.Lock()
	defer mutex.Unlock()

	if client.role != "viewer" && client.role != "desktop" {
		log.Println("Unknown role attempted to send control command, ignoring.")
		return
	}

	// 根据角色确定接收方
	var forwardClient *Client
	if client.role == "viewer" {
		if desktopClient == nil {
			log.Println("No desktop client connected, cannot forward control command.")
			return
		}
		forwardClient = desktopClient
	} else if client.role == "desktop" {
		if viewerClient == nil {
			log.Println("No viewer client connected, cannot forward control command.")
			return
		}
		forwardClient = viewerClient
	}

	log.Printf("Forwarding control command from %s to %s: %s\n", client.role, forwardClient.role, string(payload))

	// 封装 payload 为新的 Message
	forwardMsg := Message{
		Type:    "control_command",
		Payload: payload,
	}

	// 序列化新的 Message
	forwardBytes, err := json.Marshal(forwardMsg)
	if err != nil {
		log.Println("Failed to marshal control_command message:", err)
		return
	}

	// 转发 Control Command
	forwardClient.send <- forwardBytes
	log.Printf("Forwarded control command from %s to %s.\n", client.role, forwardClient.role)
}

// notifyDesktop 通知 Desktop 客户端
func notifyDesktop(msgType string, payload interface{}) {
	if desktopClient == nil {
		return
	}

	var payloadBytes json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			log.Println("Marshal payload failed:", err)
			return
		}
		payloadBytes = json.RawMessage(b)
	} else {
		payloadBytes = json.RawMessage(`{}`)
	}

	message := Message{
		Type:    msgType,
		Payload: payloadBytes,
	}
	sendMessage(desktopClient, message)
}

// notifyViewer 通知 Viewer 客户端
func notifyViewer(msgType string, payload interface{}) {
	if viewerClient == nil {
		return
	}

	var payloadBytes json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			log.Println("Marshal payload failed:", err)
			return
		}
		payloadBytes = json.RawMessage(b)
	} else {
		payloadBytes = json.RawMessage(`{}`)
	}

	message := Message{
		Type:    msgType,
		Payload: payloadBytes,
	}
	sendMessage(viewerClient, message)
}

// sendMessage 发送消息给指定客户端
func sendMessage(client *Client, message Message) {
	msg, err := json.Marshal(message)
	if err != nil {
		log.Println("Marshal message failed:", err)
		return
	}
	client.send <- msg
}
