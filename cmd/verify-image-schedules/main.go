// Read-only post-apply audit against the exact pre-image BSON backup.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/config"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/mvp"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/scheduleapply"
	"flag"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func protected(r bson.Raw) string {
	m := map[string]string{}
	els, _ := r.Elements()
	for _, e := range els {
		k := e.Key()
		if k == "schedules" || k == "notice_policy" || strings.HasPrefix(k, "schedule_") || (k == "image_analysis_used" || k == "image_analysis_input_hash" || k == "image_analysis_content_hash" || k == "image_schedule_count" || k == "image_evidence_count") {
			continue
		}
		v := e.Value()
		m[k] = fmt.Sprintf("%s:%x", v.Type, v.Value)
	}
	b, _ := json.Marshal(m)
	return mvp.Hash(b)
}
func run() error {
	baseline := flag.String("baseline", "", "initial pre-image backup directory")
	out := flag.String("output", "", "private audit directory")
	flag.Parse()
	if *baseline == "" || *out == "" {
		return fmt.Errorf("baseline and output required")
	}
	os.MkdirAll(*out, 0700)
	cfg, e := config.LoadMongoConnection("", "")
	if e != nil || cfg.Database != "ssu_crawler" {
		return fmt.Errorf("expected configured ssu_crawler")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, e := repository.OpenReadOnly(ctx, cfg.URI, cfg.Database)
	if e != nil {
		return fmt.Errorf("database unavailable")
	}
	defer s.Client.Disconnect(context.Background())
	db := s.Client.Database(cfg.Database)
	before, e := mvp.ReadDump(filepath.Join(*baseline, "crawled_contents.bson"))
	if e != nil {
		return e
	}
	current, _, e := mvp.Collection(ctx, db, "crawled_contents")
	if e != nil {
		return e
	}
	by := map[string]bson.Raw{}
	for _, r := range current {
		by[r.Lookup("_id").ObjectID().Hex()] = r
	}
	if len(before) != 226 || len(current) != 226 {
		return fmt.Errorf("notice count mismatch")
	}
	for _, r := range before {
		now, ok := by[r.Lookup("_id").ObjectID().Hex()]
		if !ok || protected(r) != protected(now) {
			return fmt.Errorf("original/summary/audience field changed")
		}
	}
	old, e := mvp.ReadDump(filepath.Join(*baseline, "schedule_analyses.bson"))
	if e != nil {
		return e
	}
	for _, r := range old {
		var now bson.Raw
		if db.Collection("schedule_analyses").FindOne(ctx, bson.M{"_id": r.Lookup("_id").StringValue()}).Decode(&now) != nil || !bytes.Equal(r, now) {
			return fmt.Errorf("old analysis changed")
		}
	}
	for _, name := range []string{"crawl_sources", "content_classifications"} {
		before, e := mvp.ReadDump(filepath.Join(*baseline, name+".bson"))
		if e != nil {
			return e
		}
		now, _, e := mvp.Collection(ctx, db, name)
		if e != nil || len(now) != len(before) {
			return fmt.Errorf("reference collection changed")
		}
		for i := range before {
			if !bytes.Equal(before[i], now[i]) {
				return fmt.Errorf("reference collection modified")
			}
		}
	}
	imageNotices, imageSchedules, imageEvidence, allSchedules, partial, unsafe := 0, 0, 0, 0, 0, 0
	qualityStates := map[string]int{}
	types := map[string]int{}
	sources := map[string]int{}
	details := []map[string]any{}
	for _, r := range current {
		var v model.Content
		bson.Unmarshal(r, &v)
		sources[v.SourceName]++
		allSchedules += len(v.Schedules)
		if !v.ImageAnalysisUsed {
			continue
		}
		imageNotices++
		if v.ScheduleInputHash != scheduleapply.InputHash(v) {
			return fmt.Errorf("image input linkage changed")
		}
		n, e := db.Collection("schedule_analyses").CountDocuments(ctx, bson.M{"_id": v.ScheduleAnalysisID, "content_id": v.ID, "image_input_hash": v.ImageAnalysisInputHash})
		if e != nil || n != 1 {
			return fmt.Errorf("image history linkage broken")
		}
		count := 0
		for _, s := range v.Schedules {
			if len(s.ImageEvidence) == 0 {
				continue
			}
			count++
			types[s.Type]++
			imageSchedules++
			if s.Quality != nil {
				qualityStates[s.Quality.State]++
			}
			imageEvidence += len(s.ImageEvidence)
			if s.StartPartial != nil || s.EndPartial != nil {
				partial++
			}
			if !s.ReviewRequired || s.Quality == nil || (s.Quality.State != "uncertain" && s.Quality.State != "blocked") || s.Quality.DeadlineCandidate {
				unsafe++
			}
		}
		if count != v.ImageScheduleCount {
			return fmt.Errorf("image count mismatch")
		}
		details = append(details, map[string]any{"id": v.ID.Hex(), "title": v.Title, "image_schedules": count})
	}
	histories, _, e := mvp.Collection(ctx, db, "schedule_analyses")
	if e != nil {
		return e
	}
	models := map[string]int{}
	imageHistory := 0
	for _, r := range histories {
		var v struct {
			Model string `bson:"model_version"`
			Mode  string `bson:"analysis_mode"`
		}
		bson.Unmarshal(r, &v)
		models[v.Model]++
		if v.Mode == "text_plus_image" {
			imageHistory++
		}
	}
	if unsafe != 0 {
		return fmt.Errorf("image schedule incorrectly trusted")
	}
	return mvp.Write(*out, "atlas-verification.json", map[string]any{"contents": len(current), "source_counts": sources, "image_analyzed_notices": imageNotices, "image_schedules": imageSchedules, "image_evidence": imageEvidence, "total_schedules": allSchedules, "image_quality_states": qualityStates, "image_types": types, "partial_image_schedules": partial, "unsafe_image_schedules": unsafe, "old_histories_preserved": len(old), "analysis_histories": len(histories), "image_histories": imageHistory, "history_models": models, "original_summary_audience_unchanged": true, "references_unchanged": true, "history_links_verified": true, "notices": details, "writes": 0})
}
