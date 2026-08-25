# 技术选型调研：多协议翻译网关

> 记录本项目「做什么、不做什么、对标哪些开源方案」的选型依据。
> 配套 `ROADMAP.md`（进度规划）与 `API Gateway.drawio.svg`（架构图）。
> 面向首次接手的开发者与 AI，用于快速建立价值判断。

---

## 1. 项目核心目标

定位：面向「多协议、可演进、可精细控制」的协议翻译内核（Protocol Translation Kernel）。

- 客户端统一使用 HTTP。
- 后端可能是 gRPC / MCP / Thrift / 私有协议等请求-响应类协议。Redis/MySQL 等非请求-响应协议不在翻译范围（见 §3 支柱二）。
- 网关居中做协议翻译，将异构后端统一为 HTTP 对外暴露。
- 纯 Go 标准库实现，在 `net.Conn` 上自管连接生命周期，不交给 `http.Server`。

核心判断：本项目不是「更差的 grpc-gateway」，而是「协议作为可插拔插件的翻译内核」。差异点不在 HTTP 性能，而在新增后端协议时只加一个 transformer、不改已有代码。

---

## 2. 要解决的真实痛点

按需求强度排序：

| # | 痛点 | 具体表现 | 解法 |
|---|---|---|---|
| 1 | 接入异构性 | 前端只讲 HTTP/JSON，后端是 gRPC/MCP/私有协议，浏览器无法直接调 gRPC | 网关统一翻译，后端保持纯协议 |
| 2 | 横切逻辑重复 | 鉴权/限流/路由/观测在每种协议各写一遍 | 归一为唯一中间表示，写一次全协议复用 |
| 3 | 协议演进锁定 | 后端换协议会逼所有客户端升级 | 翻译对客户端透明，可灰度分流 |
| 4 | 遗留协议现代化 | 老系统跑 SOAP/私有二进制，现代客户端无法消费 | 网关做协议适配层，老系统零改造 |
| 5 | 连接级精细控制 | 框架给不了连接级限流、建立/关闭感知、自定义帧定界 | 自管 `net.Conn` |

边界声明：若后端只有 gRPC 或只有 HTTP，本项目价值不大，直接用 grpc-gateway/Envoy 更划算。手写的价值仅在以下情况成立：

1. 后端协议种类多且异构（gRPC + MCP + Thrift 等并存）。
2. 协议持续演进（如 MCP）。
3. 需要字节级/连接级精细控制。

---

## 3. 三条核心设计支柱

选型结论最终收敛为三条不可动摇的原则。

### 支柱一：连接生命周期自管，解析借库

- 连接（Accept / 读写循环 / 关闭时机）自己管理，这是连接级限流、协议翻译、字节级控制的基础。
- 协议解析借用标准库 `http.ReadRequest`，但不交出连接（roadmap v04）。
- 翻译（HTTP ↔ 后端协议）自己实现。

总结：连接自管、解析借库、翻译自写。

### 支柱二：唯一中间表示 + 可插拔 transformer

- `handler/message` 的 `Request/Response` 是唯一中间表示，decoder/filter/transformer/encoder 共享。
- 可映射为请求-响应模型的协议，被 transformer 归一为同一 HTTP 模型，横切逻辑（鉴权/限流/路由）写一次、多协议复用。
- 复用边界：filter 复用仅在语义同构时成立。RPC 类协议（gRPC/Thrift/MCP/Dubbo）与 HTTP 同构（都是「调用→返回」），映射无损，filter 天然复用。
- 存储/流式协议（Redis/MySQL）与 HTTP 语义不同构（命令、事务、订阅、游标），强行翻译会有损语义，filter 复用前提不成立。
- `backend` 只认 `(service string, payload []byte)`，对协议无感知；协议知识封在 transformer 内。

### 支柱三：依赖单向无环，模块可替换

- `main → {connector, handler, backend}`，三模块互不 import，靠 main.go 注入函数值接线。
- `connector` 通过 `Connector` 接口 + `next func(net.Conn) error` 注入。
- 换连接层不等于免费提性能：
  - `next` 是同步阻塞语义，与 Reactor 非阻塞回调冲突，不能直接替换。
  - 性能瓶颈可能在翻译层（CPU 密集编解码），换连接层解决不了。
  - 扛 C10M 需将整条链路（含 handler 同步读后端）改为非阻塞状态机，属跨层级重构。

---

## 4. 技术选型调研

### 4.1 连接生命周期管理方式

连接生命周期管理本质是三个问题的归属：谁 Accept、谁跑读写循环、谁定关闭时机。

| 方式 | 连接建立/关闭感知 | 连接级准入 | HTTP 解析/keep-alive | 协议翻译 | 适用场景 |
|---|---|---|---|---|---|
| ① 裸 `net.Conn` 自管（本项目） | ✅ 完全 | ✅ 自写 | ❌ 全自写 | ✅ 唯一能做 | 协议网关 |
| ② `http.Server` 全托管 | ❌ 拿不到 | ❌ 需包装 | ✅ 内置 | ❌ 不能 | 普通 Web 服务 |
| ③ 半托管（`ReadRequest`/`Serve`） | ✅ 拿得到 | ✅ 自写 | ✅ 复用标准库 | ❌ 仍限 HTTP | HTTP 反向代理/中间件 |
| ④ Reactor 框架（gnet/netpoll） | ✅ 回调拿得到 | ✅ 自写 | 视框架 | ⚠️ 能做但难 | IM/推送/海量长连接 |
| ⑤ raw socket / syscall | ✅ 完全 | ✅ 自写 | ❌ 全自写 | ✅ 能做 | 造网络框架 |

结论：方式②③④⑤ 均受「后端协议不透明」限制。本项目采用 ① 为主 + ③(a) 借 `http.ReadRequest` 解析但不交出连接。

### 4.2 并发模型对比：goroutine-per-conn vs Reactor

结论：goroutine-per-conn 性能不如手写 Reactor，可舒适扛到 C100K，C10M 级是瓶颈。

事实澄清：Go 的 goroutine 阻塞在网络 IO 时由 runtime netpoller 挂起、让出 CPU，不是一连接一 OS 线程的重模型。这只解释为何能扛 C100K，不改变「性能不如 Reactor」的结论。

| 维度 | 手写 Reactor（gnet/netpoll） | goroutine-per-conn（本项目） |
|---|---|---|
| 事件循环 | 显式，自己写 `epoll_wait` | 隐式，runtime netpoller 代管 |
| 编程模型 | 回调 + 状态机 + 非阻塞 | 同步阻塞 + 线性流程 |
| 状态存放 | 显式 context 跨回调传 | goroutine 栈局部变量 |
| 阻塞语义 | 禁止阻塞 | 阻塞由 runtime 挂起，天然安全 |
| 内存/连接 | ~几百字节 | goroutine 栈 2KB 起步 |
| 海量连接(C10M) | ✅ 优势 | ❌ 瓶颈 |
| 中量连接(C100K) | ✅ | ✅ 够用 |
| 复杂有状态逻辑(协议翻译) | ❌ 复杂 | ✅ 线性代码 |
| 生态 | 薄 | 厚 |

结论：协议翻译是「有状态、双向、流式」的复杂逻辑，适配 goroutine-per-conn 线性模型；Reactor 的价值仅在「海量连接 + 简单逻辑」。撞上 C10M 瓶颈时，代价是整条链路（含 handler 同步读后端）非阻塞化，不是只换连接层。

### 4.3 「反向代理」与「协议转换」是正交维度

- 多数反向代理（nginx/Envoy/caddy）做同协议转发（HTTP → HTTP），理解 HTTP 只为路由/改 header/负载均衡，协议不变。
- 真正做跨协议转换的是少数（grpc-gateway、Envoy 的 gRPC-JSON transcoder、本项目）。
- 本项目 = 反向代理 + 协议转换，这是它与 nginx 的本质区别，也是坚持字节级自管的原因。

---

## 5. 开源方案深度对比

### 5.1 grpc-gateway

- 定位：HTTP/JSON ↔ gRPC 单协议转换网关。
- 优势：成熟、生态全、streaming 支持完整、代码生成自动化。
- 劣势：只支持 gRPC 一种协议，加 MCP/私有协议需重写；仍是 goroutine-per-conn。
- 关系：品类最接近，本项目把「单协议转换」推广为「任意协议可插拔」。

### 5.2 Envoy

- 定位：L4/L7 通用服务代理。
- 多协议方式（已核实官方文档）：filter 分三层，listener filter（连接级）→ network filter（L3/L4，`http_connection_manager`、`redis_proxy`、`mongo_proxy` 等并列）→ HTTP filter（只挂在 `http_connection_manager` 内部）。各协议是并列 network filter，HTTP filter 链无法跨协议复用；跨协议转换只有 gRPC-JSON transcoder 一个边缘 filter。
- 劣势：事件驱动 + 状态机，代码复杂度高；新增协议需新增 network filter 并独立实现协议逻辑，无法复用 HTTP filter 链。
- 关系：Envoy 的「通用」靠生态堆出来（几百个 filter）。本项目 filter 挂在统一 `message` 模型上，对 RPC 类协议能复用同一套 filter，这是相对 Envoy 的优势。

### 5.3 nginx

- 定位：高性能 HTTP 反向代理/负载均衡。
- 劣势：同协议转发，不做翻译（`grpc_pass` 也只转发 gRPC 流量）；与「协议翻译」无关。

### 5.4 gnet / netpoll / evio（Reactor 框架）

- 定位：基于 epoll/kqueue 的高并发网络框架，扛 C10M 级连接。
- 优势：海量连接下内存/调度开销低。
- 劣势：几乎不做协议翻译；回调式编程，复杂双向翻译逻辑难写；生态薄。
- 关系：不是竞品。若撞上 C10M 瓶颈，可替换连接层，但代价是整条同步链路非阻塞化（见 §4.2）。

### 5.5 Connect / Buf

- 定位：gRPC 生态的现代化网关/RPC 框架。
- 特点：支持 gRPC/gRPC-Web/Connect 三种协议，仍是 goroutine-per-conn。
- 劣势：锁死 gRPC 协议族，不做任意协议翻译。

### 5.6 WebSocket 网关（gorilla / nhooyr）

- 定位：长连接 + 全双工流的 HTTP 扩展。
- 特点：goroutine-per-conn，证明流式翻译用同步模型可行。
- 关系：佐证协议翻译 + 流式不需要换 Reactor。

### 对比总表

| 方案 | 协议翻译 | 跨协议通用 | 连接级控制 | 并发模型 | 依赖 | 新增协议成本 |
|---|---|---|---|---|---|---|
| grpc-gateway | ✅ 仅 gRPC | ❌ | ❌ | goroutine/conn | 重 | 重写 |
| Envoy | ⚠️ transcoder 边缘 | ❌ 各协议各一套 filter | ⚠️ 包装 | 事件驱动 | 重 | 新 filter + 子链 |
| nginx | ❌ | ❌ | ⚠️ 模块 | 多进程 epoll | 中 | 不适用 |
| gnet/netpoll | ❌ 透传为主 | ❌ | ✅ | Reactor | 轻 | 不适用 |
| Connect/Buf | ✅ 仅 gRPC 族 | ❌ | ❌ | goroutine/conn | 中 | 重写 |
| **本项目** | ✅ RPC 类协议 | ⚠️ 仅 RPC 类可复用 | ✅ 字节级 | goroutine/conn | 零（标准库） | 只加一个 transformer |

---

## 6. 优势与劣势总结

### 优势

1. 扩展零侵入：新增请求-响应类协议只加一个 transformer，核心链路不改。
2. 横切逻辑多协议复用：鉴权/限流/路由/观测写一次，对 gRPC/MCP/Thrift 等全生效。
3. 协议复杂度隔离：帧定界/编解码/流式语义封死在 transformer 内，核心链路简洁。
4. 核心链路可独立测试：backend 对协议无感知，连接池/负载均衡可脱离协议测试；transformer 是纯函数，易单测。
5. 轻量零依赖：纯标准库，无 CGO，体积小、可交叉编译。
6. 模块边界清晰、依赖单向无环：connector/handler/backend 互不 import，便于演进与测试。

### 劣势

1. 纯 HTTP→HTTP 同协议代理：nginx/Envoy 更合适，手写反而亏（keep-alive/HTTP2/缓存是现成的）。
2. 海量长连接（C10M）：goroutine-per-conn 的瓶颈，需换连接层。
3. 无现成 filter 生态：JWT、限流、熔断需自己写。

---

## 7. 适用场景与边界

### 适用场景

| 场景 | 形态 |
|---|---|
| HTTP → gRPC 网关 | Web/移动端讲 HTTP/JSON，后端微服务是 gRPC |
| MCP 接入层 | 外部走 HTTP，内部翻译成 MCP 调工具/服务 |
| 多协议聚合网关 | 统一 HTTP API 背后聚合 gRPC + MCP + 私有协议 |
| 协议迁移/灰度 | 后端换协议客户端零感知，同一服务挂两个 transformer 灰度分流 |
| 企业异构协议统一出口 | 老 Thrift/SOAP/私有协议统一对外 HTTP |
| 边缘/嵌入式网关 | 资源受限、需深度定制，零依赖轻量 |
| 协议测试/模拟层 | 假 transformer 模拟后端协议做集成测试 |

### 不适用

- 纯 HTTP→HTTP 同协议代理 → nginx/Envoy。
- C10M 海量连接 + 简单透传 → gnet/netpoll。
- 后端永远单一协议（只有 gRPC） → grpc-gateway。

---

## 8. 结论

本项目解决「后端 RPC 协议注定异构、注定要变」的问题。

价值主张：

- 用 HTTP 做统一入口。
- 将异构 RPC 类后端协议（gRPC/Thrift/MCP/私有 RPC）的复杂度封进 transformer。
- 网关其余部分只面对同一个 `message` 模型。

后端 RPC 类协议越多、越异构、越在演进，回报越大。

定位边界：本项目是「RPC 协议翻译内核」，不是「任意协议统一内核」。一致性优势严格限定在语义同构的 RPC 类协议内；存储/消息/流式协议（Redis/MySQL/Kafka）语义不同构，不纳入翻译范围。

> 配套决策链：`ROADMAP.md`（怎么做） → 本文档（为什么） → `API Gateway.drawio.svg`（结构）。
