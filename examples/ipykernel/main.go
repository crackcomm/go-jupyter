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

func main() {
	config, err := jupyter.ReadConfigFile("/tmp/kernel.json")
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
			Code:         "print(123 * 9999); my_var = 3; nn = np.random.rand(22, 33)",
			StoreHistory: true,
			UserExpressions: map[string]string{
				"x": "13 * 66",
			},
		},
		{
			Code: "give me error !@#$",
		},
		{
			Code: "%who", // ipykernel magic
		},
		{
			Code:         "my_var * 8",
			StoreHistory: true,
		},
	}

	for _, req := range executeRequests {
		rep, ch, err := client.Execute(req)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Response: %#v\n", rep)
		consumeMessages(ch)
	}

	inspectRep, err := client.Inspect(&jupyter.IntrospectionRequest{
		Code: "my_var",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Inspection reply:\n%s\n", inspectRep.Data["text/plain"])

	historyReply, err := client.History(&jupyter.HistoryRequest{
		Unique:         true,
		Output:         true,
		HistAccessType: "tail",
		N:              5,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Response: %#v", historyReply)
}
