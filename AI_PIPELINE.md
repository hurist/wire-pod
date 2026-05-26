# Wire-Pod AI 对话流水线文档

本文档详细分析 Wire-Pod 中从语音识别到 AI 回复再到机器人语音输出的完整链路。这是 Wire-Pod 最核心的价值所在——将现代大语言模型（LLM）接入消费级机器人。

---

## 1. 流水线总览

```
用户说话
    │
    ▼
┌──────────────────────────────────────────────────────────────┐
│  Phase 1: 语音识别（STT）                                     │
│  机器人麦克风 → Opus/PCM → gRPC → VAD → STT 引擎 → 文本      │
└──────────────────────────┬───────────────────────────────────┘
                           │  transcribedText
                           ▼
┌──────────────────────────────────────────────────────────────┐
│  Phase 2: 意图路由（Intent Routing）                          │
│  插件 → 自定义意图 → 内置意图 → LLM fallback                  │
└──────────────────────────┬───────────────────────────────────┘
                           │  若未匹配且 KG 启用
                           ▼
┌──────────────────────────────────────────────────────────────┐
│  Phase 3: LLM 对话（Knowledge Graph / Intent Graph）          │
│  构造 Prompt → 流式请求 → 接收 SSE → 句子切分 → 命令提取      │
└──────────────────────────┬───────────────────────────────────┘
                           │  sentence + commands
                           ▼
┌──────────────────────────────────────────────────────────────┐
│  Phase 4: 机器人执行（Robot Execution）                       │
│  BehaviorControl → 动画 → SayText/TTS → 释放控制权            │
└──────────────────────────────────────────────────────────────┘
```

---

## 2. Phase 1: 语音识别（STT）

详见 [AUDIO_PIPELINE.md](AUDIO_PIPELINE.md)。本阶段输出为纯文本 `transcribedText`。

**关键输出**：`transcribedText`（小写、去除首尾空格后的识别文本）

**进入 AI 流水线的条件**：意图匹配失败，且 `vars.APIConfig.Knowledge.Enable == true`，且满足以下之一：
- 请求类型为 `KnowledgeGraph`（用户说 "I have a question"）
- 请求类型为 `IntentGraph` 且 `vars.APIConfig.Knowledge.IntentGraph == true`

---

## 3. Phase 2: 意图路由

**位置**：`pkg/wirepod/ttr/matchIntentSend.go: ProcessTextAll()`

**匹配优先级**（严格按此顺序，一旦匹配立即停止）：

```go
func ProcessTextAll(req, voiceText string, intentList []JsonIntent, isOpus bool) bool {
    // 1. Go 插件
    if pluginFunctionHandler(req, voiceText, botSerial) {
        return true
    }
    
    // 2. 自定义意图
    if customIntentHandler(req, voiceText, botSerial) {
        return true
    }
    
    // 3. 内置意图
    for _, intent := range intentList {
        // 3a. 精确匹配
        if voiceText == keyphrase { match }
        // 3b. 子串匹配（除非 RequireExactMatch）
        if !intent.RequireExactMatch && strings.Contains(voiceText, keyphrase) { match }
    }
    
    if match {
        ParamChecker(req, matchedIntent, voiceText, botSerial)
        IntentPass(req, matchedIntent, voiceText, params, isParam)
        return true
    }
    
    return false  // 未匹配，进入 LLM fallback
}
```

**为什么先匹配意图再 fallback 到 LLM？**

这是出于**响应速度和确定性**的考虑：
- 内置意图（如 "go to sleep", "what time is it"）有确定的机器人动画和音效，响应极快
- LLM 需要网络往返 + 流式生成，延迟在数百毫秒到数秒
- 本地命令（如音量调节、颜色切换）不应走 LLM

---

## 4. Phase 3: LLM 对话核心

**入口函数**：`pkg/wirepod/ttr/kgsim.go: StreamingKGSim()`

### 4.1 机器人连接准备

```go
func StreamingKGSim(req interface{}, esn string, transcribedText string, isKG bool) {
    // 1. 从 botSdkInfo.json 查找机器人
    var robot *vector.Vector
    var guid, target string
    for _, bot := range vars.BotInfo.Robots {
        if esn == bot.Esn {
            guid = bot.GUID
            target = bot.IPAddress + ":443"
            break
        }
    }
    
    // 2. 使用 vector-go-sdk 创建连接
    robot, _ = vector.New(
        vector.WithSerialNo(esn),
        vector.WithToken(guid),
        vector.WithTarget(target),
    )
    
    // 3. 验证连接（BatteryState 是最轻的 ping）
    _, err := robot.Conn.BatteryState(ctx, &vectorpb.BatteryStateRequest{})
    if err != nil { return "", err }
```

> **注意**：这里通过 SDK 直接连接机器人，绕过了 gRPC 返回路径。这意味着 LLM 的响应不是通过 `IntentGraphResponse` 发回，而是直接操控机器人。

### 4.2 行为控制接管

```go
    // 创建 channel 用于 behavior control 信号
    start := make(chan bool)      // 控制已获取
    stop := make(chan bool)       // 请求释放控制
    stopStop := make(chan bool)   // 确认释放完成
    
    // 请求 OVERRIDE_BEHAVIORS 级别控制
    BControl(robot, ctx, start, stop)
```

**`BControl` 内部**（`pkg/wirepod/ttr/bcontrol.go`）：
```go
func BControl(robot *vector.Vector, ctx context.Context, start, stop chan bool) {
    go func() {
        r, _ := robot.Conn.BehaviorControl(ctx)
        r.Send(&vectorpb.BehaviorControlRequest{
            RequestType: &vectorpb.BehaviorControlRequest_ControlRequest{
                ControlRequest: &vectorpb.ControlRequest{
                    Priority: vectorpb.ControlRequest_OVERRIDE_BEHAVIORS,
                },
            },
        })
        // 等待 ControlGrantedResponse
        for {
            ctrlresp, _ := r.Recv()
            if ctrlresp.GetControlGrantedResponse() != nil {
                start <- true
                break
            }
        }
        // 等待 stop 信号
        <-stop
        r.Send(&vectorpb.BehaviorControlRequest{
            RequestType: &vectorpb.BehaviorControlRequest_ControlRelease{...},
        })
    }()
}
```

> **设计意图**：必须先获得行为控制权，才能播放动画和语音，否则机器人的自主行为会打断回答。

### 4.3 "思考中" 动画

若 `isKG == true`（知识图谱请求），在请求 LLM 之前播放 searching 动画：

```go
    if isKG {
        BControl(robot, ctx, start, stop)
        go func() {
            for {
                if kgStopLooping { break }
                robot.Conn.PlayAnimation(ctx, &vectorpb.PlayAnimationRequest{
                    Animation: &vectorpb.Animation{
                        Name: "anim_knowledgegraph_searching_01",
                    },
                    Loops: 1,
                })
                time.Sleep(time.Second / 3)
            }
        }()
    }
```

### 4.4 Prompt 构造

**函数**：`CreateAIReq()`（`kgsim.go` 中）

**Prompt 组成**：

```
System Message:
├── 基础人设: "You are a helpful, animated robot called Vector. 
│             Keep the response concise yet informative."
├── 用户自定义: vars.APIConfig.Knowledge.OpenAIPrompt（若非空则覆盖）
├── 格式约束: "No special characters... No lists. No formatting."
└── 命令参考（若 CommandsEnable）:
    "You are able to perform some actions... 
     playAnimationWI||animName (without interrupting speech)
     playAnimation||animName (interrupts speech)
     getImage||front (captures image)
     newVoiceRequest||now (triggers new voice request)"

Message History（若 SaveChat 启用）:
├── 最多 16 条消息（8 轮对话）
└── 按 ESN 隔离（vars.RememberedChats）

User Message:
└── transcribedText（STT 识别出的用户语音文本）
```

**模型选择**：
```go
if gpt3tryagain {
    model = openai.GPT3Dot5Turbo
} else if vars.APIConfig.Knowledge.Provider == "openai" {
    model = openai.GPT4oMini
} else {
    model = vars.APIConfig.Knowledge.Model
}
```

**请求参数**：
```go
openai.ChatCompletionRequest{
    Model:            model,
    MaxTokens:        2048,
    Temperature:      1,
    TopP:             1,
    FrequencyPenalty: 0,
    PresencePenalty:  0,
    Messages:         nChat,  // System + History + User
    Stream:           true,   // 必须流式
}
```

### 4.5 流式响应处理

```go
stream, _ := c.CreateChatCompletionStream(ctx, aireq)

// LLM 读取 goroutine
go func() {
    for {
        response, err := stream.Recv()
        if errors.Is(err, io.EOF) {
            // 流结束，处理尾部内容
            isDone = true
            // 保存对话记忆
            if vars.APIConfig.Knowledge.SaveChat {
                Remember(userMsg, assistantMsg, esn)
            }
            return
        }
        
        // 追加内容
        fullRespText += removeSpecialCharacters(response.Choices[0].Delta.Content)
        
        // 句子切分（按标点）
        if strings.Contains(fullRespText, ".") || strings.Contains(fullRespText, "?") || 
           strings.Contains(fullRespText, "!") || strings.Contains(fullRespText, "...") {
            // 切分出完整句子，发送到 speakReady channel
            speakReady <- sentence
            successIntent <- true
        }
    }
}()
```

**句子切分逻辑**：
1. 检查文本中是否包含句子结束标点：`...` > `.'` > `."` > `.` > `?` > `!`
2. 按优先级选择分隔符，切分出第一个完整句子
3. 将句子放入 `fullRespSlice` 数组
4. 通过 `speakReady` channel 通知主循环可以播放了

**为什么需要句子切分？**

因为 LLM 的流式输出是一个字一个字来的，但机器人的 TTS 需要完整的句子才能自然播报。同时，切分后可以在句子之间插入动画命令的执行。

### 4.6 对话记忆（Memory）

**存储位置**：`vars.RememberedChats`（内存数组）

**结构**：
```go
type RememberedChat struct {
    ESN   string
    Chats []openai.ChatCompletionMessage  // 最多 16 条
}
```

**保存时机**：LLM 流完全结束后（`io.EOF`），将完整的 User + Assistant 消息对追加到对应 ESN 的记忆中。

**淘汰策略**：FIFO。当达到 16 条时，移除最前面的 2 条（即最早的一轮对话）。

**隔离性**：按机器人 ESN 隔离，每台机器人有自己的独立记忆。

**清空方式**：调用 `POST /api/delete_chats` 或重启进程。

> **设计限制**：记忆仅保存在内存中，进程重启后丢失。若需持久化，需自行扩展 `vars.go` 的保存逻辑。

---

## 5. Phase 4: 机器人执行

### 5.1 主执行循环

```go
// 等待 behavior control granted
for range start {
    // 停止 searching 动画
    if isKG { kgStopLooping = true }
    
    // 播放 "准备说话" 动画
    robot.Conn.PlayAnimation(ctx, &vectorpb.PlayAnimationRequest{
        Animation: &vectorpb.Animation{Name: TTSGetinAnimation},  // "anim_getin_tts_01"
        Loops: 1,
    })
    
    // 启动 "说话中" 循环动画（除非启用了 Commands）
    if !vars.APIConfig.Knowledge.CommandsEnable {
        go func() {
            for {
                if stopTTSLoop { break }
                robot.Conn.PlayAnimation(ctx, &vectorpb.PlayAnimationRequest{
                    Animation: &vectorpb.Animation{Name: TTSLoopAnimation},  // "anim_tts_loop_02"
                    Loops: 1,
                })
            }
        }()
    }
    
    // 逐句播放
    numInResp := 0
    for {
        respSlice := fullRespSlice
        if len(respSlice)-1 < numInResp {
            if !isDone {
                // 等待 LLM 输出更多内容
                <-speakReady
            } else {
                break  // 全部说完
            }
        }
        
        if interrupted { break }
        
        // 提取并执行命令
        acts := GetActionsFromString(respSlice[numInResp])
        disconnect := PerformActions(nChat, acts, robot, stopStop)
        if disconnect { break }
        
        numInResp++
    }
    
    // 清理
    stopTTSLoop = true
    if !interrupted {
        stopStop <- true
        stop <- true  // 释放 behavior control
    }
}
```

### 5.2 命令提取与执行

**位置**：`pkg/wirepod/ttr/kgsim_cmds.go`

**命令语法**：LLM 在回复中嵌入 `{{command||parameter}}`，由 Wire-Pod 解析执行。

**支持的命令**：

| 命令 | 参数示例 | 作用 | 是否打断语音 |
|---|---|---|---|
| `playAnimationWI` | `happy`, `sad`, `thinking` | 播放动画 | 否（WI = Without Interrupting） |
| `playAnimation` | `celebrate`, `love` | 播放动画 | 是 |
| `getImage` | `front` | 拍摄摄像头画面 | 是（等待回传） |
| `newVoiceRequest` | `now` | 触发新的语音请求 | 是 |

**提取函数**：
```go
func GetActionsFromString(str string) []string {
    re := regexp.MustCompile(`\{\{(.*?)\}\}`)
    matches := re.FindAllString(str, -1)
    return matches
}
```

**执行函数 `PerformActions()`**：
```go
func PerformActions(nChat []openai.ChatCompletionMessage, 
                    acts []string, robot *vector.Vector, stopStop chan bool) bool {
    for _, act := range acts {
        split := strings.Split(act, "||")
        cmd := split[0]  // "playAnimationWI"
        param := split[1] // "happy"
        
        switch cmd {
        case "playAnimationWI":
            // goroutine 中播放动画，不阻塞语音
            go robot.Conn.PlayAnimation(ctx, ...)
            
        case "playAnimation":
            // 阻塞播放，打断语音
            robot.Conn.PlayAnimation(ctx, ...)
            
        case "getImage":
            // 1. 获取摄像头画面
            img := captureImage(robot)
            // 2. base64 编码
            b64 := base64.StdEncoding.EncodeToString(img)
            // 3. 构造图片 URL 注入到对话中
            imgURL := "data:image/jpeg;base64," + b64
            nChat = append(nChat, openai.ChatCompletionMessage{
                Role: openai.ChatMessageRoleUser,
                MultiContent: []openai.ChatMessagePart{
                    {Type: openai.ChatMessagePartTypeImageURL, 
                     ImageURL: &openai.ChatMessageImageURL{URL: imgURL}},
                },
            })
            // 4. 发送新一轮 LLM 请求获取对图片的分析
            
        case "newVoiceRequest":
            // 触发新的语音请求（通过修改机器人状态）
            return true  // disconnect = true，退出当前循环
        }
    }
    return false
}
```

**动画队列**（`kgsim_cmds.go`）：
- 为避免动画重叠，使用 `AnimationQueues` 管理待播放动画
- `playAnimationWI` 的动画进入队列，在语音间隙播放
- `playAnimation` 立即播放，打断当前语音

### 5.3 TTS 实现方式

**方式一：Vector 内置 TTS（默认）**
```go
robot.Conn.SayText(ctx, &vectorpb.SayTextRequest{
    Text:           sentence,
    UseVectorVoice: true,
    DurationScalar: 1.0,
})
```

**方式二：OpenAI TTS API**
使用条件：
- `vars.APIConfig.STT.Language != "en-US"`（非英语）
- 或 `vars.APIConfig.Knowledge.OpenAIVoiceWithEnglish == true`

```go
// 调用 OpenAI TTS API（tts-1 模型，PCM 输出）
audioData := openai.CreateSpeech(...)
// 从 24kHz 降采样到 16kHz
// 通过 ExternalAudioStreamPlayback 发送到机器人
```

### 5.4 中断机制

**位置**：`pkg/wirepod/ttr/kgsim_interrupt.go`

**中断源**：
1. **唤醒词**：机器人检测到 "Hey Vector" 时，立即停止当前回答
2. **触摸传感器**：触摸值超过 baseline + 50 连续 5 次读取，视为用户想要打断

```go
func InterruptKGSimWhenTouchedOrWaked(robot *vector.Vector, 
                                       stop, stopStop chan bool) bool {
    // 订阅 EventStream
    for {
        resp, _ := eventStream.Recv()
        // 检查 WakeWord 事件
        if wakeWordDetected { 
            stop <- true
            return true 
        }
        // 检查 Touch 事件
        if touchValue > baseline + 50 {
            consecutiveTouch++
            if consecutiveTouch >= 5 {
                stop <- true
                return true
            }
        }
    }
}
```

---

## 6. 完整的 AI 对话时序图

```
User          Robot           Wire-Pod            LLM API
 |              |                  |                  |
 |  "Hey Vector"|                  |                  |
 |─────────────►|                  |                  |
 |              | Hotword          |                  |
 |              |─────────────────►|                  |
 |              |                  |                  |
 |  "What's the  |                  |                  |
 |   weather?"  |                  |                  |
 |              | Audio Stream     |                  |
 |              ══════════════════►|                  |
 |              | (Opus/PCM)       |                  |
 |              |                  | STT (Vosk)       |
 |              |                  │────►             |
 |              |                  │◄──── "what's the weather"
 |              |                  |                  |
 |              |                  | Intent Match     |
 |              |                  │────► "intent_weather_extend"
 |              |                  │◄──── params: {location: "..."}
 |              |                  |                  |
 |              |                  | Weather API      |
 |              |                  │──────────────►   |
 |              |                  │◄────────────── "Sunny, 25C"
 |              |                  |                  |
 |              | IntentResponse   |                  |
 |              |◄─────────────────|                  |
 |              |                  |                  |
 |              | PlayAnimation    |                  |
 |              | SayText          |                  |
 |◄─────────────| "It's sunny..."  |                  |
 |              |                  |                  |

--- 若意图未匹配，进入 LLM 模式 ---

 |  "Tell me a   |                  |                  |
 |   joke"      |                  |                  |
 |              | Audio Stream     |                  |
 |              ══════════════════►|                  |
 |              |                  | STT              |
 |              |                  | "tell me a joke" |
 |              |                  |                  |
 |              |                  | Intent Match     |
 |              |                  | No match         |
 |              |                  |                  |
 |              |                  | Create Prompt    |
 |              |                  | + History        |
 |              |                  |                  |
 |              |                  | ChatCompletion   |
 |              |                  │═════════════════►|
 |              |                  |                  |
 |              |                  │◄════ SSE Stream ═|
 |              |                  | "Why did..."     |
 |              |                  |                  |
 |              |                  | Sentence Split   |
 |              |                  | "Why did the..." |
 |              |                  |                  |
 |              | BControl Granted |                  |
 |              | PlayAnimation    |                  |
 |              | SayText          |                  |
 |◄─────────────| "Why did the..."|                  |
 |              |                  |                  |
 |              |                  │◄════ "."         |
 |              |                  | "Because it..."  |
 |              |                  |                  |
 |              | SayText          |                  |
 |◄─────────────| "Because it..." |                  |
 |              |                  |                  |
 |              |                  | Save to Memory   |
 |              |                  |                  |
```

---

## 7. Prompt 工程详解

### 7.1 System Prompt 模板

```
You are a helpful, animated robot called Vector. 
Keep the response concise yet informative.

[用户自定义 prompt 插入位置]

No special characters, especially these: & ^ * # @ - . 
No lists. No formatting.

[若 CommandsEnable == true]
You are able to perform some actions. ...
Valid commands:
 - playAnimationWI||animName ...
 - playAnimation||animName ...
 - getImage||front ...
 - newVoiceRequest||now ...
```

### 7.2 为什么限制特殊字符？

Vector 的内置 TTS 引擎对某些符号读法奇怪（如 `#` 读成 "hash"，`&` 读成 "ampersand"），会严重影响自然度。因此 Prompt 中强制要求避免特殊字符。

### 7.3 多语言支持

- Prompt 使用英文编写，但 LLM 会根据 User 消息的语言自动切换回复语言
- 非英语场景建议使用 OpenAI TTS（`tts-1`），因为 Vector 内置 TTS 仅支持英语
- `removeSpecialCharacters()` 函数对越南语等带声调标记的语言有特殊处理（保留 Unicode 组合字符）

---

## 8. 上下文管理策略

### 8.1 短期记忆（LLM Chat History）

- **容量**：16 条消息（8 轮 User-Assistant 对话）
- **隔离**：按 ESN 隔离
- **生命周期**：进程内存，重启丢失
- **用途**：让 LLM 了解对话上下文，支持指代消解（如用户说 "tell me another"）

### 8.2 长期记忆（JDocs）

- 机器人设置（`vic.RobotSettings`）保存在 `jdocs/jdocs.json` 中
- 包含：默认位置、时区、温度单位、音量、眼睛颜色等
- `ParamChecker()` 在回答天气等问题时会读取这些设置

### 8.3 系统提示中的动态信息

`ParamChecker()` 从 `vic.RobotSettings` 读取：
- `default_location` — 用于天气查询的默认城市
- `temp_is_fahrenheit` — 决定天气返回摄氏还是华氏
- `clock_24_hour` — 决定时间格式

---

## 9. Function Calling / Tool Calling

Wire-Pod **未使用** OpenAI 的 Function Calling 机制。而是采用更简单的**文本命令嵌入**方式：

| 方式 | Wire-Pod 做法 | 标准 Function Calling |
|---|---|---|
| 触发方式 | LLM 在回复文本中嵌入 `{{cmd||param}}` | LLM 输出 structured JSON |
| 解析方式 | 正则提取 `\{\{(.*?)\}\}` | 解析 `tool_calls` 字段 |
| 结果回传 | 直接执行，图片通过新 User message 注入 | 通过 `function` role message |
| 优点 | 简单、兼容所有 LLM（包括非 OpenAI） | 类型安全、结构化 |
| 缺点 | 依赖 LLM 遵守格式约定 | 仅 OpenAI/部分厂商支持 |

**为什么不用标准 Function Calling？**

1. **兼容性**：Together AI、Custom 端点可能不支持 function calling
2. **流式处理复杂**：function calling 在 stream 模式下处理更复杂
3. **文本嵌入更自然**：对于 TTS 场景，文本回复本身就是最终输出，命令只是附带效果

---

## 10. 性能与延迟分析

| 阶段 | 典型延迟 | 优化空间 |
|---|---|---|
| 音频传输 | 50-100ms（局域网） | 依赖网络 |
| Opus 解码 + VAD | 10-20ms | 已足够快 |
| STT（Vosk 本地） | 100-300ms | grammar 优化可加速 |
| STT（Whisper API） | 500-2000ms | 必须用本地引擎降低 |
| 意图匹配 | <1ms | 可忽略 |
| LLM 首 token | 200-800ms | 依赖提供商和模型 |
| LLM 完整响应 | 1-5s | 流式输出已优化 |
| TTS（Vector 内置） | 100-200ms | 已足够快 |
| Behavior Control | 200-500ms | 机器人固件限制 |

**总延迟**（本地 STT + OpenAI LLM）：
- 首句听到：约 1-2 秒
- 完整回答：3-8 秒

---

## 11. 常见 AI 流水线问题

| 现象 | 原因 | 排查方法 |
|---|---|---|
| LLM 不回复 | API key 无效/欠费 | 查看日志 `Error creating chat completion stream` |
| 回复太长 | Prompt 未限制长度 | 检查 System Prompt 是否包含 "concise" |
| 机器人不说中文 | 未启用 OpenAI TTS | 确认 `STT_LANGUAGE != "en-US"` 或 `openai_voice_with_english == true` |
| 命令不执行 | LLM 未输出正确格式 | 在日志中查看原始 LLM 输出是否包含 `{{...}}` |
| 对话不连贯 | SaveChat 未启用 | 检查 Web UI 中 "Save Chat" 是否勾选 |
| 动画和语音重叠 | 命令解析失败 | 检查 `kgsim_cmds.go` 中动画队列状态 |

---

*本文档基于 `pkg/wirepod/ttr/kgsim.go`、`kgsim_cmds.go`、`kgsim_interrupt.go` 和 `matchIntentSend.go` 的实际代码编写。*
