package journal

import "fmt"

// ErrorCode 用稳定的字符串标识错误类别，便于调用方区分处理。
type ErrorCode string

const (
	ErrNotInitialized     ErrorCode = "not_initialized"
	ErrAlreadyInitialized ErrorCode = "already_initialized"
	ErrInvalidAccount     ErrorCode = "invalid_account"
	ErrUnknownAccount     ErrorCode = "unknown_account"
	ErrInvalidDate        ErrorCode = "invalid_date"
	ErrInvalidPeriod      ErrorCode = "invalid_period"
	ErrDuplicateVoucher   ErrorCode = "duplicate_voucher"
	ErrVoucherNotFound    ErrorCode = "voucher_not_found"
	ErrInvalidEntry       ErrorCode = "invalid_entry"
	ErrUnbalanced         ErrorCode = "unbalanced"
	ErrAlreadyReversed    ErrorCode = "already_reversed"
	ErrReverseOfReversal  ErrorCode = "reverse_of_reversal"
	ErrReverseDateEarly   ErrorCode = "reverse_date_before_original"
)

// Error 携带可区分的错误码与人类可读信息。
type Error struct {
	Code    ErrorCode
	Message string
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

func errf(code ErrorCode, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
