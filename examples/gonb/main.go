package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/crackcomm/go-jupyter/jupyter"
)

func consumeMessages(ch <-chan interface{}) {
	for msg := range ch {
		switch msg := msg.(type) {
		case *jupyter.ExecuteInputMessage:
		case *jupyter.StatusMessage:
		case *jupyter.ErrorMessage:
			fmt.Printf("Kernel error message:\n%s", strings.Join(msg.Traceback, "\n"))
		case *jupyter.StreamMessage:
			fmt.Println(msg.Text)
		default:
			fmt.Printf("Received: %#v\n", msg)
		}
	}
}

var productCode = strings.TrimSpace(`
func product(a, b int) int {
	return a * b
}
%%
my_var := product(8, 6)
fmt.Printf("8 * 6 = %d", my_var)
`)

func main() {
	config, err := jupyter.ReadConfigFile("/tmp/gokernel.json")
	if err != nil {
		log.Fatal(err)
	}

	client, err := jupyter.NewClient(context.Background(), &config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	executeRequests := []*jupyter.ExecutionRequest{
		{
			Code:         productCode,
			StoreHistory: true,
		},
		{
			Code:         "%%\nfmt.Println(\"Hello world!\")",
			StoreHistory: true,
		},
		{
			// This fails just for fun
			Code:         "%%\n8 * product(16, 22)",
			StoreHistory: true,
		},
	}

	for _, req := range executeRequests {
		rep, ch, err := client.Execute(req)
		if err != nil {
			log.Fatal(err)
		}
		consumeMessages(ch)
		fmt.Printf("Response: %#v\n", rep)
	}

	inspectRep, err := client.Inspect(&jupyter.IntrospectionRequest{
		Code:      productCode,
		CursorPos: strings.Index(productCode, "my_var"),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(inspectRep.Data["text/markdown"])
}
