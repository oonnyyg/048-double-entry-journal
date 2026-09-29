# 复式记账簿（Double-Entry Journal）

一个仅依赖 Go 标准库的命令行复式记账工具。通过 `--state` 指定数据目录，
进程从标准输入逐行读取 JSON 请求，每行返回一条 JSON 结果，不依赖任何在线服务。

## 构建与运行

```sh
go build ./cmd/journal
./journal --state ./data
```

每行输入一个 JSON 请求对象（统一信封），输出同样是每行一个 JSON 响应：

- 成功：`{"ok":true,"result":...}`
- 失败：`{"ok":false,"error":{"code":"...","message":"..."}}`

空行被忽略；非法 JSON 返回 `invalid_json` 错误，进程继续处理后续行。

## 金额口径

- 所有金额均为 **整数分**（int64），不使用浮点、不做币种换算。
- 分录金额必须为正整数；科目余额允许为正或为负。
- 借方（`debit`）增加科目净额，贷方（`credit`）减少科目净额。

## 请求格式

### 初始化科目 `init`

科目编号非空、列表内唯一；初始余额全部为零。已有账簿不得重复初始化，
科目不支持删除或改名。

```json
{"op":"init","accounts":[{"code":"1001","name":"Cash"},{"code":"4001","name":"Revenue"}]}
```

### 科目表 `accounts`

```json
{"op":"accounts"}
```

### 记账 `post`

凭证编号唯一非空，日期为 `YYYY-MM-DD`，摘要非空，至少两条分录；
分录须引用已存在科目、方向为 `debit`/`credit`、金额为正整数，同一科目可出现多次；
整张凭证借方合计必须等于贷方合计。任何校验失败都不会留下部分分录或改变余额。
允许补录较早日期的凭证，报表一律按凭证日期计算。

```json
{"op":"post","voucher":{"id":"V1","date":"2026-01-15","summary":"sale",
  "entries":[
    {"account":"1001","direction":"debit","amount":500},
    {"account":"4001","direction":"credit","amount":500}
  ]}}
```

### 冲销 `reverse`

为原凭证生成金额相同、借贷方向相反的新凭证，并在原凭证与冲销凭证之间保留双向关联
（`reversal_of` / `reversed_by`）。冲销日期不得早于原日期；禁止重复冲销、
禁止冲销一张冲销凭证；失败不改变原凭证状态。原凭证仍参与历史报表。

```json
{"op":"reverse","reverse":{"original_id":"V1","new_id":"R1","date":"2026-02-01"}}
```

### 凭证查询 `get_voucher`

返回原始分录（含录入顺序）及冲销关联。

```json
{"op":"get_voucher","id":"V1"}
```

### 科目明细 `ledger`

按凭证日期、凭证编号、原分录顺序排序，给出每条借方/贷方发生额与累计净额。

```json
{"op":"ledger","account":"1001"}
```

### 期间报表 `period_report`

起止日期均包含端点。对每个科目给出：
`opening_net`（期初净额，只累计起日之前的凭证）、`debit_movement`、
`credit_movement`、`closing_net`（= 期初 + 期间借方 − 期间贷方）。

```json
{"op":"period_report","from":"2026-01-01","to":"2026-01-31"}
```

### 试算平衡表 `trial_balance`

在同一期间口径下，正期末余额列入借方，负余额的绝对值列入贷方，零余额两边均为零，
并给出 `total_debit` / `total_credit` 合计。

```json
{"op":"trial_balance","from":"2026-01-01","to":"2026-01-31"}
```

## 错误码

错误通过 `error.code` 区分：`already_initialized`、`not_initialized`、
`empty_account_list`、`empty_account_code`、`duplicate_account`、`unknown_account`、
`duplicate_voucher_id`、`unknown_voucher`、`invalid_date`、`invalid_period`、
`empty_voucher_id`、`empty_summary`、`invalid_entry`、`voucher_not_balanced`、
`already_reversed`、`reversing_reversal`、`reversal_before_original_date`、
`invalid_json`。

## 持久化

全部状态保存在 `--state` 目录下的 `journal.json`，采用“临时文件 + 重命名”原子写入；
重新打开进程后，科目明细与期间报表保持一致。

## 测试

```sh
go test ./...
```

测试覆盖：初始化保护、不平衡/非法分录不落库、多分录与同科目多次出现、
较早日期补录、冲销规则（提前日期、重复冲销、冲销冲销凭证）、
冲销后原凭证仍参与历史报表、跨期期初/期末统计、试算平衡，以及重开持久化一致性。
