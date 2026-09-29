package journal

import "sort"

// orderedVouchers 按凭证日期、凭证编号排序返回全部凭证（忽略录入顺序）。
func (l *Ledger) orderedVouchers() []*Voucher {
	list := make([]*Voucher, 0, len(l.st.Vouchers))
	for _, v := range l.st.Vouchers {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Date != list[j].Date {
			return list[i].Date < list[j].Date
		}
		return list[i].ID < list[j].ID
	})
	return list
}

// Ledger 返回某科目的明细账：按日期、凭证编号及原分录顺序排序，
// 每条给出借/贷发生额与累计净额（借为增加、贷为减少）。
func (l *Ledger) Ledger(account string) (*AccountMovement, error) {
	if !l.accountExists(account) {
		return nil, journalError(ErrUnknownAccount, "unknown account %q", account)
	}
	lines := []LedgerLine{}
	var running int64
	for _, v := range l.orderedVouchers() {
		for _, e := range v.Entries {
			if e.Account != account {
				continue
			}
			var debit, credit int64
			if e.Direction == Debit {
				debit = e.Amount
				running += e.Amount
			} else {
				credit = e.Amount
				running -= e.Amount
			}
			lines = append(lines, LedgerLine{
				Date:       v.Date,
				VoucherID:  v.ID,
				Seq:        e.Seq,
				Summary:    v.Summary,
				Debit:      debit,
				Credit:     credit,
				RunningNet: running,
			})
		}
	}
	return &AccountMovement{Account: account, Lines: lines}, nil
}

// PeriodReport 生成包含端点的期间报表。
// 期初只累计起日之前（date < from）的凭证；
// 期末 = 期初 + 期间借方 - 期间贷方。
func (l *Ledger) PeriodReport(from, to string) (*PeriodReport, error) {
	fromT, err := parseDate(from)
	if err != nil {
		return nil, err
	}
	toT, err := parseDate(to)
	if err != nil {
		return nil, err
	}
	if toT.Before(fromT) {
		return nil, journalError(ErrInvalidPeriod, "to date %s is earlier than from date %s", to, from)
	}
	opening := make(map[string]int64)
	deb := make(map[string]int64)
	cred := make(map[string]int64)
	for _, v := range l.orderedVouchers() {
		for _, e := range v.Entries {
			switch {
			case v.Date < from:
				if e.Direction == Debit {
					opening[e.Account] += e.Amount
				} else {
					opening[e.Account] -= e.Amount
				}
			case v.Date >= from && v.Date <= to:
				if e.Direction == Debit {
					deb[e.Account] += e.Amount
				} else {
					cred[e.Account] += e.Amount
				}
			}
		}
	}
	lines := make([]PeriodLine, 0, len(l.st.Accounts))
	for _, a := range l.accountList() {
		closeNet := opening[a.Code] + deb[a.Code] - cred[a.Code]
		lines = append(lines, PeriodLine{
			Account:    a.Code,
			OpeningNet: opening[a.Code],
			DebitMove:  deb[a.Code],
			CreditMove: cred[a.Code],
			ClosingNet: closeNet,
		})
	}
	return &PeriodReport{From: from, To: to, Lines: lines}, nil
}

// TrialBalance 生成期间试算平衡表：正期末余额列入借方，
// 负余额的绝对值列入贷方，零余额两边均为零，并分别合计。
func (l *Ledger) TrialBalance(from, to string) (*TrialBalance, error) {
	report, err := l.PeriodReport(from, to)
	if err != nil {
		return nil, err
	}
	lines := make([]TrialLine, 0, len(report.Lines))
	var totalDebit, totalCredit int64
	for _, p := range report.Lines {
		line := TrialLine{Account: p.Account}
		switch {
		case p.ClosingNet > 0:
			line.Debit = p.ClosingNet
			totalDebit += p.ClosingNet
		case p.ClosingNet < 0:
			line.Credit = -p.ClosingNet
			totalCredit += -p.ClosingNet
		}
		lines = append(lines, line)
	}
	return &TrialBalance{
		From:        from,
		To:          to,
		Lines:       lines,
		TotalDebit:  totalDebit,
		TotalCredit: totalCredit,
	}, nil
}

func (l *Ledger) accountExists(code string) bool {
	for _, a := range l.st.Accounts {
		if a.Code == code {
			return true
		}
	}
	return false
}
