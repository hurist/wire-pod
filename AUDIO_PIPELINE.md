# Wire-Pod 音频处理流水线文档

本文档详细分析 Wire-Pod 中音频从机器人采集到语音识别、再到语音合成的完整技术链路。音频处理是 Wire-Pod 最底层、最基础的能力，也是系统复杂度的主要来源之一。

---

## 1. 音频流水线总览

```
机器人侧
┌─────────────────────────────────────────────────────────────┐
│  [麦克风] → PCM 16kHz 16bit LE                              │
│       │                                                     │
│       ├── 新固件：Opus 编码 (66kbps, complexity 0)           │
│       └── 旧固件：PCM 原始数据                               │
│       │                                                     │
│       └── gRPC StreamingIntent/KG (InputAudio)              │
└─────────────────────────┬───────────────────────────────────┘
                          │  TLS over WiFi/LAN
                          ▼
Wire-Pod 服务端
┌─────────────────────────────────────────────────────────────┐
│  gRPC Receive                                               │
│       │                                                     │
│       ▼                                                     │
│  Opus 检测 (首字节 0x4F ?)                                   │
│       ├── 是 → opus.OggStream.Decode() → PCM               │
│       └── 否 → 直接使用 PCM                                 │
│       │                                                     │
│       ▼                                                     │
│  高通过滤器 (300Hz 截止，增益 7.5x)                          │
│       │                                                     │
│       ▼                                                     │
│  WebRTC VAD (模式 2，320 字节/帧)                            │
│       │                                                     │
│       ├── 活跃帧计数 → ActiveFrames++                        │
│       └── 非活跃帧计数 → InactiveFrames++                    │
│       │                                                     │
│       └── Inactive >= 23 && Active > 18 → 语音结束           │
│                                                             │
│       ▼                                                     │
│  STT 引擎 (Vosk/Whisper/Coqui/...)                          │
│       │                                                     │
│       └── 输出：transcribedText                              │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
AI 处理 / 意图匹配
                          │
                          ▼
TTS 输出
┌─────────────────────────────────────────────────────────────┐
│  方式 A: Vector 内置 TTS                                     │
│       SayText(text, UseVectorVoice=true)                    │
│       → 机器人本地语音合成                                   │
│                                                             │
│  方式 B: OpenAI TTS API                                      │
│       tts-1 模型 → PCM 24kHz                                │
│       → 降采样到 16kHz                                       │
│       → ExternalAudioStreamPlayback → 机器人播放             │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. 音频格式与协议

### 2.1 机器人原始音频

Vector 机器人的麦克风规格：
- **采样率**：16 kHz
- **位深度**：16 bit
- **编码**：有符号小端（Little-Endian, Signed）
- **通道**：单声道（Mono）

### 2.2 传输编码

**固件版本差异**：

| 固件版本 | 编码方式 | 首字节特征 |
|---|---|---|
| 早期固件 | PCM 原始数据 | 任意值（通常 0x00-0xFF） |
| 新固件（≥1.7） | Opus 封装为 Ogg | `0x4F` ('O'，Ogg 魔数) |

**Opus 参数**（机器人端编码）：
- 码率：66 kbps
- 复杂度：0（低延迟）
- 帧大小：60ms
- 封装：Ogg 容器

### 2.3 gRPC 传输

音频通过 `chipperpb.IntentGraphRequest.input_audio` 字段传输，类型为 `bytes`。

**流式传输特点**：
- 机器人持续调用 `stream.Send(request)`，每包包含一帧音频
- 服务端持续调用 `stream.Recv()` 接收
- 连接保持直到语音结束或超时

---

## 3. 音频接收与解码

### 3.1 SpeechRequest 创建

**位置**：`pkg/wirepod/speechrequest/speechrequest.go: ReqToSpeechRequest()`

**代码逻辑**：
```go
func ReqToSpeechRequest(req interface{}) SpeechRequest {
    var request SpeechRequest
    request.VADInst, _ = webrtcvad.New()
    request.VADInst.SetMode(2)  // 中等激进程度
    
    // 从 gRPC 首包提取音频
    switch r := req.(type) {
    case *vtt.IntentRequest:
        request.FirstReq = r.FirstReq.InputAudio
    case *vtt.IntentGraphRequest:
        request.FirstReq = r.FirstReq.InputAudio
    case *vtt.KnowledgeGraphRequest:
        request.FirstReq = r.FirstReq.InputAudio
    }
    
    // 检测 Opus
    isOpus := request.OpusDetect()
    if isOpus {
        request.OpusStream = &opus.OggStream{}
        decodedFirstReq, _ := request.OpusStream.Decode(request.FirstReq)
        request.FirstReq = highPassFilter(decodedFirstReq)
        request.FilteredMicData = append(request.FilteredMicData, request.FirstReq...)
        request.DecodedMicData = append(request.DecodedMicData, decodedFirstReq...)
        request.LastAudioChunk = request.FilteredMicData[request.PrevLen:]
        request.PrevLen = len(request.DecodedMicData)
        request.IsOpus = true
    }
    
    return request
}
```

**Opus 检测**：
```go
func (req *SpeechRequest) OpusDetect() bool {
    if len(req.FirstReq) > 0 && req.FirstReq[0] == 0x4f {
        return true  // 'O' = Ogg/Opus
    }
    return false   // PCM
}
```

> **注意**：仅检测首字节。若首包恰好以 0x4F 开头但实际是 PCM，会误判。不过这种情况概率极低。

### 3.2 Opus 解码

**库**：`github.com/digital-dream-labs/opus-go/opus`

**解码接口**：
```go
type OggStream struct{}

func (o *OggStream) Decode(data []byte) ([]byte, error)
```

**实现细节**：
- 输入：Ogg 封装的 Opus 数据包
- 输出：PCM 16kHz 16bit LE
- 内部维护 Ogg 页解析状态和 Opus 解码器状态

### 3.3 音频流持续接收

**位置**：`pkg/wirepod/speechrequest/speechrequest.go: GetNextStreamChunk()`

```go
func (req *SpeechRequest) GetNextStreamChunk() ([]byte, error) {
    // 根据 stream 类型断言为具体的 gRPC stream
    chunk, err := stream.Recv()  // 接收下一帧
    
    // 追加原始数据
    req.MicData = append(req.MicData, chunk.InputAudio...)
    
    // Opus 解码（如果是 Opus）
    decoded := req.OpusDecode(chunk.InputAudio)
    req.DecodedMicData = append(req.DecodedMicData, decoded...)
    
    // 高通过滤
    filtered := highPassFilter(decoded)
    req.FilteredMicData = append(req.FilteredMicData, filtered...)
    
    // 返回自上次调用以来的新增 PCM 数据
    dataReturn := req.DecodedMicData[req.PrevLen:]
    req.LastAudioChunk = req.FilteredMicData[req.PrevLen:]
    req.PrevLen = len(req.DecodedMicData)
    
    return dataReturn, nil
}
```

**内存管理问题**：
- `DecodedMicData` 和 `FilteredMicData` 持续追加，直到语音结束
- 对于长语音（如 10 秒），会积累约 320KB 的音频数据
- 语音结束后这些数据被 GC，但峰值内存占用较高

---

## 4. 音频过滤

### 4.1 高通过滤器

**位置**：`pkg/wirepod/speechrequest/speechrequest.go: highPassFilter()`

**参数**：
- 采样率：16 kHz
- 截止频率：300 Hz
- 增益：先 5x，再 1.5x（总增益 7.5x）

**算法**：一阶 RC 高通过滤器

```go
func highPassFilter(data []byte) []byte {
    sampleRate := 16000
    cutoffFreq := 300.0
    
    samples, _ := bytesToInt16(data)  // []int16
    samples = applyGain(samples, 5)   // 增益 5x
    
    filteredSamples := make([]float64, len(samples))
    rc := 1.0 / (2.0 * math.Pi * cutoffFreq)
    dt := 1.0 / float64(sampleRate)
    alpha := dt / (rc + dt)
    
    previous := float64(samples[0])
    for i := 1; i < len(samples); i++ {
        current := float64(samples[i])
        filtered := alpha * (filteredSamples[i-1] + current - previous)
        filteredSamples[i] = filtered
        previous = current
    }
    
    // 转回 int16，再增益 1.5x
    int16Filtered := make([]int16, len(filteredSamples))
    for i, s := range filteredSamples {
        int16Filtered[i] = int16(s)
    }
    gained := applyGain(int16Filtered, 1.5)
    
    return int16ToBytes(gained)
}
```

**为什么需要高通过滤？**

1. **去除低频噪声**：机器人电机、轮子摩擦会产生 <300Hz 的低频噪声
2. **Vosk 优化**：Vosk 在语音频段的识别率更高，去除低频可减少误识别
3. **增益补偿**：过滤后信号衰减，需要增益恢复音量

**性能**：
- 单次过滤 320 字节（160 个 sample）约需 5-10μs
- 可通过环境变量 `DEBUG_PRINT_HIGHPASS=true` 输出耗时

---

## 5. 语音活动检测（VAD）

### 5.1 VAD 引擎

**库**：`github.com/maxhawkins/go-webrtcvad`

**配置**：
```go
vad, _ := webrtcvad.New()
vad.SetMode(2)  // 模式 0-3，2 为中等敏感度
```

| 模式 | 敏感度 | 适用场景 |
|---|---|---|
| 0 | 最宽松 | 噪声环境，可能漏掉语音结束 |
| 1 | 宽松 | 一般环境 |
| 2 | 中等 | **默认，平衡误检和漏检** |
| 3 | 严格 | 安静环境，可能提前结束 |

### 5.2 帧处理

**帧大小**：320 字节 = 160 个 sample = 10ms @ 16kHz

```go
func SplitVAD(buf []byte) [][]byte {
    var chunks [][]byte
    for len(buf) >= 320 {
        chunks = append(chunks, buf[:320])
        buf = buf[320:]
    }
    return chunks
}
```

### 5.3 结束判定

```go
func (req *SpeechRequest) DetectEndOfSpeech() (bool, bool) {
    inactiveNumMax := 23  // 连续 23 帧（230ms）非活跃
    
    for _, chunk := range SplitVAD(req.LastAudioChunk) {
        active, err := req.VADInst.Process(16000, chunk)
        if err != nil { return true, false }
        
        if active {
            req.ActiveFrames++
            req.InactiveFrames = 0
        } else {
            req.InactiveFrames++
        }
        
        // 结束条件：足够长的非活跃期 + 之前有足够长的活跃期
        if req.InactiveFrames >= inactiveNumMax && req.ActiveFrames > 18 {
            return true, true  // (speechIsDone, doProcess)
        }
    }
    
    if req.ActiveFrames < 5 {
        return false, false  // 语音太短，不处理
    }
    return false, true  // 继续接收
}
```

**阈值含义**：
- `ActiveFrames > 18`：确保至少检测到 180ms 的有效语音（排除噪声突发）
- `InactiveFrames >= 23`：用户停止说话 230ms 后判定结束
- `ActiveFrames < 5`：若总活跃帧不足 50ms，视为无效语音，丢弃

**时序示例**：
```
时间轴 (ms)
0    100   200   300   400   500   600   700   800
|-----|-----|-----|-----|-----|-----|-----|-----|
AAAAAAAAAAAAAAAAAAAAAIIIIIIIIIIIIIIIIIIIIIIIIIIII
↑ 活跃帧              ↑ 非活跃帧 (23帧=230ms)
ActiveFrames=50      InactiveFrames=23
                     ↓
                触发结束判定
```

---

## 6. STT 引擎输入

### 6.1 统一接口

所有 STT 引擎接收的输入：
```go
func STT(req sr.SpeechRequest) (string, error)
```

引擎从 `req.DecodedMicData` 或 `req.FilteredMicData` 获取完整音频。

### 6.2 各引擎的音频消费方式

| 引擎 | 消费方式 | 音频来源 |
|---|---|---|
| **Vosk** | 流式 chunk-by-chunk | `FilteredMicData`（过滤后） |
| **whisper.cpp** | 整体一次性 | `DecodedMicData`（仅解码，未过滤） |
| **Whisper API** | 整体一次性，转 WAV | `DecodedMicData` |
| **Coqui** | 流式 | `DecodedMicData` |
| **Leopard** | 整体一次性 | `DecodedMicData` |
| **Houndify** | 流式原始 Opus | `MicData`（原始接收数据） |

**Vosk 的特殊处理**（`pkg/wirepod/stt/vosk/Vosk.go`）：
```go
func STT(req sr.SpeechRequest) (string, error) {
    // 从 pool 获取 recognizer
    rec := getRecognizerFromPool()
    defer returnRecognizerToPool(rec)
    
    // 逐帧送入 VAD chunk
    for _, chunk := range sr.SplitVAD(req.FilteredMicData) {
        rec.AcceptWaveform(chunk)
    }
    
    return rec.FinalResult(), nil
}
```

---

## 7. TTS 输出

### 7.1 方式一：Vector 内置 TTS

**调用**：`robot.Conn.SayText(ctx, &vectorpb.SayTextRequest{...})`

**参数**：
```go
&vectorpb.SayTextRequest{
    Text:           "Hello, I am Vector",
    UseVectorVoice: true,     // 使用 Vector 自己的语音
    DurationScalar: 1.0,       // 语速倍率
}
```

**流程**：
1. gRPC 请求发送到机器人
2. 机器人本地 TTS 引擎（基于 Festival/LTM）合成语音
3. 通过机器人扬声器播放
4. 同时可配合动画

**限制**：
- 仅支持英语
- 语音质量一般，机械感明显
- 无法调节音色

### 7.2 方式二：OpenAI TTS

**触发条件**：
- `vars.APIConfig.STT.Language != "en-US"`
- 或 `vars.APIConfig.Knowledge.OpenAIVoiceWithEnglish == true`

**调用链**：
```go
// 1. 请求 OpenAI TTS API
audioData, _ := c.CreateSpeech(ctx, openai.CreateSpeechRequest{
    Model: openai.TTSModel1,
    Input: text,
    Voice: openai.VoiceAlloy,  // 或其他 voice
})

// 2. 读取 PCM 数据（24kHz）
// 3. 降采样到 16kHz
// 4. 切分为 1024 字节块
// 5. 通过 ExternalAudioStreamPlayback 发送到机器人
```

**ExternalAudioStreamPlayback 协议**：
```go
// 准备阶段
audioClient.SendMsg(&vectorpb.ExternalAudioStreamRequest{
    AudioRequestType: &vectorpb.ExternalAudioStreamRequest_AudioStreamPrepare{
        AudioStreamPrepare: &vectorpb.ExternalAudioStreamPrepare{
            AudioFrameRate: 16000,
            AudioVolume:    100,
        },
    },
})

// 发送音频块
for _, chunk := range audioChunks {
    audioClient.SendMsg(&vectorpb.ExternalAudioStreamRequest{
        AudioRequestType: &vectorpb.ExternalAudioStreamRequest_AudioStreamChunk{
            AudioStreamChunk: &vectorpb.ExternalAudioStreamChunk{
                AudioChunkSizeBytes: uint32(len(chunk)),
                AudioChunkSamples:   chunk,
            },
        },
    })
}

// 完成
audioClient.SendMsg(&vectorpb.ExternalAudioStreamRequest{
    AudioRequestType: &vectorpb.ExternalAudioStreamRequest_AudioStreamComplete{
        AudioStreamComplete: &vectorpb.ExternalAudioStreamComplete{},
    },
})
```

---

## 8. SDK App 音频播放

**位置**：`pkg/wirepod/sdkapp/server.go: /api-sdk/play_sound`

**功能**：用户上传音频文件，Wire-Pod 通过 `ExternalAudioStreamPlayback` 在机器人上播放。

**要求**：
- 格式：WAV（客户端负责转换为 8kHz 单声道）
- 上传方式：`multipart/form-data`，字段名 `sound`

**处理**：
```go
file, _, _ := r.FormFile("sound")
pcmFile, _ := io.ReadAll(file)

// 切分为 1024 字节块
var audioChunks [][]byte
for len(pcmFile) >= 1024 {
    audioChunks = append(audioChunks, pcmFile[:1024])
    pcmFile = pcmFile[1024:]
}

// 以 8000Hz 播放
audioClient.SendMsg(&vectorpb.ExternalAudioStreamRequest{
    AudioRequestType: &vectorpb.ExternalAudioStreamRequest_AudioStreamPrepare{
        AudioStreamPrepare: &vectorpb.ExternalAudioStreamPrepare{
            AudioFrameRate: 8000,
            AudioVolume:    100,
        },
    },
})

// 每块间隔 60ms 发送
for _, chunk := range audioChunks {
    audioClient.SendMsg(...)
    time.Sleep(time.Millisecond * 60)
}
```

---

## 9. 音频缓存与调试

### 9.1 调试音频写入

**位置**：`pkg/wirepod/speechrequest/speechrequest.go`

将 `debugWriteFile` 设为 `true`：
```go
var debugWriteFile bool = true
```

效果：
- 在 `/tmp/wirepodtest.ogg` 写入原始接收的音频数据
- 可用于离线分析机器人发送的音频质量

### 9.2 音频数据生命周期

```
接收 frame
    ├── MicData (原始)          ──► 仅 Houndify 使用
    ├── DecodedMicData (PCM)    ──► whisper.cpp, Whisper API, Coqui, Leopard
    └── FilteredMicData (过滤)  ──► Vosk, VAD
        │
        └── VAD 处理
            │
            └── STT 引擎
                │
                └── 结果返回后，所有音频数据被 GC
```

### 9.3 内存优化建议

当前实现中，音频数据在 `SpeechRequest` 中以 `[]byte` 持续追加。对于长语音：
- 10 秒语音 ≈ 16000 sample/s × 2 bytes × 10s = 320KB
- 同时保存 `MicData` + `DecodedMicData` + `FilteredMicData` ≈ 1MB

**优化方向**：
1. 使用 ring buffer 替代无限追加
2. 对于整体式 STT 引擎（whisper.cpp），不需要保存全部历史，只需保留一个完整 buffer
3. `FilteredMicData` 和 `DecodedMicData` 可以共享底层数组（过滤是原地操作）

---

## 10. 常见问题排查

### 10.1 音频问题诊断流程

```
机器人无响应
    │
    ├── 检查日志中是否有 "Stream type: OPUS/PCM"
    │       └── 无 → 机器人未连接或 gRPC 流未建立
    │
    ├── 检查 "End of speech detected."
    │       └── 无 → VAD 未触发，可能音频未到达或全是噪声
    │           ├── 检查 debugWriteFile 输出的音频
    │           └── 确认麦克风未被遮挡
    │
    ├── 检查 STT 输出
    │       └── 空或乱码 → 音频质量差或模型不匹配
    │           ├── 确认 STT 语言设置与说话语言一致
    │           └── 尝试更换安静环境
    │
    └── 检查意图匹配日志
            └── "Intent matched: intent_system_unmatched"
                └── 说明语音识别成功但意图未匹配，非音频问题
```

### 10.2 常见音频故障

| 现象 | 可能原因 | 解决 |
|---|---|---|
| 语音结束判定过早 | 环境噪声大，VAD 模式太严格 | 改用模式 1，或让机器人离噪声源远些 |
| 语音结束判定过晚 | 回声或持续背景音 | 改用模式 3，或检查高通过滤是否正常 |
| 识别结果全是乱码 | Opus/PCM 检测错误 | 检查首字节，确认固件版本 |
| 识别准确率极低 | 使用了错误的语言模型 | 确认 `STT_LANGUAGE` 与实际语言匹配 |
| 音频播放卡顿 | 网络抖动或块大小不匹配 | 检查 `time.Sleep` 间隔是否与帧率匹配 |
| OpenAI TTS 无声 | 降采样失败或格式错误 | 确认采样率为 16kHz，16bit LE |

---

## 11. 音频相关配置项

| 配置项 | 位置 | 说明 |
|---|---|---|
| `STT_LANGUAGE` | 环境变量 / `apiConfig.json` | 决定加载哪个意图文件和 Vosk 模型 |
| `VOSK_WITH_GRAMMER` | 环境变量 | 启用 Vosk 语法优化模式 |
| `DEBUG_PRINT_HIGHPASS` | 环境变量 | 输出高通过滤耗时 |
| `debugWriteFile` | 代码常量 | 写调试音频到 `/tmp/wirepodtest.ogg` |
| VAD 模式 | 代码常量 (`speechrequest.go`) | 当前硬编码为模式 2 |

---

*本文档基于 `pkg/wirepod/speechrequest/speechrequest.go`、各 `pkg/wirepod/stt/*/` 目录和 `pkg/wirepod/sdkapp/server.go` 的实际代码编写。*
