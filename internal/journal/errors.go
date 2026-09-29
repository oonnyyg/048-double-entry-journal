package journal

import "fmt"

// ErrorCode 是机器可区分的错误类别。
type ErrorCode string

const (
	ErrAlreadyInitialized ErrorCode = "already_initialized"
	ErrNotInitialized     ErrorCode = "not_initialized"
	ErrEmptyAccountList   ErrorCode = "empty_account_list"
	ErrEmptyAccountCode   ErrorCode = "empty_account_code"
	ErrDuplicateAccount   ErrorCode = "duplicate_account"
	ErrUnknownAccount     ErrorCode = "unknown_account"
	ErrDuplicateVoucher   ErrorCode = "duplicate_voucher_id"
	ErrUnknownVoucher     ErrorCode = "unknown_voucher"
	ErrInvalidDate        ErrorCode = "invalid_date"
	ErrInvalidPeriod      ErrorCode = "invalid_period"
	ErrEmptyVoucherID     ErrorCode = "empty_voucher_id"
	ErrEmptySummary       ErrorCode = "empty_summary"
	ErrInvalidEntry       ErrorCode = "invalid_entry"
	ErrNotBalanced        ErrorCode = "voucher_not_balanced"
	ErrAlreadyReversed    ErrorCode = "already_reversed"
	ErrReversingReversal  ErrorCode = "reversing_reversal"
	ErrReversalBeforeDate ErrorCode = "reversal_before_original_date"
)

// Error 携带错误码与可读信息，便于调用方区分错误类别。
type Error struct {
	Code    ErrorCode
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func journalError(code ErrorCode, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
