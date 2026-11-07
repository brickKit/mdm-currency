# itest/：集成测试与测试索引

币种组件对着真实的 PostgreSQL 与消息系统、经它真实的接口（用户面带 be-sdk-go `sectest` 测试部署的令牌，系统面走 mTLS，发件表）的测试（L2）。构建标签 `integration`；一条业务流程一个文件。

## 怎样跑

- `make test-integration`（`go test -count=1 -tags integration ./itest/`），不到一秒。
- 环境变量：`BE_SDK_TEST_PG_ADMIN=postgres://postgres:<口令>@localhost:5432/brickkit_test_db`（口令是装配项目 `.env` 的 `POSTGRES_PASSWORD`）、`BE_SDK_TEST_NATS_URL=nats://localhost:4222`。
- 前提：装配项目里 `make up` 起了 `be-postgres` 与 `be-nats`；`be-postgres` 里有库 `brickkit_test_db`（没有就 `docker exec be-postgres createdb -U postgres brickkit_test_db`）；测试每次开始时删掉再建自己的 schema `mdm_currency_it`，跑完留着；端口 28226、9226 要空着，被占时测试直接失败。
- **只有一个测试** `TestCurrencyEndToEnd`：按下表的顺序跑各步（普通的方法调用，不是子测试），后面的步用前面留下的汇率，所以一步不能单独跑。

## 文件

按运行顺序：

| 文件 | 步 | 测的流程 | 被测的模块 |
| --- | --- | --- | --- |
| `currencies_integration_test.go` | `currencies` | ISO 表随迁移提供（KWD 三位小数、JPY 没有），按最小单位舍入；没有授予看不见（404）；部署的决定（现金舍入、译名）、过期的版本被拒、同样的修改只产生一个版本；关键字（译名、拼音首字母）与分页 | `currencies`、`core` |
| `rates_integration_test.go` | `rates` | 汇率类型；每个类型、日期、币种对一条，相同的值不产生新版本，更正是新版本；有效汇率（不晚于某日的最新）；停用的币种被拒；列表最新在前 | `rates` |
| `outbox_integration_test.go` | `outbox` | 每次变化在同一事务里写一个完整状态事件、记审计 | 全部写入的模块 |
| `source_integration_test.go` | `source` | 系统面（mTLS）：`Read`（存在、已清除）、戳、`Snapshot` 分页、超过 500 个被拒、有效汇率 | `source` |
| `recoverycheck_integration_test.go` | `recoveryChecks` | 整套恢复的核对：汇率与汇率类型按版本答“还在不在”（`checksStory`） | 入口 `currency/checks.go` |

共用的：

| 文件 | 内容 |
| --- | --- |
| `endtoend_integration_test.go` | `TestCurrencyEndToEnd`（各步的顺序）；`fixture`（运行中的成员、世界、两个用户、步与步之间传的汇率）；`start`（建世界、迁移并运行成员）；`client.do`、`portFree` |

## 加测试

新流程加一个文件与一步，在 `TestCurrencyEndToEnd` 里按依赖排进顺序，在上表加一行；测试不为了通过而改。
