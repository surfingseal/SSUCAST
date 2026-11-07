package main

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/service"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	execute := flag.Bool("execute", false, "explicitly import the approved local SSU snapshot to configured MongoDB")
	localURI := flag.String("local-uri", "mongodb://127.0.0.1:27030", "local snapshot server")
	localDB := flag.String("local-database", "ssu_recent_20261106", "local snapshot database")
	reportPath := flag.String("report", "", "safe report file (URI omitted)")
	flag.Parse()
	if !*execute {
		return fmt.Errorf("explicit -execute required; no connections made")
	}
	cfg, e := config.LoadMongoConnection("", "")
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	local, e := repository.Open(ctx, *localURI, *localDB)
	if e != nil {
		return config.RedactMongoError(e, *localURI)
	}
	defer local.Client.Disconnect(context.Background())
	target, e := repository.Open(ctx, cfg.URI, cfg.Database)
	if e != nil {
		return config.RedactMongoError(e, cfg.URI)
	}
	defer target.Client.Disconnect(context.Background())
	result, e := service.TransferSSUSnapshot(ctx, local, target, cfg.Database)
	b, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(b))
	if *reportPath != "" {
		if x := os.WriteFile(*reportPath, append(b, '\n'), 0600); x != nil {
			return x
		}
	}
	return config.RedactMongoError(e, cfg.URI)
}
