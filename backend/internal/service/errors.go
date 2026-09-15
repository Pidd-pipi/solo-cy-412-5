package service

import "errors"

// 业务哨兵错误：handler 据此映射 HTTP 状态（404/409/400/403）。
var (
	ErrNotFound       = errors.New("resource not found")
	ErrConflict       = errors.New("conflict")
	ErrInvalidState   = errors.New("invalid state transition")
	ErrForbidden      = errors.New("operation forbidden")
	ErrAlreadyClaimed = errors.New("task already claimed")
	ErrImmutable      = errors.New("record is finalized and cannot be changed")
)
