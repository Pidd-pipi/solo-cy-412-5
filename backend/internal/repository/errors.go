package repository

import (
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// ErrNotFound 实体不存在（全仓储共享）。
var ErrNotFound = errors.New("not found")

// ErrDuplicate 唯一约束冲突：同设施同周期计划、同期次任务、一张任务只关联一张维修单等。
var ErrDuplicate = errors.New("duplicate record")

// IsDuplicate 判定是否为唯一约束冲突，兼容 MySQL（1062）与 SQLite（错误文本）。
func IsDuplicate(e error) bool {
	if e == nil {
		return false
	}
	var me *mysql.MySQLError
	if errors.As(e, &me) && me.Number == 1062 {
		return true
	}
	msg := strings.ToLower(e.Error())
	return strings.Contains(msg, "unique constraint failed") || strings.Contains(msg, "duplicate key")
}
