package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/crackcomm/go-jupyter/jupyter"
)

func consumeMessages(ch <-chan any) {
	for msg := range ch {
		switch msg := msg.(type) {
		case *jupyter.ExecuteInputMessage:
			// log.Printf("ExecuteInputMessage: %#v", msg)
		case *jupyter.StatusMessage:
			// log.Printf("StatusMessage: %s", msg)
		case *jupyter.ErrorMessage:
			log.Printf("ErrorMessage: %s\n", strings.Join(msg.Traceback, "\n"))
		case *jupyter.StreamMessage:
			log.Printf("StreamMessage: %s\n", msg.Text)
		default:
			log.Printf("Unknown message: %#v", msg)
		}
	}
}

func main() {
	quit := flag.Bool("quit", false, "quit")
	kf := flag.String("config", "/tmp/kernel.json", "kernel config")
	flag.Parse()

	config, err := jupyter.ReadConfigFile(*kf)
	if err != nil {
		log.Fatal(err)
	}

	client, err := jupyter.NewClient(context.Background(), config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	log.Print("start")

	executeRequests := []*jupyter.ExecutionRequest{
		{
			Code: `
import numpy as np
mat = np.random.rand(2, 2)
print(mat)`,
			StoreHistory: true,
			UserExpressions: map[string]string{
				"x": "13 * 66",
			},
		},
		{
			Code: "%who", // ipykernel magic
		},
		{
			Code: `print(666)`,
			UserExpressions: map[string]string{
				"m": "mat",
			},
			StoreHistory: true,
		},
		{
			Code:         `mat * 8`,
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
		fmt.Println("-----------------------------")
	}

	inspectRep, err := client.Inspect(&jupyter.IntrospectionRequest{
		Code: "mat",
		OmitSections: []string{
			"docstring",
			"class_docstring",
			"file",
			"length",
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	if inspectRep.Found {
		fmt.Printf("Inspection reply:\n%s\n", inspectRep.Data["text/plain"])
	} else {
		fmt.Printf("Inspection reply:\n%#v\n", inspectRep)
	}

	historyReply, err := client.History(&jupyter.HistoryRequest{
		Unique:         false,
		Output:         true,
		HistAccessType: "tail",
		N:              5,
	})
	if err != nil {
		log.Fatal(err)
	}

	for _, item := range historyReply.History {
		log.Printf("- %#v", item)
	}

	if *quit {
		_, err := client.Shutdown()
		if err != nil {
			log.Fatal(err)
		}
	}
}
