# Wire-Pod 全量配置参考

本文档梳理当前程序的配置入口、环境变量、Docker 设置、启动脚本参数、持久化文件、Web/API 可配置项和端口用途。路径以仓库根目录为基准；原生运行时通常在 `chipper/` 目录启动，Docker 运行时会把关键数据持久化到 `/data`。

## 配置优先级

Wire-Pod 的配置分为四层：

| 层级 | 配置位置 | 生效时机 | 作用 |
| --- | --- | --- | --- |
| Docker Compose / Docker 环境 | `compose.yaml` 的 `environment`，或 `docker run -e` | 容器启动 | 设置 `WIREPOD_*` 覆盖项和 `/data` 数据目录 |
| Shell 环境 | `chipper/source.sh`，或启动前导出的环境变量 | 程序启动 | 决定 STT 编译入口、日志、云服务密钥、TTS 默认值等 |
| 主 JSON 配置 | `chipper/apiConfig.json` | 程序启动后读取，Web UI 修改后写回 | 保存天气、LLM、STT 语言、TTS、服务器模式等运行配置 |
| 机器人/JDocs 文件 | `chipper/jdocs/`、`certs/`、`session-certs/` 等 | 机器人连接和设置同步时 | 保存机器人设置、会话证书、连接地址和自定义意图 |

关键规则：

| 规则 | 说明 |
| --- | --- |
| 首次生成 `apiConfig.json` | `vars.CreateConfigFromEnv()` 会从环境变量生成初始配置 |
| 后续运行 | `apiConfig.json` 是主要持久化配置源 |
| `STT_SERVICE` 例外 | 每次启动都会用环境变量覆盖 `apiConfig.json` 里的 `STT.provider` |
| TTS 默认值 | `tts` 字段缺失时会自动补齐，默认 `provider=tencent`、`voiceType=601009` |
| Docker 覆盖 | `docker/entrypoint.sh` 会把 `WIREPOD_*` 写入 `/data/chipper/source.sh` 后再启动程序 |
| Web UI 修改 | `/api/set_*` 接口会写回 `apiConfig.json`，通常不需要重启 |

## Docker 配置

### `compose.yaml`

| 配置项 | 当前值 | 作用 |
| --- | --- | --- |
| `services.wire-pod.container_name` | `wire-pod` | 容器名称 |
| `services.wire-pod.hostname` | `escapepod` | 容器主机名，配合 Escape Pod 模式使用 |
| `build.context` | `.` | Docker 构建上下文 |
| `build.dockerfile` | `dockerfile` | 使用仓库内 Dockerfile |
| `image` | `ghcr.io/kercre123/wire-pod:main` | 镜像名 |
| `restart` | `unless-stopped` | 容器异常退出后自动重启 |
| `environment.WIREPOD_DATA_DIR` | `/data` | 容器持久化数据目录 |
| `ports` | `80:80` | connCheck、SDK App、摄像头流 |
| `ports` | `443:443` | Chipper/JDocs/TMS/Token 等 TLS 服务 |
| `ports` | `8080:8080` | Web 配置 UI |
| `ports` | `8084:8084` | Escape Pod 2.0.1 兼容 TLS 端口 |
| `volumes.wire-pod-data` | `/data` | 持久化配置、证书、模型、JDocs |
| `volumes.wire-pod-images` | `/images` | 图片/媒体类持久化目录 |
| `healthcheck` | `curl http://localhost:8080/ok` | 检查 Web 服务是否可用 |

### `dockerfile`

| 类型 | 名称 | 默认值 | 作用 |
| --- | --- | --- | --- |
| Build ARG | `TARGETOS` | 构建平台传入 | Go 目标系统 |
| Build ARG | `TARGETARCH` | 构建平台传入 | Go 目标架构 |
| Build ARG | `TARGETVARIANT` | 构建平台传入 | ARM 变体，如 `v7` |
| Build ARG | `VOSK_VERSION` | `0.3.45` | 下载 Vosk native runtime 的版本 |
| Build ARG | `COMMIT_SHA` | `unknown` | 注入二进制版本信息和 OCI label |
| Builder ENV | `DEBIAN_FRONTEND` | `noninteractive` | apt 非交互安装 |
| Builder ENV | `CGO_ENABLED` | `1` | Vosk/音频依赖需要 CGO |
| Runtime ENV | `WIREPOD_DATA_DIR` | `/data` | Docker 持久化根目录 |
| Runtime ENV | `LD_LIBRARY_PATH` | `/opt/vosk/libvosk` | 运行时加载 Vosk native library |
| `VOLUME` | `/data` | - | Docker 数据卷挂载点 |
| `EXPOSE` | `80 443 8080 8084` | - | 容器声明端口 |
| `ENTRYPOINT` | `/opt/wire-pod/docker/entrypoint.sh` | - | 初始化持久化链接和环境覆盖 |
| `CMD` | `/opt/wire-pod/chipper/start.sh` | - | 启动 Wire-Pod |

Docker 镜像当前固定编译 `cmd/vosk`，即容器内默认 STT 二进制是 Vosk。若要换成 Tencent/Whisper/Coqui 这类不同 STT 编译入口，需要调整构建流程或重新构建对应入口。

### Docker 持久化映射

`docker/entrypoint.sh` 会把容器内文件链接到 `/data`：

| 容器内路径 | 持久化路径 | 作用 |
| --- | --- | --- |
| `/opt/wire-pod/certs` | `/data/certs` | 服务器证书和 `server_config.json` |
| `/opt/wire-pod/stt` | `/data/stt` | Coqui 模型/资源 |
| `/opt/wire-pod/vosk` | `/data/vosk` | Vosk 模型 |
| `/opt/wire-pod/whisper.cpp` | `/data/whisper.cpp` | whisper.cpp 模型/构建资源 |
| `/opt/wire-pod/vector-cloud/build` | `/data/vector-cloud/build` | Vector cloud 构建产物 |
| `/opt/wire-pod/chipper/jdocs` | `/data/chipper/jdocs` | JDocs 和机器人 SDK 信息 |
| `/opt/wire-pod/chipper/plugins` | `/data/chipper/plugins` | 插件目录 |
| `/opt/wire-pod/chipper/session-certs` | `/data/chipper/session-certs` | 机器人会话证书 |
| `/opt/wire-pod/chipper/apiConfig.json` | `/data/chipper/apiConfig.json` | 主配置 |
| `/opt/wire-pod/chipper/botConfig.json` | `/data/chipper/botConfig.json` | 机器人配置 |
| `/opt/wire-pod/chipper/customIntents.json` | `/data/chipper/customIntents.json` | 自定义语音意图 |
| `/opt/wire-pod/chipper/pico.key` | `/data/chipper/pico.key` | Picovoice key |
| `/opt/wire-pod/chipper/useepod` | `/data/chipper/useepod` | Escape Pod 模式标记 |
| `/opt/wire-pod/chipper/source.sh` | `/data/chipper/source.sh` | Shell 环境配置 |

首次启动时，`source.sh` 会从 `docker/default-source.sh` 复制默认值。

## Docker 环境变量

Docker 使用 `WIREPOD_*` 变量作为容器入口覆盖项。入口脚本会把这些变量写入持久化的 `source.sh`。

| Docker 变量 | 写入的运行变量 | 默认值 | 作用 |
| --- | --- | --- | --- |
| `WIREPOD_DATA_DIR` | 不写入 | `/data` | Docker 持久化根目录 |
| `WIREPOD_DEBUG_LOGGING` | `DEBUG_LOGGING` | `true` | 是否输出调试日志 |
| `WIREPOD_STT_SERVICE` | `STT_SERVICE` | `vosk` | STT 引擎名称 |
| `WIREPOD_STT_LANGUAGE` | `STT_LANGUAGE` | `en-US` | Vosk/whisper.cpp 语言 |
| `WIREPOD_USE_INBUILT_BLE` | `USE_INBUILT_BLE` | `false` | 启用内置 BLE 构建标签 |
| `WIREPOD_PICOVOICE_APIKEY` | `PICOVOICE_APIKEY` | - | Picovoice/Leopard STT key，同时写入 `pico.key` |
| `WIREPOD_TENCENTCLOUD_SECRET_ID` | `TENCENTCLOUD_SECRET_ID` | - | 腾讯云 SecretId，ASR/TTS 共用 |
| `WIREPOD_TENCENTCLOUD_SECRET_KEY` | `TENCENTCLOUD_SECRET_KEY` | - | 腾讯云 SecretKey，ASR/TTS 共用 |
| `WIREPOD_TENCENT_ASR_REGION` | `TENCENT_ASR_REGION` | `ap-guangzhou` | 腾讯云 ASR 地域 |
| `WIREPOD_TENCENT_ASR_ENGINE_MODEL_TYPE` | `TENCENT_ASR_ENGINE_MODEL_TYPE` | `16k_zh` | 腾讯云一句话识别模型 |
| `WIREPOD_TENCENT_ASR_VOICE_FORMAT` | `TENCENT_ASR_VOICE_FORMAT` | `pcm` | 腾讯云 ASR 上传格式 |
| `WIREPOD_TENCENT_ASR_FILTER_DIRTY` | `TENCENT_ASR_FILTER_DIRTY` | 腾讯云默认 | 脏词过滤 |
| `WIREPOD_TENCENT_ASR_FILTER_MODAL` | `TENCENT_ASR_FILTER_MODAL` | 腾讯云默认 | 语气词过滤 |
| `WIREPOD_TENCENT_ASR_FILTER_PUNC` | `TENCENT_ASR_FILTER_PUNC` | 腾讯云默认 | 标点过滤 |
| `WIREPOD_TENCENT_ASR_CONVERT_NUM_MODE` | `TENCENT_ASR_CONVERT_NUM_MODE` | 腾讯云默认 | 数字转换 |
| `WIREPOD_TENCENT_ASR_TIMEOUT_SECONDS` | `TENCENT_ASR_TIMEOUT_SECONDS` | `15` | ASR 请求超时 |
| `WIREPOD_TTS_PROVIDER` | `TTS_PROVIDER` | `tencent` | TTS 提供商 |
| `WIREPOD_TENCENT_TTS_REGION` | `TENCENT_TTS_REGION` | `ap-guangzhou` | 腾讯云 TTS 地域 |
| `WIREPOD_TENCENT_TTS_VOICE_TYPE` | `TENCENT_TTS_VOICE_TYPE` | `601009` | 腾讯云 TTS 音色 |
| `WIREPOD_TENCENT_TTS_SAMPLE_RATE` | `TENCENT_TTS_SAMPLE_RATE` | `16000` | TTS 返回采样率，Vector 播放要求 16kHz |
| `WIREPOD_TENCENT_TTS_CODEC` | `TENCENT_TTS_CODEC` | `pcm` | TTS 返回格式，Vector 播放要求 PCM |
| `WIREPOD_TENCENT_TTS_SPEED` | `TENCENT_TTS_SPEED` | `0` | 腾讯云 TTS 语速 |
| `WIREPOD_TENCENT_TTS_VOLUME` | `TENCENT_TTS_VOLUME` | `0` | 腾讯云 TTS 音量 |
| `WIREPOD_TENCENT_TTS_TIMEOUT_SECONDS` | `TENCENT_TTS_TIMEOUT_SECONDS` | `15` | TTS 请求超时 |

## 原生启动脚本

### `setup.sh`

`setup.sh` 用于首次安装依赖、选择 STT、生成 `source.sh`、下载模型并可选创建 systemd 服务。

| 启动方式 | 作用 |
| --- | --- |
| `sudo ./setup.sh` | 默认安装依赖并进入 STT 选择流程 |
| `sudo ./setup.sh --bypass-target-check` | 目标系统不在 apt/pacman/dnf/macOS 检测范围内时继续 |
| `sudo ./setup.sh -f` | 跳过 x86_64 AVX 检查 |
| `sudo ./setup.sh scp <botAddress> <keyPath>` | 通过 SSH/SCP 把 `server_config.json` 和证书部署到机器人 |
| `sudo ./setup.sh -f scp <botAddress> <keyPath>` | SCP 时使用兼容老 OpenSSH 的 `-O` 参数 |
| `sudo ./setup.sh daemon-enable` | 安装依赖、根据 `source.sh` 构建二进制并创建 `wire-pod.service` |
| `sudo ./setup.sh daemon-disable` | 停止并删除 `wire-pod.service` |

首次生成的 `chipper/source.sh` 默认包含：

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `DEBUG_LOGGING` | `true` | 输出调试日志 |
| `TTS_PROVIDER` | `tencent` | 默认 TTS 使用腾讯云 |
| `TENCENT_TTS_REGION` | `ap-guangzhou` | 腾讯云 TTS 地域 |
| `TENCENT_TTS_VOICE_TYPE` | `601009` | 腾讯云 TTS 音色 |
| `TENCENT_TTS_SAMPLE_RATE` | `16000` | TTS 采样率 |
| `TENCENT_TTS_CODEC` | `pcm` | TTS 音频编码 |
| `TENCENT_TTS_SPEED` | `0` | TTS 语速 |
| `TENCENT_TTS_VOLUME` | `0` | TTS 音量 |
| `TENCENT_TTS_TIMEOUT_SECONDS` | `15` | TTS 超时 |

STT 选择会追加：

| STT 选择 | 写入变量 | 资源/说明 |
| --- | --- | --- |
| Coqui | `STT_SERVICE=coqui` | 下载 Coqui native client 和模型 |
| Picovoice Leopard | `STT_SERVICE=leopard`、`PICOVOICE_APIKEY` | 写入 `pico.key` |
| Vosk | `STT_SERVICE=vosk` | 下载 Vosk native runtime 和模型 |
| Whisper | `STT_SERVICE=whisper.cpp`、`WHISPER_MODEL` | 下载/构建 `whisper.cpp` |
| Tencent Cloud ASR | `STT_SERVICE=tencent`、腾讯云 ASR 凭证和参数 | 腾讯云凭证同时可供 Tencent TTS 使用 |

### `chipper/start.sh`

`start.sh` 必须以 root 运行，并且要求当前目录或子目录存在 `chipper/source.sh`。

| 配置/行为 | 说明 |
| --- | --- |
| `source source.sh` | 加载运行环境变量 |
| `GOTAGS=nolibopusfile` | 默认 Go build tag |
| `USE_INBUILT_BLE=true` | 追加 `inbuiltble` build tag |
| `GOLDFLAGS` | 注入 Git commit SHA |
| `STT_SERVICE=leopard` | 运行/构建 `cmd/leopard/main.go` |
| `STT_SERVICE=rhino` | 运行/构建 `cmd/experimental/rhino/main.go` |
| `STT_SERVICE=houndify` | 运行/构建 `cmd/experimental/houndify/main.go` |
| `STT_SERVICE=whisper` | 运行/构建 `cmd/experimental/whisper/main.go` |
| `STT_SERVICE=tencent` | 运行/构建 `cmd/tencent/main.go` |
| `STT_SERVICE=whisper.cpp` | 设置 whisper.cpp CGO/库路径并运行/构建 |
| `STT_SERVICE=vosk` | 设置 Vosk CGO/库路径并运行/构建 |
| 其他值 | 走 Coqui 路径 |

### `update.sh`

`update.sh` 会安装依赖、执行 `git fetch --all` 和 `git reset --hard origin/main`，然后根据 `source.sh` 中的 `STT_SERVICE` 重新构建并重启 systemd 服务。这个脚本会覆盖本地未提交代码，开发环境慎用。

支持自动重建的 STT：`leopard`、`vosk`、`coqui`、`tencent`。其他 STT 会提示需要手动构建。

## 运行环境变量

这些变量可直接放入 `chipper/source.sh`，也可在启动前导出。Docker 推荐使用对应的 `WIREPOD_*`。

### 基础与日志

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `WEBSERVER_PORT` | `8080` | Web UI 监听端口 |
| `DEBUG_LOGGING` | `false`，Docker/安装脚本默认 `true` | 开启详细日志 |
| `DEBUG_PRINT_HIGHPASS` | `false` | 输出音频高通过滤耗时 |
| `DEBUG_PRINT_PROMPT` | `false` | 输出 LLM prompt |
| `DISABLE_MDNS` | `false` | 设为 `true` 禁用 Escape Pod 模式的 mDNS 发布 |
| `PRINT_MDNS` | `false` | 输出 mDNS 调试日志 |
| `NO8084` | `false` | 设为 `true` 禁用 8084 兼容监听 |
| `JDOCS_PINGER_ENABLED` | `true` | 设为 `false` 禁用 JDocs 保活/同步 pinger |
| `USE_INBUILT_BLE` | `false` | 使用内置 BLE build tag |
| `DDL_RPC_PORT` | - | 旧版配置兼容读取；不是当前推荐配置入口 |

### STT

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `STT_SERVICE` | 安装脚本/Docker 默认 `vosk` | 选择 STT 引擎，并决定启动/构建入口 |
| `STT_LANGUAGE` | Docker 默认 `en-US` | Vosk 和 whisper.cpp 的语言 |
| `OPENAI_KEY` | - | Whisper API STT 的 OpenAI key |
| `VOSK_WITH_GRAMMER` | `false` | Vosk 使用语法优化 recognizer；变量名沿用原拼写 |
| `PICOVOICE_APIKEY` | - | Picovoice Leopard key |
| `LEOPARD_APIKEY` | - | Leopard 旧变量名，`PICOVOICE_APIKEY` 未设置时使用 |
| `PICOVOICE_INSTANCES` | `3` | Leopard 实例池大小 |
| `HOUNDIFY_STT_ID` | - | Houndify STT Client ID |
| `HOUNDIFY_STT_KEY` | - | Houndify STT Client Key |
| `WHISPER_MODEL` | `tiny` | whisper.cpp 模型名称 |

### Tencent Cloud ASR

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `TENCENTCLOUD_SECRET_ID` | - | 腾讯云 SecretId，ASR/TTS 共用 |
| `TENCENTCLOUD_SECRET_KEY` | - | 腾讯云 SecretKey，ASR/TTS 共用 |
| `TENCENT_ASR_REGION` | `ap-guangzhou` | ASR 地域 |
| `TENCENT_ASR_ENGINE_MODEL_TYPE` | `16k_zh` | 一句话识别模型，例如中文 16k |
| `TENCENT_ASR_VOICE_FORMAT` | `pcm` | 上传音频格式；当前链路默认 16kHz/16bit/mono PCM |
| `TENCENT_ASR_TIMEOUT_SECONDS` | `15` | ASR 请求超时时间 |
| `TENCENT_ASR_FILTER_DIRTY` | 腾讯云默认 | 脏词过滤 |
| `TENCENT_ASR_FILTER_MODAL` | 腾讯云默认 | 语气词过滤 |
| `TENCENT_ASR_FILTER_PUNC` | 腾讯云默认 | 标点过滤 |
| `TENCENT_ASR_CONVERT_NUM_MODE` | 腾讯云默认 | 数字转换 |

### TTS

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `TTS_PROVIDER` | `tencent` | TTS 提供商：`tencent`、`openai`、`vector` |
| `TENCENT_TTS_REGION` | `ap-guangzhou` | Tencent TTS 地域 |
| `TENCENT_TTS_VOICE_TYPE` | `601009` | Tencent TTS 音色 ID |
| `TENCENT_TTS_SAMPLE_RATE` | `16000` | Tencent TTS 采样率；Vector 外放链路要求 16kHz |
| `TENCENT_TTS_CODEC` | `pcm` | Tencent TTS 返回编码；Vector 外放链路要求 PCM |
| `TENCENT_TTS_SPEED` | `0` | Tencent TTS 语速 |
| `TENCENT_TTS_VOLUME` | `0` | Tencent TTS 音量 |
| `TENCENT_TTS_TIMEOUT_SECONDS` | `15` | Tencent TTS 请求超时 |

当前默认 TTS 是 Tencent Cloud TTS。它请求腾讯云返回 16kHz PCM 后，通过 Vector `ExternalAudioStreamPlayback` 按 1024 字节左右的块播放。若腾讯云凭证缺失或请求失败，运行时会记录错误并回退到 Vector 内置 `SayText`。

### 天气和知识图谱/LLM

这些环境变量只在首次创建 `apiConfig.json` 时读取。之后以 Web UI 或直接编辑 `apiConfig.json` 为准。

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `WEATHERAPI_ENABLED` | `false` | 首次启动时启用天气 API |
| `WEATHERAPI_PROVIDER` | - | 天气提供商 |
| `WEATHERAPI_KEY` | - | 天气 API key |
| `WEATHERAPI_UNIT` | - | 温度单位 |
| `KNOWLEDGE_ENABLED` | `false` | 首次启动时启用 LLM/知识图谱 |
| `KNOWLEDGE_PROVIDER` | - | LLM 提供商，如 `openai`、`together`、`custom`、`houndify` |
| `KNOWLEDGE_KEY` | - | LLM API key；OpenAI TTS 也复用该字段 |
| `KNOWLEDGE_ID` | - | Houndify KG Client ID |

## `apiConfig.json`

主配置文件定义在 `chipper/pkg/vars/config.go`，开发/原生路径为 `chipper/apiConfig.json`，Docker 路径为 `/data/chipper/apiConfig.json`。

示例结构：

```json
{
  "weather": {
    "enable": false,
    "provider": "",
    "key": "",
    "unit": ""
  },
  "knowledge": {
    "enable": false,
    "provider": "",
    "key": "",
    "id": "",
    "model": "",
    "intentgraph": false,
    "robotName": "",
    "openai_prompt": "",
    "openai_voice": "",
    "openai_voice_with_english": false,
    "save_chat": false,
    "commands_enable": false,
    "endpoint": "",
    "top_p": 0,
    "temp": 0
  },
  "STT": {
    "provider": "vosk",
    "language": "en-US"
  },
  "tts": {
    "provider": "tencent",
    "tencent_region": "ap-guangzhou",
    "tencent_voice_type": 601009,
    "tencent_sample_rate": 16000,
    "tencent_codec": "pcm",
    "tencent_speed": 0,
    "tencent_volume": 0,
    "tencent_timeout_seconds": 15
  },
  "server": {
    "epconfig": false,
    "port": "443"
  },
  "hasreadfromenv": true,
  "pastinitialsetup": true
}
```

### `weather`

| 字段 | 作用 |
| --- | --- |
| `enable` | 是否启用天气能力 |
| `provider` | 天气服务商 |
| `key` | 天气 API key |
| `unit` | 温度单位 |

### `knowledge`

| 字段 | 作用 |
| --- | --- |
| `enable` | 是否启用 LLM/知识图谱回答 |
| `provider` | 提供商，如 OpenAI、Together、custom、Houndify |
| `key` | API key；OpenAI TTS 当前也使用这里的 key |
| `id` | Houndify Client ID |
| `model` | 模型名称 |
| `intentgraph` | 是否启用 IntentGraph 模式 |
| `robotName` | 注入 prompt 的机器人名字 |
| `openai_prompt` | OpenAI 自定义系统提示 |
| `openai_voice` | OpenAI TTS 音色 |
| `openai_voice_with_english` | 含英文时是否使用 OpenAI voice |
| `save_chat` | 是否保存对话上下文 |
| `commands_enable` | 是否允许 LLM 命令能力 |
| `endpoint` | 自定义兼容 API endpoint |
| `top_p` | LLM top-p 参数 |
| `temp` | LLM temperature 参数 |

### `STT`

| 字段 | 作用 |
| --- | --- |
| `provider` | STT 提供商；启动时会被 `STT_SERVICE` 覆盖 |
| `language` | Vosk/whisper.cpp 语言 |

### `tts`

| 字段 | 默认值 | 作用 |
| --- | --- | --- |
| `provider` | `tencent` | `tencent`、`openai`、`vector` |
| `tencent_region` | `ap-guangzhou` | Tencent TTS 地域 |
| `tencent_voice_type` | `601009` | Tencent TTS 音色 |
| `tencent_sample_rate` | `16000` | 播放链路采样率 |
| `tencent_codec` | `pcm` | 返回音频编码 |
| `tencent_speed` | `0` | 语速 |
| `tencent_volume` | `0` | 音量 |
| `tencent_timeout_seconds` | `15` | 请求超时 |

### `server`

| 字段 | 作用 |
| --- | --- |
| `epconfig` | `true` 表示 Escape Pod 域名模式；`false` 表示 IP 模式 |
| `port` | Chipper TLS 服务端口，通常 `443` |

### 状态字段

| 字段 | 作用 |
| --- | --- |
| `hasreadfromenv` | 标记是否已从环境变量初始化过 |
| `pastinitialsetup` | 是否完成初始设置；未完成时只启动 Web UI 引导 |

## 其他持久化配置文件

| 文件 | Docker 路径 | 作用 |
| --- | --- | --- |
| `chipper/source.sh` | `/data/chipper/source.sh` | 原生/Docker 启动环境变量 |
| `chipper/customIntents.json` | `/data/chipper/customIntents.json` | 自定义语音意图 |
| `chipper/botConfig.json` | `/data/chipper/botConfig.json` | 机器人配置存储 |
| `chipper/jdocs/jdocs.json` | `/data/chipper/jdocs/jdocs.json` | JDocs，保存机器人设置和 App Tokens |
| `chipper/jdocs/botSdkInfo.json` | `/data/chipper/jdocs/botSdkInfo.json` | 已认证机器人列表、IP、GUID |
| `chipper/session-certs/` | `/data/chipper/session-certs/` | 每台机器人会话证书 |
| `certs/cert.crt` | `/data/certs/cert.crt` | IP 模式服务器证书 |
| `certs/cert.key` | `/data/certs/cert.key` | IP 模式服务器私钥 |
| `certs/server_config.json` | `/data/certs/server_config.json` | 下发给机器人的服务地址配置 |
| `chipper/useepod` | `/data/chipper/useepod` | Escape Pod 模式标记文件 |
| `chipper/pico.key` | `/data/chipper/pico.key` | Picovoice key 备份 |
| `chipper/version` | 镜像内或打包路径 | 安装版本号 |

Packaged 模式下，多数文件会迁移到 `os.UserConfigDir()/wire-pod`；Android/iOS 会使用 `AndroidPath` 下的静态和配置目录。

## Web/API 配置端点

Web UI 默认监听 `WEBSERVER_PORT`，默认 `8080`。以下端点由 `chipper/pkg/wirepod/config-ws/webserver.go` 和 `chipper/pkg/initwirepod/web.go` 提供。

### 主配置接口

| 端点 | 作用 | 写入位置 |
| --- | --- | --- |
| `GET /api/get_config` | 返回完整 `apiConfig.json` | 只读 |
| `POST /api/set_weather_api` | 设置天气 provider/key | `apiConfig.json.weather` |
| `GET /api/get_weather_api` | 获取天气配置 | 只读 |
| `POST /api/set_kg_api` | 设置 LLM/知识图谱配置 | `apiConfig.json.knowledge` |
| `GET /api/get_kg_api` | 获取 LLM/知识图谱配置 | 只读 |
| `POST /api/set_tts_api` | 设置 TTS provider 和 Tencent 参数 | `apiConfig.json.tts` |
| `GET /api/get_tts_api` | 获取 TTS 配置 | 只读 |
| `POST /api/set_stt_info` | 设置 Vosk/whisper.cpp 语言，必要时下载 Vosk 模型 | `apiConfig.json.STT.language` |
| `GET /api/get_stt_info` | 获取 STT 配置 | 只读 |
| `GET /api/get_download_status` | 获取模型下载状态 | 只读 |
| `GET /api/get_logs` | 获取日志 | 只读 |
| `GET /api/get_debug_logs` | 获取调试日志 | 只读 |
| `GET /api/is_running` | Web 服务存活检查 | 只读 |
| `POST /api/delete_chats` | 清空内存对话上下文 | 内存 |
| `GET /api/get_version_info` | 查询当前/远端版本 | 只读，依赖 GitHub 网络 |
| `POST /api/generate_certs` | 生成本地证书组合 | `certs/` |
| `GET /api/get_ota/<filename>` | 代理下载 OTA 包 | 只读/网络 |
| `GET /api/is_api_v3` | API 版本探测 | 只读 |

### 自定义意图接口

| 端点 | 作用 | 写入位置 |
| --- | --- | --- |
| `POST /api/add_custom_intent` | 新增自定义意图 | `customIntents.json` |
| `POST /api/edit_custom_intent` | 修改自定义意图 | `customIntents.json` |
| `POST /api/remove_custom_intent` | 删除自定义意图 | `customIntents.json` |
| `GET /api/get_custom_intents_json` | 返回自定义意图 JSON | 只读 |

自定义意图字段包括 `name`、`description`、`utterances`、`intent`、`params`、`exec`、`execargs`、`issystem`、`luascript`。

### 服务器模式接口

| 端点 | 作用 | 写入位置 |
| --- | --- | --- |
| `GET/POST /api-chipper/use_ep` | 切换 Escape Pod 模式，端口设为 `443` | `apiConfig.json.server`、`certs/server_config.json` |
| `GET/POST /api-chipper/use_ip?port=<port>` | 切换 IP 模式并生成证书 | `apiConfig.json.server`、`certs/` |
| `GET/POST /api-chipper/restart` | 重启 Chipper TLS 服务 | 内存 |

### 证书接口

| 端点 | 作用 |
| --- | --- |
| `GET /session-certs/<ESN>` | 下载对应机器人 ESN 的会话证书 |

## 机器人设置与 SDK App 接口

SDK App/connCheck 服务监听 80 端口。多数 `/api-sdk/*` 接口需要 `serial=<ESN>` 参数，用于定位已认证机器人。

| 类别 | 端点/设置 | 作用 |
| --- | --- | --- |
| 连接 | `/api-sdk/conn_test` | 检查 SDK 连接 |
| Alexa | `/api-sdk/alexa_sign_in`、`/api-sdk/alexa_sign_out` | 切换 Alexa opt-in |
| 语音/行为 | `/api-sdk/say_text` | 让机器人说话，走当前 TTS provider |
| 机器人设置 | `/api-sdk/eye_color`、`custom_eye_color`、`volume`、`locale`、`location`、`timezone` | 写入机器人设置/JDocs |
| 时间温度 | `/api-sdk/time_format_12`、`time_format_24`、摄氏/华氏相关接口 | 写入机器人设置 |
| 状态查询 | `/api-sdk/get_sdk_info`、`get_sdk_settings`、`get_battery` | 查询机器人和设置状态 |
| 动作控制 | 轮子、lift、head、behavior control 等接口 | 直接通过 Vector SDK 控制机器人 |
| 媒体 | `/api-sdk/play_sound`、`/cam-stream` | 播放 PCM 或摄像头流 |

这些接口的机器人侧配置通常最终同步到 `jdocs/jdocs.json`，机器人认证信息保存在 `jdocs/botSdkInfo.json` 和 `session-certs/`。

## BLE 和 SSH 设置接口

这些接口用于初次配网、OTA、部署 `server_config.json` 和证书，不是长期运行配置。

| 前缀 | 典型操作 | 作用 |
| --- | --- | --- |
| `/api-ble/` | `init`、`scan`、`connect`、`send_pin`、`scan_wifi`、`connect_wifi`、`start_ota`、`stop_ota`、`do_auth`、`onboard`、`disconnect` | 通过 BLE 配置机器人、Wi-Fi、OTA、认证 |
| `/api-ssh/` | `setup`、`get_setup_status` | 通过 SSH/SCP 部署 `server_config.json` 和证书 |

## 端口

| 端口 | 服务 | 配置来源 | 说明 |
| --- | --- | --- | --- |
| `80` | SDK App、connCheck、`/cam-stream` | 固定 | 机器人连接检查和 Web SDK 辅助功能 |
| `443` | Chipper/JDocs/TMS/Token TLS 服务 | `apiConfig.json.server.port` 默认常用值 | 机器人主要云服务入口 |
| `8080` | Web 配置 UI | `WEBSERVER_PORT` | Docker Compose 默认映射 |
| `8084` | Escape Pod 2.0.1 兼容 TLS 服务 | `NO8084` 可关闭 | 仅 `server.epconfig=true` 时启用 |

## 机器人连接配置

`certs/server_config.json` 是部署到机器人端的配置文件，告诉机器人连接哪个 Wire-Pod 地址。

Escape Pod 模式通常生成：

```json
{
  "jdocs": "escapepod.local:443",
  "tms": "escapepod.local:443",
  "chipper": "escapepod.local:443",
  "check": "escapepod.local/ok:80",
  "logfiles": "s3://anki-device-logs-prod/victor",
  "appkey": "oDoa0quieSeir6goowai7f"
}
```

IP 模式会使用当前 Wire-Pod 主机 IP 和指定端口，例如：

```json
{
  "jdocs": "192.168.1.100:443",
  "tms": "192.168.1.100:443",
  "chipper": "192.168.1.100:443",
  "check": "192.168.1.100/ok:80",
  "logfiles": "s3://anki-device-logs-prod/victor",
  "appkey": "oDoa0quieSeir6goowai7f"
}
```

相关证书：

| 文件 | 作用 |
| --- | --- |
| `certs/cert.crt` | Wire-Pod 服务器证书，需让机器人信任 |
| `certs/cert.key` | Wire-Pod 服务器私钥 |
| `chipper/epod/ep.crt`、`ep.key` | Escape Pod 模式内置证书 |
| 机器人 `/data/data/server_config.json` | Wire-Pod 定制固件常用部署位置 |
| 机器人 `/anki/data/assets/cozmo_resources/config/server_config.json` | 生产固件常用部署位置 |
| 机器人 `/data/data/wirepod-cert.crt`、`/anki/etc/wirepod-cert.crt` | 机器人信任的 Wire-Pod 证书 |

## 常用配置示例

### Docker 使用 Tencent ASR + Tencent TTS

```yaml
services:
  wire-pod:
    environment:
      WIREPOD_DATA_DIR: /data
      WIREPOD_STT_SERVICE: tencent
      WIREPOD_TENCENTCLOUD_SECRET_ID: your-secret-id
      WIREPOD_TENCENTCLOUD_SECRET_KEY: your-secret-key
      WIREPOD_TENCENT_ASR_ENGINE_MODEL_TYPE: 16k_zh
      WIREPOD_TTS_PROVIDER: tencent
      WIREPOD_TENCENT_TTS_VOICE_TYPE: "601009"
```

注意：当前 Dockerfile 默认编译 Vosk 入口。若容器内二进制不是 Tencent 入口，仅改 `WIREPOD_STT_SERVICE=tencent` 不等于完成 Tencent ASR 编译切换；需要确保构建产物来自 `cmd/tencent`。

### 原生运行使用 Tencent TTS

```bash
export TENCENTCLOUD_SECRET_ID="your-secret-id"
export TENCENTCLOUD_SECRET_KEY="your-secret-key"
export TTS_PROVIDER="tencent"
export TENCENT_TTS_VOICE_TYPE="601009"
sudo ./chipper/start.sh
```

### 更换 Web UI 端口

```bash
export WEBSERVER_PORT=8081
sudo ./chipper/start.sh
```

### 关闭 8084 兼容监听

```bash
export NO8084=true
sudo ./chipper/start.sh
```

## 敏感信息

以下文件或字段包含密钥、token 或证书，应避免提交到公开仓库：

| 位置 | 敏感内容 |
| --- | --- |
| `chipper/source.sh` | 云服务密钥、Picovoice key |
| `chipper/apiConfig.json` | 天气/LLM key |
| `chipper/pico.key` | Picovoice key |
| `certs/cert.key` | TLS 私钥 |
| `chipper/session-certs/` | 机器人会话证书 |
| `chipper/jdocs/jdocs.json` | App Tokens 和机器人设置 |
| `chipper/jdocs/botSdkInfo.json` | 机器人 ESN、IP、GUID |

建议生产环境限制权限，例如：

```bash
chmod 600 chipper/source.sh chipper/apiConfig.json certs/cert.key
```
