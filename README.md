# Wire-Pod

**Wire-Pod** 是一款功能完善的开源替代云服务器，专为 Anki/Digital Dream Labs 的 [Vector](https://www.digitaldreamlabs.com/) 机器人设计。它在本地网络中完全替代官方付费云服务，使 Vector 1.0 和 2.0 机器人无需订阅即可使用语音命令、AI 对话等全部功能。

> **核心理念**：Wire-Pod 复刻了 Digital Dream Labs 官方云端的协议接口，让机器人"以为"自己在连接官方服务器，而实际上所有处理都在本地或由用户自主控制的服务器上完成。

---

## 目录

- [系统概览](#系统概览)
- [核心功能](#核心功能)
- [系统架构](#系统架构)
- [技术栈](#技术栈)
- [安装部署](#安装部署)
- [运行方式](#运行方式)
- [Docker 部署](#docker-部署)
- [配置说明](#配置说明)
- [AI / LLM 配置](#ai--llm-配置)
- [Web 后台](#web-后台)
- [调试方法](#调试方法)
- [常见问题](#常见问题)
- [文档索引](#文档索引)

---

## 系统概览

```
┌──────────────┐      WiFi/LAN       ┌──────────────────────────────────────┐
│   Vector     │◄───────────────────►│           Wire-Pod Server            │
│   Robot      │   mTLS over gRPC    │  ┌─────────┐  ┌─────────┐  ┌──────┐ │
│              │                     │  │ Chipper │  │ JDocs   │  │Token │ │
│ (vic-cloud)  │◄──语音/认证/设置────►│  │ (gRPC)  │  │ Service │  │Svc   │ │
│              │                     │  └────┬────┘  └─────────┘  └──────┘ │
│ (vic-gateway)│◄──SDK 远程控制──────►│       │                             │
└──────────────┘                     │  ┌────┴───────────────────────────┐   │
                                     │  │         语音处理流水线          │   │
                                     │  │  ┌──────┐ ┌────┐ ┌──────────┐ │   │
                                     │  │  │ STT  │→│AI  │→│ TTS/响应 │ │   │
                                     │  │  │Vosk/ │  │LLM │  │Vector  │ │   │
                                     │  │  │Whisper│ │OpenAI│ │SDK    │ │   │
                                     │  │  └──────┘ └────┘ └──────────┘ │   │
                                     │  └────────────────────────────────┘   │
                                     │  ┌─────────────────────────────────┐  │
                                     │  │     Web 管理后台 (Port 8080)      │  │
                                     │  │  ┌─────────┐  ┌──────────────┐  │  │
                                     │  │  │ 配置 API │  │ 自定义意图    │  │  │
                                     │  │  │ Web UI  │  │ 机器人管理    │  │  │
                                     │  │  └─────────┘  └──────────────┘  │  │
                                     │  └─────────────────────────────────┘  │
                                     └───────────────────────────────────────┘
```

### Wire-Pod 替代了哪些官方服务？

| 官方 DDL 服务 | Wire-Pod 对应模块 |
|---|---|
| Chipper（语音识别/意图处理） | `pkg/servers/chipper` + `pkg/wirepod` |
| JDocs（机器人设置同步） | `pkg/servers/jdocs` + `jdocs/jdocs.json` |
| Token Service（认证） | `pkg/servers/token` + 本地 JWT |
| Knowledge Graph（AI 问答） | OpenAI/Together/自定义 LLM 集成 |
| SDK Gateway（远程控制） | `pkg/wirepod/sdkapp` + 直连机器人控制 |

---

## 核心功能

### 语音与 AI
- **多引擎语音识别（STT）**：Vosk（本地，默认）、Coqui、Picovoice Leopard、OpenAI Whisper API、whisper.cpp（本地）、Houndify
- **大语言模型集成**：OpenAI GPT-4o-mini、Together AI（Llama 3）、任意 OpenAI 兼容接口
- **流式响应**：LLM 响应按句子实时切分、实时语音合成，机器人边说边播动画
- **对话记忆**：按机器人 ESN 保存最近 16 轮对话上下文
- **AI 机器人指令**：LLM 可通过 `{{command||param}}` 语法触发机器人动画、拍照、发起新语音请求
- **自定义意图**：用户可定义新的语音命令，支持 Lua 脚本或调用外部程序

### 机器人管理
- **Web 仪表盘**：端口 8080 提供完整的配置界面
- **SDK App**：直接远程控制机器人（摄像头、移动、设置、人脸管理）
- **BLE 配网**：生产版机器人可通过蓝牙低功耗配对 onboarding
- **OSKR/开发版支持**：通过 SSH 为解锁机器人部署配置
- **OTA 代理**：从 archive.org 代理固件更新
- **多机器人支持**：单服务器可同时管理多台 Vector

### 可扩展性
- **Lua 脚本引擎**：自定义行为，完全访问机器人 SDK
- **Go 插件系统**：原生 `.so` 插件，高性能扩展
- **意图系统**：基于 JSON 的意图定义，支持模糊匹配

---

## 系统架构

Wire-Pod 采用**分层服务架构**：

```
┌─────────────────────────────────────────────────────────────┐
│                        传输层                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │   gRPC/443   │  │  HTTP/8080   │  │    HTTP/80       │  │
│  │  (Chipper)   │  │  (Web UI)    │  │  (SDK/connCheck) │  │
│  └──────┬───────┘  └──────┬───────┘  └────────┬─────────┘  │
└─────────┼─────────────────┼───────────────────┼────────────┘
          │                 │                   │
┌─────────┼─────────────────┼───────────────────┼────────────┐
│         ▼                 ▼                   ▼            │
│  ┌────────────────────────────────────────────────────┐   │
│  │                   应用层                              │   │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐           │   │
│  │  │  意图图   │ │ 知识图谱 │ │  JDocs   │           │   │
│  │  │IntentGraph│ │   KG     │ │  Server  │           │   │
│  │  └────┬─────┘ └────┬─────┘ └────┬─────┘           │   │
│  │       └─────────────┴─────────────┘                │   │
│  │                    │                               │   │
│  │              ┌─────┴─────┐                        │   │
│  │              │ VTT/Preqs │ ← 语音请求统一处理      │   │
│  │              └─────┬─────┘                        │   │
│  └────────────────────┼───────────────────────────────┘   │
│                       │                                   │
│  ┌────────────────────┼───────────────────────────────┐   │
│  │                  处理层                               │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────────────┐  │   │
│  │  │   STT    │  │  意图匹配 │  │ LLM/TTS/SDK 控制 │  │   │
│  │  │  引擎    │  │ Intent   │  │   (ttr/kgsim)    │  │   │
│  │  └──────────┘  └──────────┘  └──────────────────┘  │   │
│  └────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────┘
```

详细架构说明请参阅 [ARCHITECTURE.md](doc/ARCHITECTURE.md)。

---

## 技术栈

| 层级 | 技术选型 |
|---|---|
| **编程语言** | Go 1.18+ |
| **Web UI** | 原生 HTML/JS/CSS（无前端框架） |
| **gRPC 服务** | `google.golang.org/grpc` + `cmux` 多路复用 |
| **语音识别** | Vosk(CGO)、whisper.cpp(CGO)、OpenAI Whisper API、Coqui、Picovoice Leopard、Houndify |
| **大语言模型** | OpenAI API (`sashabaranov/go-openai`) |
| **音频处理** | Opus(DDL fork)、WebRTC VAD、go-audio/wav |
| **机器人 SDK** | `fforchino/vector-go-sdk` |
| **脚本引擎** | Gopher-Lua |
| **插件系统** | Go `plugin` 包 (`.so`) |
| **配置存储** | JSON 文件（`apiConfig.json`、`customIntents.json`） |
| **安全认证** | 工厂证书 + 本地 CA、JWT (`golang-jwt/jwt`) |
| **服务发现** | `kercre123/zeroconf` (mDNS) |

---

## 安装部署

### 环境要求

- **操作系统**：Linux（推荐）、macOS、Windows（WSL）或 Docker
- **Go 版本**：1.18 或更高
- **硬件**：任何能常驻局域网运行的设备（树莓派 4/5、旧电脑、NAS 等）
- **网络**：机器人与服务器必须在同一局域网

### 快速安装（Linux/macOS）

```bash
git clone https://github.com/kercre123/wire-pod.git
cd wire-pod
./setup.sh
```

setup.sh 脚本会自动完成：
1. 安装系统依赖（libopus、libsox 等）
2. 下载默认 Vosk 语音模型
3. 生成 TLS 证书
4. 编译 `chipper` 二进制文件

### 手动编译

```bash
cd chipper
go build -o chipper ./cmd/vosk   # 或 ./cmd/whisper.cpp、./cmd/coqui 等
```

---

## 运行方式

### 开发/直接运行

```bash
cd chipper
./chipper
```

首次运行会启动端口 8080 的配置向导。在浏览器中打开 `http://<服务器IP>:8080` 完成初始配置。

### 环境变量

| 变量名 | 说明 | 默认值 |
|---|---|---|
| `STT_SERVICE` | STT 引擎（`vosk`、`whisper`、`whisper.cpp`、`coqui`、`leopard`、`houndify`） | `vosk` |
| `STT_LANGUAGE` | 语言代码（如 `en-US`、`de-DE`、`zh-CN`） | `en-US` |
| `WEBSERVER_PORT` | Web UI 端口 | `8080` |
| `DDL_RPC_PORT` | gRPC 服务端口 | `443` |
| `OPENAI_KEY` | OpenAI API 密钥（用于 Whisper STT 或 LLM） | — |
| `PICOVOICE_APIKEY` | Picovoice API 密钥（用于 Leopard） | — |
| `HOUNDIFY_STT_ID` / `HOUNDIFY_STT_KEY` | Houndify 凭证 | — |
| `WEATHERAPI_ENABLED` | 启用天气 API | `false` |
| `KNOWLEDGE_ENABLED` | 启用 LLM 知识图谱 | `false` |
| `KNOWLEDGE_PROVIDER` | LLM 提供商（`openai`、`together`、`custom`、`houndify`） | — |
| `KNOWLEDGE_KEY` | LLM 提供商 API 密钥 | — |
| `WIREPOD_DATA_DIR` | 数据目录（Docker 环境） | `./` |

---

## Docker 部署

### 使用 Docker Compose（推荐）

```bash
docker compose up -d
```

暴露端口：
- `80` — SDK App / 连通性检查
- `443` — gRPC（Chipper）
- `8080` — Web 配置后台
- `8084` — gRPC 备用（兼容 2.0.1 固件）

### 数据卷

| 卷名 | 容器内路径 | 用途 |
|---|---|---|
| `wire-pod-data` | `/data` | 持久化配置、JDocs、证书、模型 |
| `wire-pod-images` | `/images` | 机器人照片 |

---

## 配置说明

Wire-Pod 的所有配置以 JSON 文件形式保存在工作目录（Docker 环境下为 `/data`）：

| 文件 | 用途 |
|---|---|
| `apiConfig.json` | 服务器主配置（STT、天气、LLM、服务器模式） |
| `customIntents.json` | 用户自定义语音意图 |
| `jdocs/jdocs.json` | 机器人设置与 App Token |
| `botSdkInfo.json` | 已认证机器人列表 |
| `session-certs/` | 每台机器人的会话证书 |
| `intent-data/<lang>.json` | 各语言的内置意图定义 |

所有配置均可通过 Web UI (`http://<服务器IP>:8080`) 管理。

### 服务器模式

- **Escape Pod 模式**（`epconfig: true`）：使用 `escapepod.local` 主机名和预置的 `ep.crt`/`ep.key`。推荐生产版机器人使用。
- **IP 模式**（`epconfig: false`）：生成绑定到外网 IP 的证书。推荐 OSKR/开发版机器人使用。

---

## AI / LLM 配置

1. 打开 Web UI → **Server Settings（服务器设置）**
2. 在 **Knowledge Graph（知识图谱）** 区域选择提供商：
   - **OpenAI**：输入 API 密钥。默认使用 `gpt-4o-mini`（不可用时回退到 `gpt-3.5-turbo`）。
   - **Together AI**：输入 API 密钥。默认使用 `meta-llama/Llama-3-70b-chat-hf`。
   - **Custom**：输入任意 OpenAI 兼容接口的地址和密钥。
   - **Houndify**：旧版非 LLM 提供商。
3. 启用 **Intent Graph Forwarding** 使未匹配到的语音命令转发给 LLM 处理。
4. 可选：启用 **Save Chat** 保存每台机器人的对话记忆。
5. 可选：启用 **Commands Enable** 允许 LLM 控制机器人动画和摄像头。

### LLM 机器人指令

启用 Commands 后，LLM 可在回复中嵌入指令：
- `{{playAnimationWI||happy}}` — 播放动画，不打断语音
- `{{playAnimation||sad}}` — 播放动画，打断语音
- `{{getImage||front}}` — 拍摄前方画面并分析
- `{{newVoiceRequest||now}}` — 触发新的语音请求

---

## Web 后台

Web UI (`http://<服务器IP>:8080`) 提供以下功能：

- **Bot Setup**：BLE 配对、SSH 部署、证书管理
- **Server Settings**：天气 API、知识图谱（LLM）、STT 语言
- **Custom Intents**：添加/编辑/删除自定义语音命令
- **Bot Settings**：通过 SDK App 管理单台机器人配置
- **Logs**：实时服务器日志，可切换调试模式
- **Version Info**：检查 GitHub 上的版本更新

### SDK App

SDK App 提供直接控制机器人的界面 (`http://<服务器IP>/sdkapp/`)：

- 实时摄像头画面 (`/cam-stream`)
- 轮子驱动、头部/抬升臂运动
- 眼睛颜色、音量、区域设置
- 人脸注册管理
- 行为控制接管/释放
- 播放动画和音效
- 电池与刺激度监控

---

## 调试方法

### 启用调试日志

启动前设置环境变量：
```bash
export DEBUG_LOGGING=true
```

或在 Web UI → Log → 勾选 "Enable Debug Logging"。

### 关键日志标识

| 组件 | 应关注的关键日志 |
|---|---|
| 语音识别 | `Initiating <engine> voice processor`、`(Bot <ESN>) End of speech detected` |
| 意图匹配 | `Intent matched: <intent>, transcribed text: '<text>'` |
| LLM | `LLM response for <ESN>: <text>`、`Using <model>` |
| 机器人连接 | `Loaded bot info file, known bots: [...]` |
| gRPC | `stream.Recv()` 或 `stream.Send()` 的错误信息 |

### 调试音频

在 `chipper/pkg/wirepod/speechrequest/speechrequest.go` 中将 `debugWriteFile` 设为 `true`，可在 `/tmp/wirepodtest.ogg` 获取原始音频流。

---

## 常见问题

详细排查指南请参阅 [TROUBLESHOOTING.md](doc/TROUBLESHOOTING.md)。

### 快速诊断

| 现象 | 可能原因 | 解决方法 |
|---|---|---|
| 机器人无法连接 | 机器人端 server_config.json 错误 | 重新执行 setup，核对 IP/主机名 |
| 语音识别无响应 | STT 模型缺失 | 检查 `vosk/models/` 目录，或通过 Web UI 重新下载 |
| AI 回复失败 | API 密钥无效 | 在 Web UI 中验证密钥，查看服务器日志 |
| Web UI 无法访问 | 8080 端口被占用 | 设置 `WEBSERVER_PORT` 环境变量更换端口 |
| 证书错误 | 服务器 IP 变更 | 在 Web UI 中重新生成证书 |
| 音频杂音/错乱 | Opus 解码异常 | 检查固件发送的是 Opus 还是 PCM |
| SDK App 连不上机器人 | GUID 不匹配或机器人离线 | 在 Web UI 中重新认证机器人 |

---

## 文档索引

| 文档 | 内容 |
|---|---|
| [ARCHITECTURE.md](doc/ARCHITECTURE.md) | 系统架构、数据流、生命周期、线程模型 |
| [PROJECT_STRUCTURE.md](doc/PROJECT_STRUCTURE.md) | 目录结构、关键文件、调用关系 |
| [API_REFERENCE.md](doc/API_REFERENCE.md) | HTTP、gRPC、WebSocket API 文档 |
| [AI_PIPELINE.md](doc/AI_PIPELINE.md) | LLM Prompt 流程、上下文管理、STT→LLM→TTS 全链路 |
| [AUDIO_PIPELINE.md](doc/AUDIO_PIPELINE.md) | 音频采集、Opus/PCM、VAD、编解码 |
| [ROBOT_COMMUNICATION.md](doc/ROBOT_COMMUNICATION.md) | 通信协议、认证、状态同步 |
| [CONFIGURATION_GUIDE.md](doc/CONFIGURATION_GUIDE.md) | 全部配置项、环境变量、文件格式 |
| [DEVELOPMENT_GUIDE.md](doc/DEVELOPMENT_GUIDE.md) | 本地开发、新增模块、插件开发 |
| [TROUBLESHOOTING.md](doc/TROUBLESHOOTING.md) | 常见错误诊断与修复 |

---

## 致谢与许可

- [Digital Dream Labs](https://github.com/digital-dream-labs) 开源了 Chipper
- [kercre123](https://github.com/kercre123) 创建并维护 wire-pod
- [fforchino](https://github.com/fforchino) 贡献本地化、多语言和 SDK 功能
- 所有提交 Issue 和 PR 的贡献者

本项目基于 Apache License 2.0 开源。详见 [LICENSE](LICENSE)。
