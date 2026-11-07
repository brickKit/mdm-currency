# source：系统面

给部署里其他组件的读取（装配项目协议 02 的来源方法）：按身份读、按快照分页，以及有效汇率；任何组件都可调。

## 负责与不负责

- 负责：`SourceService.Read`（按请求的顺序答：存在，或已清除，本组件从不删除）、`Snapshot`（按键分页，带同一快照的戳）、`RateService.EffectiveRate`；一次最多 `core.MaxBatch` 个。
- 不负责：记录怎样写（`currencies`、`rates`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `source.go` | `aggregates`（三种记录怎样读）、`stamp`（部署、实现实例、纪元与各分区的位置）、`Snapshot`、`Read`、`EffectiveRate` |

## 依赖与被依赖

- 依赖：`core`、`rates`（`Effective`）。
- 被依赖：入口 `currency/services.go`。

## 规则在哪

装配项目协议 02（来源要提供什么、戳）、决定 0304（批量的上限）。

## 测试

`itest/source_integration_test.go`（`source`）。
