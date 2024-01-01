# Example:

```
package main

import (
	"context"
	"fmt"

	"github.com/ottstack/gofunc"
	"github.com/ottstack/gofunc/pkg/middleware"
	"github.com/ottstack/gofunc/pkg/websocket"
	"github.com/valyala/fasthttp"
)

type Request struct {
	Name string `json:"name" schema:"name" validate:"required" comment:"Required Name"`
}

type Response struct {
	Reply string `json:"reply"`
}

func HelloFunc(ctx context.Context, req *Request, rsp *Response) error {
	rsp.Reply = "Hello " + req.Name
	return nil
}

func Stream(ctx context.Context, req websocket.RecvStream, rsp websocket.SendStream) error {
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
	// curl '127.0.0.1:9001/api/hello?name=bob'
	// curl -X PUT '127.0.0.1:9001/api/hello' -d '{"name":"tom"}'
	gofunc.New("Default").
		Get("/api/hello", HelloFunc).
		Put("/api/hello", HelloFunc)

	// websocket: 127.0.0.1:9001/api/hello-ws
	gofunc.New("OtherService").
		Stream("/api/hello-ws", Stream)

	// origin http: curl '127.0.0.1:9001/api/hello/2'
	gofunc.HandleHTTP("GET", "/api/hello/2", func(rc *fasthttp.RequestCtx) {
		rc.Response.BodyWriter().Write([]byte("HELLO FAST HTTP"))
	})

	gofunc.Use(middleware.Recover).Use(middleware.Validator)
	gofunc.Serve()
}
```