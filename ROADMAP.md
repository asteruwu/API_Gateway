# API Gateway 开发周期规划

配套 [API Gateway.drawio.svg](./API%20Gateway.drawio.svg) 架构图使用。每完成一个版本，把对应的复选框打勾，方便追踪当前进度。

## 里程碑一览

| 版本 | 里程碑 | 新增能力 |
|---|---|---|
| v00 | 项目初始化 | `main.go` 入口 + 阶段0定义的接口/数据模型骨架（空实现） |
| v01 | 网络骨架 | connector 最简监听（单 goroutine 处理一个连接）+ 假后端 echo 服务能联通 |
| v02 | 端到端打通 | handler 主流程骨架跑通（decoder/filter/router/transformer 全部假实现），完整链路能返回假数据 |
| v03 | 连接层填实 | tcp_filter 编排能力（至少一个连接级限流）+ 字节流处理健壮性 |
| v04 | 请求解析 | decoder 真正把字节流解析为结构化 HTTP 请求 |
| v05 | 路由能力 | router 真实实现，支持配置文件定义多条路由规则 |
| v06 | 连接抽象 | ConnPool 落地（单连接版）+ 第一个 transformer 真实实现（比如先做 gRPC） |
| v07 | 安全认证 | 鉴权 filter 真实逻辑（JWT 校验） |
| v08 | 管道细化 | filter chain 可配置化（不同路由挂不同 filter 组合）+ HTTP 层精细限流 |
| v09 | 协议扩展 | 第二个 transformer 实现（验证新增协议不改已有代码）+ 连接池完善（多连接、失效检测） |
| v10 | 最终完善 | 错误处理统一、响应路径补全走查、代码整理，可运行版本 |

## 汇总时间线

| 阶段 | 内容 | 对应版本 | 预计天数 |
|---|---|---|---|
| 0 | 接口与数据模型定义 | v00 | 0.5 天 |
| 1 | 端到端骨架打通 | v01-v02 | 1-2 天 |
| 2 | 连接层填实 | v03 | 1 天 |
| 3 | 处理层填实（filter+路由） | v04-v05 | 2-3 天 |
| 4 | 协议转换+连接池 | v06-v09 | 3-4 天 |
| 5 | 联调补漏 | v10 | 1-2 天 |
| **合计** | | | **约 8.5-12.5 天** |

## 当前进度

> 最后更新：2026-08-06　｜　`go build ./...`、`go vet ./...` 通过

- [x] **v00 项目初始化** —— 完成，数据流已闭合
  - [x] `main.go`：按逆序完成编排（backend → handler → connector）
  - [x] `builder/`：`config.go` 配置结构 + `build.go` 入口（字段仍为占位注释）
  - [x] `connector/`：`model.go`（`Connector` 接口）、`listener.go`（Accept 循环 + `next` 注入）、`tcp_filter/filter.go`（`TCPFilter` 接口）
  - [x] `handler/`：`model.go`（`Handler` 接口）、`handler.go`（`HTTPHandler` + `Process`/`HandleHTTPConn` + `F2T`/`T2F`）、`decoder.go`、`encoder.go`
  - [x] `handler/message/`：网关统一的 `Request`/`Response` 模型
  - [x] `handler/http_filter/filter.go`、`handler/transformer/transformer.go`：接口定义，`Transformer` 出向 `Transform` / 回程 `Restore` 双向齐备，直接收发 `*message.Request`/`*message.Response`
  - [x] `backend/`：`model.go`（`Backend`/`Service`/`Instance`）、`backend.go`（`BManager`）、`forwarder.go`、`pool.go`
- [ ] v01 网络骨架
- [ ] v02 端到端打通
- [ ] v03 连接层填实
- [ ] v04 请求解析
- [ ] v05 路由能力
- [ ] v06 连接抽象
- [ ] v07 安全认证
- [ ] v08 管道细化
- [ ] v09 协议扩展
- [ ] v10 最终完善

## 数据流（已闭合）

```
conn → Decode → message.Request → filters
     → Transform → []byte → next(service, payload) → []byte
     → Restore → message.Response → Encode → conn.Write
```

## 架构约定（已定，后续照此执行）

1. **模块独立 + main.go 编排。** 各模块之间不互相 import，跨模块的函数签名里**只允许出现标准库类型**（`func(net.Conn) error`、`func(service string, payload []byte) ([]byte, error)`）。一旦签名里出现某个模块自己的类型，两个模块就被粘死了。
2. **初始化顺序是数据流的逆序**：backend → handler → connector。构造注入让编译器强制这个顺序，同时保证依赖图无环。
3. **`builder` 是纯数据包**，允许被运行时模块 import，但它自己**永远不 import 任何运行时模块**。这是唯一对第 1 条的例外，且只作用于构造期，不涉及数据面。
4. **`handler/message` 是网关唯一的中间表示**，不是 filter 内部约定。decoder / http_filter / transformer / encoder 四方共用，`Transformer` 直接在它和后端协议字节之间翻译。**不要在它之上再摞第二层中间模型**（曾有过 `TRequest`/`TResponse` + `F2T`/`T2F`，已删）——协议特有字段一旦进了中间层，填充它的胶水函数就会退化成按协议分支的 switch，协议知识从 transformer 实现里泄出来，直接打掉 v09 的验收标准。
5. **backend 只认 `(service string, payload []byte)`**，对 gRPC/MCP 一无所知。协议转换全部在 handler 包内完成，`message` 模型不越境。这是 v09「新增协议不改已有代码」的前提。
6. **路由和负载均衡是两级映射**：路由（http_filter）决定「请求 → 哪个服务」，负载均衡（backend）决定「服务 → 哪个实例」。拆开的硬理由是重试——换实例重试时不能重跑 filter 链（会重复鉴权、重复扣限流配额）。
7. **连接池归 `Instance` 所有**，一个实例一池；`LoadBalancer` 只读实例状态、不持有连接。因为池的生命周期跟实例走，而 LB 策略要能随时替换。
8. **响应只有一个出口**：`HandleHTTPConn` 里的 `Encode` + `conn.Write`。成功、filter 中断（401/429）、后端故障（502）全部汇到这一处，避免漏写导致客户端挂死。
9. **一条 conn 由一个 goroutine 独占**；但 filter 对象是所有连接共享的，filter 内部的状态（限流计数等）必须自己保证并发安全。

## 已知待办（不影响结构，填肉时处理）

- `HandleHTTPConn`：错误路径需转成 `message.ErrorResponse(err)` 写回，而不是直接 `return err`
- `HandleHTTPConn`：`for` 循环缺退出条件（EOF 正常结束 / 非 keep-alive / 写失败），`conn.Write` 返回值未检查
- `Decode` 缺 error 返回；返回值为值类型，而 filter 收指针
- `Connect()` 缺 error 返回，`net.Listen` 的错误被吞，端口占用时 main.go 无感
- `Forwarder.Forward` 不返回选中的 `*Instance`（建议改名 `LoadBalancer.Pick`，`forwarder.go` 留给转发主流程）
- `builder` 的 `HandlerConfig.filter/transformer`、`BackendConfig.service` 仍是小写，反序列化填不进去
- `Listener.addr` 未赋值，`ConnectorConfig` 尚无端口字段
- `main.go` 中 `backend`/`handler` 变量名遮蔽了同名包

## 下一步（v01 网络骨架）

1. `ConnectorConfig` 加监听端口字段，`NewListener` 里把 `addr` 真正赋上
2. `Listener.Connect()` 补 error 返回，让启动失败能反馈到 main.go
3. 写一个假 echo 后端服务，验证「客户端 → 网关 → 后端 → 客户端」连接能通
