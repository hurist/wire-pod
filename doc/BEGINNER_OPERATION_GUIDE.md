# Wire-Pod 小白操作说明

这份文档按“完全没接触过项目”的角度写，目标是让你知道：

- 怎么启动 Wire-Pod
- 每个配置应该在哪里填
- 哪些配置是启动前填，哪些是启动后在网页里填
- Docker 和原生安装分别怎么操作
- 腾讯 TTS、腾讯 ASR、机器人连接、端口、证书分别是什么意思

如果你只想先跑起来，优先看“推荐路线”。如果你想知道每个环境变量的完整含义，再看 [FULL_CONFIGURATION_REFERENCE.md](FULL_CONFIGURATION_REFERENCE.md)。

## 先理解几个概念

Wire-Pod 可以简单理解成“给 Vector 机器人用的本地云服务器”。

机器人原来会连接官方云，现在改成连接你自己电脑、树莓派、NAS 或 Docker 里的 Wire-Pod。

常见名词：

| 名词 | 通俗解释 |
| --- | --- |
| Wire-Pod 服务器 | 运行这个程序的设备，比如电脑、树莓派、NAS |
| Vector 机器人 | 要连接到 Wire-Pod 的机器人 |
| Web UI | Wire-Pod 的网页后台，默认地址是 `http://服务器IP:8080` |
| STT | Speech To Text，把机器人听到的语音转成文字 |
| TTS | Text To Speech，把文字变成机器人说出来的声音 |
| LLM / Knowledge | AI 对话能力，比如接 OpenAI、Together 或自定义接口 |
| `source.sh` | 启动前读取的环境变量配置文件 |
| `apiConfig.json` | Wire-Pod 运行时保存的主配置文件 |
| `server_config.json` | 放到机器人里的配置，让机器人知道连接哪台 Wire-Pod |
| 证书 | 机器人和 Wire-Pod 建立安全连接时用的文件 |

## 推荐路线

如果你不确定该选哪个方式，建议这样选：

| 你的情况 | 推荐方式 |
| --- | --- |
| 你熟悉 Docker，或者在 NAS/服务器上部署 | Docker |
| 你是 Linux/树莓派用户，想按项目原生方式安装 | 原生安装 |
| 你只是开发、改代码、调试 | 原生安装更方便 |
| 你想少碰系统依赖 | Docker 更省心 |

当前项目里 Docker 默认构建的是 Vosk 语音识别入口。也就是说：

- Docker 跑起来比较省心
- 默认 STT 是 Vosk
- 默认 TTS 是 Tencent TTS
- 如果你想把 STT 也切到 Tencent ASR，需要确保二进制是用 `cmd/tencent` 构建的，不能只改环境变量

## 第一次使用要准备什么

### 必备

| 项目 | 说明 |
| --- | --- |
| 一台运行 Wire-Pod 的设备 | 电脑、Linux 小主机、树莓派、NAS 都可以 |
| Vector 和服务器在同一个局域网 | 例如连同一个 Wi-Fi 或同一个路由器 |
| 服务器能开放端口 | 至少需要 `80`、`443`、`8080` |
| 浏览器 | 用来打开 Web UI |

### 如果要中文语音

中文语音主要依赖 Tencent TTS。你需要准备：

| 项目 | 说明 |
| --- | --- |
| 腾讯云账号 | 用于调用腾讯云 TTS |
| `TENCENTCLOUD_SECRET_ID` | 腾讯云 API SecretId |
| `TENCENTCLOUD_SECRET_KEY` | 腾讯云 API SecretKey |
| TTS 权限和可用额度 | 账号需要能调用 TextToVoice |

默认音色是 `601009`，也就是当前新增的默认 voiceType。

### 如果要中文识别

中文识别可以用 Tencent ASR，或者用本地 Vosk/Whisper 模型。当前更直接的是 Tencent ASR，但需要注意：

- Tencent ASR 是 STT，也就是“听懂你说什么”
- Tencent TTS 是 TTS，也就是“机器人说出来”
- 这两个都用腾讯云账号密钥
- 只配置 TTS 不等于配置了 ASR

## 一句话流程

完整流程是：

1. 启动 Wire-Pod
2. 打开 Web UI
3. 配置服务器模式和证书
4. 把机器人连接到 Wire-Pod
5. 配置 STT、TTS、LLM、天气等能力
6. 测试机器人是否能听、能说、能连接

## 方式一：Docker 运行

### 第 1 步：进入项目目录

```bash
cd /Volumes/ORICO1TB/project/open/wire-pod
```

如果你的项目在别的目录，就进入你自己的 `wire-pod` 目录。

### 第 2 步：检查 Docker Compose 文件

Docker 配置文件在：

```text
compose.yaml
```

里面默认会暴露这些端口：

| 端口 | 用途 |
| --- | --- |
| `80` | 机器人连接检查、SDK App、摄像头流 |
| `443` | 机器人主要连接端口 |
| `8080` | Web UI |
| `8084` | Escape Pod 兼容端口 |

默认数据保存到 Docker volume：

| Volume | 容器内路径 | 作用 |
| --- | --- | --- |
| `wire-pod-data` | `/data` | 保存配置、证书、模型、机器人信息 |
| `wire-pod-images` | `/images` | 保存图片或媒体相关文件 |

### 第 3 步：配置 Docker 环境变量

最简单可以先不改 `compose.yaml`，直接用默认配置启动。

如果你要配置腾讯 TTS，建议在 `compose.yaml` 的 `environment` 里加这些：

```yaml
environment:
  WIREPOD_DATA_DIR: /data
  WIREPOD_TTS_PROVIDER: tencent
  WIREPOD_TENCENTCLOUD_SECRET_ID: 你的SecretId
  WIREPOD_TENCENTCLOUD_SECRET_KEY: 你的SecretKey
  WIREPOD_TENCENT_TTS_REGION: ap-guangzhou
  WIREPOD_TENCENT_TTS_VOICE_TYPE: "601009"
  WIREPOD_TENCENT_TTS_SAMPLE_RATE: "16000"
  WIREPOD_TENCENT_TTS_CODEC: pcm
```

这些变量的意思：

| 变量 | 要不要改 | 说明 |
| --- | --- | --- |
| `WIREPOD_DATA_DIR` | 一般不改 | Docker 里保存数据的位置 |
| `WIREPOD_TTS_PROVIDER` | 建议保持 `tencent` | 使用腾讯 TTS |
| `WIREPOD_TENCENTCLOUD_SECRET_ID` | 必填 | 腾讯云 SecretId |
| `WIREPOD_TENCENTCLOUD_SECRET_KEY` | 必填 | 腾讯云 SecretKey |
| `WIREPOD_TENCENT_TTS_REGION` | 一般不改 | 默认 `ap-guangzhou` |
| `WIREPOD_TENCENT_TTS_VOICE_TYPE` | 当前默认 `601009` | 腾讯云音色 |
| `WIREPOD_TENCENT_TTS_SAMPLE_RATE` | 不建议改 | Vector 播放要求 `16000` |
| `WIREPOD_TENCENT_TTS_CODEC` | 不建议改 | Vector 播放要求 `pcm` |

如果只是先跑起来，不填腾讯密钥也可以启动，但 Tencent TTS 会调用失败并回退到 Vector 内置英文声音。

### 第 4 步：启动容器

```bash
docker compose up -d
```

解释一下：

| 命令片段 | 意思 |
| --- | --- |
| `docker compose` | 用 Docker Compose 管理服务 |
| `up` | 启动服务 |
| `-d` | 后台运行 |

### 第 5 步：看容器是否启动成功

```bash
docker compose ps
```

如果看到 `wire-pod` 是 running 或 healthy，说明容器起来了。

也可以看日志：

```bash
docker compose logs -f wire-pod
```

看到类似下面的信息，说明 Web UI 已启动：

```text
Starting webserver at port 8080
```

### 第 6 步：打开 Web UI

在浏览器打开：

```text
http://服务器IP:8080
```

如果是在本机运行 Docker，可以先试：

```text
http://localhost:8080
```

如果是局域网另一台设备访问，需要知道服务器 IP。

Linux/macOS 可以用：

```bash
ifconfig
```

或者：

```bash
ip addr
```

找类似 `192.168.x.x` 的地址。

### 第 7 步：Docker 配置以后保存在哪里

Docker 会把配置保存到 `/data` 对应的 volume 里。你可以理解成：

| 你在网页里改的东西 | 实际保存位置 |
| --- | --- |
| TTS 设置 | `/data/chipper/apiConfig.json` |
| LLM 设置 | `/data/chipper/apiConfig.json` |
| 天气设置 | `/data/chipper/apiConfig.json` |
| 自定义意图 | `/data/chipper/customIntents.json` |
| 机器人设置 | `/data/chipper/jdocs/jdocs.json` |
| 机器人证书 | `/data/chipper/session-certs/` |
| 启动环境变量 | `/data/chipper/source.sh` |
| 服务器证书 | `/data/certs/` |

一般不要手动改 Docker 容器里的文件，优先用：

- `compose.yaml` 改 Docker 环境变量
- Web UI 改运行配置

## 方式二：原生安装运行

原生安装适合 Linux、macOS 或开发调试。

### 第 1 步：进入项目目录

```bash
cd /Volumes/ORICO1TB/project/open/wire-pod
```

### 第 2 步：运行安装脚本

```bash
sudo ./setup.sh
```

脚本会做几件事：

| 操作 | 说明 |
| --- | --- |
| 检查系统 | 判断是 Debian、Arch、Fedora、macOS |
| 安装依赖 | 安装 Go、音频库、编译工具等 |
| 选择 STT | 让你选择语音识别方案 |
| 生成 `source.sh` | 保存启动用的环境变量 |
| 下载模型 | 比如 Vosk 或 Whisper 模型 |

### 第 3 步：选择 STT

脚本会问你：

```text
Which speech-to-text service would you like to use?
```

新手建议：

| 选择 | 适合谁 | 说明 |
| --- | --- | --- |
| Vosk | 大多数人 | 本地识别，不依赖云，默认推荐 |
| Tencent Cloud ASR | 中文识别优先 | 云端识别，需要腾讯云密钥 |
| Whisper | 机器性能较强 | 本地模型较大，对硬件要求高 |
| Picovoice Leopard | 有 Picovoice key | 云账号/授权相关 |
| Coqui | 旧方案 | 现在不优先推荐 |

如果你要中文识别，选 Tencent Cloud ASR 时会让你输入：

```text
TencentCloud SecretId
TencentCloud SecretKey
```

脚本会写入：

```bash
export STT_SERVICE=tencent
export TENCENTCLOUD_SECRET_ID=你的SecretId
export TENCENTCLOUD_SECRET_KEY=你的SecretKey
export TENCENT_ASR_REGION=ap-guangzhou
export TENCENT_ASR_ENGINE_MODEL_TYPE=16k_zh
export TENCENT_ASR_VOICE_FORMAT=pcm
```

### 第 4 步：理解 `source.sh`

安装脚本会生成：

```text
chipper/source.sh
```

这是“启动前配置”。程序启动时会先读取它。

你可以用文本编辑器打开：

```bash
nano chipper/source.sh
```

里面常见内容：

```bash
export DEBUG_LOGGING=true
export STT_SERVICE=vosk
export TTS_PROVIDER=tencent
export TENCENT_TTS_REGION=ap-guangzhou
export TENCENT_TTS_VOICE_TYPE=601009
export TENCENT_TTS_SAMPLE_RATE=16000
export TENCENT_TTS_CODEC=pcm
export TENCENT_TTS_SPEED=0
export TENCENT_TTS_VOLUME=0
export TENCENT_TTS_TIMEOUT_SECONDS=15
```

如果你要腾讯 TTS，还需要确保有：

```bash
export TENCENTCLOUD_SECRET_ID=你的SecretId
export TENCENTCLOUD_SECRET_KEY=你的SecretKey
```

什么时候改 `source.sh`：

| 情况 | 是否改 `source.sh` |
| --- | --- |
| 改 STT 引擎 | 要改 |
| 填腾讯云密钥 | 建议改 |
| 改 Web UI 端口 | 可以改 |
| 改 TTS provider 默认值 | 可以改 |
| 改 LLM prompt | 不建议，去 Web UI |
| 改天气 API key | 不建议，去 Web UI |

### 第 5 步：启动 Wire-Pod

```bash
sudo ./chipper/start.sh
```

如果你已经在 `chipper/` 目录里，也可以：

```bash
cd chipper
sudo ./start.sh
```

启动成功后会看到类似：

```text
Starting webserver at port 8080
```

然后打开：

```text
http://服务器IP:8080
```

### 第 6 步：设置成开机自启

如果你想让 Wire-Pod 作为系统服务运行：

```bash
sudo ./setup.sh daemon-enable
```

之后启动服务：

```bash
sudo systemctl start wire-pod
```

查看服务状态：

```bash
sudo systemctl status wire-pod
```

查看日志：

```bash
journalctl -fe | grep start.sh
```

关闭开机自启：

```bash
sudo ./setup.sh daemon-disable
```

## Web UI 怎么配置

Web UI 是最适合新手使用的配置入口。

默认地址：

```text
http://服务器IP:8080
```

如果在本机打开：

```text
http://localhost:8080
```

### 第一次打开 Web UI 应该做什么

建议顺序：

1. 先确认服务器模式
2. 生成证书
3. 让机器人连接到 Wire-Pod
4. 配置 TTS
5. 配置 STT 语言或模型
6. 配置 LLM 或天气
7. 测试机器人说话和听话

### Server 设置

Server 设置决定机器人连接哪里。

| 模式 | 适合场景 | 说明 |
| --- | --- | --- |
| Escape Pod 模式 | 机器人能解析 `escapepod.local` | 使用固定域名 `escapepod.local:443` |
| IP 模式 | 最直观，适合新手 | 使用服务器局域网 IP，比如 `192.168.1.100:443` |

新手建议优先用 IP 模式，因为容易理解和排查。

如果服务器 IP 变化了，例如路由器重新分配了 IP，需要重新生成证书和 `server_config.json`，并重新部署到机器人。

### TTS 设置

TTS 是机器人“说话”的声音来源。

Web UI 里找到 TTS Setup 区域。

推荐配置：

| 项目 | 推荐值 | 说明 |
| --- | --- | --- |
| Provider | `tencent` | 使用腾讯云中文 TTS |
| Region | `ap-guangzhou` | 默认地域 |
| Voice Type | `601009` | 当前默认音色 |
| Sample Rate | `16000` | 保持 16kHz |
| Codec | `pcm` | 保持 PCM |
| Speed | `0` | 默认语速 |
| Volume | `0` | 默认音量 |

注意：

- 腾讯云 SecretId/SecretKey 不在 Web UI 里填
- 腾讯云密钥在 `source.sh` 或 Docker 环境变量里填
- Web UI 里改的是 `apiConfig.json` 中的 `tts` 设置

如果机器人还是英文声，通常说明 Tencent TTS 调用失败，程序回退到了 Vector 内置 TTS。优先检查：

1. `TENCENTCLOUD_SECRET_ID` 是否设置
2. `TENCENTCLOUD_SECRET_KEY` 是否设置
3. 腾讯云账号是否开通 TTS
4. 账号是否欠费
5. `VoiceType=601009` 是否在当前账号/地域可用
6. 日志里是否有 `Tencent TTS error`

### STT 设置

STT 是机器人“听懂你说话”的能力。

重要区别：

| 配置 | 在哪里改 | 是否需要重启 |
| --- | --- | --- |
| STT 引擎，比如 Vosk/Tencent | `source.sh` 或 Docker 环境变量 | 通常需要重启，甚至需要对应二进制 |
| STT 语言，比如 `en-US` | Web UI 或 `source.sh` | Vosk/whisper.cpp 可在 Web UI 改 |
| Tencent ASR 参数 | `source.sh` 或 Docker 环境变量 | 需要重启 |

如果你用 Vosk，本地模型必须已经下载。Web UI 里选择语言时，如果模型没有下载，可能会触发下载。

如果你用 Tencent ASR，常用配置是：

```bash
export STT_SERVICE=tencent
export TENCENTCLOUD_SECRET_ID=你的SecretId
export TENCENTCLOUD_SECRET_KEY=你的SecretKey
export TENCENT_ASR_REGION=ap-guangzhou
export TENCENT_ASR_ENGINE_MODEL_TYPE=16k_zh
export TENCENT_ASR_VOICE_FORMAT=pcm
```

### LLM / AI 对话设置

LLM 是让机器人能回答开放问题、聊天、执行 AI 命令的能力。

一般在 Web UI 的 Knowledge / KG / AI 相关区域配置。

常见项：

| 项目 | 说明 |
| --- | --- |
| Enable | 是否启用 AI 回答 |
| Provider | 使用哪个 AI 服务 |
| API Key | 对应服务的 key |
| Model | 模型名称 |
| Prompt | 给 AI 的系统提示词 |
| Save Chat | 是否保存上下文 |
| Commands Enable | 是否允许 AI 触发机器人动作命令 |

什么时候配置：

- 先让机器人能正常连接和说话
- 再配置 LLM
- 如果 TTS 还没通，LLM 回答可能有文字但机器人不会用中文说出来

### 天气设置

天气设置也在 Web UI 里配置。

| 项目 | 说明 |
| --- | --- |
| Provider | 天气服务商 |
| Key | 天气 API key |
| Unit | 摄氏或华氏 |

天气不是启动必需项，可以最后再配。

### 自定义意图

自定义意图是你自己添加语音命令。

例如你想让机器人听到某句话后执行脚本、触发动作，就在 Web UI 的 Custom Intents 里加。

常见字段：

| 字段 | 说明 |
| --- | --- |
| Name | 名称 |
| Description | 描述 |
| Utterances | 触发句子 |
| Intent | 意图名 |
| Params | 参数 |
| Exec | 要执行的外部程序 |
| ExecArgs | 外部程序参数 |
| LuaScript | Lua 脚本 |

新手建议先不要动 Lua，先用已有功能跑通。

## 机器人怎么连接到 Wire-Pod

这一步的核心是：机器人必须拿到正确的 `server_config.json` 和证书。

### IP 模式

IP 模式会生成类似这样的配置：

```json
{
  "jdocs": "192.168.1.100:443",
  "tms": "192.168.1.100:443",
  "chipper": "192.168.1.100:443",
  "check": "192.168.1.100/ok:80"
}
```

这里的 `192.168.1.100` 就是 Wire-Pod 服务器 IP。

适合新手，因为你能直接看懂机器人要连哪台机器。

### Escape Pod 模式

Escape Pod 模式会生成类似：

```json
{
  "jdocs": "escapepod.local:443",
  "tms": "escapepod.local:443",
  "chipper": "escapepod.local:443",
  "check": "escapepod.local/ok:80"
}
```

这个模式依赖局域网里的 mDNS 解析。若网络环境复杂，可能不如 IP 模式直观。

### 通过 SSH 部署到机器人

如果你有 OSKR 或可 SSH 的机器人，可以用：

```bash
sudo ./setup.sh scp <机器人IP> <SSH密钥路径>
```

例子：

```bash
sudo ./setup.sh scp 192.168.1.150 /home/wire/id_rsa_Vector-R2D2
```

这个命令会把这些东西复制到机器人：

| 文件 | 作用 |
| --- | --- |
| `certs/server_config.json` | 告诉机器人连接 Wire-Pod |
| `certs/cert.crt` 或 `chipper/epod/ep.crt` | 让机器人信任 Wire-Pod |
| `vector-cloud/build/vic-cloud` | 替换机器人云连接程序 |
| `vector-cloud/pod-bot-install.sh` | 机器人端安装脚本 |

### 通过 Web UI / BLE 设置

如果走生产版机器人 onboarding，一般通过 Web UI 的 BLE 流程：

1. 初始化 BLE
2. 扫描机器人
3. 连接机器人
4. 输入 PIN
5. 扫描 Wi-Fi
6. 连接 Wi-Fi
7. 执行认证
8. 部署配置

不同固件和机器人状态会有差异。新手遇到 BLE 不稳定时，先确认：

- 服务器设备有蓝牙
- Docker 容器通常不一定能直接访问主机蓝牙
- 机器人离服务器足够近
- 机器人处于可配对状态

## 哪些东西什么时候配置

这是最容易混乱的地方。记住这张表就够了。

| 配置内容 | 配置位置 | 什么时候配置 | 改完是否重启 |
| --- | --- | --- | --- |
| Docker 数据目录 | `compose.yaml` | 第一次启动前 | 要重启容器 |
| Docker 端口映射 | `compose.yaml` | 第一次启动前 | 要重启容器 |
| Docker 腾讯密钥 | `compose.yaml` 的 `environment` | 第一次启动前或后续修改 | 要重启容器 |
| 原生腾讯密钥 | `chipper/source.sh` | 第一次启动前或后续修改 | 要重启 |
| STT 引擎 | `source.sh` 或 `WIREPOD_STT_SERVICE` | 第一次启动前最好确定 | 通常要重启/重编译 |
| STT 语言 | Web UI 或 `source.sh` | 启动后也能改 | Vosk/whisper.cpp 通常不用完整重装 |
| TTS provider | Web UI 或 `apiConfig.json` | 启动后改 | 通常不用重启 |
| Tencent TTS 音色 | Web UI 或 `apiConfig.json` | 启动后改 | 通常不用重启 |
| LLM provider/key/model | Web UI | 机器人连接成功后 | 通常不用重启 |
| 天气 key | Web UI | 需要天气功能时 | 通常不用重启 |
| 服务器 IP/模式 | Web UI | 机器人连接前 | 会重启 Chipper 服务 |
| 证书 | Web UI 生成 | 机器人连接前，IP 变化后 | 需要重新部署到机器人 |
| 自定义意图 | Web UI | 随时 | 通常不用重启 |

## 常见场景操作

### 场景 1：我只想先启动看看

Docker：

```bash
cd /Volumes/ORICO1TB/project/open/wire-pod
docker compose up -d
docker compose logs -f wire-pod
```

浏览器打开：

```text
http://localhost:8080
```

原生：

```bash
cd /Volumes/ORICO1TB/project/open/wire-pod
sudo ./setup.sh
sudo ./chipper/start.sh
```

浏览器打开：

```text
http://服务器IP:8080
```

### 场景 2：我要让机器人说中文

需要配置 Tencent TTS。

Docker 在 `compose.yaml` 加：

```yaml
environment:
  WIREPOD_TTS_PROVIDER: tencent
  WIREPOD_TENCENTCLOUD_SECRET_ID: 你的SecretId
  WIREPOD_TENCENTCLOUD_SECRET_KEY: 你的SecretKey
  WIREPOD_TENCENT_TTS_VOICE_TYPE: "601009"
```

然后重启：

```bash
docker compose up -d
```

原生在 `chipper/source.sh` 加：

```bash
export TTS_PROVIDER=tencent
export TENCENTCLOUD_SECRET_ID=你的SecretId
export TENCENTCLOUD_SECRET_KEY=你的SecretKey
export TENCENT_TTS_VOICE_TYPE=601009
```

然后重启：

```bash
sudo ./chipper/start.sh
```

Web UI 里确认 TTS provider 是 `tencent`。

### 场景 3：我要让机器人听懂中文

推荐 Tencent ASR。

原生安装时选择 Tencent Cloud ASR，或者在 `source.sh` 里配置：

```bash
export STT_SERVICE=tencent
export TENCENTCLOUD_SECRET_ID=你的SecretId
export TENCENTCLOUD_SECRET_KEY=你的SecretKey
export TENCENT_ASR_REGION=ap-guangzhou
export TENCENT_ASR_ENGINE_MODEL_TYPE=16k_zh
export TENCENT_ASR_VOICE_FORMAT=pcm
```

然后要确保程序是用 Tencent 入口构建/运行：

```bash
cd chipper
go build ./cmd/tencent
```

或者通过 `start.sh` 让它按 `STT_SERVICE=tencent` 路径运行。

Docker 用户注意：当前 Dockerfile 默认构建 Vosk 入口。要把 STT 改成 Tencent，需要调整构建，不建议新手第一步就改。

### 场景 4：我要改 Web UI 端口

原生在启动前设置：

```bash
export WEBSERVER_PORT=8081
sudo ./chipper/start.sh
```

或者写进 `chipper/source.sh`：

```bash
export WEBSERVER_PORT=8081
```

Docker 需要同时改容器内环境和端口映射，例如：

```yaml
environment:
  WEBSERVER_PORT: "8081"
ports:
  - "8081:8081"
```

### 场景 5：服务器 IP 变了

例如路由器重启后，服务器从 `192.168.1.100` 变成了 `192.168.1.120`。

你需要：

1. 打开 Web UI
2. 重新选择 IP 模式
3. 重新生成证书和 `server_config.json`
4. 重新部署到机器人
5. 重启或等待机器人重新连接

否则机器人还会去连接旧 IP。

### 场景 6：我要看日志

Docker：

```bash
docker compose logs -f wire-pod
```

原生 systemd：

```bash
journalctl -u wire-pod -f
```

直接运行：

看当前终端输出。

Web UI：

打开日志页面，或者接口：

```text
http://服务器IP:8080/api/get_logs
```

## 配置文件速查

| 文件 | 谁用 | 什么时候改 | 新手建议 |
| --- | --- | --- | --- |
| `compose.yaml` | Docker | 启动容器前 | Docker 用户主要改这里 |
| `docker/default-source.sh` | Docker 默认值 | 很少改 | 一般不直接改 |
| `docker/entrypoint.sh` | Docker 启动逻辑 | 开发者改 | 新手不要改 |
| `chipper/source.sh` | 原生启动和 Docker 持久化环境 | 改密钥、STT、端口时 | 可以小心改 |
| `chipper/apiConfig.json` | 主配置 | Web UI 自动写 | 新手优先用 Web UI，不手改 |
| `chipper/customIntents.json` | 自定义意图 | Web UI 自动写 | 新手优先用 Web UI |
| `chipper/jdocs/jdocs.json` | 机器人设置 | 自动同步 | 不建议手改 |
| `certs/server_config.json` | 给机器人连接 Wire-Pod | Web UI 或脚本生成 | 不建议手改 |
| `certs/cert.crt` | 服务器证书 | 自动生成 | 不建议手改 |
| `certs/cert.key` | 服务器私钥 | 自动生成 | 不要泄露 |

## 成功标准

你可以按这个清单判断是否跑通：

| 检查项 | 成功表现 |
| --- | --- |
| Web UI | 浏览器能打开 `http://服务器IP:8080` |
| 80 端口 | `http://服务器IP/ok` 有响应 |
| 机器人连接 | Web UI 或日志里能看到机器人认证/连接 |
| TTS | 在 SDK App 或语音流程里机器人能说话 |
| 中文 TTS | 机器人能用腾讯 TTS 说中文，不是英文机械声 |
| STT | 机器人听到指令后日志里有识别文字 |
| LLM | 问开放问题后机器人能生成回答 |

## 常见问题

### Web UI 打不开

先确认服务是否在跑。

Docker：

```bash
docker compose ps
docker compose logs -f wire-pod
```

原生：

```bash
sudo systemctl status wire-pod
```

再确认端口：

```bash
curl http://localhost:8080/api/is_running
```

如果端口被占用，换 `WEBSERVER_PORT`。

### 机器人连不上

优先检查：

1. 机器人和 Wire-Pod 是否在同一个局域网
2. 服务器 IP 是否正确
3. 端口 `80`、`443` 是否开放
4. `server_config.json` 是否已经部署到机器人
5. 证书是否重新生成并部署
6. 服务器 IP 是否变过
7. Docker 是否正确映射了端口

### 机器人能说话但不是中文

通常是 Tencent TTS 没成功，回退到了 Vector 内置 TTS。

检查：

```bash
TENCENTCLOUD_SECRET_ID
TENCENTCLOUD_SECRET_KEY
TTS_PROVIDER
TENCENT_TTS_VOICE_TYPE
```

Web UI 里确认：

```text
TTS Provider = tencent
Voice Type = 601009
```

然后看日志里是否有：

```text
Tencent TTS error
```

### 机器人听不懂中文

TTS 只负责说，不负责听。

要听懂中文，需要 STT 支持中文。检查：

| 当前 STT | 中文支持情况 |
| --- | --- |
| Vosk | 需要中文模型 |
| Tencent ASR | 默认适合中文，但需要云密钥和对应构建入口 |
| whisper.cpp | 需要合适模型，硬件要求更高 |

### 改了配置没有生效

先判断你改的是哪类配置：

| 配置 | 生效方式 |
| --- | --- |
| Web UI 里的 TTS/LLM/天气 | 一般立即生效 |
| `source.sh` 里的环境变量 | 重启 Wire-Pod 后生效 |
| Docker `environment` | 重启容器后生效 |
| STT 引擎 | 可能需要重新构建 |
| 证书/IP 模式 | 需要重新部署到机器人 |

Docker 重启：

```bash
docker compose restart wire-pod
```

原生重启：

```bash
sudo systemctl restart wire-pod
```

或者停止当前终端里的程序后重新运行 `sudo ./chipper/start.sh`。

## 最推荐的新手配置

如果你只是想在中文环境里尽快跑通，可以从这个组合开始：

| 项目 | 推荐值 |
| --- | --- |
| 运行方式 | Docker 或原生都可以 |
| 服务器模式 | IP 模式 |
| STT | 先用 Vosk 跑通，中文识别再考虑 Tencent ASR |
| TTS | Tencent TTS |
| TTS VoiceType | `601009` |
| TTS SampleRate | `16000` |
| TTS Codec | `pcm` |
| LLM | 先不启用，等机器人连接和 TTS 跑通后再配 |
| 天气 | 最后再配 |

最小腾讯 TTS 配置：

```bash
export TTS_PROVIDER=tencent
export TENCENTCLOUD_SECRET_ID=你的SecretId
export TENCENTCLOUD_SECRET_KEY=你的SecretKey
export TENCENT_TTS_VOICE_TYPE=601009
```

如果机器人能连接、能说中文，再逐步加 AI、天气、自定义意图。这样排查起来最稳。
