# mdm/currency

币种与汇率参照组件的代码。用法与边界在 `BRICKKIT.md`，为什么这样设计（术语、数据、规则）在 [docs/design.md](docs/design.md)；跨组件的规则在装配项目 be-assembly-standard 的协议 02（来源要提供什么）、协议 14（标准代码、金额与舍入、列表）。

## 代码地图

代码按业务模块分层。**先读这一节，再进目录**：每个代码目录都有自己的 `AGENTS.md`（同目录的 `CLAUDE.md` 让 Claude Code 读这个目录的文件时自动带上它），写这一层负责什么、不负责什么、每个文件、依赖谁、测在哪。

| 目录 | 是什么 |
| --- | --- |
| [`currency/`](currency/AGENTS.md) | 入口：组件定义与组装（路由、系统面、后台工作、恢复核对都在这里登记，转给业务模块）；不写业务。`main.go` 与外壳只引用它 |
| [`internal/`](internal/AGENTS.md) | 业务：下面的模块各一个包；模块之间的依赖表在它的 `AGENTS.md` |
| [`itest/`](itest/AGENTS.md) | 集成测试，按业务流程分文件；它的 `AGENTS.md` 是测试索引 |
| `currency/migrations/` | `0001` 表；`0002` ISO 4217 全表与 `spot`（`scripts/iso4217.py` 生成） |
| `cmd/seed/` | 种子：角色与授予、汇率类型、双向的汇率（`make seed`） |
| `contract/` | 本组件发布的契约，单独的模块：`contract/proto/`、`contract/events/`（载荷 schema）、`contract/openapi.yaml` |
| `docs/design.md` | 设计：术语、表、规则、命令、事件、实现状态 |

业务模块（`internal/` 下）：

| 模块 | 负责 | 拥有的表 |
| --- | --- | --- |
| [`currencies`](internal/currencies/AGENTS.md) | 币种：读取、部署能改的那几项（现金舍入、译名、启用）、列表与关键字、搜索列的回填 | `currency` |
| [`rates`](internal/rates/AGENTS.md) | 汇率类型与汇率：录入与更正、有效汇率、列表 | `rate_type`、`exchange_rate` |
| [`source`](internal/source/AGENTS.md) | 系统面：给其他组件的来源方法（`Read`、`Snapshot`）与有效汇率 | — |

共用的：[`core`](internal/core/AGENTS.md)（名字与资源定义、校验、审计与事件的写法、记录的类型与读取、列表的共用部分）。

| 想改 | 从这里开始 | 然后 |
| --- | --- | --- |
| 汇率的规则（唯一性、更正、有效汇率） | `internal/rates` 的 `PutRate`、`internal/core/records.go` 的 `Effective` | `itest/rates_integration_test.go` |
| 币种表 | `scripts/iso4217.py` | 重新生成 `0002`，`itest/currencies_integration_test.go` |
| 部署能改币种的哪几项 | `internal/currencies` 的 `UpdateCurrency` | `docs/design.md` 的规则 |
| 加一个用户面路由 | `contract/openapi.yaml` | 处理函数写在它的模块里，在 `currency/module.go` 的 `Routes` 登记；`contract_test.go` 核对两边一致 |
| 加字段、加接口、改事件 | 装配项目 be-assembly-standard 规范里的“改动配方” | `docs/design.md` 的数据与命令 |

## 构建与测试

- `go test ./...`（`make test`）：单元测试：入口包的清单、OpenAPI 与路由、事件载荷、合并安全；`internal/core` 的校验。
- `make test-integration`（`go test -tags integration ./itest/`）：真实的 PostgreSQL 与消息系统；每个文件测哪条流程、要什么前提，在 [itest/AGENTS.md](itest/AGENTS.md)。
- `brickkit lint`

## 设计取舍

- **ISO 数据随迁移提供，部署只改它能决定的**：最小单位一旦被单据用过就不能改，所以不开放修改；标准有变化时随新版本的迁移更新。
- **币种对只按录入的方向**：反向与交叉汇率的取法（取倒数、经哪个币种）是业务政策，留给调用方或以后的汇率来源适配器，不在这里隐式推导。
- **相同内容再写一次不产生新版本**：重复的录入不发事件，消费者不必处理无变化的版本。
- **搜索列由回填任务写**：迁移写入的行没有搜索列（拼音只能在 SDK 里算），单例任务每 10 分钟补一次，SDK 的规范化版本升高时也由它重写。

## 易错点

- 金额不在这里舍入：本组件只提供最小单位与舍入方式，舍入在 SDK 里做（协议 14）。
- 汇率的文本没有末尾零（`trim_scale`）：契约的十进制字符串与规范编码一致；比较汇率是否相同按数值，`7.65` 与 `7.6500` 是同一个汇率。

## 改代码前自查

- 改了契约（`contract/`）：proto 改后 `make gen`，发布 `contract/v…`，再改这里的依赖；OpenAPI 与路由由测试核对。
- 改了版本号吗？每次改动都升版本。

<!-- brickkit:managed:begin lang=zh -->
<!-- 由 brickkit 维护（init、add、remove、upgrade、skills update）：这对标记之间的改动会被覆盖 -->

## BrickKit

这是一个 BrickKit 组件：平台只读 `component.yaml`。它依赖的规则：

- `configSchema` 的键就是代码读的环境变量名。不能用保留名：`COMPONENT_ID`、`COMPONENT_VERSION`、`PORT`、`BRICKKIT_SERVED_MEMBERS`、`BRICKKIT_SERVED_MEMBERS_CONFIG`，以及任何 `*_ENDPOINT`。
- 依赖写精确版本。依赖的地址以 `<ID>_ENDPOINT` 注入；缺席的可选依赖根本没有这个变量，读的时候要带兜底。
- `/healthz` 只查本进程，不查依赖。迁移命令用同一个镜像跑，遇到不认识的参数必须直接失败；`local.migrationCommand` 从源码跑同一条迁移，给 `mode: local` 用。
- `BRICKKIT.md` 会随版本进入每个使用它的项目，在那里是脱离仓库单独读的：跟代码一起改，不放相对链接。
- 发版：改 `metadata.version`，提交、推送，`brickkit release`。`brickkit lint` 会检查清单和这些文档——在项目里、在这个目录下跑，只查这个组件（`--all` 查整个项目）。
- 完整规则在 `brickkit-component` 技能里（装了技能的项目或仓库根目录下的 `.claude/skills/brickkit-component/SKILL.md`；`brickkit skills update` 会装上）；参数问 `brickkit <命令> --help`；BrickKit 自己的文档用 `brickkit docs` 看。
<!-- brickkit:managed:end -->
