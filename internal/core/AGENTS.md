# core：全组件共用的名字与写法（共用）

每个模块都引用、自己什么业务也不做的东西。改这里会影响所有模块。

## 负责与不负责

- 负责：组件 ID、流与事件类型、资源与操作、`MaxBatch`、资源定义 `Definitions()`（`names.go`）；本组件的错误原因、授权检查、校验（币种代码、汇率类型代码、日期、正的十进制、译名）、审计（`Change`）、列表的共用部分（`rules.go`）；记录的类型、读取、有效汇率的查询、发件表（`Emit`）（`records.go`）；`Base`（`base.go`）。
- 不负责：任何业务规则（在 `currencies`、`rates`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `base.go` | `Base`：运行时、部署、实现实例 |
| `names.go` | 常量；`Definitions()`（本版没有维度，显式写成空集） |
| `rules.go` | 错误原因、`Check`、`Invalid`、`NotFound`、`Change`、`CheckNames`、`SameNames`、`CheckPair`（有效汇率的参数）、`Positive`、列表的 `Build`、`Eq` |
| `records.go` | `Currency`、`RateType`、`Rate` 与 `Scan…`、`Get…`；`Effective`（类型与币种对在不晚于某日的最新汇率）；`Emit` |
| `rules_test.go` | 单元测试：`Positive` |

## 依赖与被依赖

- 依赖：只有 SDK 与契约。
- 被依赖：全部模块与入口。

## 规则在哪

`docs/design.md`；装配项目协议 14（标准代码、金额与舍入、列表）、协议 02（完整状态事件）。

## 测试

`rules_test.go`；其余由 `itest/` 的流程测试间接覆盖。
