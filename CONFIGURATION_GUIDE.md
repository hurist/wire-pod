# Wire-Pod 配置指南

本文档详细说明 Wire-Pod 的所有配置项、配置文件格式、环境变量和配置优先级。

---

## 1. 配置体系概览

Wire-Pod 采用**四层配置体系**：

```
┌─────────────────────────────────────────────┐
│  Layer 1: 编译时选择                          │
│  cmd/<engine>/main.go — 选择 STT 引擎        │
├─────────────────────────────────────────────┤
│  Layer 2: 环境变量                            │
│  首次启动时生成 apiConfig.json                │
├─────────────────────────────────────────────┤
│  Layer 3: JSON 配置文件                       │
│  apiConfig.json — 持久化真相源                │
├─────────────────────────────────────────────┤
│  Layer 4: 运行时内存                          │
│  vars.APIConfig — 唯一运行时访问点            │
└─────────────────────────────────────────────┘
```

**优先级**：Layer 4（内存）> Layer 3（JSON）> Layer 2（环境变量）> Layer 1（编译时）

> **重要**：除 `STT_SERVICE` 外，运行时修改配置后**不需要重启** Wire-Pod。但 STT 引擎在编译时已确定，无法运行时切换。

---

## 2. 主配置文件：apiConfig.json

**路径**：`./apiConfig.json`（开发模式）或 `/data/apiConfig.json`（Docker 模式）

**生成时机**：
- 首次启动时，若文件不存在，从环境变量自动生成
- 后续所有修改通过 Web UI 或 API 更新

### 2.1 完整配置结构

```json
{
  "weather": {
    "enable": false,
    "provider": "",
    "key": "",
    "unit": "C"
  },
  "knowledge": {
    "enable": false,
    "provider": "",
    "key": "",
    "id": "",
    "model": "",
    "intentgraph": false,
    "robotName": "Vector",
    "openai_prompt": "",
    "openai_voice": "",
    "openai_voice_with_english": false,
    "save_chat": false,
    "commands_enable": false,
    "endpoint": "",
    "top_p": 1.0,
    "temp": 1.0
  },
  "STT": {
    "provider": "vosk",
    "language": "en-US"
  },
  "server": {
    "epconfig": true,
    "port": "443"
  },
  "hasreadfromenv": true,
  "pastinitialsetup": true
}
```

### 2.2 配置项详解

#### Weather（天气）

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enable` | bool | `false` | 是否启用天气查询 |
| `provider` | string | `""` | 提供商：`openweathermap` 或 `weatherapi` |
| `key` | string | `""` | API 密钥 |
| `unit` | string | `"C"` | 温度单位：`C`（摄氏）或 `F`（华氏） |

**触发条件**：当用户语音匹配 `intent_weather_extend` 时，Wire-Pod 调用天气 API 并回复。

---

#### Knowledge（知识图谱 / LLM）

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enable` | bool | `false` | 是否启用 LLM |
| `provider` | string | `""` | 提供商：`openai`、`together`、`custom`、`houndify` |
| `key` | string | `""` | API 密钥 |
| `id` | string | `""` | Houndify 专用 Client ID |
| `model` | string | `""` | 模型名，如 `gpt-4o-mini`、`meta-llama/Llama-3-70b-chat-hf` |
| `intentgraph` | bool | `false` | 是否将未匹配意图转发给 LLM |
| `robotName` | string | `"Vector"` | 机器人在对话中的名字 |
| `openai_prompt` | string | `""` | 自定义 System Prompt（覆盖默认） |
| `openai_voice` | string | `""` | OpenAI TTS 音色 |
| `openai_voice_with_english` | bool | `false` | 英语也使用 OpenAI TTS |
| `save_chat` | bool | `false` | 是否保存对话历史 |
| `commands_enable` | bool | `false` | 是否允许 LLM 控制动画/摄像头 |
| `endpoint` | string | `""` | Custom 提供商的 API 地址 |
| `top_p` | float | `1.0` | LLM 采样 top_p |
| `temp` | float | `1.0` | LLM 采样 temperature |

**Provider 专用说明**：

- **openai**：`key` 填 OpenAI API key，`model` 留空则自动使用 `gpt-4o-mini`
- **together**：`key` 填 Together API key，`model` 留空则使用 `meta-llama/Llama-3-70b-chat-hf`
- **custom**：`endpoint` 填完整 URL（如 `https://api.myservice.com/v1`），`key` 填对应 key
- **houndify**：`id` 和 `key` 分别填 Houndify 的 Client ID 和 Key（非 LLM，传统 KG）

---

#### STT（语音识别）

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `provider` | string | `"vosk"` | STT 引擎名称（与编译的入口对应） |
| `language` | string | `"en-US"` | 语言代码 |

**支持的语言代码**（Vosk/whisper.cpp）：
```
en-US, de-DE, fr-FR, es-ES, it-IT, pt-BR,
ja-JP, zh-CN, ru-RU, ko-KR, pl-PL, tr-TR,
nl-NL, sv-SE
```

> 注意：`language` 仅在 `provider` 为 `vosk` 或 `whisper.cpp` 时有效。其他引擎（Leopard、Houndify、Whisper API）固定使用英语或云端的自动语言检测。

---

#### Server（服务器）

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `epconfig` | bool | `true` | `true`=Escape Pod 模式，`false`=IP 模式 |
| `port` | string | `"443"` | gRPC 服务端口 |

**Escape Pod 模式 vs IP 模式**：

| 特性 | Escape Pod 模式 (`epconfig: true`) | IP 模式 (`epconfig: false`) |
|---|---|---|
| 主机名 | `escapepod.local` | Wire-Pod 服务器 IP |
| 证书 | 使用预置 `ep.crt`/`ep.key` | 动态生成，绑定到 IP |
| 适用机器人 | 生产版（需 BLE onboarding） | OSKR/开发版（SSH 部署） |
| mDNS | 需要 | 不需要 |

---

#### 系统标志

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `hasreadfromenv` | bool | `false` | 是否已从环境变量初始化过 |
| `pastinitialsetup` | bool | `false` | 是否已完成首次安装向导 |

---

## 3. 环境变量

### 3.1 全部环境变量

| 变量名 | 作用 | 默认值 | 适用场景 |
|---|---|---|---|
| `STT_SERVICE` | 指定编译的 STT 引擎 | `vosk` | 首次启动 |
| `STT_LANGUAGE` | 默认 STT 语言 | `en-US` | 首次启动 |
| `WEBSERVER_PORT` | Web UI 端口 | `8080` | 始终 |
| `DDL_RPC_PORT` | gRPC 端口 | `443` | 首次启动 |
| `OPENAI_KEY` | OpenAI API 密钥 | — | 首次启动 / Whisper STT |
| `PICOVOICE_APIKEY` | Picovoice API 密钥 | — | Leopard STT |
| `LEOPARD_APIKEY` | Leopard API 密钥（同上） | — | Leopard STT |
| `HOUNDIFY_STT_ID` | Houndify STT Client ID | — | Houndify STT |
| `HOUNDIFY_STT_KEY` | Houndify STT Client Key | — | Houndify STT |
| `VOSK_WITH_GRAMMER` | 启用 Vosk 语法优化 | `false` | Vosk STT |
| `PICOVOICE_INSTANCES` | Leopard 实例池大小 | `3` | Leopard STT |
| `WHISPER_MODEL` | whisper.cpp 模型大小 | `tiny` | whisper.cpp STT |
| `WEATHERAPI_ENABLED` | 启用天气 API | `false` | 首次启动 |
| `WEATHERAPI_PROVIDER` | 天气提供商 | — | 首次启动 |
| `WEATHERAPI_KEY` | 天气 API 密钥 | — | 首次启动 |
| `WEATHERAPI_UNIT` | 天气温度单位 | `C` | 首次启动 |
| `KNOWLEDGE_ENABLED` | 启用 LLM | `false` | 首次启动 |
| `KNOWLEDGE_PROVIDER` | LLM 提供商 | — | 首次启动 |
| `KNOWLEDGE_KEY` | LLM API 密钥 | — | 首次启动 |
| `KNOWLEDGE_ID` | Houndify KG Client ID | — | 首次启动 |
| `WIREPOD_DATA_DIR` | 数据目录 | `./` | Docker |
| `DEBUG_LOGGING` | 启用调试日志 | `false` | 始终 |
| `JDOCS_PINGER_ENABLED` | 启用 JDocs 保活 | `true` | 始终 |
| `DEBUG_PRINT_HIGHPASS` | 输出高通过滤耗时 | `false` | 调试 |

### 3.2 环境变量 vs JSON 配置的优先级

**首次启动**：
```
环境变量 → apiConfig.json（生成）
```

**后续运行**：
```
apiConfig.json → 内存（vars.APIConfig）
```

**例外**：`STT_SERVICE` 环境变量每次启动都会被检查，若与 `apiConfig.json` 中的不一致，会强制覆盖 JSON 中的值。这是为了允许通过 systemd/docker 环境变量强制指定 STT 引擎。

---

## 4. 其他配置文件

### 4.2 customIntents.json

**路径**：`./customIntents.json`

**用途**：用户自定义语音意图。

**结构**：
```json
[
  {
    "name": "开灯",
    "description": "打开客厅灯",
    "utterances": ["开灯", "打开灯"],
    "intent": "intent_system_lightson",
    "params": {
      "paramname": "room",
      "paramvalue": "living_room"
    },
    "exec": "/path/to/script.sh",
    "execargs": ["living_room"],
    "issystem": false,
    "luascript": "sayText('好的', true)"
  }
]
```

**字段说明**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 显示名称 |
| `description` | string | 是 | 描述 |
| `utterances` | []string | 是 | 触发语句列表 |
| `intent` | string | 是 | 对应的 Vector 内置意图名 |
| `params` | object | 否 | 意图参数（`paramname` + `paramvalue`） |
| `exec` | string | 否 | 外部程序路径 |
| `execargs` | []string | 否 | 外部程序参数 |
| `issystem` | bool | 否 | 是否为系统意图 |
| `luascript` | string | 否 | Lua 脚本代码 |

---

### 4.3 jdocs/jdocs.json

**路径**：`./jdocs/jdocs.json`

**用途**：所有机器人的 JDocs 存储。

**结构**：
```json
[
  {
    "thing": "vic:00e20145",
    "name": "vic.RobotSettings",
    "jdoc": {
      "doc_version": 5,
      "fmt_version": 1,
      "client_metadata": "",
      "json_doc": "{\"button_wakeword\":0,...}"
    }
  }
]
```

> **警告**：手动修改此文件可能导致版本号冲突。建议通过机器人的正常同步流程更新。

---

### 4.4 botSdkInfo.json

**路径**：`./jdocs/botSdkInfo.json`

**用途**：已认证机器人列表。

**结构**：
```json
{
  "global_guid": "xxxxxxxx",
  "robots": [
    {
      "esn": "00e20145",
      "ip_address": "192.168.1.150",
      "guid": "abc-def-123",
      "activated": true
    }
  ]
}
```

**字段说明**：

| 字段 | 说明 |
|---|---|
| `esn` | 机器人序列号 |
| `ip_address` | 机器人当前 IP（用于 SDK 直连） |
| `guid` | 认证 GUID（用于 SDK 连接时的 token） |
| `activated` | 是否已完成 onboarding |

---

### 4.5 server_config.json

**路径**：`../certs/server_config.json`（Wire-Pod 生成）

**用途**：机器人连接 Wire-Pod 所需的地址配置。

**Escape Pod 模式**：
```json
{
  "chipper": "escapepod.local:443",
  "jdocs": "escapepod.local:443",
  "token": "escapepod.local:443",
  "check": "http://escapepod.local/ok"
}
```

**IP 模式**：
```json
{
  "chipper": "192.168.1.100:443",
  "jdocs": "192.168.1.100:443",
  "token": "192.168.1.100:443",
  "check": "http://192.168.1.100/ok"
}
```

---

### 4.6 session-certs/

**路径**：`./session-certs/`

**用途**：每台已认证机器人的会话证书。

**文件命名**：`<ESN>`（如 `00e20145`）

**生成时机**：机器人首次读取 `vic.AppTokens` JDoc 时，Wire-Pod 自动生成。

---

## 5. 配置动态更新机制

### 5.1 Web UI 修改流程

```
用户操作 Web UI
    │
    ▼
浏览器 fetch('/api/set_kg_api', {method: 'POST', body: JSON.stringify(newConfig)})
    │
    ▼
config-ws/webserver.go: handleSetKGAPI()
    │
    ├── json.NewDecoder(r.Body).Decode(&vars.APIConfig.Knowledge)
    │
    └── vars.WriteConfigToDisk()
            │
            └── os.WriteFile(ApiConfigPath, json.Marshal(APIConfig), 0644)
    │
    └── 返回 "Changes successfully applied."
```

### 5.2 哪些配置立即生效？

| 配置项 | 生效时机 | 说明 |
|---|---|---|
| LLM provider/key/model | 下次 LLM 请求 | 每次 `StreamingKGSim()` 都读 `vars.APIConfig` |
| Weather key/provider | 下次天气意图 | `ParamChecker()` 时读取 |
| Save chat | 下次 LLM 对话 | `CreateAIReq()` 时检查 |
| Commands enable | 下次 LLM 对话 | `CreatePrompt()` 时检查 |
| STT language | 调用 `/api/set_stt_info` 后 | 触发 `ReloadVosk()` 重新加载模型 |
| Custom intents | 调用 add/edit/remove 后 | 立即更新内存和 JSON |
| Server port | 重启后 | 需重新执行 `StartChipper()` |
| EPConfig | 调用 `/api-chipper/use_ep` 或 `use_ip` | 重启 gRPC 服务 |

### 5.3 配置热重载的局限

**当前未实现的特性**：
- STT 引擎切换（需要重新编译）
- 证书热更新（需要重启 gRPC）
- JDocs 结构变更（需要重启）

---

## 6. 配置备份与恢复

### 6.1 需要备份的文件

```bash
# 核心配置
apiConfig.json
customIntents.json

# 机器人数据
jdocs/jdocs.json
jdocs/botSdkInfo.json
session-certs/

# 证书（若不想重新生成）
../certs/cert.crt
../certs/cert.key
../certs/server_config.json

# 语音模型（体积大，可重新下载）
../vosk/models/
../whisper.cpp/models/
```

### 6.2 Docker 备份

```bash
# 备份数据卷
docker run --rm -v wire-pod-data:/data -v $(pwd):/backup alpine \
    tar czf /backup/wire-pod-backup.tar.gz -C /data .

# 恢复
docker run --rm -v wire-pod-data:/data -v $(pwd):/backup alpine \
    tar xzf /backup/wire-pod-backup.tar.gz -C /data
```

---

## 7. 配置安全注意事项

### 7.1 API 密钥存储

- API 密钥（OpenAI、Weather 等）以**明文**保存在 `apiConfig.json` 中
- 文件权限为 `0644`，即系统上所有用户可读
- **建议**：在生产环境中，将 `apiConfig.json` 权限设为 `0600`：
  ```bash
  chmod 600 apiConfig.json
  ```

### 7.2 证书安全

- `cert.key` 是服务器私钥，应严格保护
- `session-certs/` 中的证书包含机器人身份信息
- 若服务器被入侵，攻击者可冒充 Wire-Pod 拦截机器人通信

### 7.3 机器人 Token

- `botSdkInfo.json` 中的 `guid` 相当于机器人的 SDK 访问令牌
- 获取此文件即可远程控制对应机器人
- **切勿将 `botSdkInfo.json` 提交到 Git 或公开分享**

---

*本文档基于 `pkg/vars/config.go`、`pkg/vars/vars.go` 和 `pkg/wirepod/config-ws/webserver.go` 的实际代码编写。*
