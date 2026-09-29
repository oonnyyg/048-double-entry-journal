# 复式记账账簿（Double-Entry Journal）

面向小团队的可追溯复式记账工具：初始化科目、登记凭证、冲销凭证、查询凭证、
科目明细账、期间报表与试算平衡表。仅依赖 Go 标准库，数据保存在本地 JSON 文件，
不依赖任何在线服务。

## 金额口径

- 所有金额均为 **整数分**（例如 `1050` 表示 10.50 元），不使用浮点，也不做多币种换算。
- 分录金额必须为 **正整数**；方向用 `debit`（借）/ `credit`（贷）表示。
- 科目净额口径：**借方增加、贷方减少**，因此净额可为负（表示贷方余额）。
- 一张凭证的借方合计必须严格等于贷方合计，否则整单拒绝，不会留下任何部分分录。

## 构建与运行

```sh
go build ./...
go run ./cmd/journal --state ./data < examples/requests.jsonl
```

程序从标准输入逐行读取 JSON 请求，向标准输出逐行写出 JSON 结果。
`--state` 指定数据目录（必填），账簿持久化在该目录的 `journal_state.json`
（临时文件 + 原子重命名写入）。每次修改类操作成功后立即落盘，正常重开后
明细与报表保持一致。

## 请求格式

每行一个 JSON 对象，通过 `op` 区分操作：

| op               | 说明                 | 关键字段 |
|------------------|----------------------|----------|
| `init`           | 初始化科目（仅一次） | `accounts: [{id, name}]` |
| `post`           | 记账                 | `id`, `date`, `summary`, `entries` |
| `reverse`        | 冲销已记账凭证       | `original_id`, `id`, `date`, `summary?` |
| `get_voucher`    | 查询单张凭证         | `id` |
| `list_accounts`  | 列出全部科目         | 无 |
| `account_ledger` | 科目明细账           | `account_id` |
| `period_report`  | 期间报表（含端点）   | `from`, `to` |
| `trial_balance`  | 截至某日试算平衡表   | `as_of` |

通用字段：

- `date` / `from` / `to` / `as_of` 均为 `YYYY-MM-DD`。
- `entries`：至少两条，`[{account_id, direction, amount}]`；同一科目可出现多次。
  返回结果中附带 `line`（原分录顺序，从 1 开始）。

记账请求示例：

```json
{"op":"post","id":"V1","date":"2026-01-10","summary":"销售收入",
 "entries":[
   {"account_id":"cash","direction":"debit","amount":1000},
   {"account_id":"income","direction":"credit","amount":1000}
 ]}
```

响应统一为 `{"ok":true,"data":...}` 或 `{"ok":false,"error":{"code","message"}}`。

## 业务规则

### 科目

- `init` 接收非空科目列表；科目编号非空且唯一，科目名称非空，初始余额全部为零。
- 已有账簿不能再次初始化覆盖；不支持删除或改名科目。

### 凭证

- 编号唯一非空；日期合法；摘要非空；至少两条分录，科目必须已存在，金额为正整数。
- 已记账凭证不可修改或删除；纠正错误请使用冲销。
- 允许补录较早日期的凭证，所有报表均按 **凭证日期**（而非录入顺序）计算。

### 冲销

- `reverse` 生成金额相同、借贷方向相反的新凭证，并保留双向关联：
  原凭证 `reversed_by` 指向新凭证，新凭证 `reverses_id` 指向原凭证。
- 冲销日期不得早于原凭证日期。
- 禁止重复冲销同一张凭证，也禁止冲销一张本身就是冲销的凭证。
- 冲销不删除原凭证，原凭证仍参与历史报表；冲销后科目的累计净额自动抵消。
- `summary` 可省略，默认为“冲销凭证 <原编号>”。

### 明细账

按日期、凭证编号、原分录顺序（`line`）排序，逐条给出方向、发生额与
累计净额 `net`（借增贷减）。冲销凭证的分录带 `reverses_id`。

### 期间报表

- `from`/`to` 均包含在期间内，且不得倒置。
- 每个科目给出：`opening_net`（期初净额，只累计严格早于 `from` 的凭证）、
  `debit_period`（期间借方发生额）、`credit_period`（期间贷方发生额）、
  `closing_net = opening_net + debit_period - credit_period`。

### 试算平衡表

截至 `as_of`（含当日）：正的期末净额列入借方，负的期末余额取绝对值列入贷方，
零余额两边均为 0；分别给出 `debit_total`、`credit_total` 与 `balanced`。

## 错误码

错误可按 `error.code` 区分：

| code | 含义 |
|------|------|
| `not_initialized` / `already_initialized` | 未初始化 / 已初始化不可覆盖 |
| `invalid_account` / `unknown_account` | 科目定义非法 / 引用了不存在的科目 |
| `invalid_date` / `invalid_period` | 日期格式非法 / 期间倒置 |
| `duplicate_voucher` / `voucher_not_found` | 凭证编号重复（或为空）/ 凭证不存在 |
| `invalid_entry` | 分录非法（条数不足、金额非正、方向非法、摘要为空等） |
| `unbalanced` | 借贷合计不平衡 |
| `already_reversed` / `reverse_of_reversal` / `reverse_date_before_original` | 冲销相关校验失败 |

## 测试

```sh
go test ./...
```

测试覆盖：不平衡凭证不产生部分状态、多分录凭证、非法分录分类、
重复编号/非法日期、补录较早凭证后的排序、冲销全流程（早日期、重复冲销、
冲销冲销凭证）、跨期统计与试算平衡，以及持久化重开后的一致性。

## 目录结构

- `journal/`：核心模型、记账/冲销规则、报表与 JSON 持久化。
- `cmd/journal/`：逐行 JSON 标准输入/输出入口。
- `examples/requests.jsonl`：可直接管道运行的示例请求。
