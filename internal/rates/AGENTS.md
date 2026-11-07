# rates：汇率类型与汇率

汇率类型（例 `spot`）与按类型、日期、币种对的汇率：录入、更正（同一条记录的新版本）、有效汇率（不晚于某日的最新一条）。

## 负责与不负责

- 负责：表 `rate_type`、`exchange_rate`；路由 `/api/currency/rate-types` 的列表与写入、`/api/currency/rates` 的列表、写入与有效汇率；事件 `mdm.rate_type.changed.v1`、`mdm.exchange_rate.published.v1`。
- 不负责：反向与交叉汇率（业务政策，不在这里推导）；系统面的有效汇率（`source`，它调这里的 `Effective`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `rates.go` | `PutRateType`；`PutRate`（每个类型、日期与币种对一条；相同的值不产生新版本，`7.65` 与 `7.6500` 相同；停用的币种拒绝）；`EffectiveHTTP`、`Effective`；列表 `rateTypeList`、`rateList`、`ListRateTypes`、`ListRates`（最新的日期在前） |

## 依赖与被依赖

- 依赖：`core`。
- 被依赖：`source`（`Effective`）；入口登记路由。

## 规则在哪

`docs/design.md` 的规则；装配项目协议 14（汇率的精度与快照）。

## 测试

`itest/rates_integration_test.go`（`rates`）；系统面的有效汇率在 `itest/source_integration_test.go`。
