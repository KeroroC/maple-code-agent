# MapleCode 第二阶段验收清单

每一项可勾选、可观测。带 `e2e` 标签的项需要真实 API key 跑通。

## 启动与回归

- [x] 跑 `go build ./...` 成功
- [x] 跑 `go vet ./...` 无输出
- [x] 跑 `go test ./...` 全绿
- [x] 普通聊天请求仍能流式显示 TextDelta
- [x] Anthropic thinking 开关原有测试仍通过
- [x] OpenAI-compatible 自定义 base_url 原有测试仍通过

## 工具接口与注册中心

- [x] `pkg/tool` 下存在工具接口、注册中心和结果结构定义
- [x] 注册中心登记两个 fake tool 后，按名称查找都成功
- [x] 查找未知工具返回结构化错误
- [x] 工具元信息包含名称、描述和参数 schema
- [x] 工具结果 JSON 含 `ok`、`content`、`error`、`truncated` 字段
- [x] 构造超过结果大小上限的 content 后，返回结果含 `truncated=true`

## 文件安全边界

- [x] `read_file` 读取 repo 内相对路径成功
- [x] `read_file` 传绝对路径失败
- [x] `read_file` 传 `../` 越界路径失败
- [x] `read_file` 读取目录失败
- [x] 指向 repo 外文件的符号链接读取失败
- [x] `write_file` 写入 repo 内已有父目录的文件成功
- [x] `write_file` 写入父目录不存在的路径失败
- [x] `edit_file` 对唯一 old text 替换成功
- [x] `edit_file` 对不存在 old text 返回失败，错误含匹配不到的含义
- [x] `edit_file` 对出现多次 old text 返回失败，错误含匹配多次的含义

## 命令工具

- [x] `run_command` 执行 `go test ./pkg/tool` 成功并返回退出码 0
- [x] `run_command` 执行不在白名单内的命令失败且不执行
- [x] `run_command` 工作目录固定为 repo root
- [x] `run_command` 返回 stdout、stderr 和 exit code
- [x] 超时命令被终止，结果 metadata 标记 timeout
- [x] 命令输出超过大小上限时 `truncated=true`

## 文件查找与代码搜索

- [x] `glob_files` 用 `pkg/**/*.go` 返回至少一个相对路径
- [x] `glob_files` 结果不包含绝对路径
- [x] `glob_files` 结果数量超过上限时被截断
- [x] `grep_code` 搜索已存在字符串返回 `path:line:match`
- [x] `grep_code` 的 include glob 能限制搜索范围
- [x] `grep_code` 搜索结果超过大小上限时 `truncated=true`

## Provider 工具定义转换

- [x] fake registry 能转换为 Anthropic tools 定义
- [x] fake registry 能转换为 OpenAI tools 定义
- [x] Anthropic tools 定义包含 name、description、input schema
- [x] OpenAI tools 定义包含 function name、description、parameters
- [x] Provider 层新增统一工具调用 chunk，并能被 type switch 识别

## Anthropic 工具流解析

- [x] httptest 模拟普通 Anthropic text_delta，仍收到 TextDelta
- [x] httptest 模拟 Anthropic tool_use，收到统一工具调用 chunk
- [x] Anthropic 工具调用 ID 被保留
- [x] Anthropic 工具名被保留
- [x] Anthropic 工具参数 JSON 被正确拼接
- [x] Anthropic 401 仍映射为 ErrAuth
- [x] Anthropic 429 仍映射为 ErrRateLimit

## OpenAI / OpenAI-compatible 工具流解析

- [x] httptest 模拟普通 OpenAI delta，仍收到 TextDelta
- [x] httptest 模拟 `delta.tool_calls` 多 chunk 分片，收到统一工具调用 chunk
- [x] OpenAI tool call id 被保留
- [x] OpenAI function name 被保留
- [x] OpenAI function arguments 被正确拼接为完整 JSON
- [x] finish reason 为 `tool_calls` 时工具调用完成
- [x] OpenAI-compatible 工具请求仍发送到自定义 base_url

## 会话持久化

- [x] 工具调用记录写入 JSONL 后可读回
- [x] 工具结果记录写入 JSONL 后可读回
- [x] 工具结果记录包含结构化 result JSON 和 summary
- [x] 旧的只含 `type=turn` 的会话文件仍能恢复
- [x] 坏工具记录行不会导致 Open panic
- [x] 恢复会话后普通消息顺序不变

## TUI 展示

- [x] 工具成功结果渲染包含 `tool:` 与 `done`
- [x] 工具失败结果渲染包含 `tool:` 与 `failed`
- [x] 工具完整 content 不出现在 TUI 渲染文本中
- [x] 恢复历史后工具状态行可见
- [x] 普通 assistant markdown 渲染仍可用
- [x] 流式中断状态仍显示 interrupted

## 主流程

- [x] 启动时六个内置工具全部注册
- [x] fake streamer 发 `read_file` 工具调用后，read tool 执行一次
- [x] fake streamer 发未知工具名后，session 写入失败工具结果
- [x] fake streamer 发坏 JSON 参数后，session 写入失败工具结果
- [x] 一轮内两个工具调用时，只执行第一个
- [x] 工具执行完成后不自动二次请求 provider
- [x] 工具执行 summary 出现在 TUI
- [x] 工具结构化结果写入 session

## 端到端

- [ ] e2e: fake provider 请求 `glob_files`，TUI 显示工具 done
- [ ] e2e: fake provider 请求越界 `read_file`，TUI 显示工具 failed
- [ ] e2e: fake provider 请求 `edit_file` 唯一替换，目标文件内容改变
- [ ] e2e: fake provider 请求非白名单命令，命令不执行且显示 failed
- [ ] e2e: 会话恢复后能看到之前的工具状态行
- [ ] e2e: 真实 Anthropic 或 OpenAI provider 触发一次工具调用并成功执行
