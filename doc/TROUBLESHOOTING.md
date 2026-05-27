# Wire-Pod 故障排查指南

本文档汇总 Wire-Pod 运行中可能遇到的各种问题，提供系统性的诊断步骤和解决方案。

---

## 1. 启动失败

### 1.1 端口冲突

**症状**：
```
Error binding to 8080: listen tcp :8080: bind: address already in use
```
或
```
A process is already using port 80 - connCheck functionality will not work
```

**原因**：其他进程占用了 Wire-Pod 需要的端口。

**解决**：
```bash
# 查找占用端口的进程
sudo lsof -i :8080
sudo lsof -i :80
sudo lsof -i :443

# 更换 Web UI 端口
export WEBSERVER_PORT=8081
./chipper
```

> 端口 80 被占用时，Wire-Pod 会继续运行，但机器人的 `connCheck` 会失败，可能导致机器人周期性断开重连。

---

### 1.2 缺少 STT 模型

**症状**：
```
Failed to load Vosk model: no such file or directory
```

**解决**：
```bash
# Vosk 模型路径
ls ../vosk/models/en-US/
# 应包含 am/final.mdl, graph/phones/word_boundary.int 等

# 若缺失，运行 setup 脚本或手动下载
./setup.sh
# 或在 Web UI 中选择语言，会自动触发下载
```

---

### 1.3 CGO 编译错误

**症状**：
```
undefined reference to `vosk_model_new'
```

**原因**：缺少 libvosk 或 CGO 环境变量未设置。

**解决**：
```bash
# Linux
export CGO_LDFLAGS="-L/path/to/vosk -lvosk -lopus -lopusfile -lsodium -lasound"
export CGO_CFLAGS="-I/path/to/vosk"

# macOS
export CGO_LDFLAGS="-L/opt/homebrew/lib -lvosk"
export CGO_CFLAGS="-I/opt/homebrew/include"
```

---

### 1.4 证书错误

**症状**：
```
open ../certs/cert.crt: no such file or directory
```

**解决**：
```bash
# 在 chipper 目录下运行
cd chipper

# 生成证书（需要 root 权限绑定低端口时）
go run ./cmd/vosk  # 首次启动会自动引导到证书生成

# 或在 Web UI 中点击 "Generate Certs" 按钮
```

---

## 2. 机器人无法连接

### 2.1 诊断流程

```
机器人无法连接 Wire-Pod
    │
    ├── 机器人 WiFi 是否正常？
    │       └── 检查路由器管理页面，确认机器人在线
    │
    ├── 机器人 server_config.json 是否正确？
    │       └── SSH/BLE 进入机器人，检查配置文件
    │       └── 确认地址是 Wire-Pod 的 IP 或 escapepod.local
    │
    ├── Wire-Pod 证书是否匹配当前 IP？
    │       └── 若 Wire-Pod IP 变化，必须重新生成证书
    │       └── Web UI → "Generate Certs" → 重新部署 server_config.json
    │
    ├── 防火墙是否放行？
    │       └── sudo ufw allow 443/tcp
    │       └── sudo ufw allow 80/tcp
    │       └── sudo ufw allow 8080/tcp
    │
    └── TLS 握手是否成功？
            └── openssl s_client -connect <wirepod-ip>:443
            └── 检查证书 CN 是否匹配
```

### 2.2 常见错误日志

**日志**：`Loaded bot info file, known bots: []`
- **含义**：没有机器人完成认证
- **解决**：重新执行 BLE 配对或 SSH 部署流程

**日志**：`error creating chipper connection: connection refused`
- **含义**：机器人无法连接到 Wire-Pod 的 443 端口
- **解决**：检查防火墙、确认 Wire-Pod 正在运行、确认 IP 正确

**日志**：`certificate signed by unknown authority`
- **含义**：机器人不信任 Wire-Pod 的证书
- **解决**：重新生成证书并重新部署到机器人

---

## 3. AI 无响应

### 3.1 LLM 完全无回复

**诊断步骤**：

1. **检查配置**：
   ```bash
   curl http://localhost:8080/api/get_kg_api
   # 确认 enable=true, key 非空, provider 正确
   ```

2. **检查日志**：
   ```
   Error creating chat completion stream: ...
   ```
   
   | 错误信息 | 原因 | 解决 |
   |---|---|---|
   | `401 Unauthorized` | API key 无效 | 检查 key 是否被撤销或过期 |
   | `404 Not Found` | 模型不存在 | 检查 model 名称拼写 |
   | `429 Too Many Requests` | 速率限制 | 降低请求频率或升级账户 |
   | `does not exist` | OpenAI 账户未充值 | 充值至少 $5 |

3. **测试 API 连通性**：
   ```bash
   curl https://api.openai.com/v1/models \
     -H "Authorization: Bearer $OPENAI_KEY"
   ```

### 3.2 LLM 回复但机器人不说话

**诊断**：

1. 检查日志是否有 `LLM response for <ESN>:`
   - 若没有 → LLM 未返回内容，检查 Prompt 或 API 状态
   
2. 检查是否有 `KG SayText error:`
   - 若有 → 机器人连接问题，检查 `botSdkInfo.json` 中的 IP 和 GUID

3. 检查机器人是否被其他行为占用
   - Wire-Pod 使用 `OVERRIDE_BEHAVIORS` 优先级，但某些固件状态可能异常

### 3.3 LLM 回复内容异常

| 现象 | 原因 | 解决 |
|---|---|---|
| 回复太长 | Prompt 未限制 | 在 System Prompt 中强调 "concise" |
| 回复包含乱码 | 特殊字符过滤问题 | 检查 `removeSpecialCharacters()` |
| 回复是英文（应为中文） | LLM 未跟随语言 | 确认 User 消息是目标语言 |
| 命令不执行 | LLM 未输出 `{{cmd}}` 格式 | 检查 CommandsEnable 和 Prompt |

---

## 4. 音频问题

### 4.1 语音识别完全失败

**诊断流程**：
```
用户说话 → 机器人无反应
    │
    ├── 检查日志是否有 "Stream type: OPUS/PCM"
    │       └── 无 → 音频未到达 Wire-Pod，检查连接
    │
    ├── 检查日志是否有 "End of speech detected."
    │       └── 无 → VAD 未触发
    │           ├── 麦克风是否被遮挡？
    │           ├── 环境是否太吵？（VAD 可能无法区分语音和噪声）
    │           └── 临时将 debugWriteFile=true，分析音频质量
    │
    ├── 检查 STT 输出
    │       └── 空或 "[unk]" → 模型不匹配或音频质量差
    │           ├── 确认 STT_LANGUAGE 与实际语言一致
    │           └── 尝试在安静环境近距离说话
    │
    └── 检查意图匹配
            └── "intent_system_unmatched" → 识别成功但无匹配意图
                └── 这不是音频问题，是意图/LLM 配置问题
```

### 4.2 识别率低

**优化建议**：
1. **距离**：与机器人距离 0.5-2 米最佳
2. **环境**：减少背景噪声（电视、音乐、多人说话）
3. **语速**：正常语速，不要过快或过慢
4. **模型**：尝试不同的 Vosk 模型大小（小模型快但准确率较低）
5. **语言**：确保 `STT_LANGUAGE` 与实际说话语言完全一致

### 4.3 音频播放问题

**OpenAI TTS 无声**：
- 检查 `STT_LANGUAGE != "en-US"` 或 `openai_voice_with_english == true`
- 检查日志是否有 TTS API 错误
- 确认机器人支持 `ExternalAudioStreamPlayback`（固件 ≥1.7）

**SDK App 播放声音失败**：
- 确认上传的是 WAV 格式
- 确认采样率为 8kHz（SDK App 前端会自动转换）
- 检查 `play_sound` handler 是否有错误日志

---

## 5. Web UI 问题

### 5.1 Web UI 无法访问

```bash
# 检查 Wire-Pod 是否运行
ps aux | grep chipper

# 检查端口监听
netstat -tlnp | grep 8080

# 尝试本地访问
curl http://localhost:8080/api/is_running

# 检查防火墙
sudo iptables -L -n | grep 8080
```

### 5.2 Web UI 功能异常

**保存配置后未生效**：
- 部分配置（如 LLM key）立即生效
- 部分配置（如 STT 语言）需要调用特定 API 触发重载
- 若完全未生效，检查 `apiConfig.json` 文件权限是否可写

**自定义意图添加后无效**：
- 确认 `utterances` 非空
- 确认 `intent` 字段是有效的 Vector 意图名
- 检查日志是否有 Lua 验证错误

### 5.3 SDK App 问题

**机器人列表为空**：
- 检查 `botSdkInfo.json` 是否存在且包含机器人
- 若无，说明机器人未完成认证，重新执行 onboarding

**连接机器人失败**：
- 检查机器人 IP 是否可达：`ping <robot-ip>`
- 检查 GUID 是否正确：对比 `botSdkInfo.json` 和机器人实际 GUID
- 检查机器人是否在线：观察机器人眼睛动画

**摄像头流黑屏**：
- 确认机器人固件支持摄像头流
- 检查浏览器是否支持 MJPEG
- 检查 `cam-stream` URL 中的 `serial` 参数是否正确

---

## 6. 配置错误

### 6.1 apiConfig.json 损坏

**症状**：启动时崩溃或配置异常。

**解决**：
```bash
# 备份后删除，让 Wire-Pod 从环境变量重新生成
mv apiConfig.json apiConfig.json.bak
STT_SERVICE=vosk STT_LANGUAGE=en-US ./chipper

# 然后重新通过 Web UI 配置
```

### 6.2 JDocs 版本冲突

**症状**：机器人设置无法保存，或设置不断重置。

**日志特征**：
```
REJECTED_DOC_VERSION
```

**解决**：
```bash
# 停止 Wire-Pod
# 删除该机器人的 JDoc 条目
# 编辑 jdocs/jdocs.json，删除对应 ESN 的所有条目
# 重启 Wire-Pod，让机器人重新全量同步
```

### 6.3 证书与 IP 不匹配

**症状**：机器人之前能连接，突然断开且无法重连。

**原因**：Wire-Pod 服务器 IP 变化（DHCP 重新分配）。

**解决**：
1. Web UI → "Generate Certs" 重新生成证书
2. 重新部署 `server_config.json` 到机器人
3. 对于生产机器人，重新执行 BLE onboarding

---

## 7. 性能问题

### 7.1 系统资源占用高

**排查**：
```bash
# CPU
top -p $(pgrep chipper)

# 内存
ps -o pid,vsz,rss,comm -p $(pgrep chipper)

# 磁盘 I/O
iotop -p $(pgrep chipper)
```

**常见原因**：

| 现象 | 原因 | 解决 |
|---|---|---|
| CPU 持续 100% | Vosk 模型太大或并发请求多 | 换更小的模型，或减少并发 |
| 内存持续增长 | 音频数据未释放 | 检查是否有 stream 未关闭 |
| 磁盘频繁写入 | 日志过大 | 限制日志大小或关闭 DEBUG_LOGGING |

### 7.2 响应延迟高

**测量方法**：
- 开启 `DEBUG_LOGGING=true`
- 查看日志中从 `End of speech detected` 到 `Intent matched` 的时间差

**优化建议**：
1. **使用本地 STT**：Vosk 或 whisper.cpp 比 OpenAI Whisper API 快得多
2. **换更快的 LLM**：`gpt-4o-mini` 比 `gpt-4` 快且便宜
3. **减少网络跳数**：确保 Wire-Pod 和机器人在同一局域网
4. **关闭 Save Chat**：若不需要对话记忆，可减少 LLM 请求大小

---

## 8. 并发问题

### 8.1 症状

- 随机 panic（`concurrent map read and map write`）
- 配置修改后偶发失效
- 日志中出现不一致的状态

### 8.2 常见并发风险点

| 位置 | 风险 | 缓解 |
|---|---|---|
| `vars.APIConfig` | 多个 goroutine 读写 | 避免在高峰期修改配置 |
| `vars.BotJdocs` | JDocs 并发读写 | 目前依赖单 goroutine 处理 |
| `vars.RememberedChats` | LLM 并发访问 | 按 ESN 隔离，但追加操作无锁 |
| `speechrequest.go` | 音频流并发 | 每个 stream 独立，通常安全 |

### 8.3 调试并发问题

```bash
# 启用 Go 竞态检测器（会显著降低性能，仅调试使用）
go build -race -o chipper ./cmd/vosk
./chipper
```

若检测到 data race，会输出详细的调用栈。

---

## 9. 日志排查方法

### 9.1 关键日志关键词

| 关键词 | 含义 | 排查方向 |
|---|---|---|
| `Initiating <engine> voice processor` | STT 引擎初始化成功 | 确认引擎名称正确 |
| `Stream type: OPUS/PCM` | 收到机器人音频流 | 确认音频格式 |
| `End of speech detected` | VAD 判定语音结束 | 确认语音长度正常 |
| `Intent matched:` | 意图匹配成功 | 检查匹配结果是否符合预期 |
| `intent_system_unmatched` | 无匹配意图 | 考虑添加自定义意图或启用 LLM |
| `Using gpt-4o-mini` | LLM 请求已发起 | 确认模型和 provider |
| `LLM response for` | LLM 返回结果 | 检查内容是否正常 |
| `Error creating chat completion stream` | LLM 请求失败 | 检查 API key 和网络 |
| `Loaded bot info file, known bots:` | 机器人认证状态 | 确认机器人是否在列表中 |
| `KG SayText error:` | 机器人语音输出失败 | 检查机器人连接 |

### 9.2 日志过滤

```bash
# 实时查看并过滤
tail -f /dev/null | while read line; do echo "$line"; done  # 占位，实际看 stdout

# 若重定向到文件
tail -f wirepod.log | grep "Intent matched"
tail -f wirepod.log | grep "Bot 00e20145"  # 只看特定机器人
```

---

## 10. 获取帮助

如果以上方法无法解决问题：

1. **收集信息**：
   - Wire-Pod 版本（`cat version` 或 Git commit）
   - 机器人固件版本
   - 完整的错误日志（脱敏后）
   - 操作系统和硬件信息

2. **检查社区资源**：
   - GitHub Issues
   - Wiki 页面

3. **最小化复现**：
   - 尝试在干净环境（如新目录、默认配置）复现问题
   - 确认是配置问题还是代码问题

---

*本文档基于实际运行经验和代码逻辑编写。遇到问题时应结合日志和代码上下文分析。*
