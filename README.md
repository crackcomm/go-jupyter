# go-jupyter

`go-jupyter` is a Go package that provides a Jupyter Protocol client for communication with Jupyter kernels. It enables Go programs to interact with Jupyter kernels, execute code, and receive responses.

## Installation

To install the package, use the following `go get` command:

```shell
go get -u github.com/crackcomm/go-jupyter
```

## Usage

Here's a basic example of using the `go-jupyter` package to create a Jupyter kernel client and execute code:

```Go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/crackcomm/go-jupyter/jupyter"
)

func main() {
	// Load Jupyter kernel connection info from a configuration file
	info, err := jupyter.ReadConfigFile("path/to/connection_info.json")
	if err != nil {
		log.Fatal(err)
	}

	// Create a new Jupyter kernel client
	client, err := jupyter.NewClient(context.Background(), &info)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Define the code to be executed
	code := "print('Hello, Jupyter!')"

	// Execute the code
	result, ch, err := client.Execute(&jupyter.ExecutionRequest{
		Code: code,
	})
	if err != nil {
		log.Fatal(err)
	}

	for msg := range ch {
		fmt.Printf("Kernel output: %#v", msg)
	}

	// Print the execution result
	fmt.Printf("Execution Status: %s\n", result.Status)
	fmt.Printf("Execution Count: %d\n", result.ExecutionCount)
}

```

This example demonstrates loading connection information from a file, creating a client, and executing a simple code snippet. The `go-jupyter` package provides additional functionality for inspecting code, handling input/output channels, and more.

## Documentation

For more detailed information and advanced usage, please refer to the package documentation on [godoc](https://godoc.org/github.com/crackcomm/go-jupyter/jupyter) or [GitHub](https://github.com/crackcomm/go-jupyter) and [Jupyter Protocol documentation](https://jupyter-protocol.readthedocs.io/en/latest/messaging.html).

## Contribution

Contributions and feedback are welcome! If you encounter issues or have suggestions for improvements, please open an issue on the [GitHub repository](https://github.com/crackcomm/go-jupyter).

Happy coding with Jupyter in Go!
