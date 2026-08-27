package domain

import "errors"

var (
	ErrNotFound          = errors.New("档案不存在")
	ErrVersionConflict   = errors.New("档案版本冲突，请刷新后重试")
	ErrInvalidTransition = errors.New("当前状态不允许执行该操作")
	ErrSealed            = errors.New("档案已经封存，禁止继续修改")
	ErrValidation        = errors.New("输入校验失败")
	ErrOpenRequests      = errors.New("仍有未关闭的参与者修订请求")
	ErrConsentMismatch   = errors.New("段落开放级别超出参与者授权边界")
	ErrIntegrity         = errors.New("数据完整性校验失败")
	ErrRevisionStale     = errors.New("审阅修订已过期，请重新载入当前清单")
	ErrPreflightStale    = errors.New("封存预检已经过期，请重新执行预检")
)

type RuleError struct {
	Code    string
	Message string
	Field   string
}

type PreflightError struct{ Issues []ValidationIssue }

func (e *PreflightError) Error() string { return "封存预检存在阻断项" }

func (e *RuleError) Error() string { return e.Message }

func Invalid(code, message, field string) error {
	return &RuleError{Code: code, Message: message, Field: field}
}
