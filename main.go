package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/TheLazarusNetwork/erebrus-gateway/app"
)

func main() {
	app.Init()
	// wait for a SIGINT or SIGTERM signal
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println("Received signal, shutting down...")
}
