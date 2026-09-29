// journal 是逐行 JSON 的复式记账命令行服务。
// 通过 --state 指定数据目录；每行读入一个 JSON 请求，输出一行 JSON 结果。
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"example.com/double-entry-journal/journal"
)

type request struct {
	Op         string            `json:"op"`
	Accounts   []journal.Account `json:"accounts"`
	ID         string            `json:"id"`
	Date       string            `json:"date"`
	Summary    string            `json:"summary"`
	Entries    []journal.Entry   `json:"entries"`
	OriginalID string            `json:"original_id"`
	AccountID  string            `json:"account_id"`
	From       string            `json:"from"`
	To         string            `json:"to"`
	AsOf       string            `json:"as_of"`
}

type response struct {
	OK    bool           `json:"ok"`
	Data  any            `json:"data,omitempty"`
	Error *responseError `json:"error,omitempty"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func main() {
	stateDir := flag.String("state", "", "数据目录（保存账簿状态）")
	flag.Parse()
	if *stateDir == "" {
		fmt.Fprintln(os.Stderr, "missing required --state directory")
		os.Exit(2)
	}

	ledger, err := journal.Load(*stateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load state failed: %v\n", err)
		os.Exit(1)
	}

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			writeResponse(out, response{Error: &responseError{Code: "bad_json", Message: err.Error()}})
			continue
		}
		data, mutates, err := dispatch(ledger, &req)
		if err != nil {
			writeResponse(out, response{Error: toError(err)})
			continue
		}
		if mutates {
			if err := ledger.Save(*stateDir); err != nil {
				writeResponse(out, response{Error: &responseError{Code: "persist_failed", Message: err.Error()}})
				continue
			}
		}
		writeResponse(out, response{OK: true, Data: data})
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "read stdin failed: %v\n", err)
		os.Exit(1)
	}
}

// dispatch 返回数据、该操作是否修改状态（需要落盘）以及错误。
func dispatch(l *journal.Ledger, req *request) (any, bool, error) {
	switch req.Op {
	case "init":
		return nil, true, l.Init(req.Accounts)
	case "post":
		v, err := l.Post(req.ID, req.Date, req.Summary, req.Entries)
		return v, true, err
	case "reverse":
		v, err := l.Reverse(req.OriginalID, req.ID, req.Date, req.Summary)
		return v, true, err
	case "get_voucher":
		v, err := l.GetVoucher(req.ID)
		return v, false, err
	case "list_accounts":
		return l.Accounts, false, nil
	case "account_ledger":
		r, err := l.AccountLedger(req.AccountID)
		return r, false, err
	case "period_report":
		r, err := l.PeriodReport(req.From, req.To)
		return r, false, err
	case "trial_balance":
		r, err := l.TrialBalanceAt(req.AsOf)
		return r, false, err
	default:
		return nil, false, fmt.Errorf("unknown op: %q", req.Op)
	}
}

func toError(err error) *responseError {
	var je *journal.Error
	if errors.As(err, &je) {
		return &responseError{Code: string(je.Code), Message: je.Message}
	}
	return &responseError{Code: "error", Message: err.Error()}
}

func writeResponse(out *bufio.Writer, r response) {
	data, _ := json.Marshal(r)
	out.Write(data)
	out.WriteByte('\n')
	out.Flush()
}
