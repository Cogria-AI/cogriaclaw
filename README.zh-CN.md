<div align="right">

[English](./README.md) | **简体中文**

</div>

# cogriaclaw

> 一个连接 WhatsApp 与大模型的极简工具。轻量、实用、不臃肿。

`cogriaclaw` 是一个 Go 编写的单二进制服务，把一个 WhatsApp 账号接到大模型上。它接收白名单好友/群的消息，交给一个能调用工具、遵循技能的大模型处理，然后回复。同时提供一个极简 HTTP API，让外部系统主动推消息或触发任务。

## 设计原则

- **轻量** —— 单一静态二进制，零运行时依赖，无 CGO
- **实用** —— 白名单、群内 @ 触发、tool-use、技能、HTTP 触发，到此为止
- **不臃肿** —— 没有插件市场、没有多渠道抽象层、没有"记忆框架"。你不需要的东西，这里就没有

## 它跟别的有什么不一样

- 不用 Puppeteer / 无头浏览器 —— 底层是 [whatsmeow](https://github.com/tulir/whatsmeow)，纯 Go 实现 WhatsApp Web 协议
- 任意 OpenAI 兼容大模型（Kimi、Moonshot、DeepSeek、OpenAI、Groq、OpenRouter、本地 Ollama……）—— 配置里换后端，不改代码
- nginx 式进程控制：`reload` 热重载配置不断连接；可自安装为 launchd/systemd 服务
- 一份配置文件，一个进程，一件事可调

## 快速开始

需要 Go 1.23+ 编译。

```sh
git clone https://github.com/Cogria-AI/cogriaclaw
cd cogriaclaw
go build -o cogriaclaw .

cp config.example.yaml config.yaml   # 然后编辑：白名单、大模型 key 等
./cogriaclaw run                      # 用 WhatsApp 扫码登录
```

首次启动终端会打印 QR 码，用 **WhatsApp → 设置 → 已关联的设备 → 关联设备** 扫码。session 存在 `data/` 下，之后启动无需再扫码。

用白名单内的号给登录账号发消息，它就会经大模型回复。

### 装成后台服务

```sh
./cogriaclaw install      # 注册 launchd(macOS)/systemd(Linux) 用户级服务并启动
./cogriaclaw status
./cogriaclaw reload       # 不断 WhatsApp 连接，热重载配置
./cogriaclaw stop
./cogriaclaw uninstall
```

`run` 是前台模式（日志打到终端，关终端即停）。`install` 让它在后台由系统服务管理器托管 —— 注销/重启都活着、崩溃自动重启。全部命令见 `cogriaclaw help`。

## 配置

一切都在一份 `config.yaml`（见 [`config.example.yaml`](./config.example.yaml)）。要点：

- **`filter`** —— `allowed_dms`（E.164 号码）和 `allowed_groups`（群 JID）。其他来源一律 drop。`group_require_mention` 控制群里是否要 @ 才回。
- **`llm`** —— `base_url` + `api_key` + `model` 选任意 OpenAI 兼容后端；`headers`、`extra_body` 处理各家差异；`${ENV_NAME}` 插值让 key 不进文件。
- **`conversation`** —— 按 chat 的内存短期会话；`reset_command`（默认 `/new`）开新会话。不落盘。
- **`api`** —— 可选 HTTP 控制接口（见下），建议只绑 localhost。

## 工具与技能

两层：

- **工具（Tools）** 是模型直接调用的函数原语 —— `http_get`，以及 `read_file`、`run_script`（两者都锁死在 skills 目录内）。Go 实现。
- **技能（Skills）** 是 `skills/` 下的 `SKILL.md` 文件夹（markdown 指令 + 可选附带脚本）。模型只看到每个技能的 name + description；命中时读 `SKILL.md` 并按其指令、用工具去落地。这套渐进式加载对应 [Anthropic Agent Skills](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/overview) 模型。

示例见 [`skills/server-time/`](./skills/server-time/)。`run_script`（文件夹内脚本执行）需通过 `skills.exec.enabled` 显式开启。

## HTTP API

设置 `api.listen`（和 bearer `api.token`）即启用。建议只绑 localhost，对外暴露用你自己的 tunnel/反代。

| 端点 | 鉴权 | 用途 |
|---|---|---|
| `GET /healthz` | 无 | 存活 + WhatsApp 连接状态 |
| `POST /send` | bearer | 直接发消息，不过模型 |
| `POST /trigger` | bearer | 执行一个工具，可选把结果推到指定 chat |

```sh
curl -XPOST localhost:8787/send -H "Authorization: Bearer $TOKEN" \
  -d '{"to":"+447700900123","text":"hello"}'

curl -XPOST localhost:8787/trigger -H "Authorization: Bearer $TOKEN" \
  -d '{"tool":"http_get","input":{"url":"https://example.com"},"notify":{"to":"+447700900123"}}'
```

## 免责声明

cogriaclaw **与 WhatsApp、Meta、Anthropic 均无任何关联**。本项目通过第三方 [whatsmeow](https://github.com/tulir/whatsmeow) 库与 WhatsApp Web 协议交互；运行该软件可能违反 WhatsApp 服务条款，并可能导致账号被封禁。软件按"原样"提供，不附任何担保（详见 [LICENSE](./LICENSE)）。仅供个人、教育及经授权的自动化用途 —— **不得用于未经请求的群发消息**。

## 许可证

[MIT](./LICENSE)
