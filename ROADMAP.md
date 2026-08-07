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

> 最后更新：2026-08-07　｜　`go build ./...`、`go vet ./...` 通过

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

1. **模块独立 + main.go 编排**：模块间不互相 import，跨模块函数签名只用标准库类型（`func(net.Conn) error`、`func(service string, payload []byte) ([]byte, error)`）。`builder` 是唯一例外——纯数据包，只用于构造期，自身不 import 任何运行时模块。
2. **初始化逆序 = 数据流逆序**：backend → handler → connector。构造注入保证依赖图无环。
3. **`handler/message` 是唯一中间表示**：decoder / filter / transformer / encoder 四方共用，禁止在其上再摞第二层中间模型。
4. **backend 只认 `(service string, payload []byte)`**，对协议一无所知；协议转换全在 handler 内完成——这是「新增协议不改已有代码」的前提。
5. **路由与负载均衡两级映射**：filter 定「请求→服务」，backend 定「服务→实例」。拆开是为重试时换实例不重跑 filter 链（避免重复鉴权、重复扣配额）。
6. **一条 conn 一个 goroutine，filter 对象全局共享**，filter 内部状态须自保并发安全。
7. **`Call` / 连接池 / transformer 三者绑定演进**：长连接复用需要协议边界，协议边界由 transformer 定义，而 backend 对协议一无所知——连接池不能脱离 transformer 先做。现阶段（v01-v05）`Call` 用短连接 + half-close（`CloseWrite` + 读 EOF）；v06 transformer 落地后，`Call` + `ConnPool` + 协议帧定界一起升级为长连接版。

## 已知待办（不影响结构，填肉时处理）

- `HandleHTTPConn`：错误路径需转成 `message.ErrorResponse(err)` 写回，而不是直接 `return err`
- `HandleHTTPConn`：`for` 循环缺退出条件（EOF 正常结束 / 非 keep-alive / 写失败），`conn.Write` 返回值未检查
- `Decode` 缺 error 返回；返回值为值类型，而 filter 收指针
- `builder` 的 `HandlerConfig.filter/transformer`、`BackendConfig.service` 仍是小写，反序列化填不进去

## 测试基础设施

- 方向：集成 testbed（`go test` 自动化），不依赖手动脚本/独立二进制。
- 假后端作为测试 helper（goroutine 监听真实端口），行为/协议可配置，越易配置越好。
- 网关通过配置注入拿到后端实例地址，不硬编码——测试代码启动假后端、构造 `builder.Config`、用构造函数组装网关，`main.go` 不被测试侵入。
- 假后端协议和 transformer 绑定演进：现阶段裸字节 echo（half-close 定界），v06 起挂 gRPC handler（协议帧定界）。
- 行为模式先做 `echo` + `fixed`，`error`/`delay` 等做到重试/熔断（v03/v06）时再加。

## 下一步（v02 端到端打通）

1. 配置注入：`ServiceConfig` 加 `Name`/`Addr`，`NewBManager` 建表，`Call` 恢复短连接网络转发（dial → half-close → 读 EOF），网关不硬编码后端地址
2. `handler.Process` 跑通主流程骨架：decode → filters（假实现）→ transform → next(Call) → restore → encode
3. decoder / encoder / filter / transformer 全部假实现，完整链路能返回数据
4. 搭建集成 testbed，验证「客户端 → 网关 → 后端 → 客户端」端到端返回 echo 数据
