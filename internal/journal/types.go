package journal

// Account 是一个会计科目。余额以整数分表示，允许为正或为负。
type Account struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Direction 是分录方向：借或贷。
type Direction string

const (
	Debit  Direction = "debit"
	Credit Direction = "credit"
)

// Entry 是凭证中的一条分录。Seq 记录其在原始凭证中的录入顺序（从 0 开始）。
type Entry struct {
	Account   string    `json:"account"`
	Direction Direction `json:"direction"`
	Amount    int64     `json:"amount"`
	Seq       int       `json:"seq,omitempty"`
}

// Voucher 是一张记账凭证。
type Voucher struct {
	ID         string  `json:"id"`
	Date       string  `json:"date"`
	Summary    string  `json:"summary"`
	Entries    []Entry `json:"entries"`
	ReversalOf string  `json:"reversal_of,omitempty"` // 冲销凭证指向原凭证
	ReversedBy string  `json:"reversed_by,omitempty"` // 原凭证指向冲销凭证
}

// LedgerLine 是科目明细账中的一行。
type LedgerLine struct {
	Date       string `json:"date"`
	VoucherID  string `json:"voucher_id"`
	Seq        int    `json:"seq"`
	Summary    string `json:"summary"`
	Debit      int64  `json:"debit"`
	Credit     int64  `json:"credit"`
	RunningNet int64  `json:"running_net"`
}

// AccountMovement 是科目明细返回结果。
type AccountMovement struct {
	Account string       `json:"account"`
	Lines   []LedgerLine `json:"lines"`
}

// PeriodLine 是期间报表中单个科目的一行。
type PeriodLine struct {
	Account    string `json:"account"`
	OpeningNet int64  `json:"opening_net"`
	DebitMove  int64  `json:"debit_movement"`
	CreditMove int64  `json:"credit_movement"`
	ClosingNet int64  `json:"closing_net"`
}

// PeriodReport 是期间发生额及余额报表。
type PeriodReport struct {
	From  string       `json:"from"`
	To    string       `json:"to"`
	Lines []PeriodLine `json:"lines"`
}

// TrialLine 是试算表中单个科目的一行。
type TrialLine struct {
	Account string `json:"account"`
	Debit   int64  `json:"debit"`
	Credit  int64  `json:"credit"`
}

// TrialBalance 是试算平衡表。
type TrialBalance struct {
	From        string      `json:"from"`
	To          string      `json:"to"`
	Lines       []TrialLine `json:"lines"`
	TotalDebit  int64       `json:"total_debit"`
	TotalCredit int64       `json:"total_credit"`
}

// ledgerState 是持久化到数据目录的完整账簿状态。
type ledgerState struct {
	Accounts []*Account          `json:"accounts"`
	Vouchers map[string]*Voucher `json:"vouchers"`
}
