package client

import "context"

func client() {
	SetCaller(nil)
	SayHello(context.Background(), &SayHelloRequest{})
}
