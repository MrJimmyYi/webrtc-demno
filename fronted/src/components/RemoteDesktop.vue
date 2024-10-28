<!-- src/components/RemoteDesktop.vue -->
<template>
  <div class="remote-desktop-container">
    <video ref="remoteVideo" autoplay playsinline></video>
    <div
        class="control-area"
        @mousemove="handleMouseMove"
        @click="handleClick"
        @keydown="handleKeyPress"
        tabindex="0"
    >
      <p>控制区域：移动鼠标或点击</p>
    </div>
    <button v-if="!isController && !isDenied" @click="requestControl">
      请求控制
    </button>
    <button v-if="isController" @click="releaseControl">
      释放控制
    </button>
    <p v-if="isDenied" class="warning">控制权已被占用，请稍后再试。</p>
  </div>
</template>

<script>
import SimplePeer from 'simple-peer'

export default {
  name: 'RemoteDesktop',
  data() {
    return {
      peer: null,
      ws: null,
      isController: false,
      isDenied: false,
    }
  },
  mounted() {
    this.initWebRTC()
  },
  methods: {
    initWebRTC() {
      // 连接到中继服务器的 WebSocket
      this.ws = new WebSocket('ws://127.0.0.1:8080/ws') // 替换为服务端B的实际IP

      this.ws.onopen = () => {
        console.log('Connected to signaling server.')

        // 注册为 viewer
        const register = {
          type: 'register',
          payload: { role: 'viewer' },
        }
        this.ws.send(JSON.stringify(register))
        console.log('Sent register message:', register)

        // 初始化 SimplePeer，作为 Initiator
        this.peer = new SimplePeer({
          initiator: true, // 设置为 true，确保 Viewer 作为 Initiator
          trickle: true, // 设置为 true，以便即时发送 ICE Candidates
          config: {
            iceServers: [
              {
                urls: 'turn:192.168.40.100:23478',
                username: 'jimmy',
                credential: 'apple',
              },
            ],
            iceTransportPolicy: 'relay', // 强制使用 TURN 中继
          },
        })

        // 处理信令数据
        this.peer.on('signal', data => {
          const message = {
            type: data.type, // 'offer', 'candidate'
            payload: data,    // 直接传递对象
          }
          this.ws.send(JSON.stringify(message))
          console.log('Sent signal message:', message)
        })

        // 处理接收到的视频流
        this.peer.on('stream', stream => {
          console.log('Received stream:', stream)
          this.$refs.remoteVideo.srcObject = stream
          this.$refs.remoteVideo.onloadedmetadata = () => {
            console.log('Video metadata loaded.')
            this.$refs.remoteVideo.play().then(() => {
              console.log('Video is playing.')
            }).catch(err => {
              console.error('Failed to play video:', err)
            })
          }
        })

        // 处理来自 DataChannel 的数据（可选）
        this.peer.on('data', data => {
          // 可选：处理来自控制端的数据
          console.log('Data received from peer:', data.toString())
        })

        // 处理错误
        this.peer.on('error', err => {
          console.error('Peer error:', err)
        })

        // 处理连接关闭
        this.peer.on('close', () => {
          console.log('Peer connection closed.')
        })

        // 监听 ICE 连接状态变化
        this.peer.on('iceconnectionstatechange', state => {
          console.log('ICE Connection State changed:', state)
          if (state === 'failed' || state === 'disconnected') {
            console.error('ICE connection failed or disconnected.')
          }
        })
      }

      // 处理来自信令服务器的消息
      this.ws.onmessage = event => {
        try {
          const data = JSON.parse(event.data)
          console.log('Received message:', data) // 确认消息内容

          switch (data.type) {
            case 'register_success':
              // 注册成功，可以进行下一步操作
              console.log('Registered successfully as viewer.')
              break
            case 'register_failed':
              // 注册失败，显示错误
              const reason = data.payload.reason
              console.error('Register failed:', reason)
              alert(`注册失败: ${reason}`)
              break
            case 'offer':
              this.handleOffer(data.payload)
              break
            case 'answer':
              this.handleAnswer(data.payload)
              break
            case 'candidate':
              console.log(data.payload)
              this.handleCandidate(data.payload)
              break
            case 'desktop_disconnected':
              alert('桌面已断开连接。')
              break
            default:
              console.log('Unknown message type:', data.type)
          }
        } catch (error) {
          console.error('Error parsing message:', error)
        }
      }

      this.ws.onerror = error => {
        console.error('WebSocket error:', error)
      }

      this.ws.onclose = () => {
        console.log('WebSocket connection closed.')
      }
    },
    handleOffer(payload) {
      // Viewer端通常不需要处理来自服务端的Offer
      console.log('Received unexpected offer:', payload)
    },
    handleAnswer(payload) {
      const answer = payload // 已经是对象
      console.log('Received answer:', answer)
      this.peer.signal(answer)
    },
    handleCandidate(payload) {
      try {
        // 构建 RTCIceCandidateInit 对象
        const iceCandidateInit = new RTCIceCandidate({
          candidate: payload.candidate,
          sdpMLineIndex: payload.sdpMLineIndex,
          sdpMid: payload.sdpMid
        });

        console.log('Received ICE candidate:', iceCandidateInit.candidate);

        // 传递给 peer
        this.peer.signal(iceCandidateInit);

      } catch (error) {
        console.error('Error constructing ICE candidate:', error);
      }
    },
    handleControlGranted() {
      console.log('Control granted.')
      this.isController = true
      this.isDenied = false
      alert('您已获得控制权！')
      // 绑定键盘事件
      window.addEventListener('keydown', this.handleKeyPressGlobal)
    },
    handleControlDenied() {
      console.log('Control denied.')
      this.isController = false
      this.isDenied = true
      alert('控制权已被占用，请稍后再试。')
    },
    requestControl() {
      const request = {
        type: 'control_command',
        payload: { action: 'request_control' },
      }
      this.ws.send(JSON.stringify(request))
      console.log('Sent control request.')
    },
    releaseControl() {
      const release = {
        type: 'control_command',
        payload: { action: 'release_control' },
      }
      this.ws.send(JSON.stringify(release))
      console.log('Sent control release.')
      this.isController = false
      this.isDenied = false
      // 移除键盘事件
      window.removeEventListener('keydown', this.handleKeyPressGlobal)
    },
    handleMouseMove(event) {
      if (!this.isController) return

      // 计算鼠标在控制区域内的相对位置
      const rect = event.target.getBoundingClientRect()
      const x = Math.round(event.clientX - rect.left)
      const y = Math.round(event.clientY - rect.top)

      const cmd = {
        action: 'mouse_move',
        params: [x.toString(), y.toString()],
      }
      this.sendCommand(cmd)
    },
    handleClick(event) {
      if (!this.isController) return

      const cmd = {
        action: 'mouse_click',
        params: ['left'], // 可根据需要更改为 'right' 等
      }
      this.sendCommand(cmd)
    },
    handleKeyPress(event) {
      // 仅当作为控制者时处理
      if (!this.isController) return

      const key = event.key
      const cmd = {
        action: 'key_press',
        params: [key],
      }
      this.sendCommand(cmd)
    },
    handleKeyPressGlobal(event) {
      // 全局键盘事件处理
      this.handleKeyPress(event)
    },
    sendCommand(cmd) {
      const cmdStr = JSON.stringify(cmd)
      if (this.peer && this.peer.connected) {
        this.peer.send(cmdStr)
      }
    },
  },
}
</script>

<style scoped>
.remote-desktop-container {
  position: relative;
  width: 800px;
  height: 600px;
  margin: auto;
  border: 1px solid #ccc;
}

video {
  width: 100%;
  height: 100%;
  background-color: #000;
}

.control-area {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  /* 使控制区域透明但捕获鼠标事件 */
  background-color: rgba(0, 0, 0, 0);
  cursor: crosshair;
  outline: none;
}

button {
  position: absolute;
  top: 10px;
  left: 10px;
  padding: 10px 20px;
  z-index: 10;
}

.warning {
  position: absolute;
  top: 50px;
  left: 10px;
  color: red;
  z-index: 10;
}
</style>
