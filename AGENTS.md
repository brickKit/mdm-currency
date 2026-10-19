# mdm/currency

币种与汇率参照组件的代码。先读 `BRICKKIT.md`；依据是装配项目 be-assembly-standard 的边界卡 `docs/zh/07-components/mdm.md`（`mdm/currency`）、协议 02（来源要提供什么）、协议 14（标准代码、金额与舍入、列表）。

## 代码地图

| 位置 | 内容 |
| --- | --- |
| `currency/definition.go` | 组件定义：身份、流与事件类型、资源与操作 |
| `currency/module.go` | 用户面：币种的修改、汇率类型、汇率的录入与更正、有效汇率；审计与事件 |
| `currency/lists.go` | 三个列表（游标分页、筛选、关键字） |
| `currency/records.go` | 记录的类型、读取与事件 |
| `currency/source.go` | 系统面（来源方法、有效汇率）与搜索列的回填任务 |
| `currency/migrations/` | `0001` 表；`0002` ISO 4217 全表与 `spot`（`scripts/iso4217.py` 生成） |
| `contract/` | 本组件发布的契约，单独的模块：`contract/proto/`、`contract/events/`（载荷 schema）、`contract/openapi.yaml` |

| 想改 | 从这里开始 | 然后 |
| --- | --- | --- |
| 汇率的规则（唯一性、更正、有效汇率） | `module.go` 的 `putRate`、`records.go` 的 `effective` | 集成测试的汇率一段 |
| 币种表 | `scripts/iso4217.py` | 重新生成 `0002`，集成测试 |
| 加一个用户面路由 | `contract/openapi.yaml` | `module.go` 的 `Routes`；`contract_test.go` 核对两边一致 |

## 构建与测试

- `go test ./...`：单元测试，核对清单的 `configSchema`、OpenAPI 与路由、事件载荷与 schema、合并安全。
- `go test -tags integration ./currency/`：对真实的 PostgreSQL 与消息系统（`BE_SDK_TEST_PG_ADMIN`、`BE_SDK_TEST_NATS_URL`，与 be-sdk-go 的集成测试相同），用 be-sdk-go 的 `sectest` 测试部署。
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
