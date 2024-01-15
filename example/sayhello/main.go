package main

import (
	"context"
	"fmt"

	"github.com/ottstack/goapi"
	"github.com/ottstack/goapi/pkg/middleware"
	"github.com/ottstack/goapi/pkg/websocket"
	"github.com/valyala/fasthttp"
)

type SayHelloRequest struct {
	Name string `json:"name" validate:"required" comment:"Required Name"`
}

type SayHelloResponse struct {
	Reply string `json:"reply"`
}

type HelloService struct{}

func (s *HelloService) SayHello(ctx context.Context, req *SayHelloRequest, rsp *SayHelloResponse) error {
	rsp.Reply = "Hello " + req.Name
	return nil
}

func (s *HelloService) StreamHello(ctx context.Context, req websocket.RecvStream, rsp websocket.SendStream) error {
	ct := 0
	for {
		msg, err := req.Recv()
		if err != nil {
			return err
		}
		fmt.Println("recv", string(msg))
		ct++
		if err := rsp.Send([]byte(fmt.Sprintf("hello %s %d times", string(msg), ct))); err != nil {
			return err
		}
		fmt.Println("send", string(msg))
		if ct > 2 {
			return nil
		}
	}
}

func main() {
	srv := goapi.NewServer()
	srv.Use(middleware.Recover).Use(middleware.Validator)

	// curl '127.0.0.1:8081/api/HelloService/SayHello' -d '{"name": "alice"}'
	// websocket: 127.0.0.1:8081/api/HelloService/StreamHello
	srv.RegisterService(&HelloService{})

	// origin http: curl '127.0.0.1:8081/api/hello/2'
	srv.RegisterHTTP("/api/hello/2", func(rc *fasthttp.RequestCtx) {
		rc.Response.BodyWriter().Write([]byte("HELLO FAST HTTP"))
	})

	srv.Serve()
}
