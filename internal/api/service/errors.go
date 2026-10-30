package service

import "errors"

// 业务层哨兵错误（handler 据此映射 HTTP 状态码）。
var (
	ErrPathRequired   = errors.New("path 不能为空")
	ErrInvalidPath    = errors.New("路径不存在或不是目录")
	ErrSourceNotFound = errors.New("source not found")
	ErrDimNotFound    = errors.New("node not found")
	ErrBadDimID       = errors.New("bad dimension id")
	ErrMissingQ       = errors.New("missing q")
	ErrMissingQN      = errors.New("missing qn")
	ErrBadBody        = errors.New("invalid body")
	ErrNoActiveSource = errors.New("no active source")
)

// knownErr 判断是否属于已知业务错误（未知错误交由上层按 500 处理）。
func KnownErr(err error) bool {
	switch err {
	case ErrPathRequired, ErrInvalidPath, ErrSourceNotFound, ErrDimNotFound, ErrBadDimID, ErrMissingQ, ErrMissingQN, ErrBadBody, ErrNoActiveSource:
		return true
	}
	return false
}
