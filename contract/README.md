# mdm-currency 的契约

`mdm/currency` 发布的契约包：系统面的 protobuf、事件载荷的 JSON Schema、用户面的 OpenAPI。被调方发布契约，放在组件仓库的 `contract/` 子目录里单独发布（与审计组件相同），不另建仓库。

依据：装配项目 [be-assembly-standard](https://github.com/brickKit/be-assembly-standard) 的协议 02（来源要提供什么：S1–S3）、协议 14（标准代码、金额与汇率）、边界卡 `mdm/currency`。

## 内容

| 位置 | 是什么 |
| --- | --- |
| `proto/be/mdm/currency/v3/currency.proto` | `SourceService.Snapshot`、`SourceService.Read`（来源方法）；`RateService.EffectiveRate`（某日的有效汇率） |
| `gen/go/be/mdm/currency/v3/` | Go 代码，包 `currencyv3`；模块 `github.com/brickKit/mdm-currency/contract/v3` |
| `python/` | Python 包 `contract-mdm-currency`，模块 `be.mdm.currency.v3`；`be` 是各契约包共用的命名空间 |
| `events/` | 三种事件的载荷：`mdm.currency.changed.v1`、`mdm.rate_type.changed.v1`、`mdm.exchange_rate.published.v1`；同时是来源方法里 `data` 的格式 |
| `openapi.yaml` | 用户面（给人与前端）；每个路由的操作写在 `x-operation` |

## 来源的义务（协议 02 的 S1–S3）

1. **版本**（S1）：每条记录一个版本，每次变化加一；内容没有变化的写入不产生新版本，也不发事件。
2. **事件**（S2）：每次变化在同一事务里写进发件表，完整状态（载荷就是 `events/` 的 schema），流 `mdm.currency`，4 个分区，分区键是记录身份。
3. **按身份读取**（S3）：每个身份一个结果，顺序与请求相同；存在的给当前版本与完整状态；没有这个身份的答“已清理”（`READ_STATE_PURGED`），从不答“查无此记录”；本组件不删除记录，所以没有墓碑。一次最多 500 个，多了整次拒绝（`BATCH_TOO_LARGE`），不截断。
4. **快照**：按身份升序分页，`next_cursor` 为空时结束；每页最多 500 条。
5. **声明**（S3、协议 02 第 3 步）：每次回答带 `Stamp`：部署身份、实现实例、数据库纪元（`<部署恢复代次>.<时间线>`）与流的各分区位置高点，全部取自与数据同一个读快照。

## 有效汇率

`EffectiveRate` 取同一类型、同一币种对在不晚于请求日期的最近一个日期上的汇率；没有这样的汇率答 `NOT_FOUND`。币种对只按录入的方向，不推导反向与交叉汇率。单据保存回答中的汇率、日期、类型、记录身份与版本。
