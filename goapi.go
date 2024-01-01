package goapi

import (
	"fmt"
	"os"

	"github.com/go-errors/errors"
	"github.com/ottstack/goapi/internal/goapi"
	"github.com/ottstack/goapi/pkg/middleware"
	"github.com/valyala/fasthttp"
)

var globalServer = goapi.NewServer()

func Use(m middleware.Middleware) *goapi.Server {
	return globalServer.Use(m)
}

func HandleHTTP(path string, f func(*fasthttp.RequestCtx)) {
	err := globalServer.HandleRaw(path, f)
	if err != nil {
		panic(err)
	}
}

func Serve(services ...interface{}) {
	if err := globalServer.Serve(services); err != nil {
		if e, ok := err.(*errors.Error); ok {
			fmt.Println("exit error:", e.ErrorStack())
			os.Exit(1)
		} else {
			panic(err)
		}
	}

}
