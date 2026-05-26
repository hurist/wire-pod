# Wire-Pod 项目目录结构详解

本文档说明仓库中每个目录和关键文件的作用、入口关系和协作方式。适合需要快速定位代码或新增功能的开发者阅读。

---

## 目录总览

```
wire-pod/
├── chipper/                    ← 服务端核心代码（整个项目的业务重心）
│   ├── cmd/                    ← 主入口（按 STT 引擎区分）
│   ├── epod/                   ← Escape Pod 兼容证书/配置
│   ├── intent-data/            ← 内置语音意图定义（多语言 JSON）
│   ├── jdocs/                  ← 运行时生成的 JDocs 存储
│   ├── pkg/                    ← Go 库代码（真正业务逻辑）
│   ├── plugins/                ← Go 原生插件示例
│   ├── session-certs/          ← 运行时生成的机器人会话证书
│   └── webroot/                ← Web UI 静态资源
├── vector-cloud/               ← 机器人端协议参考实现（辅助理解）
│   ├── cloud/                  ← vic-cloud 入口
│   ├── gateway/                ← vic-gateway 入口
│   └── internal/               ← 协议实现细节
├── docker/                     ← Docker 构建辅助脚本
├── compose.yaml                ← Docker Compose 配置
├── dockerfile                  ← 多阶段 Docker 构建文件
├── setup.sh                    ← 一键安装脚本
└── update.sh                   ← 更新脚本
```

> **维护提示**：`chipper/` 是服务端运行时，占 90% 的维护工作量。`vector-cloud/` 中的代码**不参与** Wire-Pod 服务端运行，但阅读它有助于理解机器人端行为。

---

## chipper/ 详细结构

### cmd/ — 编译入口

```
cmd/
├── vosk/main.go                ← 【默认入口】Vosk 本地语音识别
├── coqui/main.go               ← Coqui 本地语音识别（已较少使用）
├── leopard/main.go             ← Picovoice Leopard 语音识别
└── experimental/
    ├── whisper/main.go         ← OpenAI Whisper API（云端）
    ├── whisper.cpp/main.go     ← 本地 whisper.cpp（CGO）
    └── houndify/main.go        ← Houndify 云端语音识别
```

**设计说明**：每个 `main.go` 仅做一件事——传入对应的 STT 引擎初始化函数和名称，然后调用 `initwirepod.StartFromProgramInit()`。STT 引擎是在**编译时**绑定的，不是运行时插件。

**入口调用链**：
```
cmd/vosk/main.go
    └── initwirepod.StartFromProgramInit(stt.Init, stt.STT, "vosk")
        ├── vars.Init()
        ├── wp.New(stt.Init, stt.STT, voiceProcessorName)
        ├── sdkWeb.BeginServer()          // goroutine
        ├── http.HandleFunc("/api-chipper/", ...)
        └── wpweb.StartWebServer()        // 阻塞主 goroutine
```

---

### pkg/initwirepod/ — 启动编排

```
pkg/initwirepod/
├── start.go                    ← StartFromProgramInit(), StartChipper()
└── web.go                      ← /api-chipper/ HTTP handler（重启、EP/IP 切换）
```

**核心职责**：
- `StartFromProgramInit`：整个服务的启动总控，决定先进入 setup 模式还是正常服务模式
- `StartChipper`：加载 TLS 证书，启动 `cmux` 多路复用，注册 gRPC 服务

**关键状态机**：
```
启动
  ├── 若 PastInitialSetup == false
  │     └── 阻塞在 wpweb.StartWebServer()（仅提供 setup 页面）
  └── 若 PastInitialSetup == true
        ├── goroutine: sdkWeb.BeginServer()   // 端口 80
        ├── goroutine: StartChipper()          // 端口 443
        └── 阻塞: wpweb.StartWebServer()       // 端口 8080
```

---

### pkg/vars/ — 全局状态与配置（系统心脏）

```
pkg/vars/
├── vars.go                     ← 全局变量定义、Init()、机器人信息、JDocs 操作
└── config.go                   ← apiConfig.json 结构定义与读写
```

**vars.go 中的关键全局变量**：
```go
var APIConfig apiConfig                 // 运行时唯一配置源
var BotInfo RobotInfoStore              // 已认证机器人列表（ESN/IP/GUID）
var BotJdocs []botjdoc                  // 所有机器人的 JDocs
var CustomIntents []CustomIntent        // 用户自定义意图
var RememberedChats []RememberedChat    // 按 ESN 保存的 LLM 对话历史
var IntentList []JsonIntent             // 当前加载的内置意图
var ChipperCert, ChipperKey []byte      // TLS 证书
```

**config.go 中的配置结构**：
```go
type apiConfig struct {
    Weather   struct { Enable bool; Provider string; Key string; Unit string }
    Knowledge struct { Enable bool; Provider string; Key string; ... }
    STT       struct { Service string; Language string }
    Server    struct { EPConfig bool; Port string }
    ...
}
```

**维护风险**：
- 这些变量在**无锁**条件下被多个 goroutine 读写
- 任何修改必须评估并发安全性
- `vars.Init()` 中的路径计算逻辑对目录结构敏感（特别是 `SDKIniPath` 的推导）

---

### pkg/vtt/ — VTT 抽象层（解耦 gRPC 与业务）

```
pkg/vtt/
├── intent.go                   ← IntentRequest（包装 StreamingIntent）
├── intentgraph.go              ← IntentGraphRequest（包装 StreamingIntentGraph）
└── knowledgegraph.go           ← KnowledgeGraphRequest（包装 StreamingKnowledgeGraph）
```

**设计目的**：将 `chipperpb` 生成的 protobuf 类型与上层业务逻辑解耦。如果未来 protobuf 定义变更，只需修改 VTT 层。

**核心字段**：
```go
type IntentGraphRequest struct {
    Time       time.Time
    Stream     pb.ChipperGrpc_StreamingIntentGraphServer
    Device     string      // ESN
    Session    string
    LangString string      // 如 "en-US"
    FirstReq   *pb.IntentGraphRequest
    AudioCodec string
}
```

---

### pkg/servers/ — gRPC 服务实现

```
pkg/servers/
├── chipper/                    ← ChipperGrpc 服务（语音流入口）
│   ├── server.go               ← Server 结构体 + 3 个 processor 接口
│   ├── options.go              ← functional options 模式
│   ├── intent.go               ← StreamingIntent handler
│   ├── intent_graph.go         ← StreamingIntentGraph handler
│   ├── knowledgegraph.go       ← StreamingKnowledgeGraph handler
│   ├── textintent.go           ← TextIntent（未实现）
│   └── connectioncheck.go      ← StreamingConnectionCheck handler
│
├── jdocs/                      ← JDocs 服务（机器人设置同步）
│   ├── server.go               ← ReadDocs / WriteDoc 实现
│   └── botInfoStorer.go        ← 机器人认证信息存储
│
└── token/                      ← Token 服务（JWT 签发）
    ├── token.go                ← AssociatePrimaryUser / RefreshToken
└── └── hashing.go              ← Token hash 计算
```

**调用关系**：
```
chipper/server.go
    ├── chipper.WithIntentProcessor(wpServer)         ← 来自 pkg/wirepod/preqs
    ├── chipper.WithKnowledgeGraphProcessor(wpServer)
    └── chipper.WithIntentGraphProcessor(wpServer)

jdocs/server.go
    └── 读取/写入 vars.BotJdocs
    └── 认证时更新 vars.BotInfo + session-certs/

token/token.go
    └── 签发 JWT，使用 vars.BotInfo 验证
```

---

### pkg/wirepod/ — 语音处理核心

#### preqs/ — 请求处理中枢

```
pkg/wirepod/preqs/
├── server.go                   ← New() 初始化，STT handler 注册
├── intent.go                   ← ProcessIntent (legacy)
├── intent_graph.go             ← ProcessIntentGraph (modern)
└── knowledgegraph.go           ← ProcessKnowledgeGraph
```

**`server.go` 的核心逻辑**：
```go
type Server struct{}

var sttHandler func(sr.SpeechRequest) (string, error)
var stiHandler func(sr.SpeechRequest) (string, map[string]string, error)
var isSti bool

func New(InitFunc func() error, SttHandler interface{}, voiceProcessor string) {
    // 1. 设置语言
    // 2. 加载意图列表 vars.LoadIntents()
    // 3. 调用 InitFunc() 加载 STT 模型
    // 4. 用反射判断 SttHandler 签名（STT vs STI）
    // 5. ttr.LoadPlugins() 加载 Go 插件
}
```

**三个 Process 方法的分工**：

| 方法 | 适用固件 | 说明 |
|---|---|---|
| `ProcessIntent` | ≤1.6 | Legacy 接口，直接返回 IntentResult |
| `ProcessIntentGraph` | ≥1.7 | Modern 接口，支持 Intent/KG 混合路由 |
| `ProcessKnowledgeGraph` | 全部 | 处理 "I have a question" 请求 |

#### speechrequest/ — 音频流状态机

```
pkg/wirepod/speechrequest/
└── speechrequest.go            ← SpeechRequest + 音频处理函数
```

**这是全项目最重要的单一文件之一**。任何涉及音频编解码、语音活动检测的修改都必须经过此文件。

**核心方法**：
- `ReqToSpeechRequest(req interface{}) SpeechRequest` — 从 gRPC 首包创建
- `GetNextStreamChunk() ([]byte, error)` — 接收并解码下一帧
- `DetectEndOfSpeech() (bool, bool)` — VAD 判断语音是否结束
- `OpusDetect() bool` — 检测 Opus/PCM
- `highPassFilter(data []byte) []byte` — 300Hz 高通过滤

#### stt/ — 语音识别引擎

```
pkg/wirepod/stt/
├── vosk/
│   ├── Vosk.go                 ← Vosk STT 实现
│   └── context.go              ← 语法优化 recognizer（VOSK_WITH_GRAMMER）
├── coqui/Coqui.go              ← Coqui STT 实现
├── leopard/Leopard.go          ← Picovoice Leopard 实现
├── whisper/Whisper.go          ← OpenAI Whisper API 实现
├── whisper.cpp/WhisperCpp.go   ← 本地 whisper.cpp 实现
└── houndify/Houndify.go        ← Houndify 实现
```

**每个引擎的契约**：
```go
var Name string
func Init() error
func STT(req sr.SpeechRequest) (string, error)
```

**Vosk 的特殊性**：
- 使用 **recognizer pool** 管理并发：`grmRecs`（带语法）和 `gpRecs`（通用）
- `context.go` 从意图数据构建受限语法列表，加速命令式语音识别

#### ttr/ — 文本响应系统（项目大脑）

```
pkg/wirepod/ttr/
├── matchIntentSend.go          ← 意图匹配路由 + IntentPass()
├── intentparam.go              ← 参数提取（天气、颜色、音量、名字等）
├── kgsim.go                    ← LLM 流式对话核心（最复杂文件）
├── kgsim_cmds.go               ← LLM 命令解析与动画执行
├── kgsim_interrupt.go          ← 触摸/唤醒词中断监听
├── weather.go                  ← 天气 API 调用（OpenWeatherMap / WeatherAPI）
├── plugins.go                  ← Go 原生插件加载
├── words2num.go                ← 英文数字词转整数（如 "twenty one" → 21）
└── convert.go                  ← 编码转换辅助
```

**调用优先级**（在 `matchIntentSend.go` 的 `ProcessTextAll()` 中）：
```
1. Go 插件 (pluginFunctionHandler)
2. 自定义意图 (customIntentHandler)
3. 内置意图（精确匹配 → 子串匹配）
4. LLM fallback（若启用 IntentGraph + Knowledge）
5. 返回 intent_system_unmatched
```

#### sdkapp/ — SDK 远程控制服务

```
pkg/wirepod/sdkapp/
├── server.go                   ← /api-sdk/* HTTP handler + /cam-stream
└── jdocspinger.go              ← JDocs 保活与 connCheck
```

**`server.go` 的分支规模**：约 500 行 switch-case 处理 30+ 个 SDK API 端点。

**关键端点**：
- `GET /api-sdk/get_sdk_info` — 返回所有已认证机器人
- `POST /api-sdk/get_battery` — 查询电池状态
- `POST /api-sdk/assume_behavior_control` — 接管机器人行为
- `POST /api-sdk/say_text` — 让机器人说话
- `GET /cam-stream?serial=xxx` — MJPEG 摄像头流

#### config-ws/ — Web 配置服务

```
pkg/wirepod/config-ws/
└── webserver.go                ← 端口 8080 的 HTTP API + 静态文件服务
```

**API 路由表**（约 20 个端点）：
```
POST   /api/add_custom_intent
POST   /api/edit_custom_intent
POST   /api/remove_custom_intent
GET    /api/get_custom_intents_json
POST   /api/set_weather_api
GET    /api/get_weather_api
POST   /api/set_kg_api
GET    /api/get_kg_api
POST   /api/set_stt_info
GET    /api/get_stt_info
GET    /api/get_download_status
GET    /api/get_config
GET    /api/get_logs
GET    /api/get_debug_logs
GET    /api/is_running
POST   /api/delete_chats
GET    /api/get_ota/<filename>
GET    /api/get_version_info
POST   /api/generate_certs
```

#### setup/ — 机器人 Onboarding

```
pkg/wirepod/setup/
├── certs.go                    ← 生成 TLS CA + 服务器证书
├── ble.go                      ← BLE 配对 HTTP API（/api-ble/*）
└── ssh.go                      ← SSH 部署 HTTP API（/api-ssh/*）
```

**证书生成**：
- `CreateCertCombo()` 生成 1028-bit RSA CA + 服务器证书，有效期 10 年
- 证书绑定到服务器外网 IP（通过 `vars.GetOutboundIP()`）
- 输出到 `../certs/cert.crt` 和 `../certs/cert.key`

#### localization/ — 多语言与模型下载

```
pkg/wirepod/localization/
└── ...                         ← Vosk 模型下载、语言列表
```

---

### pkg/scripting/ — Lua 脚本引擎

```
pkg/scripting/
├── scripting.go                ← Lua 运行时、脚本执行、HTTP API 注册
├── bcontrol.go                 ← Lua API：行为控制、移动、说话
└── display.go                  ← Lua API：屏幕显示图片
```

**Lua 可用 API**：
```lua
assumeBehaviorControl(priority)   -- 接管行为
releaseBehaviorControl()          -- 释放行为
sayText(text, goroutine)          -- 说话
playAnimation(anim, goroutine)    -- 播放动画
moveHead(speed)                   -- 移动头部
moveLift(speed)                   -- 移动抬升臂
moveWheels(l, r, l2, r2)          -- 驱动轮子
showImage(path, durationMs)       -- 显示图片
postHTTPRequest(url, ctype, body, timeout)  -- HTTP POST
getHTTPRequest(url, timeout)      -- HTTP GET
```

---

### webroot/ — Web UI 静态文件

```
webroot/
├── index.html                  ← 主仪表盘（bot 状态、意图管理、日志）
├── setup.html                  ← 服务器设置（天气、LLM、STT 语言）
├── initial.html                ← 首次安装向导
├── css/
│   ├── style.css               ← 暗色主题主样式
│   └── wing.css                ← 辅助样式
├── js/
│   ├── main.js                 ← 主逻辑
│   ├── ui.js                   ← UI 主题切换
│   ├── ble.js                  ← BLE 配对流程
│   ├── ssh.js                  ← SSH 部署流程
│   ├── battery.js              ← 电池状态显示
│   └── play_audio.js           ← 音频播放
└── sdkapp/
    ├── index.html              ← 机器人选择器
    ├── settings.html           ← 单机器人设置
    ├── control.html            ← 实时遥控
    └── js/
        ├── main.js
        ├── control.js          ← 键盘遥控逻辑
        ├── auth.js             ← 机器人认证
        └── ...
```

> **前端技术栈**：纯原生 HTML/CSS/JS，无 React/Vue/Angular。使用 CSS 变量实现主题切换，使用原生 `fetch()` 调用后端 API。

---

### intent-data/ — 内置意图定义

```
intent-data/
├── en-US.json                  ← 英语意图（最完整）
├── de-DE.json                  ← 德语
├── fr-FR.json                  ← 法语
├── es-ES.json                  ← 西班牙语
├── it-IT.json                  ← 意大利语
├── pt-BR.json                  ← 巴西葡萄牙语
├── ja-JP.json                  ← 日语
├── zh-CN.json                  ← 简体中文
└── ...                         ← 共 14 种语言
```

**JSON 结构**：
```json
[
  {
    "name": "intent_weather_extend",
    "keyphrases": ["weather", "whether", "the other", "the water"],
    "requiresexact": false
  }
]
```

**设计说明**：`keyphrases` 不仅包含标准说法，还包含语音识别易混淆的近似音（如 "whether" 对应 "weather"），这是提高识别率的关键设计。

---

### plugins/ — Go 插件示例

```
plugins/
├── whatdate/
│   └── whatdate.go             ← 示例：返回当前日期
└── sdkTest/
    └── sdkTest.go              ← 示例：使用 Vector SDK 控制机器人
```

**插件编译方式**：
```bash
cd plugins/whatdate
go build -buildmode=plugin -o whatdate.so whatdate.go
```

---

## vector-cloud/ 详细结构

```
vector-cloud/
├── cloud/
│   └── main.go                 ← vic-cloud 入口（机器人端云客户端）
├── gateway/
│   ├── main.go                 ← vic-gateway 入口（SDK 服务端）
│   ├── ipc_manager.go          ← Unix socket IPC 管理
│   ├── message_handler.go      ← CLAD ↔ protobuf 转换
│   ├── tokens.go               ← SDK client token 管理
│   └── switchboard_proxy.go    ← BLE 代理
└── internal/
    ├── voice/                  ← 语音流上传/下载
    │   ├── process.go          ← 语音事件循环
    │   └── stream/
    │       ├── connect.go      ← 建立 gRPC 连接
    │       └── context.go      ← 音频缓冲与响应处理
    ├── token/                  ← JWT 管理
    ├── jdocs/                  ← JDocs 客户端
    ├── clad/                   ← CLAD 消息定义
    │   ├── cloud/              ← 云端相关消息（mic, token, docs）
    │   └── gateway/            ← 网关相关消息
    ├── proto/                  ← protobuf 生成代码
    └── robot/                  ← 机器人身份与证书
```

**维护价值**：
- 修改 Wire-Pod 的 gRPC 服务时，需要对照 `vector-cloud/internal/proto/` 中的 protobuf 定义
- 调试音频问题时，需要理解 `vector-cloud/internal/voice/process.go` 中如何打包音频
- 调试认证问题时，需要理解 `vector-cloud/internal/token/` 的 JWT 刷新逻辑

---

## 关键文件调用关系图

```
┌─────────────────────────────────────────────────────────────────────┐
│                         启动阶段                                      │
│  cmd/vosk/main.go                                                   │
│    └── initwirepod.StartFromProgramInit()                           │
│          ├── vars.Init()                                            │
│          │     ├── config.go: ReadConfig()                          │
│          │     └── vars.go: LoadIntents(), LoadCustomIntents()      │
│          ├── wp.New()                                               │
│          │     ├── vars.LoadIntents()                               │
│          │     ├── stt.Init() [vosk.Init()]                         │
│          │     └── ttr.LoadPlugins()                                │
│          ├── sdkWeb.BeginServer()                                   │
│          └── wpweb.StartWebServer()                                 │
│                └── config-ws/webserver.go                           │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      语音请求处理阶段                                 │
│  pkg/servers/chipper/intent_graph.go                                │
│    └── vtt.IntentGraphRequest                                       │
│          └── pkg/wirepod/preqs/intent_graph.go                      │
│                ├── sr.ReqToSpeechRequest()                          │
│                │     └── speechrequest.go                           │
│                ├── stt.STT() [vosk.STT()]                           │
│                └── ttr.ProcessTextAll()                             │
│                      ├── matchIntentSend.go: pluginFunctionHandler  │
│                      ├── matchIntentSend.go: customIntentHandler    │
│                      ├── matchIntentSend.go: 内置意图匹配           │
│                      └── (失败) kgsim.go: StreamingKGSim()          │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      Web 配置操作阶段                                 │
│  webroot/index.html                                                 │
│    └── fetch('/api/set_kg_api')                                     │
│          └── config-ws/webserver.go: handleSetKGAPI()               │
│                ├── 更新 vars.APIConfig.Knowledge                    │
│                └── vars.WriteConfigToDisk()                         │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 目录重要性分级

| 优先级 | 目录/文件 | 维护频率 | 说明 |
|---|---|---|---|
| P0 | `pkg/vars/` | 高 | 任何配置变更都涉及 |
| P0 | `pkg/wirepod/speechrequest/` | 高 | 音频问题必查 |
| P0 | `pkg/wirepod/preqs/` | 高 | 请求调度中枢 |
| P0 | `pkg/wirepod/ttr/` | 高 | AI 和意图核心 |
| P1 | `pkg/servers/chipper/` | 中 | 协议兼容性关键 |
| P1 | `pkg/servers/jdocs/` | 中 | 认证和设置同步 |
| P1 | `pkg/wirepod/sdkapp/` | 中 | 用户直接使用的功能 |
| P1 | `pkg/wirepod/config-ws/` | 中 | Web 后台功能迭代 |
| P2 | `pkg/scripting/` | 低 | 扩展功能 |
| P2 | `plugins/` | 低 | 示例代码 |
| P3 | `vector-cloud/` | 低 | 协议参考，不直接参与运行 |
| P3 | `webroot/` | 中 | UI 调整 |

---

*本文档应与 ARCHITECTURE.md 配合阅读，以建立完整的代码位置认知。*
