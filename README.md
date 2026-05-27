# Gateway-a AI Gateway

一个高性能的 AI 网关，支持多种 LLM 提供商的统一接入。

## 功能特性

- **统一的 API 接口**: 支持多种 LLM 提供商
- **流式响应支持**: 完整的 LLM streaming 实现
- **真实 Token 统计 + 配额**: 可靠的用量记录与配额控制（含兜底估算）
- **高性能**: 使用 Sonic 等高性能库进行 JSON 处理
- **速率限制 + 熔断 + 重试**: 生产级稳定性
- **鉴权机制**: 完整的认证和授权系统（基于 api_keys 表）
- **可观测性**: Prometheus 指标 + 结构化日志 + 配额响应头

## 项目阶段

- Phase 1-10: 基础能力与关键修复
- **Phase 11**: 核心 LLM Proxy Hardening（稳定转发 + 真实 Token 统计 + 可靠配额）—— 已基本完成

## 快速开始

```bash
# 编译
go build -o gateway

# 运行（推荐通过环境变量配置）
LLM_UPSTREAM_URL=https://api.openai.com/v1 \
LLM_MAX_RETRIES=2 \
LLM_TIMEOUT=2m \
./gateway
```

## 运维建议

### 核心监控指标
- `gateway_a_llm_requests_total`
- `gateway_a_llm_tokens_total`
- `gateway_a_llm_latency_seconds`
- `gateway_a_tenant_usage_tokens_total`
- `gateway_a_quota_exceeded_total`
- `gateway_a_usage_record_failures_total`

### 告警建议
- 配额超限事件突然增加
- 用量记录失败率上升（`usage_record_failures_total`）
- LLM 请求错误率或延迟异常

### 故障排查
- 用量记录失败 → 检查 `usage_deadletter.log`
- 流式用量不准 → 确认上游是否支持 `stream_options.include_usage`
- 配额不生效 → 检查是否使用了正确的 API Key 表

### 恢复失败的用量记录
```go
processed, remaining, err := handler.ProcessUsageDeadLetter()
log.Printf("Re-processed %d records, %d remaining", processed, remaining)
```
建议定期（例如每小时）调用此函数，或通过后台 worker 运行。

### 推荐
- 定期处理 `usage_deadletter.log` 中的记录
- 为重要租户设置合理的 `QuotaDaily`

## 许可证

MIT License
