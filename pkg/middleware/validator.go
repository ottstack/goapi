package middleware

import (
	"context"

	validate "github.com/go-playground/validator/v10"
	"github.com/ottstack/goapi/pkg/ecode"
	"github.com/valyala/fasthttp"
)

var validator = validate.New()

func Validator(ctx context.Context, fastReq *fasthttp.RequestCtx, method MethodFunc, req, rsp interface{}) (err error) {
	if req == nil {
		return method(ctx, req, rsp)
	}
	if err := validator.Struct(req); err != nil {
		return &ecode.APIError{Code: 400, Message: err.Error()}
	}
	return method(ctx, req, rsp)
}
