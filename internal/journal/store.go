package journal

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const stateFileName = "journal.json"

// store 负责账簿状态在数据目录中的加载与原子持久化。
type store struct {
	path string
}

func newStore(dir string) *store {
	return &store{path: filepath.Join(dir, stateFileName)}
}

// exists 报告数据目录中是否已经存在账簿文件。
func (s *store) exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

// load 读取账簿状态；文件不存在时返回空状态（未初始化标志由调用方判断）。
func (s *store) load() (*ledgerState, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var st ledgerState
	if len(data) > 0 {
		if err := json.Unmarshal(data, &st); err != nil {
			return nil, err
		}
	}
	if st.Vouchers == nil {
		st.Vouchers = make(map[string]*Voucher)
	}
	return &st, nil
}

// save 以“临时文件 + 重命名”方式原子写入，避免崩溃留下半截文件。
func (s *store) save(st *ledgerState) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".journal-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}
