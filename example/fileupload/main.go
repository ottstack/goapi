package main

import (
	"fmt"

	"github.com/ottstack/goapi"
	"github.com/ottstack/goapi/pkg/middleware"
	"github.com/valyala/fasthttp"
)

func main() {
	srv := goapi.NewServer()
	srv.Use(middleware.Recover).Use(middleware.Validator)

	// curl -F file=@main.go http://127.0.0.1:8081/api/upload
	srv.RegisterHTTP("/api/upload", func(ctx *fasthttp.RequestCtx) {
		fh, err := ctx.FormFile("file")
		if err != nil {
			ctx.Response.SetStatusCode(400)
			fmt.Fprint(ctx.Response.BodyWriter(), err)
			return
		}
		if err := fasthttp.SaveMultipartFile(fh, "filename.ext"); err != nil {
			ctx.Response.SetStatusCode(500)
			fmt.Fprint(ctx.Response.BodyWriter(), err)
			return
		}
		fmt.Fprint(ctx.Response.BodyWriter(), "upload ok")
	})

	srv.Serve()
}
