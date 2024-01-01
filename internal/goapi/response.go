package goapi

import (
	json "github.com/goccy/go-json"
	"github.com/ottstack/goapi/pkg/ecode"
	"github.com/valyala/fasthttp"
)

var encoder = json.Marshal
var jsonDecoder = json.Unmarshal

func writeErrResponse(w *fasthttp.RequestCtx, err error) {
	if _, ok := err.(*ecode.APIError); !ok {
		err = ecode.Errorf(500, err.Error())
	}
	w.Response.SetStatusCode(ecode.ToHttpCode(err))
	bs, _ := encoder(err)
	w.Write(bs)
}
