# Wire-Pod API 参考文档

本文档列出 Wire-Pod 暴露的所有接口，包括 gRPC 服务、HTTP REST API 和内部调用接口。

---

## 1. gRPC 服务接口

gRPC 服务监听在 **端口 443**（TLS），通过 `cmux` 与 HTTP/1.1 共享同一端口。

### 1.1 ChipperGrpc 服务

**包路径**：`github.com/digital-dream-labs/api/go/chipperpb`

**说明**：这是机器人语音交互的核心服务。机器人通过流式方法上传音频，服务端返回意图结果或知识图谱答案。

#### StreamingIntentGraph

```protobuf
rpc StreamingIntentGraph(stream IntentGraphRequest) returns (stream IntentGraphResponse);
```

**现代固件（≥1.7）的主要语音入口。**

**请求流**（`IntentGraphRequest`）：
| 字段 | 类型 | 说明 |
|---|---|---|
| `device_id` | string | 机器人 ESN（如 `00e20145`） |
| `session` | string | 会话 ID（机器人生成） |
| `language_code` | string | 语言代码（如 `en-US`） |
| `input_audio` | bytes | 音频帧（Opus 或 PCM） |
| `audio_codec` | string | `opus` 或空（PCM） |

**响应流**（`IntentGraphResponse`）：
| 字段 | 类型 | 说明 |
|---|---|---|
| `response_type` | IntentGraphMode | `INTENT` 或 `KNOWLEDGE_GRAPH` |
| `is_final` | bool | 是否为最终结果 |
| `intent_result` | IntentResult | 意图名称和参数 |
| `command_type` | string | `VOICE_COMMAND` |

**服务端实现**：`pkg/servers/chipper/intent_graph.go` → `wpServer.ProcessIntentGraph(req)`

---

#### StreamingIntent

```protobuf
rpc StreamingIntent(stream IntentRequest) returns (stream IntentResponse);
```

**旧版固件（≤1.6）的语音入口。结构与 StreamingIntentGraph 类似，但返回 `IntentResponse`。**

**服务端实现**：`pkg/servers/chipper/intent.go` → `wpServer.ProcessIntent(req)`

---

#### StreamingKnowledgeGraph

```protobuf
rpc StreamingKnowledgeGraph(stream KnowledgeGraphRequest) returns (stream KnowledgeGraphResponse);
```

**处理 "I have a question" 类请求的专用流。**

**请求流**与 `IntentGraphRequest` 结构相同。

**响应流**（`KnowledgeGraphResponse`）：
| 字段 | 类型 | 说明 |
|---|---|---|
| `session` | string | 会话 ID |
| `query_text` | string | 识别出的用户 query |
| `response_text` | string | 回答文本 |
| `spoken_text` | string | 机器人实际说出的文本 |
| `command_type` | string | 命令类型 |
| `status` | KnowledgeGraphResponseStatus | `KGRESPONSE_PROCESSED` 等 |

**服务端实现**：`pkg/servers/chipper/knowledgegraph.go` → `wpServer.ProcessKnowledgeGraph(req)`

---

#### StreamingConnectionCheck

```protobuf
rpc StreamingConnectionCheck(stream ConnectionCheckRequest) returns (stream ConnectionCheckResponse);
```

**连接质量检测。机器人发送随机数据，服务端计数后返回延迟和连通性状态。**

**服务端实现**：`pkg/servers/chipper/connectioncheck.go`

---

#### TextIntent

```protobuf
rpc TextIntent(TextIntentRequest) returns (IntentResponse);
```

**未实现。** 直接返回 `codes.Unimplemented`。

---

### 1.2 JDocs 服务

**包路径**：`github.com/digital-dream-labs/api/go/jdocspb`

**说明**：JSON 文档同步服务。机器人用此服务读写设置、Token 等持久化数据。

#### ReadDocs

```protobuf
rpc ReadDocs(ReadDocsReq) returns (ReadDocsResp);
```

**请求**：
| 字段 | 类型 | 说明 |
|---|---|---|
| `account` | string | 用户账户标识 |
| `thing` | string | 机器人标识（如 `vic:00e20145`） |
| `items` | repeated DocRequestItem | 请求的文档名称和版本 |

**响应**：
| 字段 | 类型 | 说明 |
|---|---|---|
| `status` | ReadDocsRespStatus | `CHANGED`, `UNCHANGED`, `NOT_FOUND` 等 |
| `items` | repeated DocResponseItem | 文档内容 |

**服务端实现**：`pkg/servers/jdocs/server.go`

**关键逻辑**：读取 `vic.AppTokens` 时，会触发机器人认证流程——匹配 IP、生成 GUID、写入 `botSdkInfo.json`、生成 session cert。

---

#### WriteDoc

```protobuf
rpc WriteDoc(WriteDocReq) returns (WriteDocResp);
```

**请求**：
| 字段 | 类型 | 说明 |
|---|---|---|
| `account` | string | 用户账户 |
| `thing` | string | 机器人标识 |
| `doc_name` | string | 文档名（如 `vic.RobotSettings`） |
| `doc` | Jdoc | 文档内容（含 version 和 json_doc） |

**响应**：
| 字段 | 类型 | 说明 |
|---|---|---|
| `status` | WriteDocRespStatus | `ACCEPTED`, `REJECTED_DOC_VERSION` 等 |

**服务端实现**：`pkg/servers/jdocs/server.go`

**存储位置**：写入 `vars.BotJdocs` 内存数组，然后调用 `vars.WriteJdocs()` 持久化到 `jdocs/jdocs.json`。

---

### 1.3 Token 服务

**包路径**：`github.com/digital-dream-labs/api/go/tokenpb`

**说明**：认证服务。机器人在初始配对时通过此服务获取 JWT Token。

#### AssociatePrimaryUser

```protobuf
rpc AssociatePrimaryUser(AssociatePrimaryUserReq) returns (TokenBundle);
```

**请求**：包含 session certificate。

**响应**：`TokenBundle` 包含 JWT、refresh token 等。

**服务端实现**：`pkg/servers/token/token.go`

---

#### RefreshToken

```protobuf
rpc RefreshToken(RefreshTokenReq) returns (TokenBundle);
```

**请求**：包含需要刷新的 token。

**响应**：新的 `TokenBundle`。

---

## 2. HTTP REST API

### 2.1 Web 配置 API（端口 8080）

**Base URL**：`http://<wirepod-ip>:8080/api/`

**CORS**：所有响应头包含 `Access-Control-Allow-Origin: *`

---

#### 自定义意图管理

##### POST /api/add_custom_intent

**功能**：添加自定义语音意图。

**请求体**（JSON）：
```json
{
  "name": "开灯",
  "description": "打开客厅灯",
  "utterances": ["开灯", "打开灯", "把灯打开"],
  "intent": "intent_system_lightson",
  "params": {
    "paramname": "room",
    "paramvalue": "living_room"
  },
  "exec": "/home/user/scripts/light_on.sh",
  "execargs": ["living_room"],
  "luascript": "sayText('好的，开灯', true)"
}
```

**字段说明**：
| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 意图显示名称 |
| `description` | string | 是 | 意图描述 |
| `utterances` | []string | 是 | 触发语音（至少一个） |
| `intent` | string | 是 | 对应的 Vector 内置意图名 |
| `params` | object | 否 | 意图参数 |
| `exec` | string | 否 | 匹配后执行的外部程序路径 |
| `execargs` | []string | 否 | 外部程序参数 |
| `luascript` | string | 否 | 匹配后执行的 Lua 脚本 |

**响应**：`Intent added successfully.`（200）或错误信息（400/500）

**服务端实现**：`config-ws/webserver.go: handleAddCustomIntent()`

**调用链**：
```
HTTP POST → handleAddCustomIntent()
    ├── json.NewDecoder(r.Body).Decode(&intent)
    ├── scripting.ValidateLuaScript(intent.LuaScript)  // 若提供 Lua
    ├── vars.CustomIntents = append(vars.CustomIntents, intent)
    └── saveCustomIntents() → os.WriteFile(vars.CustomIntentsPath, ...)
```

---

##### POST /api/edit_custom_intent

**功能**：编辑已有自定义意图。

**请求体**（JSON）：
```json
{
  "number": 1,
  "name": "新名称",
  "utterances": ["新说法"]
}
```

> `number` 为 1-based 索引，对应 `vars.CustomIntents` 数组位置。

**服务端实现**：`config-ws/webserver.go: handleEditCustomIntent()`

---

##### POST /api/remove_custom_intent

**功能**：删除自定义意图。

**请求体**（JSON）：
```json
{
  "number": 1
}
```

**服务端实现**：`config-ws/webserver.go: handleRemoveCustomIntent()`

---

##### GET /api/get_custom_intents_json

**功能**：获取所有自定义意图的 JSON。

**响应**：`customIntents.json` 的原始内容。

---

#### 天气 API 配置

##### POST /api/set_weather_api

**功能**：设置天气服务提供商。

**请求体**（JSON）：
```json
{
  "provider": "openweathermap",
  "key": "your-api-key"
}
```

> `provider` 为空字符串时关闭天气功能。

**服务端实现**：`config-ws/webserver.go: handleSetWeatherAPI()`

---

##### GET /api/get_weather_api

**响应**：
```json
{
  "enable": true,
  "provider": "openweathermap",
  "key": "your-api-key",
  "unit": "C"
}
```

---

#### LLM / 知识图谱配置

##### POST /api/set_kg_api

**功能**：设置 LLM 提供商和参数。

**请求体**（JSON，直接对应 `vars.APIConfig.Knowledge` 结构）：
```json
{
  "enable": true,
  "provider": "openai",
  "key": "sk-...",
  "model": "gpt-4o-mini",
  "intentgraph": true,
  "robotName": "Vector",
  "openai_prompt": "你是一个聪明的机器人",
  "save_chat": true,
  "commands_enable": true,
  "endpoint": "",
  "top_p": 1.0,
  "temp": 1.0
}
```

**服务端实现**：`config-ws/webserver.go: handleSetKGAPI()`

---

##### GET /api/get_kg_api

**响应**：`vars.APIConfig.Knowledge` 的 JSON 序列化。

---

#### STT 配置

##### POST /api/set_stt_info

**功能**：切换 STT 语言（仅对 Vosk/whisper.cpp 有效）。

**请求体**（JSON）：
```json
{
  "language": "zh-CN"
}
```

**服务端逻辑**：
1. 验证语言是否在 `localization.ValidVoskModels` 列表中
2. 若未下载，后台触发 `localization.DownloadVoskModel()`
3. 若已下载，更新 `vars.APIConfig.STT.Language`
4. 调用 `processreqs.ReloadVosk()` 重新加载模型和意图

**服务端实现**：`config-ws/webserver.go: handleSetSTTInfo()`

---

##### GET /api/get_stt_info

**响应**：
```json
{
  "provider": "vosk",
  "language": "en-US"
}
```

---

##### GET /api/get_download_status

**功能**：查询 Vosk 模型下载状态。

**响应**：文本 — `"not downloading"` / `"downloading..."` / `"success"` / `"error: ..."`

---

#### 系统与日志

##### GET /api/get_config

**功能**：获取完整配置。

**响应**：`vars.APIConfig` 的 JSON 序列化。

---

##### GET /api/get_logs

**功能**：获取普通日志。

**响应**：`logger.LogList` 的纯文本内容。

---

##### GET /api/get_debug_logs

**功能**：获取调试日志。

**响应**：`logger.LogTrayList` 的纯文本内容。

---

##### GET /api/is_running

**功能**：健康检查。

**响应**：`"true"`

---

##### POST /api/delete_chats

**功能**：清除所有 LLM 对话记忆。

**服务端实现**：`vars.RememberedChats = []vars.RememberedChat{}`

---

##### GET /api/get_ota/<filename>

**功能**：OTA 固件下载代理。

**行为**：将请求转发到 `https://archive.org/download/vector-pod-firmware/<filename>`，流式返回。

---

##### GET /api/get_version_info

**功能**：获取版本和更新信息。

**响应**：
```json
{
  "fromsource": true,
  "installedversion": "v1.2.3",
  "installedcommit": "abc1234",
  "currentversion": "v1.2.4",
  "currentcommit": "def5678",
  "avail": true
}
```

---

##### POST /api/generate_certs

**功能**：重新生成 TLS 证书。

**响应**：`"done"` 或错误信息。

---

### 2.2 Chipper 控制 API（端口 8080）

**前缀**：`/api-chipper/`

**注册位置**：`pkg/initwirepod/web.go`

| 端点 | 方法 | 功能 |
|---|---|---|
| `/api-chipper/restart_server` | GET | 重启 gRPC 服务 |
| `/api-chipper/use_ep` | GET | 切换到 Escape Pod 模式 |
| `/api-chipper/use_ip` | GET | 切换到 IP 模式（参数 `port`） |

---

### 2.3 SDK App API（端口 80）

**Base URL**：`http://<wirepod-ip>/api-sdk/`

**说明**：这些端点通过 `vector-go-sdk` 直接连接机器人。Wire-Pod 在这里充当**代理/网关**。

**通用参数**：几乎所有端点都需要 `serial=<ESN>` 参数指定目标机器人。

---

#### 机器人信息

##### GET /api-sdk/get_sdk_info

**响应**：`botSdkInfo.json` 内容，包含所有已认证机器人列表。

```json
{
  "global_guid": "...",
  "robots": [
    {
      "esn": "00e20145",
      "ip_address": "192.168.1.150",
      "guid": "abc-def",
      "activated": true
    }
  ]
}
```

---

##### GET /api-sdk/conn_test?serial=<ESN>

**功能**：测试与指定机器人的连接。

**响应**：`"success"` 或错误信息。

---

##### GET /api-sdk/get_battery?serial=<ESN>

**响应**：电池状态的 JSON。

```json
{
  "battery_level": 42,
  "battery_volts": 3.85,
  "is_on_charger_platform": true,
  "is_charging": false,
  "suggested_charger_sec": 0,
  "round_trip_time_ms": 15
}
```

---

#### 设置控制

| 端点 | 方法 | 参数 | 功能 |
|---|---|---|---|
| `/api-sdk/volume` | POST | `volume=0~5` | 设置音量 |
| `/api-sdk/eye_color` | POST | `color=0~6` | 设置预设眼睛颜色 |
| `/api-sdk/custom_eye_color` | POST | `hue=0~360`, `sat=0~1` | 自定义眼睛颜色 |
| `/api-sdk/locale` | POST | `locale=en-US` 等 | 设置区域 |
| `/api-sdk/location` | POST | `location=城市名` | 设置默认位置 |
| `/api-sdk/timezone` | POST | `timezone=Asia/Shanghai` | 设置时区 |
| `/api-sdk/time_format_12` | POST | 无 | 12 小时制 |
| `/api-sdk/time_format_24` | POST | 无 | 24 小时制 |
| `/api-sdk/temp_c` | POST | 无 | 摄氏温度 |
| `/api-sdk/temp_f` | POST | 无 | 华氏温度 |
| `/api-sdk/button_hey_vector` | POST | 无 | 按钮唤醒词设为 Hey Vector |
| `/api-sdk/button_alexa` | POST | 无 | 按钮唤醒词设为 Alexa |

**实现**：均通过 `vector-go-sdk` 调用 `PullJdocs`/`UpdateSettings` 或直接 gRPC 方法。

---

#### 动作控制

| 端点 | 方法 | 参数 | 功能 |
|---|---|---|---|
| `/api-sdk/say_text` | POST | `text=要说的话` | 让机器人说话 |
| `/api-sdk/cloud_intent` | POST | `intent=intent_name` | 触发内置意图 |
| `/api-sdk/move_wheels` | POST | `lw=左轮速度`, `rw=右轮速度` | 驱动轮子 |
| `/api-sdk/move_head` | POST | `speed=速度` | 移动头部 |
| `/api-sdk/move_lift` | POST | `speed=速度` | 移动抬升臂 |
| `/api-sdk/assume_behavior_control` | POST | `priority=10` | 接管行为控制 |
| `/api-sdk/release_behavior_control` | POST | 无 | 释放行为控制 |
| `/api-sdk/play_animation` | POST | `animation=anim_name` | 播放动画 |

---

#### 摄像头

##### GET /cam-stream?serial=<ESN>

**功能**：MJPEG 实时视频流。

**响应头**：`Content-Type: multipart/x-mixed-replace; boundary=--boundary`

**实现**：`sdkapp/server.go: camStreamHandler()`

- 调用 `robot.Conn.EnableImageStreaming(ctx, true)`
- 创建 `CameraFeed` gRPC 流
- 每帧解码为 `image.Image`，再编码为 JPEG（质量 50%）
- 使用 `multipart/x-mixed-replace` 推送

---

#### 人脸管理

| 端点 | 方法 | 参数 | 功能 |
|---|---|---|---|
| `/api-sdk/get_faces` | GET | `serial` | 获取已注册人脸列表 |
| `/api-sdk/add_face` | POST | `serial`, `name` | 添加人脸（触发 `intent_meet_victor`） |
| `/api-sdk/rename_face` | POST | `serial`, `id`, `oldname`, `newname` | 重命名人脸 |
| `/api-sdk/delete_face` | POST | `serial`, `id` | 删除人脸 |

---

#### 照片管理

| 端点 | 方法 | 参数 | 功能 |
|---|---|---|---|
| `/api-sdk/get_image_ids` | GET | `serial` | 获取照片 ID 列表 |
| `/api-sdk/get_image` | GET | `serial`, `id` | 获取原图 |
| `/api-sdk/get_image_thumb` | GET | `serial`, `id` | 获取缩略图 |
| `/api-sdk/delete_image` | POST | `serial`, `id` | 删除照片 |

---

#### 事件流与刺激度

| 端点 | 方法 | 功能 |
|---|---|---|
| `/api-sdk/begin_event_stream` | POST | 开始监听 `stimulation_info` 事件 |
| `/api-sdk/get_stim_status` | GET | 获取当前刺激度数值 |
| `/api-sdk/stop_event_stream` | POST | 停止事件监听 |

---

#### 音频播放

##### POST /api-sdk/play_sound

**功能**：上传并播放音频文件。

**请求**：`multipart/form-data`，字段名 `sound`，文件格式 WAV（客户端需转换为 8kHz 单声道）。

**服务端处理**：
1. 读取文件内容
2. 切分为 1024 字节块
3. 通过 `ExternalAudioStreamPlayback` gRPC 流发送
4. 帧率：8000Hz，音量：100

---

### 2.4 BLE 配对 API（端口 8080）

**前缀**：`/api-ble/`

**注册位置**：`pkg/wirepod/setup/ble.go`

| 端点 | 功能 |
|---|---|
| `/api-ble/init` | 初始化 BLE 适配器 |
| `/api-ble/scan` | 扫描附近 Vector |
| `/api-ble/connect` | 连接指定机器人 |
| `/api-ble/send_pin` | 发送 PIN 码 |
| `/api-ble/wifi_scan` | 扫描 WiFi |
| `/api-ble/wifi_connect` | 连接 WiFi |
| `/api-ble/do_auth` | 执行认证流程 |
| `/api-ble/do_onboarding` | 完成 onboarding |

---

### 2.5 SSH 部署 API（端口 8080）

**前缀**：`/api-ssh/`

**注册位置**：`pkg/wirepod/setup/ssh.go`

| 端点 | 功能 |
|---|---|
| `/api-ssh/setup` | 上传 SSH 密钥并连接机器人 |
| `/api-ssh/check` | 检查部署状态 |

---

### 2.6 会话证书下载

##### GET /session-certs/<ESN>

**功能**：下载指定机器人的会话证书。

**响应**：PEM 格式的证书文件。

---

### 2.7 健康检查

##### GET /ok

##### GET /ok:80

**功能**：连通性检查。机器人会定期请求此端点验证与 Wire-Pod 的连接。

**响应**：`200 OK`，正文 `"ok"`

---

## 3. 内部接口

### 3.1 STT 引擎接口

所有 STT 包必须暴露以下符号：

```go
package stt

var Name string          // 引擎名称

func Init() error        // 初始化（加载模型等）

func STT(req sr.SpeechRequest) (string, error)
// 输入：SpeechRequest（含完整音频数据）
// 输出：识别文本，或错误
```

**特殊情况**：Rhino STI（Speech-to-Intent）返回 `(string, map[string]string, error)`，在 `preqs/server.go` 中通过反射检测。

---

### 3.2 Go 插件接口

插件必须导出以下符号：

```go
package main

var Utterances *[]string   // 触发语句列表

var Name *string           // 插件名称

var Action func(transcribedText, botSerial, guid, target string) (string, string)
// 参数：识别文本、机器人序列号、GUID、目标地址
// 返回：（响应文本，意图名）
// 若响应文本非空，直接作为回答；若意图名非空，作为意图传递
```

---

### 3.3 Lua 脚本接口

自定义意图可通过 `luascript` 字段关联 Lua 代码。运行时注入的变量：

```lua
-- 预定义变量
robot                       -- 当前机器人对象

-- 可用函数
assumeBehaviorControl(priority)
releaseBehaviorControl()
sayText(text, useGoroutine)
playAnimation(name, useGoroutine)
moveHead(speed)
moveLift(speed)
moveWheels(l, r, l2, r2)
showImage(path, durationMs)
postHTTPRequest(url, contentType, body, timeoutSec)
getHTTPRequest(url, timeoutSec)
sleep(ms)
```

---

### 3.4 Intent Processor 接口

```go
// pkg/servers/chipper/server.go
type intentProcessor interface {
    ProcessIntent(*vtt.IntentRequest) (*vtt.IntentResponse, error)
}

type kgProcessor interface {
    ProcessKnowledgeGraph(*vtt.KnowledgeGraphRequest) (*vtt.KnowledgeGraphResponse, error)
}

type intentGraphProcessor interface {
    ProcessIntentGraph(*vtt.IntentGraphRequest) (*vtt.IntentGraphResponse, error)
}
```

**实现者**：`pkg/wirepod/preqs/server.go` 中的 `Server` 结构体。

---

## 4. WebSocket

**当前状态**：Wire-Pod **未使用原生 WebSocket**。

SDK App 的 stimulation 事件流通过以下方式实现：
- 前端轮询 `GET /api-sdk/get_stim_status`
- 后端在 goroutine 中通过 gRPC `EventStream` 接收事件，存入内存变量

摄像头流使用 **MJPEG over HTTP**（`multipart/x-mixed-replace`），不是 WebSocket。

---

## 5. 接口变更注意事项

| 接口层 | 变更影响 |
|---|---|
| gRPC protobuf | 必须与机器人固件同步更新，变更成本高 |
| `/api/*` HTTP | 直接影响 Web UI，需同步更新前端 |
| `/api-sdk/*` HTTP | 影响 SDK App 前端，但后端可直接增删 |
| `/api-ble/*` / `/api-ssh/*` | 仅影响 setup 流程 |
| STT 引擎接口 | 新增引擎需创建新目录并在 `cmd/` 添加入口 |
| Go 插件接口 | 变更会破坏现有插件，需保持向后兼容 |

---

*本文档基于实际代码中的 handler 注册和 gRPC 服务实现编写。如需查看最新端点列表，可搜索代码中的 `http.HandleFunc` 和 `grpc.Register*` 调用。*
