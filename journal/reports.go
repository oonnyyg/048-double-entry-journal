package journal

import (
	"sort"
	"strings"
)

// LedgerLine 是科目明细账中的一条发生额。
type LedgerLine struct {
	Date       string `json:"date"`
	VoucherID  string `json:"voucher_id"`
	Line       int    `json:"line"`
	Summary    string `json:"summary"`
	Direction  string `json:"direction"`
	Amount     int64  `json:"amount"` // 该条分录金额（正整数）
	Net        int64  `json:"net"`    // 累计净额：借增贷减
	ReversesID string `json:"reverses_id,omitempty"`
}

// LedgerReport 是单个科目的明细账。
type LedgerReport struct {
	AccountID string       `json:"account_id"`
	Lines     []LedgerLine `json:"lines"`
	Net       int64        `json:"net"` // 期末累计净额
}

// AccountLedger 返回某科目的明细，按日期、凭证编号、原分录顺序排序。
func (l *Ledger) AccountLedger(accountID string) (*LedgerReport, error) {
	if _, ok := l.Accounts[accountID]; !ok {
		return nil, errf(ErrUnknownAccount, "unknown account %q", accountID)
	}
	report := &LedgerReport{AccountID: accountID, Lines: []LedgerLine{}}
	var net int64
	for _, v := range l.sortedVouchers() {
		for _, e := range v.Entries {
			if e.AccountID != accountID {
				continue
			}
			if e.Direction == "debit" {
				net += e.Amount
			} else {
				net -= e.Amount
			}
			report.Lines = append(report.Lines, LedgerLine{
				Date:       v.Date,
				VoucherID:  v.ID,
				Line:       e.Line,
				Summary:    v.Summary,
				Direction:  e.Direction,
				Amount:     e.Amount,
				Net:        net,
				ReversesID: v.ReversesID,
			})
		}
	}
	report.Net = net
	return report, nil
}

// PeriodRow 是期间报表中单个科目的一行。
type PeriodRow struct {
	AccountID    string `json:"account_id"`
	OpeningNet   int64  `json:"opening_net"`   // 期初净额
	DebitPeriod  int64  `json:"debit_period"`  // 期间借方发生额
	CreditPeriod int64  `json:"credit_period"` // 期间贷方发生额
	ClosingNet   int64  `json:"closing_net"`   // 期末净额 = 期初 + 借 - 贷
}

// PeriodReport 是包含端点的期间发生额/余额报表。
type PeriodReport struct {
	From string      `json:"from"`
	To   string      `json:"to"`
	Rows []PeriodRow `json:"rows"`
}

func (l *Ledger) PeriodReport(from, to string) (*PeriodReport, error) {
	if _, err := parseDate(from); err != nil {
		return nil, err
	}
	tto, err := parseDate(to)
	if err != nil {
		return nil, err
	}
	if from > to {
		return nil, errf(ErrInvalidPeriod, "from %s after to %s", from, to)
	}
	rows := map[string]*PeriodRow{}
	ids := make([]string, 0, len(l.Accounts))
	for id := range l.Accounts {
		ids = append(ids, id)
		rows[id] = &PeriodRow{AccountID: id}
	}
	sort.Strings(ids)
	for _, v := range l.sortedVouchers() {
		for _, e := range v.Entries {
			r := rows[e.AccountID]
			switch {
			case v.Date < from:
				if e.Direction == "debit" {
					r.OpeningNet += e.Amount
				} else {
					r.OpeningNet -= e.Amount
				}
			case v.Date <= tto.Format(DateLayout):
				if e.Direction == "debit" {
					r.DebitPeriod += e.Amount
				} else {
					r.CreditPeriod += e.Amount
				}
			}
		}
	}
	rep := &PeriodReport{From: from, To: to, Rows: make([]PeriodRow, 0, len(ids))}
	for _, id := range ids {
		r := rows[id]
		r.ClosingNet = r.OpeningNet + r.DebitPeriod - r.CreditPeriod
		rep.Rows = append(rep.Rows, *r)
	}
	return rep, nil
}

// TrialRow 是试算平衡表的一行。
type TrialRow struct {
	AccountID     string `json:"account_id"`
	DebitBalance  int64  `json:"debit_balance"`
	CreditBalance int64  `json:"credit_balance"`
}

// TrialBalance 依据某日（含当日）累计净额生成试算表。
type TrialBalance struct {
	AsOf        string     `json:"as_of"`
	Rows        []TrialRow `json:"rows"`
	DebitTotal  int64      `json:"debit_total"`
	CreditTotal int64      `json:"credit_total"`
	Balanced    bool       `json:"balanced"`
}

// TrialBalanceAt 生成截至 asOf（含）的试算平衡表。
func (l *Ledger) TrialBalanceAt(asOf string) (*TrialBalance, error) {
	if strings.TrimSpace(asOf) == "" {
		return nil, errf(ErrInvalidDate, "empty as_of date")
	}
	if _, err := parseDate(asOf); err != nil {
		return nil, err
	}
	net := map[string]int64{}
	for _, v := range l.sortedVouchers() {
		if v.Date > asOf {
			continue
		}
		for _, e := range v.Entries {
			if e.Direction == "debit" {
				net[e.AccountID] += e.Amount
			} else {
				net[e.AccountID] -= e.Amount
			}
		}
	}
	ids := make([]string, 0, len(l.Accounts))
	for id := range l.Accounts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	tb := &TrialBalance{AsOf: asOf, Rows: []TrialRow{}}
	for _, id := range ids {
		n := net[id]
		row := TrialRow{AccountID: id}
		if n > 0 {
			row.DebitBalance = n
			tb.DebitTotal += n
		} else if n < 0 {
			row.CreditBalance = -n
			tb.CreditTotal += -n
		}
		tb.Rows = append(tb.Rows, row)
	}
	tb.Balanced = tb.DebitTotal == tb.CreditTotal
	return tb, nil
}
