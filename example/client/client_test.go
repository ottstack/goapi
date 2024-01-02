package client

import (
	"context"
	"fmt"
	"testing"

	"github.com/ottstack/goapi/pkg/client"
)

func TestExample(t *testing.T) {
	SetClient(client.New("http://localhost:8081"))
	rsp, err := HelloService().SayHello(context.Background(), &HelloServiceSayHelloRequest{Name: "bob"})
	fmt.Println("rsp", rsp, err)
}
