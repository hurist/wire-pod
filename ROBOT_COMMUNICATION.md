# Wire-Pod 机器人通信协议文档

本文档详细说明 Vector 机器人如何与 Wire-Pod 建立连接、认证、交换数据和状态同步。理解这部分对于调试连接问题、认证失败和固件兼容性至关重要。

---

## 1. 通信概览

```
┌─────────────────────────────────────────────────────────────┐
│                      Vector 机器人                            │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐     │
│  │   麦克风    │    │   AI 引擎   │    │  动作系统   │     │
│  │  (采集语音) │    │ (执行意图)  │    │ (动画/电机) │     │
│  └──────┬──────┘    └──────┬──────┘    └──────▲──────┘     │
│         │                  │                   │            │
│         └──────────────────┼───────────────────┘            │
│                            │                                │
│                   ┌────────┴────────┐                       │
│                   │   vic-cloud     │ ← 云连接客户端         │
│                   │   vic-gateway   │ ← SDK 服务端           │
│                   └────────┬────────┘                       │
│                            │ gRPC over mTLS                 │
└────────────────────────────┼────────────────────────────────┘
                             │
┌────────────────────────────┼────────────────────────────────┐
│                      Wire-Pod                             │
│  ┌─────────────────────────▼─────────────────────────────┐ │
│  │              cmux TLS Listener (443)                  │ │
│  │  ┌──────────────┐          ┌──────────────────────┐  │ │
│  │  │ gRPC/HTTP2   │          │ HTTP/1.1 (health)    │  │ │
│  │  │ Chipper/JDocs│          │ /ok, /ok:80          │  │ │
│  │  │ Token        │          └──────────────────────┘  │ │
│  │  └──────────────┘                                     │ │
│  └───────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────┘
```

---

## 2. 连接建立

### 2.1 机器人身份

每台 Vector 机器人在出厂时烧录了唯一身份：

| 文件 | 位置 | 内容 |
|---|---|---|
| `AnkiRobotDeviceCert.pem` | `/factory/cloud/` | X.509 证书，CN=`vic:<ESN>` |
| `AnkiRobotDeviceKeys.pem` | `/factory/cloud/` | PKCS#8 私钥 |
| `Info<ESN>.json` | `/factory/cloud/` | 证书元数据 |

**ESN**（Electronic Serial Number）：机器人的唯一标识，如 `00e20145`。

### 2.2 服务器配置

机器人通过 `server_config.json` 知道连接哪个云端服务器：

```json
{
  "jdocs": "escapepod.local:443",
  "token": "escapepod.local:443",
  "chipper": "escapepod.local:443",
  "check": "http://escapepod.local/ok",
  "logfiles": "...",
  "appkey": "..."
}
```

**Wire-Pod setup 时的操作**：
- **Escape Pod 模式**：写入 `escapepod.local` 作为服务器地址
- **IP 模式**：写入 Wire-Pod 服务器的实际 IP 地址

文件位置（机器人端）：
- 生产固件：`/anki/data/assets/cozmo_resources/config/server_config.json`
- Wire-Pod 定制固件：`/data/data/server_config.json`

### 2.3 TLS 握手

** mutual TLS（mTLS）**：
1. 机器人发送客户端证书（`AnkiRobotDeviceCert.pem`）
2. Wire-Pod 验证证书链（信任工厂根 CA）
3. Wire-Pod 发送服务器证书（`cert.crt`，自签名或本地 CA）
4. 机器人验证服务器证书（通过 `/anki/etc/wirepod-cert.crt` 或 `/data/data/wirepod-cert.crt`）

**证书生成**（Wire-Pod 端）：
```go
// pkg/wirepod/setup/certs.go
func CreateCertCombo() error {
    // 生成 1028-bit RSA CA
    // 生成服务器证书，绑定到 outbound IP
    // 有效期 10 年
    // 写入 ../certs/cert.crt 和 ../certs/cert.key
}
```

---

## 3. 认证流程

### 3.1 首次配对（Primary User Association）

```
机器人          Wire-Pod
  │                 │
  │  1. gRPC 连接   │
  │────────────────►│
  │                 │
  │  2. AssociatePrimaryUser(sessionCert)
  │────────────────►│
  │                 │
  │  3. TokenBundle │
  │◄────────────────│
  │   (JWT + refresh token)
  │                 │
  │  4. 后续请求携带 JWT
  │────────────────►│
```

**session certificate**：机器人生成的临时证书，用于证明本次会话的合法性。

**JWT 内容**（`pkg/servers/token/token.go`）：
```go
claims := jwt.MapClaims{
    "token_id":     uuid,
    "token_type":   "session",
    "user_id":      userID,
    "requestor_id": requestorID,
    "iat":          time.Now().Unix(),
    "expires":      time.Now().Add(30 * 24 * time.Hour).Unix(),
}
```

### 3.2 Token 刷新

**自动刷新**（机器人端 `vector-cloud/internal/token/refresher.go`）：
- 每 5 分钟检查一次 JWT 有效期
- 在过期前 3 小时主动刷新
- 调用 `RefreshToken()` 获取新的 TokenBundle

### 3.3 SDK 认证（Secondary Client）

当手机/电脑通过 SDK 连接机器人时：

```
SDK App          vic-gateway       Wire-Pod
   │                 │                 │
   │  连接请求        │                 │
   │────────────────►│                 │
   │                 │                 │
   │                 │  读取 vic.AppTokens jdoc
   │                 │────────────────►│
   │                 │                 │
   │                 │  返回 token hashes
   │                 │◄────────────────│
   │                 │                 │
   │  发送 client token
   │────────────────►│                 │
   │                 │                 │
   │                 │  bcrypt 比对 hash
   │                 │────────────────►│
   │                 │                 │
   │  认证通过        │                 │
   │◄────────────────│                 │
```

**Token hash 存储**：`gateway/tokens.go` 将有效的 client token bcrypt hash 保存到 `/data/vic-gateway/token-hashes.json`。

---

## 4. JDocs（JSON Documents）

### 4.1 什么是 JDocs

JDocs 是 Vector 的**分布式 JSON 文档存储系统**，用于同步机器人和云端之间的设置、状态和数据。

**核心概念**：
- **Thing**：文档所有者，通常是机器人（如 `vic:00e20145`）
- **DocName**：文档名，如 `vic.RobotSettings`、`vic.AppTokens`
- **DocVersion**：单调递增版本号，用于乐观并发控制
- **FmtVersion**：格式版本号
- **JsonDoc**：实际的 JSON 内容

### 4.2 文档类型

| 文档名 | 用途 | Wire-Pod 处理 |
|---|---|---|
| `vic.RobotSettings` | 机器人设置（音量、颜色、位置等） | 读写 |
| `vic.AppTokens` | App 认证 token | 认证时自动写入 |
| `vic.RobotLifetimeStats` | 机器人终身统计 | 透传 |
| `vic.AccountSettings` | 账户设置 | 透传 |
| `vic.UserEntitlements` | 用户权益 | 透传 |

### 4.3 读写流程

**读取**（`pkg/servers/jdocs/server.go: ReadDocs`）：
```go
func (s *Server) ReadDocs(ctx context.Context, req *ReadDocsReq) (*ReadDocsResp, error) {
    for _, item := range req.Items {
        jdoc, exists := vars.GetJdoc(req.Thing, item.DocName)
        if !exists {
            // 返回 NOT_FOUND
        } else if jdoc.DocVersion == item.DocVersion {
            // 返回 UNCHANGED
        } else {
            // 返回 CHANGED + 文档内容
        }
    }
}
```

**写入**（`pkg/servers/jdocs/server.go: WriteDoc`）：
```go
func (s *Server) WriteDoc(ctx context.Context, req *WriteDocReq) (*WriteDocResp, error) {
    // 乐观并发控制
    existing, _ := vars.GetJdoc(req.Thing, req.DocName)
    if existing.DocVersion != req.Doc.DocVersion {
        return &WriteDocResp{Status: REJECTED_DOC_VERSION}
    }
    
    // 接受写入
    vars.AddJdoc(req.Thing, req.DocName, req.Doc)
    return &WriteDocResp{Status: ACCEPTED}
}
```

### 4.4 认证触发

**关键逻辑**：当机器人读取 `vic.AppTokens` 时，Wire-Pod 会触发认证流程：

```go
// pkg/servers/jdocs/botInfoStorer.go（推断）
func OnReadAppTokens(thing string, robotIP string) {
    // 1. 匹配机器人 IP 到已知列表
    // 2. 生成 GUID
    // 3. 写入 botSdkInfo.json
    // 4. 生成 session cert 到 session-certs/<ESN>
    // 5. 更新 sdk_config.ini
}
```

这是机器人第一次成功连接 Wire-Pod 时的关键步骤。

---

## 5. 语音通信协议

### 5.1 消息类型

机器人内部的 `vic-cloud` 与 AI/引擎进程通过 Unix Domain Socket 交换 CLAD 消息：

**从麦克风到 vic-cloud**：
| CLAD 消息 | 方向 | 内容 |
|---|---|---|
| `Hotword` | mic → vic-cloud | 唤醒词检测，携带模式（Normal/Blackjack/KnowledgeGraph） |
| `Audio` | mic → vic-cloud | PCM 音频数据（`[]int16`） |
| `AudioDone` | mic → vic-cloud | 用户停止说话 |

**从 vic-cloud 到 AI 引擎**：
| CLAD 消息 | 方向 | 内容 |
|---|---|---|
| `Result` | vic-cloud → AI | 意图结果（名称 + 参数 JSON） |
| `Error` | vic-cloud → AI | 错误信息 |
| `StopSignal` | vic-cloud → mic | 请求停止音频采集 |

### 5.2 gRPC 流到 CLAD 的映射

```
机器人内部                              Wire-Pod
┌─────────┐   CLAD    ┌──────────┐   gRPC    ┌──────────┐
│  mic    │──────────►│vic-cloud │──────────►│ Chipper  │
│ 进程    │  Audio    │ 进程     │  stream   │ 服务     │
└─────────┘           └──────────┘           └────┬─────┘
                                                  │
                                            ┌─────┴─────┐
                                            │  VTT/Preqs │
                                            └───────────┘
                                                  │
                                           IntentResult
                                                  │
┌─────────┐   CLAD    ┌──────────┐   gRPC    ┌───▼──────┐
│  AI/引擎 │◄──────────│vic-cloud │◄──────────│ 响应流   │
│ 进程    │  Result   │ 进程     │  stream   │          │
└─────────┘           └──────────┘           └──────────┘
```

### 5.3 连接检查（Connection Check）

机器人定期发送 `StreamingConnectionCheck` 来验证与云端的连通性：

```go
// pkg/servers/chipper/connectioncheck.go
func (s *Server) StreamingConnectionCheck(stream pb.ChipperGrpc_StreamingConnectionCheckServer) error {
    var frameCount int
    for {
        req, err := stream.Recv()
        if err != nil { break }
        frameCount++
        // 直到超时或收到结束信号
    }
    return stream.Send(&pb.ConnectionCheckResponse{
        Response: pb.ConnectionCheckResponse_AVAILABLE,
    })
}
```

---

## 6. SDK 远程控制协议

### 6.1 架构

```
SDK Client (手机/电脑)          vic-gateway (机器人)          vic-engine (机器人)
        │                              │                              │
        │  gRPC/HTTPS                  │                              │
        │─────────────────────────────►│                              │
        │                              │  CLAD IPC (Unix Socket)      │
        │                              │─────────────────────────────►│
        │                              │                              │
        │                              │◄─────────────────────────────│
        │◄─────────────────────────────│                              │
```

**vic-gateway** 是机器人上的 SDK 服务端，监听 TLS 端口（通常是 443）。

**Wire-Pod 的 SDK App**（`pkg/wirepod/sdkapp/`）不是直接连接 vic-gateway，而是**通过 vector-go-sdk 直接连接机器人**。

### 6.2 Wire-Pod 与机器人的直接连接

```
Wire-Pod SDK App          Vector 机器人
        │                      │
        │  gRPC (vector-go-sdk) │
        │──────────────────────►│
        │  IP:443               │
        │                      │
        │  BatteryState        │
        │  SayText             │
        │  DriveWheels         │
        │  PlayAnimation       │
        │  BehaviorControl     │
        │  CameraFeed          │
        │  ExternalAudio...    │
```

**连接凭据**：
- ESN：`botSdkInfo.json` 中记录的序列号
- Token：`botSdkInfo.json` 中的 GUID
- Target：机器人 IP + `:443`

### 6.3 行为控制（Behavior Control）

**优先级系统**：
```go
type ControlRequest_Priority int32
const (
    ControlRequest_UNKNOWN             ControlRequest_Priority = 0
    ControlRequest_LOW_PRIORITY        ControlRequest_Priority = 1
    ControlRequest_NORMAL_PRIORITY     ControlRequest_Priority = 2
    ControlRequest_HIGH_PRIORITY       ControlRequest_Priority = 3
    ControlRequest_OVERRIDE_BEHAVIORS  ControlRequest_Priority = 10  // Wire-Pod 使用
)
```

**Wire-Pod 使用 `OVERRIDE_BEHAVIORS`（优先级 10）**，这是最高优先级，会覆盖机器人的所有自主行为。

**控制流程**：
```go
// 1. 请求控制
r, _ := robot.Conn.BehaviorControl(ctx)
r.Send(&vectorpb.BehaviorControlRequest{
    RequestType: &vectorpb.BehaviorControlRequest_ControlRequest{
        ControlRequest: &vectorpb.ControlRequest{
            Priority: vectorpb.ControlRequest_OVERRIDE_BEHAVIORS,
        },
    },
})

// 2. 等待授权
ctrlresp, _ := r.Recv()
if ctrlresp.GetControlGrantedResponse() != nil {
    // 获得控制权，可以执行动作和语音
}

// 3. 释放控制
r.Send(&vectorpb.BehaviorControlRequest{
    RequestType: &vectorpb.BehaviorControlRequest_ControlRelease{
        ControlRelease: &vectorpb.ControlRelease{},
    },
})
```

---

## 7. 状态同步

### 7.1 JDocs 同步模型

Wire-Pod 采用**拉取为主**的同步模型：
- 机器人主动 `ReadDocs` 获取最新设置
- 机器人主动 `WriteDoc` 上报本地变更
- Wire-Pod 被动响应，不主动推送

### 7.2 SDK App 设置同步

当用户在 Web UI 的 SDK App 中修改设置时：

```
用户点击 "Save"
    │
    ▼
sdkapp/server.go 调用 vector-go-sdk
    │
    ▼
机器人收到 UpdateSettings gRPC 请求
    │
    ▼
机器人更新本地 JDoc
    │
    ▼
机器人下次 ReadDocs 时，Wire-Pod 返回最新版本
```

**延迟**：设置变更可能需要数秒才能完全同步到 Wire-Pod 的 `jdocs.json`。

### 7.3 电池状态

电池状态**不通过 JDocs 同步**，而是通过 SDK App 的实时查询：

```go
// sdkapp/server.go
resp, _ := robot.Conn.BatteryState(ctx, &vectorpb.BatteryStateRequest{})
```

Web UI 的电池显示（`js/battery.js`）每 3-6 秒轮询一次。

---

## 8. 固件兼容性

### 8.1 固件版本与接口对应

| 固件版本 | Chipper 接口 | 音频编码 | 备注 |
|---|---|---|---|
| ≤1.6 | `StreamingIntent` | PCM | Legacy 接口 |
| ≥1.7 | `StreamingIntentGraph` | Opus/PCM | 现代接口，支持 Intent/KG 混合 |
| 2.0.1 | `StreamingIntentGraph` | Opus | 需要端口 8084 兼容性监听 |

### 8.2 固件部署方式

**生产机器人（未解锁）**：
1. BLE 配对（`pkg/wirepod/setup/ble.go`）
2. WiFi 配置
3. 认证 onboarding
4. 固件检查，必要时 OTA 更新

**OSKR/开发机器人（已解锁）**：
1. SSH 连接（`pkg/wirepod/setup/ssh.go`）
2. 上传 `server_config.json` 和 Wire-Pod 证书
3. 停止/重启机器人服务

---

## 9. 常见问题排查

### 9.1 机器人无法连接

```
检查清单：
□ 机器人 WiFi 是否与 Wire-Pod 同局域网？
□ 机器人 server_config.json 中的地址是否正确？
□ Wire-Pod 证书是否匹配当前 IP？（IP 变化需重新生成）
□ 防火墙是否放行了 443/80/8080 端口？
□ 机器人时间是否正确？（TLS 证书验证需要正确时间）
```

### 9.2 认证失败

```
症状：机器人连接后立即断开，或持续重连
排查：
1. 查看 Wire-Pod 日志中是否有 "Loaded bot info file"
2. 检查 session-certs/ 目录是否有对应 ESN 的证书
3. 检查 botSdkInfo.json 中该机器人的 GUID 是否为空
4. 尝试在 Web UI 中删除该机器人后重新认证
```

### 9.3 JDocs 版本冲突

```
症状：机器人设置无法保存，或不断同步
原因：乐观并发控制失败（DocVersion 不匹配）
解决：
1. 停止 Wire-Pod
2. 删除 jdocs/jdocs.json 中该机器人的条目
3. 重启 Wire-Pod，让机器人重新全量同步
```

---

## 10. 协议扩展建议

若未来需要扩展机器人协议：

1. **gRPC 服务扩展**：在 `pkg/servers/chipper/server.go` 中新增接口方法，需同步更新 protobuf 定义
2. **JDocs 扩展**：新增 DocName，在 `pkg/servers/jdocs/server.go` 中处理
3. **CLAD 消息扩展**：修改 `vector-cloud/internal/clad/`，需要重新编译机器人端固件

---

*本文档基于 `vector-cloud/` 和 `pkg/servers/*` 的实际代码编写。理解机器人端代码对于调试连接问题至关重要。*
