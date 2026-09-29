package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"example.com/double-entry-journal/internal/journal"
)

// request 是行协议的统一请求信封。
type request struct {
	Op       string                 `json:"op"`
	Accounts []journal.AccountInput `json:"accounts,omitempty"`
	Voucher  *journal.VoucherInput  `json:"voucher,omitempty"`
	Reverse  *journal.ReverseInput  `json:"reverse,omitempty"`
	ID       string                 `json:"id,omitempty"`
	Account  string                 `json:"account,omitempty"`
	From     string                 `json:"from,omitempty"`
	To       string                 `json:"to,omitempty"`
}

// response 是统一响应信封：成功时 ok=true，失败时 ok=false 并给出可区分错误码。
type response struct {
	OK     bool           `json:"ok"`
	Result any            `json:"result,omitempty"`
	Error  *responseError `json:"error,omitempty"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func main() {
	stateDir := flag.String("state", "", "data directory for the ledger")
	flag.Parse()
	if *stateDir == "" {
		fmt.Fprintln(os.Stderr, "missing required --state directory")
		os.Exit(2)
	}
	if err := run(*stateDir, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(stateDir string, in io.Reader, out io.Writer) error {
	ledger, err := journal.Open(stateDir)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(out)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytesTrimSpace(line)) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			writeError(enc, "invalid_json", err.Error())
			continue
		}
		result, err := dispatch(ledger, &req)
		if err != nil {
			writeJournalError(enc, err)
			continue
		}
		enc.Encode(response{OK: true, Result: result})
	}
	return scanner.Err()
}

func dispatch(l *journal.Ledger, req *request) (any, error) {
	switch req.Op {
	case "init":
		return l.Init(req.Accounts)
	case "accounts":
		return l.Accounts(), nil
	case "post":
		if req.Voucher == nil {
			return nil, errors.New("missing voucher payload")
		}
		return l.Post(*req.Voucher)
	case "reverse":
		if req.Reverse == nil {
			return nil, errors.New("missing reverse payload")
		}
		return l.Reverse(*req.Reverse)
	case "get_voucher":
		return l.GetVoucher(req.ID)
	case "ledger":
		return l.Ledger(req.Account)
	case "period_report":
		return l.PeriodReport(req.From, req.To)
	case "trial_balance":
		return l.TrialBalance(req.From, req.To)
	default:
		return nil, fmt.Errorf("unknown op %q", req.Op)
	}
}

func writeError(enc *json.Encoder, code, msg string) {
	enc.Encode(response{Error: &responseError{Code: code, Message: msg}})
}

func writeJournalError(enc *json.Encoder, err error) {
	var je *journal.Error
	if errors.As(err, &je) {
		writeError(enc, string(je.Code), je.Message)
		return
	}
	writeError(enc, "internal_error", err.Error())
}

func bytesTrimSpace(b []byte) []byte {
	start := 0
	for start < len(b) && isSpace(b[start]) {
		start++
	}
	end := len(b)
	for end > start && isSpace(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}
