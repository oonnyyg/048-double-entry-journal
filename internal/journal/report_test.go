package journal

import "testing"

func postVoucher(t *testing.T, l *Ledger, in VoucherInput) {
	t.Helper()
	if _, err := l.Post(in); err != nil {
		t.Fatalf("post %s: %v", in.ID, err)
	}
}

func sampleLedger(t *testing.T) *Ledger {
	l := newTestLedger(t)
	initAccounts(t, l)
	// 1月15日：借现金 500 / 贷收入 500
	postVoucher(t, l, VoucherInput{
		ID: "V1", Date: "2026-01-15", Summary: "sale",
		Entries: []EntryInput{
			{Account: "1001", Direction: Debit, Amount: 500},
			{Account: "4001", Direction: Credit, Amount: 500},
		},
	})
	// 2月5日：借应付 200 / 贷现金 200
	postVoucher(t, l, VoucherInput{
		ID: "V2", Date: "2026-02-05", Summary: "pay",
		Entries: []EntryInput{
			{Account: "2001", Direction: Debit, Amount: 200},
			{Account: "1001", Direction: Credit, Amount: 200},
		},
	})
	return l
}

func TestLedgerMovementOrderingAndRunningNet(t *testing.T) {
	l := sampleLedger(t)
	// 补录一张更早日期、同科目多次出现的凭证。
	postVoucher(t, l, VoucherInput{
		ID: "V0", Date: "2026-01-01", Summary: "opening batch",
		Entries: []EntryInput{
			{Account: "1001", Direction: Debit, Amount: 100},
			{Account: "1001", Direction: Debit, Amount: 50},
			{Account: "4001", Direction: Credit, Amount: 150},
		},
	})
	mv, err := l.Ledger("1001")
	if err != nil {
		t.Fatal(err)
	}
	if len(mv.Lines) != 4 {
		t.Fatalf("want 4 cash lines, got %d", len(mv.Lines))
	}
	want := []struct {
		date string
		id   string
		net  int64
	}{
		{"2026-01-01", "V0", 100},
		{"2026-01-01", "V0", 150},
		{"2026-01-15", "V1", 650},
		{"2026-02-05", "V2", 450},
	}
	for i, w := range want {
		got := mv.Lines[i]
		if got.Date != w.date || got.VoucherID != w.id || got.RunningNet != w.net {
			t.Fatalf("line %d = %+v, want date=%s id=%s net=%d", i, got, w.date, w.id, w.net)
		}
	}
}

func TestPeriodReportInclusiveAndBackdated(t *testing.T) {
	l := sampleLedger(t)
	rep, err := l.PeriodReport("2026-02-01", "2026-02-28")
	if err != nil {
		t.Fatal(err)
	}
	byCode := map[string]PeriodLine{}
	for _, ln := range rep.Lines {
		byCode[ln.Account] = ln
	}
	cash := byCode["1001"]
	if cash.OpeningNet != 500 || cash.DebitMove != 0 || cash.CreditMove != 200 || cash.ClosingNet != 300 {
		t.Fatalf("cash line wrong: %+v", cash)
	}
	payable := byCode["2001"]
	if payable.OpeningNet != 0 || payable.DebitMove != 200 || payable.ClosingNet != 200 {
		t.Fatalf("payable line wrong: %+v", payable)
	}

	// 端点包含：1月15日凭证应计入 1/15 当天起的期间。
	rep2, err := l.PeriodReport("2026-01-15", "2026-01-15")
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Lines[0].OpeningNet != 0 || rep2.Lines[0].DebitMove != 500 {
		t.Fatalf("inclusive endpoints failed: %+v", rep2.Lines)
	}

	if _, err := l.PeriodReport("2026-03-01", "2026-02-01"); !errorIs(err, ErrInvalidPeriod) {
		t.Fatalf("want inverted period error, got %v", err)
	}
}

func TestTrialBalanceBalances(t *testing.T) {
	l := sampleLedger(t)
	tb, err := l.TrialBalance("2026-01-01", "2026-02-28")
	if err != nil {
		t.Fatal(err)
	}
	if tb.TotalDebit != tb.TotalCredit {
		t.Fatalf("trial balance not balanced: %d vs %d", tb.TotalDebit, tb.TotalCredit)
	}
	// 现金 300(借)、应付 200(借)；收入 -500 => 贷方 500
	if tb.TotalDebit != 500 || tb.TotalCredit != 500 {
		t.Fatalf("unexpected totals: %+v", tb)
	}
	foundZero := false
	for _, ln := range tb.Lines {
		if ln.Account == "2001" && ln.Debit == 200 && ln.Credit == 0 {
			foundZero = true
		}
	}
	if !foundZero {
		t.Fatalf("payable missing from trial: %+v", tb.Lines)
	}
}

func TestReversalRulesAndHistory(t *testing.T) {
	l := sampleLedger(t)

	// 冲销日期不得早于原日期
	if _, err := l.Reverse(ReverseInput{OriginalID: "V1", NewID: "R1", Date: "2026-01-01"}); !errorIs(err, ErrReversalBeforeDate) {
		t.Fatalf("want reversal before date, got %v", err)
	}
	// 原凭证不存在
	if _, err := l.Reverse(ReverseInput{OriginalID: "NOPE", NewID: "R1", Date: "2026-02-01"}); !errorIs(err, ErrUnknownVoucher) {
		t.Fatalf("want unknown voucher, got %v", err)
	}

	rev, err := l.Reverse(ReverseInput{OriginalID: "V1", NewID: "R1", Date: "2026-02-10"})
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if rev.ReversalOf != "V1" {
		t.Fatalf("reversal link wrong: %+v", rev)
	}
	for i, e := range rev.Entries {
		orig := l.st.Vouchers["V1"].Entries[i]
		if e.Amount != orig.Amount || e.Direction == orig.Direction {
			t.Fatalf("entry %d not flipped: %+v vs %+v", i, e, orig)
		}
	}
	orig, _ := l.GetVoucher("V1")
	if orig.ReversedBy != "R1" {
		t.Fatalf("back-link missing: %+v", orig)
	}

	// 禁止重复冲销
	if _, err := l.Reverse(ReverseInput{OriginalID: "V1", NewID: "R2", Date: "2026-02-11"}); !errorIs(err, ErrAlreadyReversed) {
		t.Fatalf("want already reversed, got %v", err)
	}
	// 禁止冲销一张冲销凭证
	if _, err := l.Reverse(ReverseInput{OriginalID: "R1", NewID: "R3", Date: "2026-02-12"}); !errorIs(err, ErrReversingReversal) {
		t.Fatalf("want reversing reversal, got %v", err)
	}
	// 冲销凭证编号不得重复
	if _, err := l.Reverse(ReverseInput{OriginalID: "V2", NewID: "R1", Date: "2026-02-10"}); !errorIs(err, ErrDuplicateVoucher) {
		t.Fatalf("want duplicate reversal id, got %v", err)
	}

	// 原凭证仍参与历史报表：2月期末现金 = 500(V1借) -200(V2贷) -500(R1贷) = -200
	tb, err := l.TrialBalance("2026-01-01", "2026-02-28")
	if err != nil {
		t.Fatal(err)
	}
	if tb.TotalDebit != tb.TotalCredit {
		t.Fatalf("trial not balanced after reversal: %d vs %d", tb.TotalDebit, tb.TotalCredit)
	}
	for _, ln := range tb.Lines {
		if ln.Account == "1001" && (ln.Debit != 0 || ln.Credit != 200) {
			t.Fatalf("cash after reversal wrong: %+v", ln)
		}
	}
}

func TestPersistenceAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	initAccounts(t, l)
	postVoucher(t, l, VoucherInput{
		ID: "V1", Date: "2026-01-15", Summary: "sale",
		Entries: []EntryInput{
			{Account: "1001", Direction: Debit, Amount: 500},
			{Account: "4001", Direction: Credit, Amount: 500},
		},
	})

	l2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(l2.Accounts()) != 3 {
		t.Fatalf("accounts lost after reopen")
	}
	v, err := l2.GetVoucher("V1")
	if err != nil {
		t.Fatalf("voucher lost after reopen: %v", err)
	}
	if len(v.Entries) != 2 {
		t.Fatalf("entries lost: %+v", v)
	}
	rep, err := l2.PeriodReport("2026-01-01", "2026-01-31")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Lines[0].ClosingNet != 500 {
		t.Fatalf("report inconsistent after reopen: %+v", rep.Lines)
	}
}
