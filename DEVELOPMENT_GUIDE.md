# Wire-Pod 开发指南

本文档面向需要修改 Wire-Pod 代码、添加功能或调试问题的开发者。

---

## 1. 本地开发环境搭建

### 1.1 系统依赖

**Linux (Debian/Ubuntu)**：
```bash
sudo apt-get update
sudo apt-get install -y \
    build-essential \
    libopus-dev \
    libopusfile-dev \
    libsox-dev \
    libsodium-dev \
    libasound2-dev \
    pkg-config \
    git \
    golang-go
```

**macOS**：
```bash
brew install opus opusfile sox libsodium pkg-config
```

**Windows (WSL2)**：使用 Ubuntu 子系统，按 Linux 方式安装。

### 1.2 Go 版本

Wire-Pod 需要 Go **1.18 或更高版本**。

```bash
go version  # 验证
```

### 1.3 项目获取

```bash
git clone https://github.com/kercre123/wire-pod.git
cd wire-pod
```

### 1.4 首次构建

```bash
cd chipper

# 下载依赖
go mod download

# 构建默认版本（Vosk）
go build -o chipper ./cmd/vosk

# 或构建其他 STT 引擎
go build -o chipper ./cmd/whisper.cpp
go build -o chipper ./cmd/coqui
```

### 1.5 运行

```bash
# 开发模式
STT_SERVICE=vosk STT_LANGUAGE=en-US ./chipper

# 或指定 Web 端口
WEBSERVER_PORT=8080 ./chipper
```

---

## 2. 开发工作流

### 2.1 推荐的 IDE 设置

**VS Code** 推荐插件：
- Go（官方）
- Todo Tree

**推荐设置**（`.vscode/settings.json`）：
```json
{
  "gopls": {
    "ui.diagnostic.annotations": {
      "bounds": true,
      "escape": true,
      "inline": true,
      "nil": true
    }
  },
  "go.toolsManagement.autoUpdate": false
}
```

### 2.2 代码结构认知

修改代码前，先确定你要改的是哪一层：

```
传输层（servers/*）     ← 修改 gRPC 接口、协议兼容性
    │
抽象层（vtt/）          ← 极少需要修改
    │
调度层（wirepod/preqs/） ← 修改请求路由逻辑
    │
处理层（wirepod/stt/）   ← 添加/修改 STT 引擎
    │
处理层（wirepod/ttr/）   ← 修改意图匹配、AI 逻辑、动画
    │
应用层（wirepod/sdkapp/）← 添加远程控制功能
    │
表现层（webroot/）       ← 修改 Web UI
    │
数据层（vars/）          ← 修改配置结构、全局状态
```

### 2.3 热重载开发

Wire-Pod **不支持**热重载。每次修改 Go 代码后必须重新编译：

```bash
cd chipper
go build -o chipper ./cmd/vosk && ./chipper
```

**前端开发技巧**：
- Web UI 是静态文件，修改 `webroot/` 中的 HTML/JS/CSS **不需要重启**
- 浏览器可能缓存静态文件，开发时建议禁用缓存（DevTools → Network → Disable cache）

---

## 3. 日志系统

### 3.1 日志输出

**位置**：`pkg/logger/`

**使用方式**：
```go
import "github.com/kercre123/wire-pod/chipper/pkg/logger"

logger.Println("普通日志")
logger.LogUI("Web UI 日志（也会显示在前端）")
```

### 3.2 日志级别

Wire-Pod 没有显式的日志级别（如 DEBUG/INFO/WARN/ERROR），而是通过以下方式区分：

| 方式 | 输出位置 | 用途 |
|---|---|---|
| `logger.Println()` | 控制台 + `LogList` | 普通运行日志 |
| `logger.LogUI()` | 控制台 + `LogList` + `LogTrayList` | 需要在 Web UI 显示的重要信息 |
| `fmt.Println()` | 仅控制台 | 开发调试 |

### 3.3 启用调试日志

```bash
export DEBUG_LOGGING=true
./chipper
```

效果：
- 输出更多内部状态（STT 中间结果、VAD 状态等）
- 部分模块会增加额外的 `fmt.Println`

### 3.4 查看日志

**控制台**：直接看终端输出

**Web UI**：`http://<ip>:8080` → Log 页面

**API**：
```bash
curl http://localhost:8080/api/get_logs        # 普通日志
curl http://localhost:8080/api/get_debug_logs  # 调试日志
```

---

## 4. 如何新增模块

### 4.1 新增 STT 引擎

**步骤**：

1. **创建目录**：`pkg/wirepod/stt/<your-engine>/`

2. **实现接口**：
```go
package yourengine

import (
    sr "github.com/kercre123/wire-pod/chipper/pkg/wirepod/speechrequest"
)

var Name = "yourengine"

func Init() error {
    // 加载模型、初始化资源
    return nil
}

func STT(req sr.SpeechRequest) (string, error) {
    // 处理 req.DecodedMicData 或 req.FilteredMicData
    // 返回识别文本
    return "recognized text", nil
}
```

3. **添加入口**：创建 `cmd/yourengine/main.go`
```go
package main

import (
    "github.com/kercre123/wire-pod/chipper/pkg/initwirepod"
    stt "github.com/kercre123/wire-pod/chipper/pkg/wirepod/stt/yourengine"
)

func main() {
    initwirepod.StartFromProgramInit(stt.Init, stt.STT, stt.Name)
}
```

4. **构建测试**：
```bash
go build -o chipper ./cmd/yourengine
STT_SERVICE=yourengine ./chipper
```

**注意事项**：
- 若使用 CGO，需确保交叉编译时设置正确的 `CC`/`CXX`
- 若需要流式处理（非整段），需要在 `STT()` 中自行管理状态
- 参考 `pkg/wirepod/stt/vosk/Vosk.go` 的 pool 设计处理并发

---

### 4.2 新增 HTTP API

**步骤**：

1. **在 `config-ws/webserver.go` 添加路由**：
```go
func apiHandler(w http.ResponseWriter, r *http.Request) {
    switch strings.TrimPrefix(r.URL.Path, "/api/") {
    // ... 现有路由 ...
    case "my_new_feature":
        handleMyNewFeature(w, r)
    }
}
```

2. **实现 handler**：
```go
func handleMyNewFeature(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Param string `json:"param"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid body", http.StatusBadRequest)
        return
    }
    
    // 业务逻辑
    result := doSomething(req.Param)
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}
```

3. **前端对接**：在 `webroot/js/main.js` 或相关页面添加调用。

---

### 4.3 新增 AI 能力

**场景**：你想让 LLM 支持一个新的 "工具" 或命令。

**步骤**：

1. **修改 Prompt**（`pkg/wirepod/ttr/kgsim.go: CreatePrompt()`）：
```go
if vars.APIConfig.Knowledge.CommandsEnable {
    smsg.Content = smsg.Content + "\nNew command: myCommand||param"
}
```

2. **添加命令解析**（`pkg/wirepod/ttr/kgsim_cmds.go: PerformActions()`）：
```go
case "myCommand":
    result := doMyCommand(param)
    // 可选：将结果反馈给 LLM
    nChat = append(nChat, openai.ChatCompletionMessage{
        Role:    openai.ChatMessageRoleUser,
        Content: "Result: " + result,
    })
```

3. **测试**：在 Web UI 中启用 Commands，然后对机器人说出触发 LLM 使用的语音。

---

### 4.4 新增机器人功能（SDK App）

**场景**：你想在 SDK App 中添加一个新的机器人控制功能。

**步骤**：

1. **在 `sdkapp/server.go` 添加端点**：
```go
case r.URL.Path == "/api-sdk/my_feature":
    param := r.FormValue("param")
    result, err := robot.Conn.MyFeature(ctx, &vectorpb.MyFeatureRequest{
        Param: param,
    })
    if err != nil {
        fmt.Fprint(w, "error: "+err.Error())
        return
    }
    fmt.Fprint(w, "success")
    return
```

2. **前端添加按钮**（`webroot/sdkapp/control.html` 或 `settings.html`）。

3. **确保 protobuf 定义支持**：若 `vectorpb` 中没有对应的方法，说明 `vector-go-sdk` 不支持此功能。

---

### 4.5 新增插件

**Go 原生插件**：

1. 在 `plugins/` 下创建新目录
2. 实现插件接口：
```go
package main

import "C"

var Utterances = []string{"my trigger phrase"}
var Name = "MyPlugin"

func Action(transcribedText, botSerial, guid, target string) (string, string) {
    return "I triggered!", "intent_greeting_hello"
}
```
3. 编译：
```bash
cd plugins/myplugin
go build -buildmode=plugin -o myplugin.so myplugin.go
```
4. 放入 `./plugins/` 目录，重启 Wire-Pod

**Lua 脚本**（推荐，更简单）：

在 Web UI 的 Custom Intents 中，直接在 `luascript` 字段填写代码：
```lua
sayText("执行自定义操作", true)
postHTTPRequest("http://homeassistant.local/api/light/on", 
                "application/json", 
                "{\"entity_id\":\"light.living_room\"}", 
                5)
```

---

## 5. 调试技巧

### 5.1 使用 Delve 调试

```bash
cd chipper
go install github.com/go-delve/delve/cmd/dlv@latest

# 调试启动
dlv debug ./cmd/vosk

# 在 delve 中
(dlv) break pkg/wirepod/ttr/kgsim.go:189
(dlv) continue
```

### 5.2 调试音频

在 `pkg/wirepod/speechrequest/speechrequest.go` 中：
```go
var debugWriteFile bool = true  // 开启调试写入
```

运行后检查 `/tmp/wirepodtest.ogg`：
```bash
ffplay /tmp/wirepodtest.ogg  # 播放检查音质
```

### 5.3 调试 LLM

临时修改 `pkg/wirepod/ttr/kgsim.go`，在 `CreateAIReq()` 中添加：
```go
fmt.Printf("LLM Request: %+v\n", aireq)
```

或在 `StreamingKGSim()` 的 goroutine 中打印每个 token：
```go
fmt.Printf("Token: %q\n", response.Choices[0].Delta.Content)
```

### 5.4 调试机器人连接

测试 gRPC 连通性：
```bash
# 检查 TLS
curl -vk https://localhost:443/ok

# 检查 gRPC（需要 grpcurl）
grpcurl -insecure localhost:443 list
```

---

## 6. 测试

### 6.1 单元测试

Wire-Pod 当前**缺乏系统性的单元测试**。现有测试：
- `vector-cloud/internal/util/util_test.go`
- `vector-cloud/internal/voice/stream/stream_test.go`

**新增测试建议**：
```go
// pkg/wirepod/ttr/matchIntentSend_test.go
package wirepod_ttr

import "testing"

func TestProcessTextAll(t *testing.T) {
    vars.IntentList = []JsonIntent{
        {Name: "intent_weather_extend", Keyphrases: []string{"weather"}},
    }
    
    matched := ProcessTextAll(nil, "what's the weather", vars.IntentList, false)
    if !matched {
        t.Error("expected match for weather intent")
    }
}
```

### 6.2 集成测试

**手动测试清单**：

| 功能 | 测试方法 |
|---|---|
| 语音识别 | 对机器人说话，检查日志中的识别文本 |
| 意图匹配 | 说内置命令（如 "go to sleep"），检查是否正确响应 |
| LLM 对话 | 说 "I have a question"，检查是否有 AI 回复 |
| 自定义意图 | 在 Web UI 添加意图，测试触发 |
| Lua 脚本 | 添加带 Lua 的意图，测试执行 |
| SDK App | 打开 `http://<ip>/sdkapp/`，测试每个功能 |
| 摄像头 | 打开 `http://<ip>/cam-stream?serial=<ESN>` |
| 多机器人 | 连接第二台机器人，确认两台都能语音交互 |

---

## 7. 代码规范

### 7.1 现有风格

- 缩进：Tab
- 行宽：无严格限制，但建议 <120 字符
- 包命名：使用下划线分隔（如 `wirepod_ttr`）
- 错误处理：通常 `if err != nil { logger.Println(err) }`，不强制返回

### 7.2 新增代码建议

- **不要用新的全局变量**：优先通过函数参数传递
- **添加日志**：关键路径添加 `logger.Println()`
- **处理错误**：至少打印错误，不要静默忽略
- **注释复杂逻辑**：尤其是 goroutine 和 channel 的使用

---

## 8. 提交与发布

### 8.1 Git 工作流

```bash
# 1. 创建功能分支
git checkout -b feature/my-feature

# 2. 修改代码
# ...

# 3. 构建验证
cd chipper && go build ./cmd/vosk

# 4. 测试
# ...

# 5. 提交
git add .
git commit -m "feat: add my feature"

# 6. 推送
git push origin feature/my-feature
```

### 8.2 版本标记

Wire-Pod 使用 Git tag 发布：
```bash
git tag v1.3.0
git push origin v1.3.0
```

---

*本文档基于项目实际代码结构和开发实践编写。建议配合 ARCHITECTURE.md 和 PROJECT_STRUCTURE.md 阅读。*
