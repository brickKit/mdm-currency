# currency/：入口与组装

组件对外的一个包：`main.go` 与外壳只引用它（`currency.Definition()`、`currency.New`）。这里只**组装**：把 `internal/` 下模块的函数登记成 SDK 要的路由、gRPC 方法、后台工作与恢复核对；业务一行也不写。

## 负责与不负责

- 负责：组件的版本、端口、迁移、组件定义；哪个路由、哪个 gRPC 方法、哪个后台工作交给哪个模块。
- 不负责：业务规则（去 [internal/AGENTS.md](../internal/AGENTS.md) 找模块）；资源、操作、事件类型的名字（`internal/core/names.go`）；集成测试（`itest/`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `definition.go` | `Version`、`GRPCPort`、迁移；`Definition()`：身份、流、资源与操作（`core.Definitions()`） |
| `module.go` | `Module` 与 `New`（`core.Base` 带着运行时、部署与实现实例）；`Routes`（每条路由 → 模块的处理函数与操作）、`Services`（来源方法与有效汇率，任何组件可调）、`Jobs`（搜索列的回填 → `currencies.Backfill`）；`handler` 把“先收 `*core.Base`”的处理函数适配成 SDK 的签名 |
| `services.go` | gRPC 方法 `Snapshot`、`Read`、`EffectiveRate` 转给 `internal/source` |
| `checks.go` | 整套恢复的核对：币种、汇率类型（审计成 `rate_type:<code>`）、汇率按版本答“还在不在” |
| `contract_test.go` | 单元测试：清单的 `configSchema` 与 `events`、OpenAPI 与路由一一对应、事件载荷合乎 schema、合并安全 |
| `migrations/` | 表（`0001`）与 ISO 4217 全表（`0002`，`scripts/iso4217.py` 生成） |

## 依赖与被依赖

- 依赖：`internal/core`、`currencies`、`rates`、`source`。
- 被依赖：`main.go`、`cmd/seed`、外壳 `be/go-infra`、`itest/`。

## 测试

`go test ./currency/`。
