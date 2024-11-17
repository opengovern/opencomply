package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	queryvalidator "github.com/opengovern/opengovernance/jobs/query-validator"
)

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	defer func() {
		signal.Stop(c)
		cancel()
	}()

	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	if err := queryvalidator.WorkerCommand().ExecuteContext(ctx); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
