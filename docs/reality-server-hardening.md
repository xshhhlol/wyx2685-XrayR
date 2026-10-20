# REALITY 服务端握手特征修复

本次调整针对 TCP + REALITY + VLESS Vision 的一个已公开识别特征：
目标站在 TLS 1.3 ClientFinished 后发送的记录，与旧 REALITY 服务端的
记录数量、长度不一致。检测不需要解密 VLESS 内容。

## 依据与改动

- [Aparecium](https://github.com/ban6cat6/aparecium) 描述了先探测目标的
  NewSessionTicket 行为，再与被观察连接进行比较的方法。
- [上游问题 #4778](https://github.com/XTLS/Xray-core/issues/4778) 确认了该缺陷。
- [Xray v25.6.8](https://github.com/XTLS/Xray-core/releases/tag/v25.6.8)
  引入服务端探测和模仿 post-handshake records 的实现。
- [Xray v25.7.26](https://github.com/XTLS/Xray-core/releases/tag/v25.7.26)
  包含后续按 ALPN 分类探测/模仿和 ServerHello 发送行为的修复。

- [Xray v26.3.27](https://github.com/XTLS/Xray-core/releases/tag/v26.3.27)
  是目前最新的正式版，服务端启动时还会探测 target 的 maxUselessRecords
  并应用于自身。

项目采用 v26.3.27 对应的 Go 模块 v1.260327.0，代替原来的
2025-01-01 提交。之后的 v26.4.x～v26.10.x 均为预发布版本，未发布 Go
模块版本，且包含禁止公网明文出站、重命名入站 `clients` 等不兼容变更，
暂不采用。这并非声称修复了所有流量识别方法。REALITY 握手实现由上游
依赖提供，不在 XrayR 中自行修改密码学或发明随机填充协议。

TCP 入站启动时，上游会调用 DetectPostHandshakeRecordsLens，根据
target、serverName 和 ALPN 分类缓存记录长度；认证后的连接使用对应
长度模仿握手后记录。保持回落限速为默认关闭，不另加固定限速指纹。

## 客户端与部署

不要求修改客户端配置：继续使用原来的 UUID、REALITY 公钥、Short ID、
SNI、TCP 和 xtls-rprx-vision。不要为了部署本次升级轮换这些参数。
本次不改面板字段，不自动更换目标站，也不切换 XHTTP。

XrayR 内嵌 Xray-core，单独更新服务器上另一个 `xray` 可执行文件不会
更新 XrayR 的内核；需要重新构建并部署 XrayR。
部署后运行 `XrayR version`，确认输出包含 `Xray-core 26.3.27`。

新依赖要求 Go 1.26。发布工作流和 Docker 构建使用 Go 1.26.10。

## 随内核升级的适配

- 上游 VLESS 入站自 v25.9.5 起改用 `DispatchLink()`，且在其返回后关闭
  连接。`app/mydispatcher` 的 `DispatchLink` 已改为同步分发，并通过
  `wrapLink` 应用设备数限制、上下行限速和用户流量统计；否则 VLESS 连接
  会立即断开，限速与流量上报也会失效。VMess、Trojan、Shadowsocks 仍走
  `Dispatch()`，行为不变。嗅探逻辑同步为上游实现。
- 上游删除了 Shadowsocks 的 IV 重放检查，`DisableIVCheck` 配置项保留
  但不再生效。
- 上游删除了 mKCP 的 srtp/utp/wechat/dtls/wireguard 伪装头包（改由
  Finalmask 提供），XrayR 本就不下发 mKCP 节点，仅移除对应导入。
- 自定义出站中的 `allowInsecure` 自 2026-06-01 起会导致配置加载失败，
  需改用 `pinnedPeerCertSha256`/`verifyPeerCertByName`。VMess、Trojan、
  Shadowsocks、WebSocket、gRPC 等会打印弃用警告，但仍可使用。
初次启动涉及目标站探测，探测时间和结果取决于节点到目标的网络、
目标 TLS 行为及证书校验。上线后应检查错误日志和首批连接。

## 验证范围

配置回归测试检查旧面板参数仍能生成 TCP/REALITY 入站，并保留原有
认证信息。它不测量实际网络的识别率。

2026-10-20 本机端到端测试（模拟 NewV2board 面板）：官方 Xray 25.1.1
与 26.3.27 客户端经 VLESS/REALITY/Vision（firefox、chrome 指纹）、
VMess、Shadowsocks 访问 HTTPS 均成功且证书校验通过；8 Mbps 用户上下行
均被限制在约 1 MB/s；达到设备数上限的用户被拒绝；面板收到的上下行流量
与实际传输量一致。

运行不依赖真实面板、ACME 服务的构建测试：

```sh
go test ./service/controller -run 'TestBuild' -count=1
go test ./... -run '^$'
go build ./...
```

现有其他测试包含真实面板访问、证书申请和等待系统信号的流程，不宜
把不加筛选的 `go test ./...` 当作离线回归。

实际验收应在相同客户端、相同 SNI/目标、相同流量负载下比较旧/新服务端，
抓取 TLS record（不能简单按 TCP packet 计数），同时检查未认证回落与
已认证业务连接。至少验证 HTTPS 代理、长连接、断连和重连。
不要把“浏览器访问节点能显示目标网页”当作抗识别测试通过。

## 未解决的边界

此修复仅针对已知的握手后记录差异，不能证明当前线路的阻断由该差异导致，
也不能保证流量与真实访问目标网站完全不可区分。客户端 ClientHello
指纹、实际应用流量分布、跨层 RTT、连接数量等不因这次服务端升级而消失。
任何客户端指纹也不因此获得逐字节的目标行为复刻保证。

相关研究：[USENIX Security 2024](https://www.usenix.org/conference/usenixsecurity24/presentation/xue-fingerprinting)、
[NDSS 跨层 RTT 研究](https://www.ndss-symposium.org/ndss-paper/the-discriminative-power-of-cross-layer-rtts-in-fingerprinting-proxy-traffic/)。
