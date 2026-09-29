package journal

import (
	"errors"
	"path/filepath"
	"testing"
)

func newTestLedger(t *testing.T) *Ledger {
	t.Helper()
	dir := t.TempDir()
	l, err := Open(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return l
}

func initAccounts(t *testing.T, l *Ledger) {
	t.Helper()
	_, err := l.Init([]AccountInput{
		{Code: "1001", Name: "Cash"},
		{Code: "4001", Name: "Revenue"},
		{Code: "2001", Name: "Payable"},
	})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
}

func TestInitValidation(t *testing.T) {
	l := newTestLedger(t)

	if _, err := l.Init(nil); !errorIs(err, ErrEmptyAccountList) {
		t.Fatalf("want empty account list, got %v", err)
	}
	if _, err := l.Init([]AccountInput{{Code: ""}}); !errorIs(err, ErrEmptyAccountCode) {
		t.Fatalf("want empty code, got %v", err)
	}
	if _, err := l.Init([]AccountInput{{Code: "A"}, {Code: "A"}}); !errorIs(err, ErrDuplicateAccount) {
		t.Fatalf("want duplicate account, got %v", err)
	}

	initAccounts(t, l)
	if _, err := l.Init([]AccountInput{{Code: "B"}}); !errorIs(err, ErrAlreadyInitialized) {
		t.Fatalf("want already initialized, got %v", err)
	}
}

func TestPostUnbalancedLeavesNoTrace(t *testing.T) {
	l := newTestLedger(t)
	initAccounts(t, l)

	bad := VoucherInput{
		ID:      "V1",
		Date:    "2026-01-10",
		Summary: "unbalanced",
		Entries: []EntryInput{
			{Account: "1001", Direction: Debit, Amount: 100},
			{Account: "4001", Direction: Credit, Amount: 90},
		},
	}
	if _, err := l.Post(bad); !errorIs(err, ErrNotBalanced) {
		t.Fatalf("want not balanced, got %v", err)
	}
	if _, err := l.GetVoucher("V1"); !errorIs(err, ErrUnknownVoucher) {
		t.Fatalf("unbalanced voucher must not be stored, got %v", err)
	}

	bad.ID = "V2"
	bad.Entries[1].Amount = 100
	bad.Entries[0].Direction = "sideways"
	if _, err := l.Post(bad); !errorIs(err, ErrInvalidEntry) {
		t.Fatalf("want invalid entry, got %v", err)
	}

	bad.ID = "V3"
	bad.Entries[0].Direction = Debit
	bad.Entries[1].Account = "9999"
	if _, err := l.Post(bad); !errorIs(err, ErrUnknownAccount) {
		t.Fatalf("want unknown account, got %v", err)
	}

	bad.ID = "V4"
	bad.Entries[1].Account = "4001"
	bad.Entries[0].Amount = 0
	if _, err := l.Post(bad); !errorIs(err, ErrInvalidEntry) {
		t.Fatalf("want non-positive amount error, got %v", err)
	}

	bad.ID = "V5"
	bad.Entries[0].Amount = 100
	bad.Date = "2026-13-40"
	if _, err := l.Post(bad); !errorIs(err, ErrInvalidDate) {
		t.Fatalf("want invalid date, got %v", err)
	}
}

func TestPostMultiEntrySameAccountAndDuplicateID(t *testing.T) {
	l := newTestLedger(t)
	initAccounts(t, l)

	v := VoucherInput{
		ID:      "V1",
		Date:    "2026-01-10",
		Summary: "multi",
		Entries: []EntryInput{
			{Account: "1001", Direction: Debit, Amount: 100},
			{Account: "4001", Direction: Credit, Amount: 60},
			{Account: "2001", Direction: Credit, Amount: 40},
		},
	}
	if _, err := l.Post(v); err != nil {
		t.Fatalf("post: %v", err)
	}
	if _, err := l.Post(v); !errorIs(err, ErrDuplicateVoucher) {
		t.Fatalf("want duplicate id, got %v", err)
	}
	got, err := l.GetVoucher("V1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Entries) != 3 || got.Entries[2].Seq != 2 {
		t.Fatalf("entries order/seq not preserved: %+v", got.Entries)
	}
}

func errorIs(err error, code ErrorCode) bool {
	var je *Error
	if errors.As(err, &je) {
		return je.Code == code
	}
	return false
}
