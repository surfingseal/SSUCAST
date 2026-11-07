package main

import (
	"encoding/json"
	"example.com/crawlserver/internal/evaluation"
	"flag"
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	dataset := flag.String("dataset", "internal/evaluation/testdata/notices.json", "versioned inputs, annotations and predictions")
	mode := flag.String("mode", "mock", "mock or replay; no live API calls")
	output := flag.String("output", "", "JSON report path (stdout when empty)")
	gold := flag.String("gold", "", "optional human annotation JSON; only explicit approvals are used")
	flag.Parse()
	cases, err := evaluation.Load(*dataset)
	if err != nil {
		return err
	}
	if *gold != "" {
		cases, err = evaluation.ApplyAnnotations(cases, *gold)
		if err != nil {
			return err
		}
	}
	r, err := evaluation.Evaluate(cases, *mode)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if *output != "" {
		return os.WriteFile(*output, b, 0600)
	}
	fmt.Print(string(b))
	return nil
}
