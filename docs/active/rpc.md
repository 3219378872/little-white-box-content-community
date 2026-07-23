# RPC 开发速查

## 当前结构

- RPC 和 MQ 模块位于 `app/`；公共错误传输在 `pkg/interceptor/`。
- proto 定义位于 `proto/`，生成代码不手动编辑。
- Gateway 到 RPC 的客户端和服务发现配置以实际 `etc/` 文件为准。

## 修改流程

1. 先阅读对应 `.proto`、server、Logic 和调用方。
2. 修改 `.proto` 后运行仓库使用的 goctl/protobuf 生成命令。
3. 所有 zrpc 调用透传原始 ctx；goroutine 使用 ctx 的副本并处理取消。
4. 跨服务业务错误使用 `pkg/errx` 和现有 interceptor 传输，不重新定义错误协议。

## 验证

- 至少覆盖调用成功、下游失败、超时/取消三类路径中适用的部分。
- 运行 `scripts/test.sh`、`scripts/vet.sh`，并检查生成文件差异。
