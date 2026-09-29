package journal

import "testing"

func accounts() []Account {
	return []Account{
		{ID: "cash", Name: "现金"},
		{ID: "income", Name: "收入"},
		{ID: "rent", Name: "房租"},
		{ID: "ap", Name: "应付款"},
	}
}

func TestInitValidation(t *testing.T) {
	l := NewLedger()
	if err := l.Init(nil); err == nil || err.(*Error).Code != ErrInvalidAccount {
		t.Fatalf("empty init: %v", err)
	}
	if err := l.Init([]Account{{ID: " ", Name: "x"}}); err == nil || err.(*Error).Code != ErrInvalidAccount {
		t.Fatalf("blank id: %v", err)
	}
	if err := l.Init([]Account{{ID: "a", Name: "A"}, {ID: "a", Name: "B"}}); err == nil || err.(*Error).Code != ErrInvalidAccount {
		t.Fatalf("dup id: %v", err)
	}
	if err := l.Init(accounts()); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := l.Init(accounts()); err == nil || err.(*Error).Code != ErrAlreadyInitialized {
		t.Fatalf("reinit: %v", err)
	}
	if err := l.requireReady(); err != nil {
		t.Fatalf("ready: %v", err)
	}
}

func TestPostUnbalancedLeavesNoState(t *testing.T) {
	l := NewLedger()
	if err := l.Init(accounts()); err != nil {
		t.Fatal(err)
	}
	bad := []Entry{
		{AccountID: "cash", Direction: "debit", Amount: 100},
		{AccountID: "income", Direction: "credit", Amount: 90},
	}
	_, err := l.Post("V1", "2026-01-01", "不平衡", bad)
	if err == nil || err.(*Error).Code != ErrUnbalanced {
		t.Fatalf("want unbalanced, got %v", err)
	}
	if len(l.Vouchers) != 0 {
		t.Fatalf("partial voucher persisted: %d", len(l.Vouchers))
	}

	// 未知科目 / 非法金额 / 非法方向也必须失败。
	cases := [][]Entry{
		{{AccountID: "nope", Direction: "debit", Amount: 1}, {AccountID: "cash", Direction: "credit", Amount: 1}},
		{{AccountID: "cash", Direction: "debit", Amount: 0}, {AccountID: "income", Direction: "credit", Amount: 0}},
		{{AccountID: "cash", Direction: "left", Amount: 1}, {AccountID: "income", Direction: "credit", Amount: 1}},
		{{AccountID: "cash", Direction: "debit", Amount: 1}},
	}
	want := []ErrorCode{ErrUnknownAccount, ErrInvalidEntry, ErrInvalidEntry, ErrInvalidEntry}
	for i, cs := range cases {
		_, err := l.Post("X", "2026-01-01", "s", cs)
		if err == nil || err.(*Error).Code != want[i] {
			t.Fatalf("case %d: want %s, got %v", i, want[i], err)
		}
	}
}

func TestMultiEntryPostAndDuplicateAndDate(t *testing.T) {
	l := NewLedger()
	_ = l.Init(accounts())
	entries := []Entry{
		{AccountID: "rent", Direction: "debit", Amount: 500},
		{AccountID: "ap", Direction: "debit", Amount: 200},
		{AccountID: "cash", Direction: "credit", Amount: 400},
		{AccountID: "income", Direction: "credit", Amount: 300},
	}
	v, err := l.Post("V1", "2026-02-03", "多分录", entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Entries) != 4 || v.Entries[3].Line != 4 {
		t.Fatalf("entries/lines wrong: %+v", v.Entries)
	}
	if _, err := l.Post("V1", "2026-02-03", "重复", entries); err.(*Error).Code != ErrDuplicateVoucher {
		t.Fatalf("dup: %v", err)
	}
	if _, err := l.Post("V2", "2026-13-01", "坏日期", entries); err.(*Error).Code != ErrInvalidDate {
		t.Fatalf("date: %v", err)
	}
}

func TestReverseFlow(t *testing.T) {
	l := NewLedger()
	_ = l.Init(accounts())
	_, _ = l.Post("V1", "2026-01-10", "收款", []Entry{
		{AccountID: "cash", Direction: "debit", Amount: 1000},
		{AccountID: "income", Direction: "credit", Amount: 1000},
	})

	// 冲销日早于原日期。
	if _, err := l.Reverse("V1", "R1", "2026-01-09", ""); err.(*Error).Code != ErrReverseDateEarly {
		t.Fatalf("early: %v", err)
	}
	// 原凭证不存在。
	if _, err := l.Reverse("NOPE", "R1", "2026-01-11", ""); err.(*Error).Code != ErrVoucherNotFound {
		t.Fatalf("notfound: %v", err)
	}

	rev, err := l.Reverse("V1", "R1", "2026-01-11", "")
	if err != nil {
		t.Fatal(err)
	}
	if rev.Entries[0].Direction != "credit" || rev.Entries[1].Direction != "debit" {
		t.Fatalf("reversal directions: %+v", rev.Entries)
	}
	if rev.ReversesID != "V1" {
		t.Fatalf("reverses link: %s", rev.ReversesID)
	}
	orig, _ := l.GetVoucher("V1")
	if orig.ReversedBy != "R1" {
		t.Fatalf("reversed-by link: %s", orig.ReversedBy)
	}

	// 重复冲销。
	if _, err := l.Reverse("V1", "R2", "2026-01-12", ""); err.(*Error).Code != ErrAlreadyReversed {
		t.Fatalf("dup reverse: %v", err)
	}
	// 冲销一张冲销凭证。
	if _, err := l.Reverse("R1", "R3", "2026-01-13", ""); err.(*Error).Code != ErrReverseOfReversal {
		t.Fatalf("reverse reversal: %v", err)
	}

	// 冲销后净额归零，且原凭证仍参与历史报表。
	rep, _ := l.AccountLedger("cash")
	if rep.Net != 0 || len(rep.Lines) != 2 {
		t.Fatalf("cash ledger net=%d lines=%d", rep.Net, len(rep.Lines))
	}
	if rep.Lines[0].Net != 1000 || rep.Lines[1].Net != 0 {
		t.Fatalf("running nets: %d %d", rep.Lines[0].Net, rep.Lines[1].Net)
	}
}

func TestBackdatedOrderingAndPeriodReport(t *testing.T) {
	l := NewLedger()
	_ = l.Init(accounts())
	// 故意先录晚日期，再补录早日期。
	_, _ = l.Post("V2", "2026-02-15", "二月", []Entry{
		{AccountID: "cash", Direction: "debit", Amount: 200},
		{AccountID: "income", Direction: "credit", Amount: 200},
	})
	_, _ = l.Post("V1", "2026-01-05", "一月补录", []Entry{
		{AccountID: "cash", Direction: "debit", Amount: 100},
		{AccountID: "income", Direction: "credit", Amount: 100},
	})

	led, _ := l.AccountLedger("cash")
	if led.Lines[0].VoucherID != "V1" || led.Lines[1].VoucherID != "V2" {
		t.Fatalf("ledger not date ordered: %s %s", led.Lines[0].VoucherID, led.Lines[1].VoucherID)
	}

	// 期间 [01-10, 02-15]：V1 计入期初，V2 计入期间，端点包含。
	rep, err := l.PeriodReport("2026-01-10", "2026-02-15")
	if err != nil {
		t.Fatal(err)
	}
	cash := findRow(rep, "cash")
	if cash.OpeningNet != 100 || cash.DebitPeriod != 200 || cash.ClosingNet != 300 {
		t.Fatalf("cash row: %+v", cash)
	}
	income := findRow(rep, "income")
	if income.OpeningNet != -100 || income.CreditPeriod != 200 || income.ClosingNet != -300 {
		t.Fatalf("income row: %+v", income)
	}

	if _, err := l.PeriodReport("2026-03-01", "2026-02-01"); err.(*Error).Code != ErrInvalidPeriod {
		t.Fatalf("inverted period: %v", err)
	}

	// 试算表：正余额入借方，负余额绝对值入贷方，合计相等。
	tb, err := l.TrialBalanceAt("2026-02-15")
	if err != nil {
		t.Fatal(err)
	}
	if tb.DebitTotal != 300 || tb.CreditTotal != 300 || !tb.Balanced {
		t.Fatalf("trial: %+v", tb)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	l, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Init(accounts())
	_, _ = l.Post("V1", "2026-01-10", "收款", []Entry{
		{AccountID: "cash", Direction: "debit", Amount: 1000},
		{AccountID: "income", Direction: "credit", Amount: 1000},
	})
	_, _ = l.Reverse("V1", "R1", "2026-01-12", "")
	if err := l.Save(dir); err != nil {
		t.Fatal(err)
	}

	l2, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	orig, _ := l2.GetVoucher("V1")
	if orig.ReversedBy != "R1" {
		t.Fatalf("link lost: %s", orig.ReversedBy)
	}
	rev, _ := l2.GetVoucher("R1")
	if rev.ReversesID != "V1" {
		t.Fatalf("reverse link lost")
	}
	led, _ := l2.AccountLedger("cash")
	if led.Net != 0 {
		t.Fatalf("net after reload: %d", led.Net)
	}
	tb, _ := l2.TrialBalanceAt("2026-12-31")
	if !tb.Balanced {
		t.Fatal("trial not balanced after reload")
	}
}

func findRow(r *PeriodReport, id string) PeriodRow {
	for _, row := range r.Rows {
		if row.AccountID == id {
			return row
		}
	}
	return PeriodRow{}
}
