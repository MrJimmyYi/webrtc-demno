// desktop_client.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/pion/webrtc/v3/pkg/media"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/go-vgo/robotgo"
	"github.com/gorilla/websocket"
	"github.com/kbinani/screenshot"
	"github.com/pion/webrtc/v3"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

type CandidatePayload struct {
	Candidate webrtc.ICECandidateInit `json:"candidate"`
}

// Message 定义信令服务器与客户端之间的消息格式
type Message struct {
	Type    string          `json:"type"`    // "register", "offer", "answer", "candidate", "control_command"
	Payload json.RawMessage `json:"payload"` // SDP 信息、ICE 候选或控制指令
}

// ControlCommand 定义从 Viewer 接收的控制指令
type ControlCommand struct {
	Action string   `json:"action"` // "mouse_move", "mouse_click", "key_press"
	Params []string `json:"params"` // 参数，例如坐标或键值
}

// Client 代表一个连接的客户端
type Client struct {
	conn *websocket.Conn
	send chan []byte
	role string // "desktop" 或 "viewer"
}

var (
	upgrader       = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	peerConnection *webrtc.PeerConnection
	videoTrack     *webrtc.TrackLocalStaticSample
	dataChannel    *webrtc.DataChannel
	mutex          = &sync.Mutex{}
	client         = &Client{send: make(chan []byte), role: "desktop"}
)

func main() {
	// 捕获中断信号以优雅关闭
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// 连接中继服务器
	signalURL := url.URL{Scheme: "ws", Host: "192.168.40.100:8080", Path: "/ws"} // 中继服务器地址
	log.Printf("Connecting to signaling server: %s", signalURL.String())

	conn, _, err := websocket.DefaultDialer.Dial(signalURL.String(), nil)
	if err != nil {
		log.Fatal("Failed to connect to signaling server:", err)
	}
	defer conn.Close()
	client.conn = conn

	// 注册为 desktop
	register := Message{
		Type:    "register",
		Payload: json.RawMessage(`{"role": "desktop"}`),
	}
	err = conn.WriteJSON(register)
	if err != nil {
		log.Fatal("Failed to send register message:", err)
	}

	// 启动一个协程来接收消息
	go handleMessages(client)

	// 定义 TURN 服务器信息
	turnServerURL := fmt.Sprintf("turn:%s:%d", "192.168.40.100", 23478)
	turnUsername := "jimmy"
	turnPassword := "apple"
	// 创建 WebRTC PeerConnection
	peerConnection, err = webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs:       []string{turnServerURL},
				Username:   turnUsername,
				Credential: turnPassword,
			},
		},
		ICETransportPolicy: webrtc.ICETransportPolicyRelay,
	})
	if err != nil {
		log.Fatal("Failed to create PeerConnection:", err)
	}

	// 创建视频轨道
	videoTrack, err = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
		MimeType: webrtc.MimeTypeH264,
	}, "video", "desktop")
	if err != nil {
		log.Fatal("Failed to create video track:", err)
	}

	_, err = peerConnection.AddTrack(videoTrack)
	if err != nil {
		log.Fatal("Failed to add video track:", err)
	}
	log.Println("Added video track.")

	// 创建 Data Channel 用于接收控制指令
	dataChannel, err = peerConnection.CreateDataChannel("control", nil)
	if err != nil {
		log.Fatal("Failed to create DataChannel:", err)
	}

	// 处理控制指令
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		handleControlCommand(msg.Data)
	})

	// 处理 ICE 连接状态变化
	peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Println("ICE Connection State changed:", state.String())

		if state == webrtc.ICEConnectionStateFailed || state == webrtc.ICEConnectionStateDisconnected {
			log.Printf("ICE Connection  %v:", state)
			log.Println("ICE connection failed or disconnected, exiting.")
			err := peerConnection.Close()
			if err != nil {
				fmt.Printf("%v", err)
				return
			}

			os.Exit(0)
		}
		if state == webrtc.ICEConnectionStateConnected {
			log.Println("ICE connection established.")
		}
	})

	// 处理 ICE Candidate
	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		candidateJSON, err := json.Marshal(candidate.ToJSON())
		if err != nil {
			log.Println("Failed to marshal ICE candidate:", err)
			return
		}
		msg := Message{
			Type:    "candidate",
			Payload: json.RawMessage(candidateJSON),
		}
		sendMessage(client, msg)
		log.Println("Sent ICE candidate.")
	})

	// 启动 FFmpeg 进程，捕获屏幕并输出到管道

	// 创建一个管道，用于捕获 FFmpeg 的输出
	reader, writer := io.Pipe()

	// 启动 FFmpeg 进程并将输出重定向到管道
	go func() {
		err := ffmpeg.Input("desktop",
			ffmpeg.KwArgs{
				"f":         "gdigrab",
				"framerate": "60", // 根据需要调整帧率
			}).
			Output("pipe:1",
				ffmpeg.KwArgs{
					"vcodec":  "libx264",   // 使用 H.264 编码器
					"preset":  "ultrafast", // 根据需要调整预设
					"tune":    "zerolatency",
					"pix_fmt": "yuv420p",
					"format":  "h264", // 使用 h264 格式
				}).
			WithOutput(writer).
			Run()
		if err != nil {
			log.Println("FFmpeg 进程出错:", err)
		}
		writer.Close()
	}()

	// 读取 FFmpeg 输出并发送到 WebRTC
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		for {
			// 从 FFmpeg 读取编码后的数据
			buf := make([]byte, 4096) // 可以根据需要调整缓冲区大小
			n, err := reader.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Println("读取 FFmpeg 输出时出错:", err)
				break
			}

			// 将读取的数据写入 WebRTC 视频轨道
			err = videoTrack.WriteSample(media.Sample{
				Data:     buf[:n],
				Duration: time.Second / 60, // 与帧率对应
			})
			if err != nil {
				log.Println("写入视频样本时出错:", err)
				break
			}
		}
	}()

	// 等待中断信号
	<-interrupt
	log.Println("Received interrupt signal, shutting down.")

	// 清理
	err = peerConnection.Close()
	if err != nil {
		log.Println("Failed to close PeerConnection:", err)
	}
	wg.Wait()
}

// handleMessages 处理来自信令服务器的消息
func handleMessages(client *Client) {
	for {
		var msg Message
		err := client.conn.ReadJSON(&msg)
		if err != nil {
			log.Println("Read message failed:", err)
			return
		}
		log.Println("Received message type:", msg.Type)

		switch msg.Type {
		case "register_success":
			log.Println("Registered successfully as desktop.")
		case "register_failed":
			var data struct {
				Reason string `json:"reason"`
			}
			err := json.Unmarshal(msg.Payload, &data)
			if err != nil {
				log.Println("Unmarshal register_failed payload failed:", err)
				return
			}
			log.Fatalf("Register failed: %s", data.Reason)
		case "offer":
			handleOffer(msg.Payload, client)
		case "answer":
			handleAnswer(msg.Payload)
		case "candidate":
			handleCandidate(msg.Payload)
		case "desktop_disconnected":
			log.Println("Viewer disconnected because desktop disconnected.")
		default:
			log.Println("Unknown message type:", msg.Type)
		}
	}
}

// handleOffer 处理来自 Viewer 的 Offer 并发送 Answer
func handleOffer(payload json.RawMessage, client *Client) {

	var offer webrtc.SessionDescription
	err := json.Unmarshal(payload, &offer)
	if err != nil {
		log.Println("Unmarshal offer failed:", err)
		return
	}

	// 设置远程描述
	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		log.Println("SetRemoteDescription failed:", err)
		return
	}

	// 创建 Answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		log.Println("CreateAnswer failed:", err)
		return
	}

	// 设置本地描述
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		log.Println("SetLocalDescription failed:", err)
		return
	}

	// 发送 Answer 回 Viewer
	answerJSON, err := json.Marshal(peerConnection.LocalDescription())
	if err != nil {
		log.Println("Marshal answer failed:", err)
		return
	}
	msg := Message{
		Type:    "answer",
		Payload: json.RawMessage(answerJSON),
	}
	sendMessage(client, msg)
	log.Println("Sent answer to Viewer.:")
}

// handleAnswer 处理来自信令服务器的 Answer（通常不需要，Answer 由 Viewer 发送）
func handleAnswer(payload json.RawMessage) {
	// 在 Desktop 端通常不需要处理 Answer，因为 Desktop 只是发送视频轨道，并不接收媒体流。
	var answer webrtc.SessionDescription
	err := json.Unmarshal(payload, &answer)
	if err != nil {
		log.Println("Unmarshal answer failed:", err)
		return
	}

	log.Println("Setting remote description with answer.")
	err = peerConnection.SetRemoteDescription(answer)
	if err != nil {
		log.Println("SetRemoteDescription failed:", err)
		return
	}
}

// handleCandidate 处理来自 Viewer 的 ICE Candidate 并添加到 PeerConnection
func handleCandidate(payload json.RawMessage) {
	var candidatePayload CandidatePayload
	fmt.Printf("%v\n", string(payload))

	err := json.Unmarshal(payload, &candidatePayload)
	if err != nil {
		log.Println("Unmarshal ICE candidate failed:", err)
		return
	}
	fmt.Printf("%v\n", candidatePayload)
	err = peerConnection.AddICECandidate(candidatePayload.Candidate)
	if err != nil {
		log.Println("AddICECandidate failed:", err)
		return
	}
	log.Println("Added ICE candidate to PeerConnection.")
}

// sendMessage 发送消息给指定客户端
func sendMessage(client *Client, message Message) {
	msg, err := json.Marshal(message)
	if err != nil {
		log.Println("Marshal message failed:", err)
		return
	}
	err = client.conn.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		log.Println("Write message failed:", err)
	}
}

// captureScreen 捕获屏幕并返回 JPEG 编码的数据
func captureScreen() ([]byte, error) {
	// 获取第一个显示器的边界
	n := screenshot.NumActiveDisplays()
	if n <= 0 {
		return nil, fmt.Errorf("no active displays found")
	}

	bounds := screenshot.GetDisplayBounds(0)
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return nil, err
	}

	// 编码为 JPEG
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 50}) // 质量可调
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// handleControlCommand 处理来自 DataChannel 的控制指令
func handleControlCommand(data []byte) {
	var cmd ControlCommand
	err := json.Unmarshal(data, &cmd)
	if err != nil {
		log.Println("Failed to unmarshal control command:", err)
		return
	}

	switch cmd.Action {
	case "mouse_move":
		if len(cmd.Params) != 2 {
			log.Println("Invalid mouse_move parameters")
			return
		}
		x, err1 := strconv.Atoi(cmd.Params[0])
		y, err2 := strconv.Atoi(cmd.Params[1])
		if err1 != nil || err2 != nil {
			log.Println("Invalid mouse_move coordinates")
			return
		}
		robotgo.MoveMouse(x, y)
		log.Printf("Moved mouse to (%d, %d)", x, y)
	case "mouse_click":
		if len(cmd.Params) != 1 {
			log.Println("Invalid mouse_click parameters")
			return
		}
		button := cmd.Params[0]
		switch button {
		case "left":
			robotgo.Click("left", false)
		case "right":
			robotgo.Click("right", false)
		default:
			log.Println("Unknown mouse button:", button)
		}
		log.Printf("Clicked mouse button: %s", button)
	case "key_press":
		if len(cmd.Params) != 1 {
			log.Println("Invalid key_press parameters")
			return
		}
		key := cmd.Params[0]
		robotgo.KeyTap(key)
		log.Printf("Pressed key: %s", key)
	default:
		log.Println("Unknown control action:", cmd.Action)
	}
}
