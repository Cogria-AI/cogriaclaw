<div align="right">

[English](./README.md) | **简体中文**

</div>

# cogriaclaw

> 一个连接 WhatsApp 与大模型的极简工具。轻量、实用、不臃肿。

`cogriaclaw` 是一个 Go 编写的单二进制服务，把一个 WhatsApp 账号接到大模型和一组可插拔 skill 上。它只做一件事并做好：接收白名单好友/群的消息，交给模型（带 tool-use），执行相应 skill，回复。同时提供一个极简的 HTTP API，让外部系统主动推消息或触发任务。

## 设计原则

- **轻量** —— 单一静态二进制，零运行时依赖，核心目标控制在 1k 行 Go 以内
- **实用** —— 只解决真实场景：白名单、群内 @ 触发、tool-use、HTTP 触发，到此为止
- **不臃肿** —— 没有插件市场、没有多渠道抽象层、没有"记忆框架"。你不需要的东西，这里就没有

## 它跟别的有什么不一样

- 不用 Puppeteer / 无头浏览器 —— 底层是 [whatsmeow](https://github.com/tulir/whatsmeow)，纯 Go 实现 WhatsApp Web 协议
- 一份配置文件，一个进程，一件事可调

## 功能

- 首次启动 QR 扫码登录，session 本地持久化
- 入站消息按 E.164 好友和群 JID 过滤，群内可选「需 @ 我」才触发
- 大模型调度，skill 以 tool 形式暴露给模型
- HTTP API：直接发消息，或触发命名任务并把结果推到指定 chat
- 自动重连，重启后消息去重

## 状态

早期开发中，架构已定，代码实现进行中。

## 免责声明

cogriaclaw **与 WhatsApp、Meta、Anthropic 均无任何关联**。本项目通过第三方 [whatsmeow](https://github.com/tulir/whatsmeow) 库与 WhatsApp Web 协议交互；运行该软件可能违反 WhatsApp 服务条款，并可能导致账号被封禁。软件按"原样"提供，不附任何担保（详见 [LICENSE](./LICENSE)）。仅供个人、教育及经授权的自动化用途 —— **不得用于未经请求的群发消息**。

## 许可证

[MIT](./LICENSE)
