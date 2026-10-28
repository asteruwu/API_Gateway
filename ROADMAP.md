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

**v00-v07 已完成**（路由/配置链路 + 连接池 + release 回调 + 帧定界 transformer + 鉴权 filter）。全部包和接口已就位，`go build ./...`、`go vet ./...`、`go test ./...` 全部通过（含 `-race`）。

**v08 进行中**：阶段 A–C 已完成，下一步 D（HTTP 限流 filter）。当前 `Build` 链路为 `Load → Calculate(单次 Plan) → ValidateWriting → Hydrate → ValidateRuntime → Compile(gw, plan)`；`Calculate` 产出统一 `Plan`，`Compile` 只做装配。C 阶段 handler 已切换为按 `RouteID` 取 pipeline 执行，旧 `Filters` 已删除。

相对 v04 的关键变化：Router 真实实现（最长前缀匹配 + Host 分发 + Method 守卫，`req.Service` 传递路由结果）；配置加载链路完整打通（`loader.go` 读 YAML → `hydrate.go` 类型水合 → `compile.go` 编译为运行态），`Build(configDir)` 串联全流程，支持多租户 `gateway.yaml` + `platform.yaml` 双文件配置；`ErrorResponse` 新增 404 映射；通用工具函数抽取到 `pkg/tools/`（`ToInt`、`StringsOverlap`、`StringsContain`、`LoadYAML`）。

相对 v05 的关键变化（v06 连接抽象）：连接池独立成 `backend/pool/` 子包并填实——`Call` 从池借出（`net.Dial` → `pool.Get`）、`next` 升级为 `(service) (net.Conn, func(error), error)`、handler 在 `Transform` 返回后 `release(err)`（成功 `Put`、失败关连接）；第一个 transformer 真实实现改为**配置驱动的帧定界协议**（4 字节长度前缀 + `io.ReadFull`，替换 half-close），并去掉 `defer conn.Close()`；transformer 按 service 绑定（`platform.yaml` 的 `transformer:` → `Compile` 反向聚合 + 校验）；fakebackend 同步升级为帧循环，支持同一后端连接连续服务多请求。

相对 v06 的关键变化（v07 安全认证）：`handler/http_filter/auth/` 落地——`Verifier` 凭证抽象 + JWT(HS256) 实现（`exp/nbf/iss/aud/leeway` + keycloak/azure/generic IdP 布局），`AuthFilter` 四段编排（提取 → 认证 → 授权 → 身份回写），`public` 匿名豁免、认证失败 401（带 `WWW-Authenticate: Bearer`）/授权失败 403、入站 `X-Gateway-*` 保留头剥离；`buildFilter` 接入 `AuthFilterConfig`，filter 短路打通；示例配置启用 `auth` 且每条 route 逐条声明。同时完成安全加固：Router 路径/Host 归一化与前缀段边界校验，handler 写响应超时、错误 body 去敏感（对外只回状态文案）、单请求读入量上限（1MB）。

相对 v07 的关键变化（v08 B 阶段）：编写态拆成 `plugins` 注册表与 `enabled` / route `filters` 激活名单；`Calculate` 单次遍历产出 `Plan`（http filter 并集、路由、服务）；校验拆为 `ValidateWriting`（声明/Plan 不变量）与 `ValidateRuntime`（水合后插件语义）；`Compile(gw, plan)` 只装配运行态，不再遍历 platforms。

相对 v08 B 的关键变化（v08 C 阶段）：handler 切换为 Router 先行 + 按 `RouteID` 取 pipeline 执行（`buildFilter` 从 `Configs` 构造名字→实例表，`buildPipelines` 预解析为 `map[RouteID][]HTTPFilter`）；pipeline 缺失 = `ErrPipelineNotFound`（fail-closed）；旧 `HTTPFilterConfig.Filters` 字段及 `Compile` 中填充它的代码已删除。

仍保留的边界：`Call` 写死 `instances[0]`，负载均衡未接（`LoadBalancer` 接口待用）；transformer 帧协议为长度前缀的**模拟协议**（非真实 gRPC/MCP）；v07 认证仅 HS256 + 单一静态密钥（多凭证机制/注册表、多 IdP、JWKS、RS256、有状态 verifier 均未做，见「明确不在 v07」）。连接池等待/唤醒的重做已在本版落地（见 v06 进度末条），不再是待办。

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
| v08 | 管道细化 | filter chain 可配置化（不同路由挂不同 filter 组合）+ HTTP 层精细限流 + 负载均衡 |
| v09 | 协议扩展 | 第二个 transformer 实现（验证新增协议不改已有代码） |
| v10 | 最终完善 | 错误处理统一、响应路径补全走查、代码整理，可运行版本 |

## 当前进度

> 最后更新：2026-10-27　｜　`go build ./...`、`go vet ./...`、`go test ./... -race` 全部通过。

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
  - [x] encoder 真实编码：状态行 + headers + Content-Length（由编码器统一计算）；`ErrorResponse` 按错误域映射状态码（403/404/502/503），`message` 保持零依赖（注：413/400 走不到 `ErrorResponse`——请求边界已不可信的错误直接断连，是 v10「错误处理统一」的待办）
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
  - [x] 连接池并发内核重做（分支 `v06/connPool-waiter-handoff`）：去掉 `sync.Cond` + `slots` 令牌 channel，改为「锁保护的空闲切片（可下标扫描）+ `numOpen` 配额 + 每个等待者一个唤醒 channel（归还/腾配额只唤醒一个）」。决策全在锁内（有 idle 必复用，杜绝「有空闲仍新建」）；reaper 继续按下标扫描、无需 drain（避免空闲空窗抖动）；等待超时改用等待者本地 `time.NewTimer`，并处理「超时 vs 被唤醒」竞态。新增测试 `TestGetPrefersIdleOverDial` / `TestWaiterWakesExactlyOne` / `TestConcurrentGetPutWithReaper`；全量 `go test ./... -race` 通过。
- [x] **v07 安全认证** —— 完成（鉴权 filter 四段编排 + JWT 校验 + 装配 + 测试 + 配置样例）
  - [x] 步骤 1：Router 提前到 filter 链最前、删 `TestFilter` 占位、修 `compile_test` 旧断言
  - [x] 步骤 2：配置结构——`RouterRule.ID`、`AuthFilterConfig`、`JWTVerifierConfig`（编写态/运行态分层）
  - [x] 步骤 3：hydrate 注册 `auth` 插件，YAML → `AuthFilterConfig`
  - [x] 步骤 4：编译与校验——`RouteID` 生成、策略表反向聚合、`validateRouteAuth` 一致性校验、`AuthFilterConfig` 分层（密钥改声明式 `SecretEnv`）
  - [x] 阶段 A（配置与编译层收口）：`RouteAuthConfig` 编写态分层（`public`/`credential`/`require{roles,scopes}`）+ 运行态 `AuthPolicy` 展平；`compileRoutes` 一次遍历展平路由并聚合全量策略；逐条声明校验（漏声明 / public 互斥 / credential 必须被接受）
  - [x] 阶段 B（消息模型与路由）：`message.Request` 增 `RouteID` 与 `Identity{Subject,Roles}`；Router 命中写 `RouteID`/`Service`；`handler.Process` 消费 filter 短路响应（跳过后续 filter / 借连接 / transformer）
  - [x] 阶段 C（鉴权 filter）：`auth/verifier` 凭证抽象 + `auth/verifier/jwt` HS256/IdP 布局 + `auth.AuthFilter` 四段编排（public 豁免 / 空 roles 只跳授权）+ `ErrUnauthorized(401, WWW-Authenticate)` / `ErrForbidden(403)` + 剥离入站 `X-Gateway-*`
  - [x] 阶段 D（装配）：`buildFilter` 新增 `case AuthFilterConfig` 构造 `AuthFilter`
  - [x] 阶段 E（测试）：`auth_test.go` / `jwt_test.go` 单测 + `testbed/auth_test.go` 端到端（200/401/403/public，拒绝不触达后端）
  - [x] 阶段 F（配置与文档）：`gateway.yaml` 启用 `auth` 并配 `credentials`/`jwt`，`platform.yaml` 逐条声明（含 `public: true` 示例），新增 `builder/config_test.go` 校验示例配置
  - [x] 安全加固 G1–G3（路由）：`path.Clean` 归一化、前缀段边界、Host 剥离端口 + 大小写不敏感
  - [x] 安全加固 G4–G6（handler）：写响应设 write deadline（10s）；错误 body 改回 `http.StatusText`、内部细节只进日志；conn 包 `io.LimitedReader`（1MB）限制单请求读入量
- [ ] **v08 管道细化** —— 进行中：A–C 已完成，下一步 D/E/F
  - [x] 阶段 A：插件注册表按类别隔离
  - [x] 阶段 B：route 级 pipeline 与单次 Plan，编译装配、校验和测试收口
  - [x] 阶段 C：handler 按 RouteID 执行 pipeline，删除旧 `Filters` 字段
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

> 仅保留未完成项，按落地阶段归类；每项随对应版本完成后删除。

### v08 管道细化
- 负载均衡：`BManager.Call` 写死 `instances[0]`（由「下一步」阶段 E 处理）

### v09 协议扩展
- transformer 在后端连接上的读写未设 deadline，后端挂死时只能靠 reaper 借出超时强杀（默认 10–15s）

### v10 最终完善
- 错误处理统一走查：`ErrBodyTooLarge` 目前归入「边界不可信错误」直接断连、不写 413，若想给客户端明确反馈可在断连前写一次 413
- 编码失败（`errE`）时的断连策略

### 未定版本（暂不确定何时做）
- **Verifier 抽象层**：多凭证机制 / 配置层注册表（等第二个凭证出现）
- **JWT 凭证层扩展**：多 IdP（per-issuer 密钥 / JWKS）、更多 IdP 布局、更多签名算法（RS256 等）
- 有状态 verifier（session、token 黑名单）
- 出站身份透传与防伪签名 / mTLS（归 transformer，真实协议时落地）
- 更多授权维度（apikey subscription、ABAC、IP 白名单）
- 更多限流维度（per-identity、per-IP `Limiter` 实现 + 动态桶淘汰）/ 分布式限流

## 测试基础设施

- 方向：集成 testbed（`go test` 自动化），不依赖手动脚本/独立二进制。
- 假后端作为测试 helper（goroutine 监听真实端口），行为/协议可配置，越易配置越好。
- 网关通过配置注入拿到后端实例地址，不硬编码——测试代码启动假后端、构造 `builder.Config`、用构造函数组装网关，`main.go` 不被测试侵入。
- 假后端协议和 transformer 绑定演进：现为 **4 字节长度前缀帧**（模拟协议，fakebackend 与 transformer 对称实现）；后续接真实协议（gRPC/MCP）时替换实现。
- 行为模式先做 `echo` + `fixed`，`error`/`delay` 等做到重试/熔断（v03/v06）时再加。

## 下一步（v08 管道细化 · 规划）

> 两条主线：**filter chain 可配置化**（不同路由执行不同 filter 组合）与 **HTTP 层精细限流**；同时接入负载均衡。A–C 已完成（插件注册表分类 + pipeline 配置/编译 + handler 按 RouteID 执行），剩余 D/E/F。

### 目标

1. 请求命中路由后，按该 route 的 pipeline 执行（B 已完成配置/编译，C 接入 handler）。
2. 新增请求级限流 filter（按路由配额），与 v03 的连接级限流分层。
3. `BManager.Call` 接入负载均衡，不再写死 `instances[0]`。
4. 插件注册表按类别隔离，并与默认链/route pipeline 解耦：注册表只提供配置体，链只提供名字，错位引用在配置加载期即报错（B 已完成）。
5. 配置加载期只计算一次 Plan，`Compile` 复用结果，不得重复遍历路由（B 已完成）。

### 设计契约（实现须遵守）

1. **Router 固定先行**：Router 不参与 route 声明；`Process` 先执行 Router 拿到 `RouteID`，再按 `RouteID` 取 pipeline。
2. **`http_filters.enabled` 定义默认链**：route 不写 `filters`（nil）→ 按 `http_filters.enabled` 顺序执行（保持 v07 行为）；写了（含显式 `[]`）→ 以 route 声明为绝对标准，只跑列出的。需要实例化的 http filter = `http_filters.enabled` ∪ 各 route 声明（契约 A；默认链同时参与构造集合，本阶段不改为 effective-pipeline 优化）。
3. **策略表按 RouteID 聚合**：`Plan` 收集路由策略，`Compile` 装入 filter 配置；filter 单实例按 `req.RouteID` 查表。
4. **fail-closed 校验**：`http_filters.enabled` / route `filters` 引用的 filter 必须在 `plugins.http` 中存在（ValidateWriting / Compile 报错）；被激活条目里的未知插件、类型错误、跨类别错位在 Hydrate 期报错；未激活条目不校验；route `filters` 不可重复；auth 的 fail-closed 规则见 B5。
5. **短路语义不变**：任何 filter 返回非 nil response 或 error，链路立即终止，后续 filter、借连接、transformer 都不执行。
6. **filter 实例全局共享**：限流/鉴权状态须并发安全（一条 conn 一个 goroutine）。
7. **两级映射不变**：filter 定「请求→服务」，backend 定「服务→实例」；v08 不做换实例重试。
8. **注册表与链分离**：`plugins.{tcp,http,transformer}` 只提供配置体；各 `enabled` 与 route `filters` 只提供名字。运行态只组装并集，注册表多余条目不得进入运行态。
9. **Plan 单次计算**：`Build` 只调用一次 `Calculate`（返回 `*Plan`）；`Compile` 只消费 Plan；`ValidateWriting` 可读原始 `platforms` 做声明层校验（nil vs `[]`、auth 是否声明），但不得重复生成 Plan。

### 阶段 A —— 插件注册表分类（builder）【done】

- A1 `pluginHydrators` 拆成 `tcpFilterHydrators` / `httpFilterHydrators` / `transformerHydrators` 三张表。
- A2 `hydratePluginSet` 接收对应的表，错误信息带类别；`tcp_limit` 写进 `http_filters` 在 hydrate 期即报错（B7 已替换为按注册表遍历的 `hydrateRegistry`，分类表隔离结论保留）。
- A3 单测：跨类别错位引用报错，各表正常水合。
- 主要文件：`builder/hydrate.go`、`builder/hydrate_test.go`。

### 阶段 B —— 路由级 pipeline 配置与编译（builder）【done】

目标链路：`Load → Calculate(单次 Plan) → ValidateWriting → Hydrate → ValidateRuntime → Compile(gw, plan)`。

- B1 route 级 `filters`：nil 继承 `http_filters.enabled`，`[]` 为显式空链。
- B2 `Calculate` 一次遍历产出 http filter 并集；`Hydrate` 只水合激活集合。
- B3 运行态 `HTTPFilterConfig` 增加 `Router` / `Configs` / `Pipelines`（旧 `Filters` 过渡保留）。
- B4 `Compile` 从 Plan 装配 Router 规则、pipeline 与策略表。
- B5 校验 pipeline 与 route auth 声明逐条一致，fail-closed。
- B6 单测覆盖默认继承、显式覆盖、空链、并集、未配置引用、重复、auth 一致性。
- B7 编写态拆成 `plugins` 注册表与 `enabled` 激活名单，注册表多余条目不进入运行态。
- B8 `Calculate` 收敛为单次 Plan（`HTTPFilters` + `Routes` + `Services`）；`Compile(gw, plan)` 只做装配，不再遍历 platforms；校验拆为 `ValidateWriting` / `ValidateRuntime`。
- B9 配置样例、全量 `build/vet/test -race` 与文档收口。

B 阶段只改 builder；`HTTPFilterConfig.Filters` 仍驱动 handler，C 切换前 route 级 `filters` 不实际执行。

### 阶段 C —— 按 RouteID 执行 pipeline（handler）【done】

- C1 `buildFilter` 从运行态 `Configs` 构造「Router 实例 + 名字→实例表」，`NewHandler` 把 `Pipelines` 预解析为 `map[RouteID][]HTTPFilter`，避免请求期查名字；切换完成后删除旧 `Filters`。
- C2 `Process`：Router 先跑 → 按 `req.RouteID` 取 pipeline → 依次执行，短路/错误语义沿用 v07。
- C3 pipeline 缺失视为配置错误（fail-closed），不静默放行。
- C4 既有行为等价：全部 route 都不声明 `filters` 时，执行顺序与 v07 完全一致。
- 主要文件：`handler/handler.go`。

### 阶段 D —— HTTP 限流 filter

#### 结构设计

三层：**filter（分发）→ Limiter（维度 + 判定）→ TokenBucket（令牌算法）**。

```
RateLimitFilter
  └── limiters map[RouteID]Limiter     ← 路由配置决定归哪个 Limiter
        ├── RouteLimiter（v08）
        │     └── buckets map[RouteID]*TokenBucket   ← 构造时参数烘进桶，预建
        ├── IPLimiter（后续）
        │     └── params + buckets map[RouteID+IP]*TokenBucket  ← 参数留存，桶懒建 + 淘汰
        └── IdentityLimiter（后续）
              └── 同 IPLimiter，key 换 Subject
```

**Filter 只做分发**：按 `req.RouteID` 找到负责的 Limiter，调 `Allow`；找不到 = 该路由无限流，放行。不持有参数，不知道维度。

```go
type RateLimitFilter struct {
    limiters map[string]Limiter
}
func (f *RateLimitFilter) HandleHTTPFilt(req) {
    limiter, ok := f.limiters[req.RouteID]  // 路由 → limiter
    if !ok { return nil, nil }               // 无限流
    ok, retryAfter := limiter.Allow(req)
    if !ok { return retryResponse(retryAfter), ErrRateLimited }
    return nil, nil
}
```

**Limiter 接口**：`Allow(req) (ok bool, retryAfter time.Duration)`。每种维度一个实现，拥有参数和桶，判定完全自包含。

**RouteLimiter（v08）**：构造时拿 `map[RouteID]RateLimitParams`，为每条路由预建一个 `TokenBucket`（QPS/burst 烘进桶里）。请求时按 `req.RouteID` 查桶，桶自己判定。参数只在构造时使用，运行时不以独立形态存在。

**TokenBucket（具体类型，非接口）**：惰性补充（每次 Allow 按 elapsed × qps 补令牌，上限 burst），时间源可注入（单测验证令牌恢复），每桶一把 mutex。各 Limiter 实现内部组合使用。

**构造时的绑定**：`buildFilter` 根据路由配置决定每条路由归哪个 Limiter。v08 所有路由指向同一个 `RouteLimiter` 实例；后续加 per-IP 时，对应路由指向 `IPLimiter` 实例，filter 代码和接口都不变。

**配置规则**：
- pipeline 含 `rate_limit` 但 route 未声明参数 → 不限流（放行）。与 auth 的 fail-closed 不同，限流不涉及安全边界。
- route 声明了 `rate_limit` 但 pipeline 不含 `rate_limit` → 编译期报错。
- 推荐顺序 `[auth, rate_limit]`：先挡非法请求再计数，避免攻击流量消耗正常配额。
- 单机限流：每个进程独立桶，多副本部署时总限额 = N × qps。

**数据流**：`builder Compile → map[RouteID]RateLimitParams → NewRouteLimiter(params) → 参数烘进 TokenBucket → 请求时 bucket.Allow() 自判定`。

#### 子任务

- D1 配置与 Plan：`RouteRuleConfig` 加 `RateLimit *RouteRateLimitConfig{QPS,Burst}`；运行态 `RateLimitFilterConfig{Params map[RouteID]RateLimitParams}` / `RateLimitParams{QPS,Burst}` 进 `config.go`；`httpFilterHydrators` 注册 `rate_limit`（照 `hydrateTCPLimit` 写法）；`PlannedRoute` 增 `RateLimitParams`，`Calculate` 展平。
- D2 校验与编译：`ValidateWriting` 新增 `validateRouteRateLimitDeclarations`（pipeline 含 `rate_limit` ↔ 声明一致性，与 auth 的校验对称）；`ValidateRuntime` 校验 `qps > 0`、`burst >= 1`；`Compile` 新增 `attachRateLimitParams` 装入参数表（同 `attachAuthPolicies`）。
- D3 令牌桶：`handler/http_filter/ratelimit/bucket.go`，`TokenBucket` 具体类型（`Allow(now) (bool, time.Duration)`）；单测覆盖连续消费耗尽、时间推进恢复、并发安全。
- D4 Limiter 接口 + RouteLimiter：`handler/http_filter/ratelimit/limiter.go`，`Limiter` 接口 + `NewRouteLimiter(params)` 预建桶；单测覆盖放行 / 超限 / 多路由隔离。
- D5 Filter + handler 接入：`handler/http_filter/ratelimit/ratelimit.go`，`RateLimitFilter` 分发；`handler.go` 的 `buildFilter` 增 `case RateLimitFilterConfig`（构造 `RouteLimiter` + 绑定路由 → limiter 映射 + `RateLimitFilter`）；`ErrorResponse` 增 `ErrRateLimited → 429` 映射；`pkg/errors` 新增 `ErrRateLimited`。
- D6 单测与配置样例：filter 级单测（放行、超限 429 + `Retry-After`、无参数放行、并发安全）；`gateway.yaml` / `platform.yaml` 更新。
- 主要文件：`handler/http_filter/ratelimit/`（bucket.go、limiter.go、ratelimit.go）、`builder/{platform,config,calculate,compile,validate_writing,validate_runtime,hydrate_http}.go`、`handler/handler.go`、`pkg/errors/errors.go`。

### 阶段 E —— 负载均衡（backend）

- E1 service 级配置 `lb`（默认 `round_robin`），随 `ServiceRef` 传入运行态；编译期校验未知算法。
- E2 实现 `LoadBalancer`：`round_robin`（原子计数）+ 候选过滤 unhealthy（全 unhealthy 时回退全量）。
- E3 `Call` 改为 `lb.Pick(instances)` 取实例后再取该实例的池；release 语义不变。
- E4 单测：多实例轮询均衡、单实例、空实例、并发计数正确。
- 主要文件：`backend/{loadbalancer,backend,model}.go`、`builder/{platform,config,compile,validate}.go`、`backend/*_test.go`。

### 阶段 F —— 测试与文档

- F1 testbed：两条路由不同 pipeline（一条 auth + 限流、一条 public）；限流 429 且后端未被触达。
- F2 testbed：多实例 `round_robin` 分发（两个 fakebackend）。
- F3 配置样例更新：`gateway.yaml` 启用 `rate_limit`，`platform.yaml` 逐条给出 `filters` / `rate_limit` / `lb` 示例。
- F4 ROADMAP 勾选，删除「已知待办」v08 负载均衡条。

### 明确不在 v08

- 换实例重试、熔断、主动健康探测（留 v09/v10）。
- per-identity / per-IP / 分布式限流（v08 只做单机 route 级 `RouteLimiter`；后续维度实现 `Limiter` 接口即可，不改编排层）。
- pipeline 热加载、按 method/host 再细分 filter 组合。
- 多凭证 / 多 IdP / JWKS / RS256（未定版本）。

### 顺序

`A → B → C → D → E → F`。A–C 已完成；D 新增限流 filter；E 与 D 无耦合可并行；F 收口。
