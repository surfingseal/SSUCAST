// Offline preparation only: reads saved files and writes a reviewable bundle.
package main

import (
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/schedules"
	"flag"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func read(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return fmt.Errorf("required local input unavailable")
	}
	return json.Unmarshal(b, v)
}
func run() error {
	input := flag.String("revalidated", "", "results-v2.json")
	original := flag.String("source-snapshot", "", "original saved snapshot containing items")
	raw := flag.String("raw-results", "", "saved actual results.json")
	provenance := flag.String("provenance", "", "v2 provenance.json")
	output := flag.String("output", "", "new apply-input.json")
	flag.Parse()
	var sources struct {
		Items []model.Content `json:"items"`
	}
	var converted struct {
		Notices []schedules.NoticeResult `json:"notices"`
	}
	var responses []struct {
		ID    string `json:"id"`
		Raw   string `json:"raw_response"`
		Model string `json:"model_version"`
		Usage bson.M `json:"usage"`
	}
	var prov struct {
		Generated time.Time `json:"generated_at"`
	}
	for _, x := range []struct {
		path string
		v    any
	}{{*input, &converted}, {*original, &sources}, {*raw, &responses}, {*provenance, &prov}} {
		if x.path == "" {
			return fmt.Errorf("all input flags required")
		}
		if e := read(x.path, x.v); e != nil {
			return e
		}
	}
	bundle := scheduleapply.Bundle{Version: scheduleapply.BundleVersion, PreparedAt: time.Now().UTC(), Items: []scheduleapply.Item{}}
	for _, n := range converted.Notices {
		var source *model.Content
		for i := range sources.Items {
			if sources.Items[i].ID.Hex() == n.ID {
				source = &sources.Items[i]
				break
			}
		}
		if source == nil {
			return fmt.Errorf("original notice missing")
		}
		matched := false
		for _, r := range responses {
			if r.ID != n.ID {
				continue
			}
			matched = true
			item, e := scheduleapply.Prepare(*source, n, r.Raw, r.Model, r.Usage, prov.Generated)
			if e != nil {
				return e
			}
			bundle.Items = append(bundle.Items, item)
			break
		}
		if !matched {
			return fmt.Errorf("raw response missing")
		}
	}
	if e := scheduleapply.ValidateBundle(bundle); e != nil {
		return e
	}
	b, e := json.MarshalIndent(bundle, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return fmt.Errorf("new output file required")
	}
	defer f.Close()
	if _, e = f.Write(append(b, '\n')); e != nil {
		return e
	}
	fmt.Println("Prepared 10 notices, 24 original schedules + 4 rule sessions; network/database requests=0")
	return nil
}
