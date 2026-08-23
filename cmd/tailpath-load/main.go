package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/GhostFlying/tailpath/internal/perfgate"
)

func main() {
	config := perfgate.DefaultConfig()
	flags := flag.NewFlagSet("tailpath-load", flag.ExitOnError)
	flags.StringVar(&config.ServerURL, "server", config.ServerURL, "loopback fixture server URL")
	flags.DurationVar(&config.Duration, "duration", config.Duration, "steady ingest duration")
	flags.IntVar(&config.ReportsPerSecond, "rps", config.ReportsPerSecond, "scheduled reports per second")
	flags.IntVar(&config.Workers, "workers", config.Workers, "maximum concurrent requests")
	flags.IntVar(&config.APISamples, "api-samples", config.APISamples, "samples per read API")
	flags.Parse(os.Args[1:])

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	result, err := perfgate.Run(ctx, config)
	if encodeErr := json.NewEncoder(os.Stdout).Encode(result); encodeErr != nil {
		fmt.Fprintln(os.Stderr, encodeErr)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
