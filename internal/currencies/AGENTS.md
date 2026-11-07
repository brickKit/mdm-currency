# currencies：币种

ISO 4217 的币种（随迁移提供）与部署能决定的几项：现金舍入、译名、启用。

## 负责与不负责

- 负责：表 `currency`；路由 `/api/currency/currencies` 的列表、读取、修改；事件 `mdm.currency.changed.v1`；搜索列（译名与拼音首字母）与它的回填任务。
- 不负责：最小单位与舍入方式的修改（不开放，随迁移更新）；金额怎样舍入（SDK）；给其他组件的读取（`source`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `currencies.go` | `GetCurrency`；`UpdateCurrency`（版本守卫；内容相同不产生新版本）；`sameCurrency`、`currencySearch`；列表 `currencyList`、`ListCurrencies`；`Backfill`（迁移写入的行与旧规范化版本的搜索列） |

## 依赖与被依赖

- 依赖：`core`。
- 被依赖：入口登记路由与回填任务。

## 规则在哪

`docs/design.md` 的规则与“设计取舍”（`AGENTS.md`，仓库根）。

## 测试

`itest/currencies_integration_test.go`（`currencies`）。
