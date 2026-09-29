// Package journal 实现小团队可追溯的复式记账账簿。
// 所有金额均为以“分”为单位的整数；借方正、贷方负用于内部净额累计。
package journal

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const DateLayout = "2006-01-02"

// Entry 是凭证中的一条分录。Direction 只取 "debit" 或 "credit"。
type Entry struct {
	AccountID string `json:"account_id"`
	Direction string `json:"direction"`
	Amount    int64  `json:"amount"` // 正整数，单位：分
	Line      int    `json:"line"`   // 原分录顺序（从 1 开始）
}

// Voucher 是一张记账凭证。
type Voucher struct {
	ID         string  `json:"id"`
	Date       string  `json:"date"`
	Summary    string  `json:"summary"`
	Entries    []Entry `json:"entries"`
	ReversesID string  `json:"reverses_id,omitempty"` // 若为冲销凭证，指向原凭证
	ReversedBy string  `json:"reversed_by,omitempty"` // 若已被冲销，指向冲销凭证
}

// Account 是科目；初始余额恒为零且不允许删除或改名。
type Account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Ledger 是账簿全部状态。
type Ledger struct {
	Accounts map[string]*Account `json:"accounts"`
	Vouchers map[string]*Voucher `json:"vouchers"`
}

func NewLedger() *Ledger {
	return &Ledger{
		Accounts: map[string]*Account{},
		Vouchers: map[string]*Voucher{},
	}
}

func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errf(ErrInvalidDate, "empty date")
	}
	t, err := time.Parse(DateLayout, s)
	if err != nil {
		return time.Time{}, errf(ErrInvalidDate, "date must be YYYY-MM-DD: %q", s)
	}
	return t, nil
}

// Init 用非空、编号唯一的科目列表初始化账簿。
func (l *Ledger) Init(accounts []Account) error {
	if len(l.Accounts) > 0 {
		return errf(ErrAlreadyInitialized, "ledger already initialized")
	}
	if len(accounts) == 0 {
		return errf(ErrInvalidAccount, "account list must be non-empty")
	}
	seen := map[string]bool{}
	pending := map[string]*Account{}
	for _, a := range accounts {
		id := strings.TrimSpace(a.ID)
		if id == "" {
			return errf(ErrInvalidAccount, "account id must be non-empty")
		}
		if seen[id] {
			return errf(ErrInvalidAccount, "duplicate account id: %s", id)
		}
		if strings.TrimSpace(a.Name) == "" {
			return errf(ErrInvalidAccount, "account %s name must be non-empty", id)
		}
		seen[id] = true
		pending[id] = &Account{ID: id, Name: a.Name}
	}
	for id, a := range pending {
		l.Accounts[id] = a
	}
	return nil
}

func (l *Ledger) requireReady() error {
	if len(l.Accounts) == 0 {
		return errf(ErrNotInitialized, "ledger has no accounts; call init first")
	}
	return nil
}

// validateEntries 校验分录并返回拷贝；任何错误都不改变账簿。
func (l *Ledger) validateEntries(entries []Entry) ([]Entry, int64, int64, error) {
	if len(entries) < 2 {
		return nil, 0, 0, errf(ErrInvalidEntry, "voucher needs at least two entries")
	}
	out := make([]Entry, len(entries))
	var debit, credit int64
	for i, e := range entries {
		if e.AccountID == "" {
			return nil, 0, 0, errf(ErrInvalidEntry, "entry %d: account_id is empty", i+1)
		}
		if _, ok := l.Accounts[e.AccountID]; !ok {
			return nil, 0, 0, errf(ErrUnknownAccount, "entry %d: unknown account %q", i+1, e.AccountID)
		}
		if e.Amount <= 0 {
			return nil, 0, 0, errf(ErrInvalidEntry, "entry %d: amount must be a positive integer", i+1)
		}
		var dir string
		switch e.Direction {
		case "debit":
			dir = "debit"
			debit += e.Amount
		case "credit":
			dir = "credit"
			credit += e.Amount
		default:
			return nil, 0, 0, errf(ErrInvalidEntry, "entry %d: direction must be debit or credit", i+1)
		}
		out[i] = Entry{AccountID: e.AccountID, Direction: dir, Amount: e.Amount, Line: i + 1}
	}
	if debit != credit {
		return nil, 0, 0, errf(ErrUnbalanced, "debit total %d != credit total %d", debit, credit)
	}
	return out, debit, credit, nil
}

// Post 校验并登记一张普通凭证。
func (l *Ledger) Post(id, date, summary string, entries []Entry) (*Voucher, error) {
	if err := l.requireReady(); err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errf(ErrDuplicateVoucher, "voucher id must be non-empty")
	}
	if _, exists := l.Vouchers[id]; exists {
		return nil, errf(ErrDuplicateVoucher, "voucher id already exists: %s", id)
	}
	if strings.TrimSpace(summary) == "" {
		return nil, errf(ErrInvalidEntry, "summary must be non-empty")
	}
	if _, err := parseDate(date); err != nil {
		return nil, err
	}
	valid, _, _, err := l.validateEntries(entries)
	if err != nil {
		return nil, err
	}
	v := &Voucher{ID: id, Date: date, Summary: summary, Entries: valid}
	l.Vouchers[id] = v
	return cloneVoucher(v), nil
}

// Reverse 为原凭证生成金额相同、方向相反的冲销凭证，并建立双向关联。
func (l *Ledger) Reverse(originalID, newID, date, summary string) (*Voucher, error) {
	if err := l.requireReady(); err != nil {
		return nil, err
	}
	orig, ok := l.Vouchers[strings.TrimSpace(originalID)]
	if !ok {
		return nil, errf(ErrVoucherNotFound, "original voucher not found: %s", originalID)
	}
	newID = strings.TrimSpace(newID)
	if newID == "" {
		return nil, errf(ErrDuplicateVoucher, "new voucher id must be non-empty")
	}
	if _, exists := l.Vouchers[newID]; exists {
		return nil, errf(ErrDuplicateVoucher, "voucher id already exists: %s", newID)
	}
	rd, err := parseDate(date)
	if err != nil {
		return nil, err
	}
	od, _ := parseDate(orig.Date)
	if rd.Before(od) {
		return nil, errf(ErrReverseDateEarly, "reversal date %s before original date %s", date, orig.Date)
	}
	if orig.ReversedBy != "" {
		return nil, errf(ErrAlreadyReversed, "voucher %s already reversed by %s", orig.ID, orig.ReversedBy)
	}
	if orig.ReversesID != "" {
		return nil, errf(ErrReverseOfReversal, "voucher %s is itself a reversal", orig.ID)
	}
	if strings.TrimSpace(summary) == "" {
		summary = fmt.Sprintf("冲销凭证 %s", orig.ID)
	}
	entries := make([]Entry, len(orig.Entries))
	for i, e := range orig.Entries {
		dir := "credit"
		if e.Direction == "credit" {
			dir = "debit"
		}
		entries[i] = Entry{AccountID: e.AccountID, Direction: dir, Amount: e.Amount, Line: i + 1}
	}
	rev := &Voucher{ID: newID, Date: date, Summary: summary, Entries: entries, ReversesID: orig.ID}
	l.Vouchers[newID] = rev
	orig.ReversedBy = newID
	return cloneVoucher(rev), nil
}

// GetVoucher 返回凭证副本，含冲销关联与原始分录。
func (l *Ledger) GetVoucher(id string) (*Voucher, error) {
	v, ok := l.Vouchers[id]
	if !ok {
		return nil, errf(ErrVoucherNotFound, "voucher not found: %s", id)
	}
	return cloneVoucher(v), nil
}

// sortedVouchers 按日期、凭证编号排序返回副本。
func (l *Ledger) sortedVouchers() []*Voucher {
	list := make([]*Voucher, 0, len(l.Vouchers))
	for _, v := range l.Vouchers {
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

func cloneVoucher(v *Voucher) *Voucher {
	cp := *v
	cp.Entries = append([]Entry(nil), v.Entries...)
	return &cp
}
