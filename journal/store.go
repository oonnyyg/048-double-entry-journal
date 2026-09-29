package journal

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const StateFileName = "journal_state.json"

// Load 从数据目录加载账簿；目录为空时返回未初始化账簿（无文件）。
func Load(dir string) (*Ledger, error) {
	path := filepath.Join(dir, StateFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewLedger(), nil
		}
		return nil, err
	}
	l := NewLedger()
	if err := json.Unmarshal(data, l); err != nil {
		return nil, err
	}
	if l.Accounts == nil {
		l.Accounts = map[string]*Account{}
	}
	if l.Vouchers == nil {
		l.Vouchers = map[string]*Voucher{}
	}
	return l, nil
}

// Save 将账簿原子写入数据目录（先写临时文件再重命名）。
func (l *Ledger) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	final := filepath.Join(dir, StateFileName)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}
