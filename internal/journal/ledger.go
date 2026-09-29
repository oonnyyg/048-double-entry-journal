package journal

import (
	"sort"
	"time"
)

const dateLayout = "2006-01-02"

// Ledger 是一个可持久化的复式账簿。所有变更先在内存校验通过，再一次性落盘。
type Ledger struct {
	dir   string
	store *store
	st    *ledgerState
}

// Open 打开数据目录下的账簿；若尚未初始化则返回未初始化状态。
func Open(dir string) (*Ledger, error) {
	s := newStore(dir)
	st, err := s.load()
	if err != nil {
		return nil, err
	}
	if st == nil {
		st = &ledgerState{Vouchers: make(map[string]*Voucher)}
	}
	return &Ledger{dir: dir, store: s, st: st}, nil
}

// Initialized 报告账簿是否已经初始化过科目。
func (l *Ledger) Initialized() bool {
	return len(l.st.Accounts) > 0
}

// AccountInput 是初始化科目的输入项。
type AccountInput struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Init 初始化科目表。初始余额全部为零；已有账簿不得重新初始化。
func (l *Ledger) Init(inputs []AccountInput) ([]Account, error) {
	if l.Initialized() {
		return nil, journalError(ErrAlreadyInitialized, "ledger already initialized; reinitialization is not allowed")
	}
	if len(inputs) == 0 {
		return nil, journalError(ErrEmptyAccountList, "account list must be non-empty")
	}
	seen := make(map[string]bool, len(inputs))
	accounts := make([]*Account, 0, len(inputs))
	for _, in := range inputs {
		if in.Code == "" {
			return nil, journalError(ErrEmptyAccountCode, "account code must be non-empty")
		}
		if seen[in.Code] {
			return nil, journalError(ErrDuplicateAccount, "duplicate account code %q", in.Code)
		}
		seen[in.Code] = true
		accounts = append(accounts, &Account{Code: in.Code, Name: in.Name})
	}
	l.st.Accounts = accounts
	if l.st.Vouchers == nil {
		l.st.Vouchers = make(map[string]*Voucher)
	}
	if err := l.store.save(l.st); err != nil {
		return nil, err
	}
	return l.accountList(), nil
}

// EntryInput 是记账分录输入。
type EntryInput struct {
	Account   string    `json:"account"`
	Direction Direction `json:"direction"`
	Amount    int64     `json:"amount"`
}

// VoucherInput 是普通记账凭证输入。
type VoucherInput struct {
	ID      string       `json:"id"`
	Date    string       `json:"date"`
	Summary string       `json:"summary"`
	Entries []EntryInput `json:"entries"`
}

// Post 登记一张普通凭证。全部校验通过后才落盘，失败不留任何痕迹。
func (l *Ledger) Post(in VoucherInput) (*Voucher, error) {
	entries, err := l.validateVoucher(in)
	if err != nil {
		return nil, err
	}
	v := &Voucher{
		ID:      in.ID,
		Date:    in.Date,
		Summary: in.Summary,
		Entries: entries,
	}
	l.st.Vouchers[v.ID] = v
	if err := l.store.save(l.st); err != nil {
		delete(l.st.Vouchers, v.ID)
		return nil, err
	}
	return cloneVoucher(v), nil
}

// ReverseInput 是冲销请求。
type ReverseInput struct {
	OriginalID string `json:"original_id"`
	NewID      string `json:"new_id"`
	Date       string `json:"date"`
}

// Reverse 为原凭证生成金额相同、借贷相反的冲销凭证，并保留双向关联。
func (l *Ledger) Reverse(in ReverseInput) (*Voucher, error) {
	if !l.Initialized() {
		return nil, journalError(ErrNotInitialized, "ledger is not initialized")
	}
	orig, ok := l.st.Vouchers[in.OriginalID]
	if !ok {
		return nil, journalError(ErrUnknownVoucher, "unknown original voucher %q", in.OriginalID)
	}
	if orig.ReversedBy != "" {
		return nil, journalError(ErrAlreadyReversed, "voucher %q already reversed by %q", orig.ID, orig.ReversedBy)
	}
	if orig.ReversalOf != "" {
		return nil, journalError(ErrReversingReversal, "voucher %q is itself a reversal and cannot be reversed", orig.ID)
	}
	if in.NewID == "" {
		return nil, journalError(ErrEmptyVoucherID, "reversal voucher id must be non-empty")
	}
	if _, dup := l.st.Vouchers[in.NewID]; dup {
		return nil, journalError(ErrDuplicateVoucher, "voucher id %q already exists", in.NewID)
	}
	newDate, err := parseDate(in.Date)
	if err != nil {
		return nil, err
	}
	origDate, _ := parseDate(orig.Date)
	if newDate.Before(origDate) {
		return nil, journalError(ErrReversalBeforeDate, "reversal date %s is earlier than original date %s", in.Date, orig.Date)
	}
	entries := make([]Entry, len(orig.Entries))
	for i, e := range orig.Entries {
		entries[i] = Entry{
			Account:   e.Account,
			Direction: opposite(e.Direction),
			Amount:    e.Amount,
			Seq:       i,
		}
	}
	rev := &Voucher{
		ID:         in.NewID,
		Date:       in.Date,
		Summary:    "REVERSAL " + orig.Summary,
		Entries:    entries,
		ReversalOf: orig.ID,
	}
	l.st.Vouchers[rev.ID] = rev
	orig.ReversedBy = rev.ID
	if err := l.store.save(l.st); err != nil {
		delete(l.st.Vouchers, rev.ID)
		orig.ReversedBy = ""
		return nil, err
	}
	return cloneVoucher(rev), nil
}

// GetVoucher 返回指定凭证（含原始分录与冲销关联）。
func (l *Ledger) GetVoucher(id string) (*Voucher, error) {
	v, ok := l.st.Vouchers[id]
	if !ok {
		return nil, journalError(ErrUnknownVoucher, "unknown voucher %q", id)
	}
	return cloneVoucher(v), nil
}

// Accounts 返回科目表副本（按编号排序）。
func (l *Ledger) Accounts() []Account {
	return l.accountList()
}

func (l *Ledger) validateVoucher(in VoucherInput) ([]Entry, error) {
	if !l.Initialized() {
		return nil, journalError(ErrNotInitialized, "ledger is not initialized")
	}
	if in.ID == "" {
		return nil, journalError(ErrEmptyVoucherID, "voucher id must be non-empty")
	}
	if _, dup := l.st.Vouchers[in.ID]; dup {
		return nil, journalError(ErrDuplicateVoucher, "voucher id %q already exists", in.ID)
	}
	if _, err := parseDate(in.Date); err != nil {
		return nil, err
	}
	if in.Summary == "" {
		return nil, journalError(ErrEmptySummary, "summary must be non-empty")
	}
	if len(in.Entries) < 2 {
		return nil, journalError(ErrInvalidEntry, "voucher must contain at least two entries")
	}
	known := make(map[string]bool, len(l.st.Accounts))
	for _, a := range l.st.Accounts {
		known[a.Code] = true
	}
	entries := make([]Entry, len(in.Entries))
	var debitTotal, creditTotal int64
	for i, e := range in.Entries {
		if !known[e.Account] {
			return nil, journalError(ErrUnknownAccount, "entry %d references unknown account %q", i, e.Account)
		}
		if e.Direction != Debit && e.Direction != Credit {
			return nil, journalError(ErrInvalidEntry, "entry %d has invalid direction %q", i, e.Direction)
		}
		if e.Amount <= 0 {
			return nil, journalError(ErrInvalidEntry, "entry %d amount must be a positive integer", i)
		}
		entries[i] = Entry{Account: e.Account, Direction: e.Direction, Amount: e.Amount, Seq: i}
		if e.Direction == Debit {
			debitTotal += e.Amount
		} else {
			creditTotal += e.Amount
		}
	}
	if debitTotal != creditTotal {
		return nil, journalError(ErrNotBalanced, "debit total %d != credit total %d", debitTotal, creditTotal)
	}
	return entries, nil
}

func (l *Ledger) accountList() []Account {
	out := make([]Account, 0, len(l.st.Accounts))
	for _, a := range l.st.Accounts {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func parseDate(s string) (time.Time, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return time.Time{}, journalError(ErrInvalidDate, "date %q must be in YYYY-MM-DD format", s)
	}
	return t, nil
}

func opposite(d Direction) Direction {
	if d == Debit {
		return Credit
	}
	return Debit
}

func cloneVoucher(v *Voucher) *Voucher {
	cp := *v
	cp.Entries = append([]Entry(nil), v.Entries...)
	return &cp
}
