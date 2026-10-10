# API Gateway 开发周期规划

配套 [API Gateway.drawio.svg](./API%20Gateway.drawio.svg) 架构图使用。每完成一个版本，把对应的复选框打勾，方便追踪当前进度。

## 项目简介

> 这一节写给第一次接触本项目的人或 AI，用于快速建立上下文。

本项目是一个**从零手写的 API 网关**，Go 语言实现（module `API_Gateway`，go 1.26.4），**只用标准库**，自己在 `net.Conn` 上管理连接生命周期，不交给 `http.Server` 接管。

它要解决的问题是：客户端统一讲 HTTP，后端却可能讲 gRPC、MCP 或别的协议。网关站在中间，负责**接入 → 准入控制 → 请求处理 → 协议翻译 → 转发 → 原路返回**。计划中的能力包括：连接级限流、鉴权（JWT）、HTTP 级限流、路由转发、协议转换、负载均衡、后端连接池与健康检查。

### 一次请求的完整旅程

```
客户端 --HTTP--> [connector 连接层] --> [handler 处理层] --> [backend 后端层]
                  监听/接受连接         解析/过滤              查表/选实例/借出连接
                  连接级准入           ↕ transformer ↕         连接池管理
                                      拿到后端连接后           ↑ 归还连接
                                      在连接上完成协议交互     |
                                      (写请求帧+读响应帧)  ---+
```

handler 拿到 backend 借出的后端连接后，交给 transformer 在连接上完成完整的协议交互（写请求帧 + 读响应帧），再翻译回 HTTP 写回客户端连接。逐环节的类型流转见下文「数据流」一节。

### 包结构与职责

| 包 | 职责 | 关键类型 |
|---|---|---|
| `main.go` | **唯一的编排点**，按逆序构造各模块并接线，然后启动监听 | — |
| `builder/` | 读配置、构建运行时配置表。**纯数据包**，不 import 任何运行时模块 | `Config` 及各子 Config |
| `connector/` | 监听端口、Accept 连接、跑连接级过滤器，再把 conn 交给下游 | `Listener`、`Connector` |
| `connector/tcp_filter/` | 连接级过滤器（连接数限流等），只做准入判断 | `TCPFilter` |
| `handler/` | 处理层主流程：解码 → 过滤 → 借后端连接 → transformer 协议交互 → 编码写回 | `HTTPHandler`、`Decoder`、`Encoder` |
| `handler/message/` | **网关唯一的中间表示**（统一 HTTP 模型），叶子包，不依赖任何包 | `Request`、`Response` |
| `handler/http_filter/` | 请求级过滤器：鉴权、限流、路由。可放行或中断并直接产出响应 | `HTTPFilter` |
| `handler/transformer/` | 协议转换，每个实现代表一种后端协议。拿到后端连接后全权负责写请求帧、读响应帧、反序列化，协议细节全部封在实现内部 | `Transformer` |
| `backend/` | 服务注册表、负载均衡选实例、连接池管理（借出/归还），不接触协议字节 | `BManager`、`Service`、`Instance`、`LoadBalancer` |
| `backend/pool/` | **连接池实现**：容量/空闲/超时管理、失效探测、reaper 回收；不碰协议 | `ConnPool`、`PoolConn` |

依赖方向单向无环：`main → {connector, handler, backend}`，三个顶层模块**互不 import**，靠 main.go 注入函数值接线；`message` 是被 `http_filter` 和 `transformer` 共享的叶子包。

### 当前状态

**v00-v06 已完成**（路由/配置链路 + 连接池 + release 回调 + 帧定界 transformer）。全部包和接口已就位，`go build ./...`、`go vet ./...`、`go test ./...` 全部通过（含 `-race`）。

相对 v04 的关键变化：Router 真实实现（最长前缀匹配 + Host 分发 + Method 守卫，`req.Service` 传递路由结果）；配置加载链路完整打通（`loader.go` 读 YAML → `hydrate.go` 类型水合 → `compile.go` 编译为运行态），`Build(configDir)` 串联全流程，支持多租户 `gateway.yaml` + `platform.yaml` 双文件配置；`ErrorResponse` 新增 404 映射；通用工具函数抽取到 `pkg/tools/`（`ToInt`、`StringsOverlap`、`StringsContain`、`LoadYAML`）。

相对 v05 的关键变化（v06 连接抽象）：连接池独立成 `backend/pool/` 子包并填实——`Call` 从池借出（`net.Dial` → `pool.Get`）、`next` 升级为 `(service) (net.Conn, func(error), error)`、handler 在 `Transform` 返回后 `release(err)`（成功 `Put`、失败关连接）；第一个 transformer 真实实现改为**配置驱动的帧定界协议**（4 字节长度前缀 + `io.ReadFull`，替换 half-close），并去掉 `defer conn.Close()`；transformer 按 service 绑定（`platform.yaml` 的 `transformer:` → `Compile` 反向聚合 + 校验）；fakebackend 同步升级为帧循环，支持同一后端连接连续服务多请求。

仍保留的边界：`Call` 写死 `instances[0]`，负载均衡未接（`LoadBalancer` 接口待用）；transformer 帧协议为长度前缀的**模拟协议**（非真实 gRPC/MCP）；连接池下一版把等待/唤醒升级为 channel + select（见「下一步」）。

**接手前请务必先读「架构约定」一节**——那是反复讨论后定下的边界（模块间怎么解耦、协议知识关在哪、连接池归谁、响应从哪出去），从代码本身看不出来，但改动时必须遵守。「已知待办」记录了已识别但尚未处理的问题，不必当成 bug 重复上报。

## 里程碑一览

| 版本 | 里程碑 | 新增能力 |
|---|---|---|
| v00 | 项目初始化 | `main.go` 入口 + 阶段0定义的接口/数据模型骨架（空实现） |
| v01 | 网络骨架 | connector 最简监听（单 goroutine 处理一个连接）+ 假后端 echo 服务能联通 |
| v02 | 端到端打通 | handler 主流程骨架跑通（decoder/filter/router/transformer 全部假实现），完整链路能返回假数据 |
| v03 | 连接层填实 | tcp_filter 编排能力（至少一个连接级限流）+ 字节流处理健壮性 |
| v04 | 请求解析 | decoder 真正产出结构化 HTTP 请求（优先用 `http.ReadRequest`，不接管连接） |
| v05 | 路由能力 | router 真实实现，支持配置文件定义多条路由规则 |
| v06 | 连接抽象 | ConnPool 落地 + 第一个 transformer 真实实现（比如先做 gRPC） |
| v07 | 安全认证 | 鉴权 filter 真实逻辑（JWT 校验） |
| v08 | 管道细化 | filter chain 可配置化（不同路由挂不同 filter 组合）+ HTTP 层精细限流 |
| v09 | 协议扩展 | 第二个 transformer 实现（验证新增协议不改已有代码） |
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

> 最后更新：2026-10-10　｜　`go build ./...`、`go vet ./...`、`go test ./...` 全部通过（含 `-race`）

- [x] **v00 项目初始化** —— 完成，数据流已闭合
  - [x] `main.go`：按逆序完成编排（backend → handler → connector）
  - [x] `builder/`：`config.go` 配置结构 + `build.go` 入口（字段仍为占位注释）
  - [x] `connector/`：`model.go`（`Connector` 接口）、`listener.go`（Accept 循环 + `next` 注入）、`tcp_filter/filter.go`（`TCPFilter` 接口）
  - [x] `handler/`：`model.go`（`Handler` 接口）、`handler.go`（`HTTPHandler` + `Process`/`HandleHTTPConn` + `F2T`/`T2F`）、`decoder.go`、`encoder.go`
  - [x] `handler/message/`：网关统一的 `Request`/`Response` 模型
  - [x] `handler/http_filter/filter.go`、`handler/transformer/transformer.go`：接口定义，`Transformer.Transform(req, conn)` 全权负责在后端连接上完成协议交互
  - [x] `backend/`：`model.go`（`Backend`/`Service`/`Instance`）、`backend.go`（`BManager`，`Call` 只借出连接）、`pool.go`
- [x] **v01 网络骨架** —— 完成，connector 最简监听 + 假后端 echo 服务联通
  - [x] `ConnectorConfig` 加监听端口字段，`NewListener` 把 `addr` 真正赋上
  - [x] `Connector` 接口 + `Listener.Connect()` 补 error 返回，`net.Listen` 失败反馈到 main.go
  - [x] `main.go` 接住启动 error，修掉 `backend`/`handler` 变量名遮蔽包名
  - [x] 假 echo 后端验证「客户端 → 网关 → 后端 → 客户端」连接能通
- [x] **v02 端到端打通** —— 完成，完整链路能返回 echo 数据
  - [x] `ServiceConfig` 加 `Name`/`Instances`/`Addr`，`NewBManager` 建表，`Call` 返回后端连接（当前仍为每次 Dial 新建），网关不硬编码后端地址
  - [x] `handler.Process` 跑通主流程骨架：decode → filters（假实现）→ next(Call) 借连接 → transformer.Transform(req, conn) → encode
  - [x] decoder / encoder / filter / transformer 全部假实现，完整链路能返回数据
  - [x] 搭建集成 testbed（`testbed/e2e_test.go` + `fakebackend`），验证「客户端 → 网关 → 后端 → 客户端」端到端返回 echo 数据
- [x] **v03 连接层填实** —— 完成，连接级限流 + 字节流健壮性落地
  - [x] 通用错误包 `pkg/errors`（gerrors）：哨兵错误按 connector/handler/backend 分域，纯 `error` 可直接返回；配套 `pkg/constant`（gconst）定义共享常数（监听端口、读缓冲、读超时、后端缓冲）
  - [x] tcp_filter 落地：`LimitFilter`（原子计数 + 上限判断 + 超限回滚计数 + `OnCloseConn` 归还）；`Listener.Process` 按配置顺序编排 filters，准入失败直接 return、由 `defer conn.Close()` 统一收尾；`NewListener` 从 `ConnectorConfig.Filters` 读取，`BuildTCPFilters` 支持 `Enable` 开关
  - [x] 字节流健壮性：`Handler.Process` 改用 `http.ReadRequest`（半包/粘包由标准库接管），decoder 读请求体到 `Raw`；`HandleHTTPConn` 补 keep-alive 退出判断 + 错误分类（`closeConnOrNot`：EOF / `net.ErrClosed` / 超时 / `*net.OpError` 穿透 syscall），连接层错误直接断连，业务错误转 `message.ErrorResponse(err)` 写回后继续循环
  - [x] 测试：`limit_filter_test.go` 三个单元测试 + testbed 升级为真实 HTTP 协议、新增 `TestEchoConcurrentLimit` 多连接并发用例，全部通过（含 `-race`）
- [x] **v04 请求解析** —— 完成，decoder 产出结构化请求 + keep-alive 落地
  - [x] `message.Request` 结构化：新增 `Method/URL/Header/Host/ContentLength/Body`（`Proto` 保留），叶子包零依赖；`Response` 同步结构化（StatusCode/Header/Body/Proto/ContentLength）
  - [x] `Decoder.Decode` 从 `*http.Request` 完整映射；body 边界：`ContentLength` 预检 + `LimitReader(maxBody+1)` 双重限制（chunked `ContentLength=-1` 也能拦截）、读失败转 `ErrDecodeRequest`、入口 `defer req.Body.Close()`
  - [x] encoder 真实编码：状态行 + headers + Content-Length（由编码器统一计算）；`ErrorResponse` 按错误域映射状态码（413/400/403/502），`message` 保持零依赖
  - [x] keep-alive：`req.Close` → `ch.close`（覆盖 `Connection: close` 与 HTTP/1.0 默认短连接），同一条连接连续处理多请求
  - [x] 测试：`decoder_test.go` 八个单元测试（字段映射、body 临界值、chunked 超限、体长不符、默认 maxBody、无 body）+ testbed 新增 `TestEchoKeepAlive`（同连接 3 个请求 + `Connection: close` 后 EOF），全部通过（含 `-race`）
- [x] **v05 路由能力** —— 完成，配置驱动路由 + 配置加载链路打通
  - [x] Router 真实实现：`http_filter/router.go`，最长前缀匹配 + Host 分发 + Method 守卫，排序保证特定域名优先于通配；路由结果通过 `req.Service` 字段传递，`Process` 消费
  - [x] 配置加载链路：`loader.go`（Load 读 YAML）→ `hydrate.go`（Hydrate 按插件注册表将 `map[string]any` 转具体类型）→ `compile.go`（Compile 压平路由/服务、校验、装配）→ `build.go`（`Build(configDir)` 串联全流程）
  - [x] 多租户配置：`gateway.yaml`（网关自身）+ `platform.yaml`（平台/服务/路由），`qualifiedServiceName` 以 `"platform.service"` 隔离跨平台同名服务
  - [x] 未命中路由：`ErrRouteNotFound` + `ErrorResponse` 映射 404，业务错误写回后 keep-alive 继续
  - [x] 通用工具抽取：`pkg/tools/`（`conv.go` / `slice.go` / `yaml.go`）
  - [x] 测试：`router_test.go` 五个单元测试（域名+路径联合匹配、未命中、方法守卫、特定域名优先、规则校验）+ `compile_test.go` 十三个编译测试 + testbed `route_test.go` 两个端到端用例（路径分发 + 多租户 Host 分发）
- [x] **v06 连接抽象** —— 完成，连接池落地 + release 回调 + 帧定界 transformer + 复用验证
  - [x] `backend/pool/` 子包：`ConnPool`（`MaxConn` 令牌 + `MinIdle` 预热 + 借出/空闲超时 + reaper 定时回收 + `discard` 唯一关闭路径）、`probe.go`（非阻塞零字节读失效探测）、`reaper.go`（超时回收/过期回收/保底探活/预热）
  - [x] release 回调：`next` 升级为 `(service) (net.Conn, func(error), error)`；handler `defer release(err)`；成功 `Put`、失败 `Close`
  - [x] 帧定界 transformer：4 字节长度前缀 + `io.ReadFull`，替换 half-close 并去掉 `defer conn.Close()`；transformer 从硬编码 `"testT"` 改为按 service 配置驱动（`Compile` 反向聚合 + 校验）
  - [x] fakebackend 从裸字节 echo 升级为帧循环，一条后端连接连续服务多请求
  - [x] 配置：`gateway.pool` 解析/校验/派生；每个 service 必须绑定 transformer（编译期校验）
  - [x] 测试：`backend/pool/pool_test.go`（借出/复用/超限/失效/关闭/并发）+ `testbed/reuse_test.go`（`MaxConn=1` 断言后端仅建 1 条连接）
- [ ] v07 安全认证
- [ ] v08 管道细化
- [ ] v09 协议扩展
- [ ] v10 最终完善

## 数据流（已闭合）

```
clientConn → Decode → message.Request → filters
           → next(service) → backendConn           （backend 借出连接）
           → transformer.Transform(req, backendConn)（在后端连接上完成协议交互）
           → message.Response → Encode → clientConn.Write
```

## 架构约定（已定，后续照此执行）

1. **模块独立 + main.go 编排**：模块间不互相 import，跨模块函数签名只用标准库类型（`func(net.Conn) error`、`func(service string) (net.Conn, error)`）。`builder` 是唯一例外——纯数据包，只用于构造期，自身不 import 任何运行时模块。
2. **初始化逆序 = 数据流逆序**：backend → handler → connector。构造注入保证依赖图无环。
3. **`handler/message` 是唯一中间表示**：decoder / filter / transformer / encoder 四方共用，禁止在其上再摞第二层中间模型。
4. **backend 只认 `(service string) → (net.Conn, error)`**，对协议一无所知，只负责借出后端连接；协议 IO（写请求帧 + 读响应帧）全由 transformer 在连接上完成——这是「新增协议不改已有代码」的前提。
5. **路由与负载均衡两级映射**：filter 定「请求→服务」，backend 定「服务→实例」。拆开是为重试时换实例不重跑 filter 链（避免重复鉴权、重复扣配额）。
6. **一条 conn 一个 goroutine，filter 对象全局共享**，filter 内部状态须自保并发安全。
7. **连接池 / transformer 职责分离**：backend 只管连接生命周期（借出、归还、池化），transformer 全权负责在连接上完成协议交互（写帧 + 读帧 + 反序列化）。长连接复用依赖协议帧定界，帧定界由 transformer 实现——因此 ConnPool 归还机制与 transformer 帧定界必须联动：`next` 已加 release 回调（`func(service string) (net.Conn, func(error), error)`），transformer 不再自行关闭连接，由 handler 在 Transform 返回后通过 release 归还或丢弃。

## 已知待办（不影响结构，填肉时处理）

- ~~`HandleHTTPConn`：错误路径需转成 `message.ErrorResponse(err)` 写回，而不是直接 `return err`~~ —— 已解决（v03 已写回 + `closeConnOrNot` 分类断连/写回）
- ~~`HandleHTTPConn`：`for` 循环缺 keep-alive 退出判断（目前仅以 EOF/err 结束生命周期）~~ —— 已解决（v03 已补 EOF 退出 + 错误分类）
- ~~`message.ErrorResponse`：目前返回空响应~~ —— 已解决（v04 已产出合法 HTTP 响应，按错误域映射 413/400/403/502；错误处理统一仍留 v10 走查）
- ~~testbed 响应侧仍按透传读回、客户端需升格 `http.ReadResponse`~~ —— 已解决（v04 编码器产出真实响应帧后已升格）
- `ErrBodyTooLarge`：目前归入「边界不可信错误」直接断连、不写 413；若后续想给客户端明确反馈，可在断连前写一次 413 —— 与 v10「错误处理统一」相关
- filter 短路响应路径（filter 拒绝时直接产出响应写回、跳过后续链路）留 v07/v08 落地；编码失败（`errE`）时的断连策略留 v10 走查
- ~~`conn.Write` 返回值未检查~~ —— 已解决（v02 已检查返回值）
- ~~`Decode` 缺 error 返回~~ —— 已解决（v02 骨架已带 error 返回）
- ~~`builder` 的 `HandlerConfig.filter/transformer`、`BackendConfig.service` 仍是小写~~ —— 已解决（字段已全部导出，配置可反序列化）

## 测试基础设施

- 方向：集成 testbed（`go test` 自动化），不依赖手动脚本/独立二进制。
- 假后端作为测试 helper（goroutine 监听真实端口），行为/协议可配置，越易配置越好。
- 网关通过配置注入拿到后端实例地址，不硬编码——测试代码启动假后端、构造 `builder.Config`、用构造函数组装网关，`main.go` 不被测试侵入。
- 假后端协议和 transformer 绑定演进：现为 **4 字节长度前缀帧**（模拟协议，fakebackend 与 transformer 对称实现）；后续接真实协议（gRPC/MCP）时替换实现。
- 行为模式先做 `echo` + `fixed`，`error`/`delay` 等做到重试/熔断（v03/v06）时再加。

## 下一步（连接池升级：channel + select）

v06 已让连接池落地并跑通长连接复用。当前池用 `slots chan struct{}`（容量令牌）+ `idle []*PoolConn` 切片 + `sync.Cond`/`Broadcast` 做「池满等待 + 空闲唤醒」。`Broadcast` 会**惊群**（一次唤醒全部等待者），在热点池 + 突发流量下放大 CPU/调度/GC 开销，且等待者数量不受池容量约束。下一步把等待/唤醒升级为 **channel + select**，去掉 `Cond`：

1. 目标
   1. 消除惊群：资源归还只唤醒一个等待者；超时在等待者本地处理，不跨等待者广播
   2. 用 channel 的阻塞队列语义替代手写等待队列 + `sync.Cond`
   3. 对外 API（`Get`/`Put`/`Close`/`PoolConn`）与 `backend`/`builder` 保持不变
2. 设计（待细化）
   1. `idle` 由切片改为 `idle chan *PoolConn`（channel 装资源），容量 = `MaxConn`；`Get` 用 `select { case c := <-idle: ...; case <-slots: Dial; case <-timer.C: ErrPoolExhausted; case <-done: ErrPoolClosed }`
   2. `Put` 非阻塞 send 回 `idle`（满 / 已关闭则关连接并归还令牌）；令牌释放作为「可新建」的唤醒信号
   3. 超时/取消改用等待者本地 `time.NewTimer(remaining)`，移除 `time.AfterFunc` 广播回调
3. 连带改动
   1. reaper 无法对 channel 按下标 peek：`reapExpiredIdle` / `probeIdle` / 预热需改为「锁内排空 idle → 按 `idleSince` 过滤 → 关死的 / 放回活的」，并注意与并发 `Get`/`Put` 的交互
4. 影响范围与前置
   1. 主要改 `backend/pool/pool.go` 与 `backend/pool/reaper.go`（约 200–270 行），`probe.go` 不动
   2. 属优化、不在关键路径；建议独立分支（如 `v06/connPool-channel`）进行
   3. 以现有 `backend/pool/pool_test.go` 为行为契约，重构前后全绿（含 `-race`）
