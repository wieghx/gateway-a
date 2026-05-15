# Gateway-a AI Gateway

一个高性能的 AI 网关，支持多种 LLM 提供商的统一接入。

## 功能特性

- **统一的 API 接口**: 支持多种 LLM 提供商
- **流式响应支持**: 完整的 LLM streaming 实现
- **高性能**: 使用 Sonic 等高性能库进行 JSON 处理
- **速率限制**: 灵活的限流机制
- **鉴权机制**: 完整的认证和授权系统

## 项目阶段

- Phase 1: 基础 API 网关
- Phase 2: 鉴权与限流
- Phase 3: LLM 流式网关

## 快速开始

```bash
# 编译
go build -o gateway

# 运行
./gateway
```

## 许可证

MIT License
