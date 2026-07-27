package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/crackcomm/go-jupyter/jupyter"
)

func consumeMessages(ch <-chan any) {
	for msg := range ch {
		log.Printf("Message: %#v", msg)
	}
}

func main() {
	quit := flag.Bool("quit", false, "quit")
	kf := flag.String("config", "conn.json", "kernel config")
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
			const x = 13 * 66;
			let y = 42;
			console.log("Hello from Deno kernel!", x, y);`,
		},
	}

	for _, req := range executeRequests {
		start := time.Now()
		rep, ch, err := client.Execute(req)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Response: %#v\n", rep)
		consumeMessages(ch)
		fmt.Println("---- Execution time:", time.Since(start))
	}

	if *quit {
		_, err := client.Shutdown()
		if err != nil {
			log.Fatal(err)
		}
	}
}
