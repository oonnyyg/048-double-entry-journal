package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func sendLines(t *testing.T, dir, lines string) []response {
	t.Helper()
	var in, out bytes.Buffer
	in.WriteString(lines)
	if err := run(filepath.Join(dir, "ledger"), &in, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	var resps []response
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r response
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		resps = append(resps, r)
	}
	return resps
}

func TestLineProtocolEndToEnd(t *testing.T) {
	dir := t.TempDir()

	req := strings.Join([]string{
		`{"op":"init","accounts":[{"code":"1001","name":"Cash"},{"code":"4001","name":"Revenue"}]}`,
		`{"op":"post","voucher":{"id":"V1","date":"2026-01-15","summary":"sale","entries":[{"account":"1001","direction":"debit","amount":500},{"account":"4001","direction":"credit","amount":500}]}}`,
		`{"op":"post","voucher":{"id":"V2","date":"2026-01-15","summary":"bad","entries":[{"account":"1001","direction":"debit","amount":500},{"account":"4001","direction":"credit","amount":499}]}}`,
		`{"op":"get_voucher","id":"V1"}`,
		`{"op":"ledger","account":"1001"}`,
		`{"op":"period_report","from":"2026-01-01","to":"2026-01-31"}`,
		`{"op":"trial_balance","from":"2026-01-01","to":"2026-01-31"}`,
		`{"op":"reverse","reverse":{"original_id":"V1","new_id":"R1","date":"2026-02-01"}}`,
		`{"op":"bogus"}`,
		`{not json`,
	}, "\n")

	resps := sendLines(t, dir, req)
	if len(resps) != 10 {
		t.Fatalf("want 10 responses, got %d", len(resps))
	}
	if !resps[0].OK || !resps[1].OK {
		t.Fatalf("init/post should succeed: %+v", resps[:2])
	}
	if resps[2].OK || resps[2].Error.Code != "voucher_not_balanced" {
		t.Fatalf("unbalanced voucher should fail with code, got %+v", resps[2])
	}
	if !resps[3].OK || !resps[4].OK || !resps[5].OK || !resps[6].OK {
		t.Fatalf("queries should succeed: %+v", resps[3:7])
	}
	if !resps[7].OK {
		t.Fatalf("reverse should succeed: %+v", resps[7])
	}
	if resps[8].Error.Code != "internal_error" {
		t.Fatalf("unknown op error, got %+v", resps[8])
	}
	if resps[9].Error.Code != "invalid_json" {
		t.Fatalf("invalid json error, got %+v", resps[9])
	}

	// 重新打开同一数据目录，状态保持一致。
	resps2 := sendLines(t, dir, `{"op":"trial_balance","from":"2026-01-01","to":"2026-02-28"}`)
	if !resps2[0].OK {
		t.Fatalf("reopen query failed: %+v", resps2[0])
	}
	raw, _ := json.Marshal(resps2[0].Result)
	var tb struct {
		TotalDebit  int64 `json:"total_debit"`
		TotalCredit int64 `json:"total_credit"`
	}
	json.Unmarshal(raw, &tb)
	if tb.TotalDebit != 0 || tb.TotalCredit != 0 {
		t.Fatalf("after reversal balances should net to zero, got %+v", tb)
	}
}

func TestDoubleInitRejected(t *testing.T) {
	dir := t.TempDir()
	req := `{"op":"init","accounts":[{"code":"A"}]}` + "\n" +
		`{"op":"init","accounts":[{"code":"B"}]}`
	resps := sendLines(t, dir, req)
	if !resps[0].OK {
		t.Fatalf("first init failed: %+v", resps[0])
	}
	if resps[1].OK || resps[1].Error.Code != "already_initialized" {
		t.Fatalf("second init must be rejected, got %+v", resps[1])
	}
}
