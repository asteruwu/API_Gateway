# API Gateway 开发周期规划

配套 [API Gateway.drawio.svg](./API%20Gateway.drawio.svg) 架构图使用。每完成一个版本，把对应的复选框打勾，方便追踪当前进度。

## 项目简介

> 这一节写给第一次接触本项目的人或 AI，用于快速建立上下文。

本项目是一个**从零手写的 API 网关**，Go 语言实现（module `API_Gateway`，go 1.26.4），**只用标准库**，自己在 `net.Conn` 上管理连接生命周期，不交给 `http.Server` 接管。

它要解决的问题是：客户端统一讲 HTTP，后端却可能讲 gRPC、MCP 或别的协议。网关站在中间，负责**接入 → 准入控制 → 请求处理 → 协议翻译 → 转发 → 原路返回**。计划中的能力包括：连接级限流、鉴权（JWT）、HTTP 级限流、路由转发、协议转换、负载均衡、后端连接池与健康检查。

### 一次请求的完整旅程

```
客户端 --HTTP--> [connector 连接层] --> [handler 处理层] --> [backend 后端层] --协议字节--> 后端服务
                  监听/接受连接         解析/过滤/协议转换      查表/选实例/取连接
                  连接级准入                                   转发并读回响应
```

响应沿原路返回，在 handler 内被翻译回 HTTP 并写回同一条连接。逐环节的类型流转见下文「数据流」一节。

### 包结构与职责

| 包 | 职责 | 关键类型 |
|---|---|---|
| `main.go` | **唯一的编排点**，按逆序构造各模块并接线，然后启动监听 | — |
| `builder/` | 读配置、构建运行时配置表。**纯数据包**，不 import 任何运行时模块 | `Config` 及各子 Config |
| `connector/` | 监听端口、Accept 连接、跑连接级过滤器，再把 conn 交给下游 | `Listener`、`Connector` |
| `connector/tcp_filter/` | 连接级过滤器（连接数限流等），只做准入判断 | `TCPFilter` |
| `handler/` | 处理层主流程：解码 → 过滤 → 协议转换 → 调后端 → 还原 → 编码写回 | `HTTPHandler`、`Decoder`、`Encoder` |
| `handler/message/` | **网关唯一的中间表示**（统一 HTTP 模型），叶子包，不依赖任何包 | `Request`、`Response` |
| `handler/http_filter/` | 请求级过滤器：鉴权、限流、路由。可放行或中断并直接产出响应 | `HTTPFilter` |
| `handler/transformer/` | 协议转换，每个实现代表一种后端协议。协议细节全部封在实现内部 | `Transformer` |
| `backend/` | 服务注册表、负载均衡选实例、连接池、实际转发 | `BManager`、`Service`、`Instance`、`LoadBalancer`、`ConnPool` |

依赖方向单向无环：`main → {connector, handler, backend}`，三个顶层模块**互不 import**，靠 main.go 注入函数值接线；`message` 是被 `http_filter` 和 `transformer` 共享的叶子包。

### 当前状态

**端到端链路已打通（v00-v02 已完成）。** 全部包和接口已就位，`go build ./...`、`go vet ./...`、`go test ./...` 全部通过，集成 testbed 已验证「客户端 → 网关 → 后端 → 客户端」能返回 echo 数据。当前各模块仍是假实现（decoder/filter/transformer/encoder 只是透传原始字节），但数据流已经闭合：从 `net.Conn` 进、经完整链路到后端、再原路返回写回 conn，每一环的入参出参都对得上，后续是在真实协议和健壮性上往里填肉。

**接手前请务必先读「架构约定」一节**——那 9 条是反复讨论后定下的边界（模块间怎么解耦、协议知识关在哪、连接池归谁、响应从哪出去），从代码本身看不出来，但改动时必须遵守。「已知待办」记录了已识别但尚未处理的问题，不必当成 bug 重复上报。

## 里程碑一览

| 版本 | 里程碑 | 新增能力 |
|---|---|---|
| v00 | 项目初始化 | `main.go` 入口 + 阶段0定义的接口/数据模型骨架（空实现） |
| v01 | 网络骨架 | connector 最简监听（单 goroutine 处理一个连接）+ 假后端 echo 服务能联通 |
| v02 | 端到端打通 | handler 主流程骨架跑通（decoder/filter/router/transformer 全部假实现），完整链路能返回假数据 |
| v03 | 连接层填实 | tcp_filter 编排能力（至少一个连接级限流）+ 字节流处理健壮性 |
| v04 | 请求解析 | decoder 真正产出结构化 HTTP 请求（优先用 `http.ReadRequest`，不接管连接） |
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

> 最后更新：2026-08-16　｜　`go build ./...`、`go vet ./...`、`go test ./...` 全部通过

- [x] **v00 项目初始化** —— 完成，数据流已闭合
  - [x] `main.go`：按逆序完成编排（backend → handler → connector）
  - [x] `builder/`：`config.go` 配置结构 + `build.go` 入口（字段仍为占位注释）
  - [x] `connector/`：`model.go`（`Connector` 接口）、`listener.go`（Accept 循环 + `next` 注入）、`tcp_filter/filter.go`（`TCPFilter` 接口）
  - [x] `handler/`：`model.go`（`Handler` 接口）、`handler.go`（`HTTPHandler` + `Process`/`HandleHTTPConn` + `F2T`/`T2F`）、`decoder.go`、`encoder.go`
  - [x] `handler/message/`：网关统一的 `Request`/`Response` 模型
  - [x] `handler/http_filter/filter.go`、`handler/transformer/transformer.go`：接口定义，`Transformer` 出向 `Transform` / 回程 `Restore` 双向齐备，直接收发 `*message.Request`/`*message.Response`
  - [x] `backend/`：`model.go`（`Backend`/`Service`/`Instance`）、`backend.go`（`BManager`）、`forwarder.go`、`pool.go`
- [x] **v01 网络骨架** —— 完成，connector 最简监听 + 假后端 echo 服务联通
  - [x] `ConnectorConfig` 加监听端口字段，`NewListener` 把 `addr` 真正赋上
  - [x] `Connector` 接口 + `Listener.Connect()` 补 error 返回，`net.Listen` 失败反馈到 main.go
  - [x] `main.go` 接住启动 error，修掉 `backend`/`handler` 变量名遮蔽包名
  - [x] 假 echo 后端验证「客户端 → 网关 → 后端 → 客户端」连接能通
- [x] **v02 端到端打通** —— 完成，完整链路能返回 echo 数据
  - [x] `ServiceConfig` 加 `Name`/`Instances`/`Addr`，`NewBManager` 建表，`Call` 恢复短连接网络转发（dial → half-close → 读 EOF），网关不硬编码后端地址
  - [x] `handler.Process` 跑通主流程骨架：decode → filters（假实现）→ transform → next(Call) → restore → encode
  - [x] decoder / encoder / filter / transformer 全部假实现，完整链路能返回数据
  - [x] 搭建集成 testbed（`testbed/e2e_test.go` + `fakebackend`），验证「客户端 → 网关 → 后端 → 客户端」端到端返回 echo 数据
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

1. **模块独立 + main.go 编排**：模块间不互相 import，跨模块函数签名只用标准库类型（`func(net.Conn) error`、`func(service string, payload []byte) ([]byte, error)`）。`builder` 是唯一例外——纯数据包，只用于构造期，自身不 import 任何运行时模块。
2. **初始化逆序 = 数据流逆序**：backend → handler → connector。构造注入保证依赖图无环。
3. **`handler/message` 是唯一中间表示**：decoder / filter / transformer / encoder 四方共用，禁止在其上再摞第二层中间模型。
4. **backend 只认 `(service string, payload []byte)`**，对协议一无所知；协议转换全在 handler 内完成——这是「新增协议不改已有代码」的前提。
5. **路由与负载均衡两级映射**：filter 定「请求→服务」，backend 定「服务→实例」。拆开是为重试时换实例不重跑 filter 链（避免重复鉴权、重复扣配额）。
6. **一条 conn 一个 goroutine，filter 对象全局共享**，filter 内部状态须自保并发安全。
7. **`Call` / 连接池 / transformer 三者绑定演进**：长连接复用需要协议边界，协议边界由 transformer 定义，而 backend 对协议一无所知——连接池不能脱离 transformer 先做。现阶段（v01-v05）`Call` 用短连接 + half-close（`CloseWrite` + 读 EOF）；v06 transformer 落地后，`Call` + `ConnPool` + 协议帧定界一起升级为长连接版。

## 已知待办（不影响结构，填肉时处理）

- `HandleHTTPConn`：错误路径需转成 `message.ErrorResponse(err)` 写回，而不是直接 `return err`
- `HandleHTTPConn`：`for` 循环缺 keep-alive 退出判断（目前仅以 EOF/err 结束生命周期）
- ~~`conn.Write` 返回值未检查~~ —— 已解决（v02 已检查返回值）
- ~~`Decode` 缺 error 返回~~ —— 已解决（v02 骨架已带 error 返回）
- ~~`builder` 的 `HandlerConfig.filter/transformer`、`BackendConfig.service` 仍是小写~~ —— 已解决（字段已全部导出，配置可反序列化）

## 测试基础设施

- 方向：集成 testbed（`go test` 自动化），不依赖手动脚本/独立二进制。
- 假后端作为测试 helper（goroutine 监听真实端口），行为/协议可配置，越易配置越好。
- 网关通过配置注入拿到后端实例地址，不硬编码——测试代码启动假后端、构造 `builder.Config`、用构造函数组装网关，`main.go` 不被测试侵入。
- 假后端协议和 transformer 绑定演进：现阶段裸字节 echo（half-close 定界），v06 起挂 gRPC handler（协议帧定界）。
- 行为模式先做 `echo` + `fixed`，`error`/`delay` 等做到重试/熔断（v03/v06）时再加。

## 下一步（v03 连接层填实）

1. tcp_filter 落地
   1. 连接数限流 filter 实现（原子计数 + 上限判断）
   2. `Listener.Process` 编排 filters：先跑连接级准入，失败直接拒绝并关闭 conn
   3. `NewListener` 从 `ConnectorConfig` 读取限流配置
2. 字节流处理健壮性（为 v04 `http.ReadRequest` 打基础）
   1. `Handler.Process` 去掉 `io.ReadAll`，改用 `bufio.Reader` 按帧读取
   2. 处理半包 / 粘包 / 空读 / 异常关闭
   3. `HandleHTTPConn` 补 keep-alive 退出判断，错误路径转 `message.ErrorResponse(err)` 写回
3. 测试：tcp_filter 限流单元测试 + testbed 增加多连接并发用例
