<template>
  <div id="app">
    <video id="remoteVideo" autoplay playsinline muted></video>
  </div>
</template>

<script>
export default {
  name: 'RemoteDesktop',
  data() {
    return {
      signalingServerUrl: 'ws://192.168.40.100:8080/ws', // 替换为您的信令服务器地址
      websocket: null,
      peerConnection: null,
      dataChannel: null,
      controlEventsSetup: false,
      iceServers: [{
        urls: ['turn:192.168.40.100:23478'], // 替换为您的 TURN 服务器地址和端口
        username: 'jimmy', // 替换为您的 TURN 服务器用户名
        credential: 'apple' // 替换为您的 TURN 服务器密码
      }],
    }
  },
  mounted() {
    this.initWebSocket();
  },
  methods: {
    initWebSocket() {
      this.websocket = new WebSocket(this.signalingServerUrl);

      this.websocket.onopen = () => {
        console.log('WebSocket 连接已打开');
        // 注册为 viewer
        const registerMessage = {
          type: 'register',
          payload: {
            role: 'viewer'
          }
        };
        this.websocket.send(JSON.stringify(registerMessage));
      };

      this.websocket.onmessage = this.handleSignalingMessage;
      this.websocket.onerror = (error) => {
        console.error('WebSocket 错误:', error);
      };
      this.websocket.onclose = () => {
        console.log('WebSocket 连接已关闭');
      };
    },
    handleSignalingMessage(event) {
      const message = JSON.parse(event.data);
      console.log('收到信令服务器消息:', message);

      switch (message.type) {
        case 'register_success':
          console.log('成功注册为 viewer');
          // 注册成功后，初始化 WebRTC 连接
          this.initPeerConnection();
          break;
        case 'register_failed':
          console.error('注册失败:', message.payload.reason);
          break;
        case 'answer':
          this.handleAnswer(message.payload);
          break;
        case 'candidate':
          this.handleCandidate(message.payload);
          break;
        case 'desktop_disconnected':
          console.log('桌面端已断开连接');
          // 处理断开连接的情况
          break;
        default:
          console.warn('未知的消息类型:', message.type);
      }
    },
    async initPeerConnection() {
      // 检查浏览器是否支持 H.264 编码器
      const h264Codec = RTCRtpReceiver.getCapabilities('video').codecs.find(
          codec => codec.mimeType.toLowerCase() === 'video/h264'
      );

      if (!h264Codec) {
        console.error('浏览器不支持 H.264 编码器');
        return;
      }

      // 创建 PeerConnection
      this.peerConnection = new RTCPeerConnection({
        iceServers: this.iceServers,
        iceTransportPolicy: 'relay' // 强制使用 TURN 服务器
      });

      // 调整编解码器优先级，将 H.264 放在首位
      const transceiver = this.peerConnection.addTransceiver('video', { direction: 'recvonly' });
      transceiver.setCodecPreferences([h264Codec]);

      // 处理 ICE 候选
      this.peerConnection.onicecandidate = (event) => {
        if (event.candidate) {
          const candidateMessage = {
            type: 'candidate',
            payload: {
              candidate: event.candidate.toJSON()
            }
          };
          this.websocket.send(JSON.stringify(candidateMessage));
          console.log('发送 ICE 候选:', candidateMessage);
        }
      };

      // 处理远程视频流
      this.peerConnection.ontrack = (event) => {
        console.log('收到远程视频流:', event);
        const remoteVideo = document.getElementById('remoteVideo');
        const [stream] = event.streams;
        if (remoteVideo.srcObject !== stream) {
          remoteVideo.srcObject = stream;
          console.log('设置远程视频流');

          // 在视频元数据加载后再设置事件监听器
          remoteVideo.onloadedmetadata = () => {
            console.log('视频元数据已加载');
            this.setupControlEvents();
          };
        }
      };

      // 接收 DataChannel
      this.peerConnection.ondatachannel = (event) => {
        console.log('收到 DataChannel:', event.channel);
        this.dataChannel = event.channel;
        this.setupDataChannel();
      };

      // 创建 DataChannel 用于发送控制指令
      this.dataChannel = this.peerConnection.createDataChannel('control');
      this.setupDataChannel();

      // 创建 Offer 并发送给信令服务器
      try {
        const offer = await this.peerConnection.createOffer();
        await this.peerConnection.setLocalDescription(offer);
        const offerMessage = {
          type: 'offer',
          payload: this.peerConnection.localDescription
        };
        this.websocket.send(JSON.stringify(offerMessage));
        console.log('发送 Offer:', offerMessage);
      } catch (error) {
        console.error('创建 Offer 出错:', error);
      }

      // 添加连接状态变化的监听器（可选）
      this.peerConnection.oniceconnectionstatechange = () => {
        console.log('ICE 连接状态:', this.peerConnection.iceConnectionState);
      };
      this.peerConnection.onconnectionstatechange = () => {
        console.log('连接状态:', this.peerConnection.connectionState);
      };
    },
    handleAnswer(answer) {
      const remoteDesc = new RTCSessionDescription(answer);
      this.peerConnection.setRemoteDescription(remoteDesc)
          .then(() => {
            console.log('设置远程描述成功');
          })
          .catch((error) => {
            console.error('设置远程描述出错:', error);
          });
    },
    handleCandidate(candidate) {
      const iceCandidate = new RTCIceCandidate(candidate);
      this.peerConnection.addIceCandidate(iceCandidate)
          .then(() => {
            console.log('添加 ICE 候选成功');
          })
          .catch((error) => {
            console.error('添加 ICE 候选出错:', error);
          });
    },
    setupDataChannel() {
      this.dataChannel.onopen = () => {
        console.log('DataChannel 已打开');
        // DataChannel 打开后，不立即设置事件监听，等待视频元数据加载完成
      };
      this.dataChannel.onmessage = (event) => {
        console.log('收到 DataChannel 消息:', event.data);
      };
      this.dataChannel.onerror = (error) => {
        console.error('DataChannel 错误:', error);
      };
      this.dataChannel.onclose = () => {
        console.log('DataChannel 已关闭');
      };
    },
    setupControlEvents() {
      if (this.controlEventsSetup) return;

      const remoteVideo = document.getElementById('remoteVideo');
      remoteVideo.addEventListener('mousemove', this.handleMouseMove);
      remoteVideo.addEventListener('click', this.handleMouseClick);
      window.addEventListener('keydown', this.handleKeyDown);
      this.controlEventsSetup = true;
      console.log('已设置控制事件监听器');
    },
    handleMouseMove(event) {
      const rect = event.target.getBoundingClientRect();
      const x = event.clientX - rect.left;
      const y = event.clientY - rect.top;

      const videoElement = event.target;
      const videoWidth = videoElement.videoWidth;
      const videoHeight = videoElement.videoHeight;

      // 防止 videoWidth 或 videoHeight 为 0
      if (videoWidth === 0 || videoHeight === 0) {
        console.warn('视频尺寸未就绪');
        return;
      }

      const scaledX = Math.floor(x * videoWidth / rect.width);
      const scaledY = Math.floor(y * videoHeight / rect.height);

      const command = {
        action: 'mouse_move',
        params: [scaledX.toString(), scaledY.toString()]
      };
      if (this.dataChannel && this.dataChannel.readyState === 'open') {
        this.dataChannel.send(JSON.stringify(command));
      }
    },
    handleMouseClick(event) {
      const command = {
        action: 'mouse_click',
        params: ['left'] // 根据需要调整
      };
      if (this.dataChannel && this.dataChannel.readyState === 'open') {
        this.dataChannel.send(JSON.stringify(command));
      }
    },
    handleKeyDown(event) {
      const key = event.key;
      const command = {
        action: 'key_press',
        params: [key]
      };
      if (this.dataChannel && this.dataChannel.readyState === 'open') {
        this.dataChannel.send(JSON.stringify(command));
      }
    }
  },
  beforeDestroy() {
    if (this.websocket) {
      this.websocket.close();
    }
    if (this.peerConnection) {
      this.peerConnection.close();
    }
  }
}
</script>

<style>
#remoteVideo {
  width: 100%;
  height: 100%;
}
</style>
