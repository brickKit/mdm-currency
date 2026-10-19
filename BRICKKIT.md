# mdm/currency

## 组件定位

币种与汇率的参照表，所有使用金额的客户都需要（单币种客户也要币种定义）：
- **币种**：ISO 4217 全表随组件提供，以代码为主键，带最小单位、舍入方式与现金舍入单位。部署能决定的只有启用与停用、舍入方式、现金舍入单位与译名；代码、数字代码、最小单位与默认名称来自标准，不在这里改。
- **汇率类型**：随组件只带 `spot`，其余由客户维护。
- **每日汇率**：同一类型、同一日期、同一币种对只有一个有效汇率；更正是同一条记录的新版本，并重新发布。
- **按日期取汇率**：取不晚于该日期的最近一条，回答带汇率日期。单据保存汇率快照（汇率、日期、类型、记录身份与版本），之后汇率被更正也不影响已有单据。

金额只在 SDK 里舍入：调用方按币种的最小单位与舍入方式调用 SDK 的舍入函数，本组件只提供这两个值。币种对只按录入的方向：不推导反向汇率与交叉汇率。

实验第 6 步的范围：汇率来源适配器还没有，汇率只能人工维护。

## 部署前准备

- 数据库：自己的 schema、运行角色与属主角色（装配项目注册表的 `mdm_currency`）。迁移写入 ISO 4217 全表与汇率类型 `spot`。
- 消息系统（`BUS_URL`）与实现实例（`IMPLEMENTATION_INSTANCE`，安装时生成的随机身份）：每次变化以完整状态事件发布到流 `mdm.currency`（4 个分区）。
- 部署的服务证书（`SERVICE_TLS_*`）：系统面只接受 mTLS，部署里每个组件都可以调用。
- 身份与授权（`IAM_*`、`AUTHZ_GRPC_URL`、`DEPLOYMENT_ID`）：用户面受授权保护；审计（`AUDIT_GRPC_URL`）记录每次维护。

## 依赖说明

- 必需：数据库、消息系统、身份成员、授权成员。
- 可选：审计组件（缺时审计记录留在本地，按协议 03 处理）。
- 不调用任何业务组件。

## 配置指南

全部键见 `component.yaml` 的 `configSchema`（由 SDK 生成，测试核对）。

| 键 | 说明 |
| --- | --- |
| `DEPLOYMENT_ID`、`IAM_ISSUER`、`IAM_JWKS_URL`、`AUTHZ_GRPC_URL`、`SERVICE_TLS_CERT_FILE`、`SERVICE_TLS_KEY_FILE`、`SERVICE_TLS_CA_FILE`（必填） | 受保护的用户面与 mTLS 的系统面要用；部署身份、身份与授权的地址在外壳里是进程级的键 |
| `PG_HOST`、`PG_DATABASE`、`PG_USER`、`PG_PASSWORD`、`PG_SCHEMA`（必填） | 自己的数据库，密码是密钥；迁移另用 `PG_OWNER_USER`、`PG_OWNER_PASSWORD` |
| `BUS_URL`（必填，密钥）、`IMPLEMENTATION_INSTANCE`（必填） | 消息系统；实现实例是安装时生成的随机身份，事件与来源方法的回答都带它（协议 02） |
| `AUDIT_GRPC_URL` | 审计组件的系统面；缺时审计记录留在本地，按协议 03 处理 |

## 契约索引

- **系统面**（本仓库 `contract/proto`，模块 `github.com/brickKit/mdm-currency/contract/v3`，标签 `contract/v3.…`）：
  - `be.mdm.currency.v3.SourceService/Snapshot`、`Read`：协议 02 的来源方法，聚合是币种、汇率类型、汇率；每次回答带部署、实现实例、纪元与流的各分区位置高点；一次最多 500 个。
  - `be.mdm.currency.v3.RateService/EffectiveRate`：某日的有效汇率。
- **用户面**：`contract/openapi.yaml`。操作 `mdm.currency.read`、`mdm.currency.manage`（币种与汇率类型）、`mdm.exchange_rate.read`、`mdm.exchange_rate.maintain`（汇率）。
- **事件**（流 `mdm.currency`，载荷的 schema 在 `contract/events/`）：`mdm.currency.changed.v1`、`mdm.rate_type.changed.v1`、`mdm.exchange_rate.published.v1`，都是完整状态。

## 外壳声明

不是外壳。可以被 Go 的基础外壳（`be/go-infra`）承载。
