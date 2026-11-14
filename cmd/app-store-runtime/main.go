package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/david6ca/Kobashi2/internal/bridge"
)

func main() {
	home := flag.String("home", "", "Disposable staging home outside the repository")
	flag.Parse()
	if *home == "" {
		fmt.Fprintln(os.Stderr, "--home is required")
		os.Exit(2)
	}
	path, err := bridge.PrepareNativeRuntime(context.Background(), *home)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(path)
}
