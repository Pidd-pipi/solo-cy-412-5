package handler

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/smartestate/smartestate/internal/constants"
	"github.com/smartestate/smartestate/internal/dto"
	"github.com/smartestate/smartestate/internal/service"
	"net/http"
)

type Handler struct{ Validate *validator.Validate }

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, dto.Response{Code: constants.CodeOK, Message: constants.MessageOK, Data: data})
}
func Fail(c *gin.Context, status, code int, msg string) {
	c.JSON(status, dto.Response{Code: code, Message: msg})
}
func Bind(c *gin.Context, v any, validate *validator.Validate) bool {
	if e := c.ShouldBindJSON(v); e != nil || validate.Struct(v) != nil {
		Fail(c, 400, constants.CodeBadRequest, constants.MessageValidation)
		return false
	}
	return true
}

// ServiceError 把服务层哨兵错误映射为合适的 HTTP 状态与业务码，其余按 500 处理。
func ServiceError(c *gin.Context, e error) {
	switch {
	case errors.Is(e, service.ErrNotFound):
		Fail(c, http.StatusNotFound, constants.CodeNotFound, e.Error())
	case errors.Is(e, service.ErrConflict), errors.Is(e, service.ErrAlreadyClaimed):
		Fail(c, http.StatusConflict, constants.CodeConflict, e.Error())
	case errors.Is(e, service.ErrImmutable):
		Fail(c, http.StatusConflict, constants.CodeConflict, e.Error())
	case errors.Is(e, service.ErrInvalidState):
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, e.Error())
	case errors.Is(e, service.ErrForbidden):
		Fail(c, http.StatusForbidden, constants.CodeForbidden, e.Error())
	default:
		Fail(c, http.StatusInternalServerError, constants.CodeInternal, e.Error())
	}
}
