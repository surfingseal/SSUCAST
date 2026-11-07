package schedulebatch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"example.com/crawlserver/internal/lifecycle"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/scheduleexperiment"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"os"
	"path/filepath"
	"time"
)

const Model = "gemini-3.5-flash-lite"

type Selection struct {
	Version       string          `json:"version"`
	SelectedAt    time.Time       `json:"selected_at"`
	Database      string          `json:"database"`
	Model         string          `json:"model"`
	SnapshotHash  string          `json:"snapshot_sha256"`
	StatusCounts  map[string]int  `json:"status_counts"`
	Total         int             `json:"total"`
	Active        int             `json:"active_candidates"`
	Completed     int             `json:"previously_analyzed"`
	Histories     int64           `json:"history_count"`
	IDs           []string        `json:"selected_ids"`
	Excluded      []string        `json:"excluded_ids"`
	SelectionHash string          `json:"selection_hash"`
	Items         []model.Content `json:"items"`
}

func Hash(v any) string { b, _ := json.Marshal(v); return fmt.Sprintf("%x", sha256.Sum256(b)) }
func SelectionHash(s Selection) string {
	return Hash([]any{s.Version, s.Database, s.Model, s.SnapshotHash, s.IDs, s.Excluded})
}
func Discover(ctx context.Context, store *repository.Store, database string, existing scheduleapply.Bundle) (Selection, error) {
	if database != "ssu_crawler" {
		return Selection{}, fmt.Errorf("expected ssu_crawler database")
	}
	if existing.Version != scheduleapply.BundleVersion || scheduleapply.ValidateBundle(existing) != nil {
		return Selection{}, fmt.Errorf("existing ten-notice bundle invalid")
	}
	rows, snapshot, e := lifecycle.Read(ctx, store.Client.Database(database))
	if e != nil {
		return Selection{}, fmt.Errorf("candidate snapshot read failed")
	}
	s := Selection{Version: "remaining-76-v1", SelectedAt: time.Now().UTC(), Database: database, Model: Model, SnapshotHash: snapshot, Total: len(rows), StatusCounts: map[string]int{}, IDs: []string{}, Excluded: []string{}, Items: []model.Content{}}
	repo := repository.NewScheduleAnalyses(store.Client.Database(database))
	done := map[bson.ObjectID]bool{}
	for _, v := range existing.Items {
		plan, e := repo.Preview(ctx, v)
		if e != nil || plan.Outcome != "duplicate" {
			return s, fmt.Errorf("existing analysis projection/input/version mismatch")
		}
		if e = repo.VerifyScheduleHistory(ctx, v); e != nil {
			return s, e
		}
		done[v.Original.ID] = true
		s.Excluded = append(s.Excluded, v.Original.ID.Hex())
	}
	s.Histories, e = store.Client.Database(database).Collection("schedule_analyses").CountDocuments(ctx, bson.M{})
	if e != nil {
		return s, fmt.Errorf("history count unavailable")
	}
	for _, row := range rows {
		v := row.Content
		if v.SourceName != "ssu_catch" {
			continue
		}
		s.StatusCounts[v.Status]++
		if v.Status != "진행" && v.Status != "unknown" {
			continue
		}
		s.Active++
		if done[v.ID] {
			s.Completed++
			continue
		}
		if v.ScheduleAnalysisID != "" || v.ScheduleInputHash != "" || v.ScheduleExtractionVersion != "" || v.Summary != "" || len(v.Schedules) > 0 {
			return s, fmt.Errorf("unexpected incomplete/preexisting analysis state")
		}
		if v.ContentHash == "" || model.ContentDigest(v) != v.ContentHash {
			return s, fmt.Errorf("candidate content hash mismatch")
		}
		s.IDs = append(s.IDs, v.ID.Hex())
		s.Items = append(s.Items, v)
	}
	if s.Total != 212 || s.StatusCounts["진행"] != 50 || s.StatusCounts["unknown"] != 36 || s.StatusCounts["완료"] != 126 || s.Active != 86 || s.Completed != 10 || s.Histories != 10 || len(s.Items) != 76 {
		return s, fmt.Errorf("expected 212 / 86 / 10 / 76 counts not met")
	}
	s.SelectionHash = SelectionHash(s)
	return s, nil
}
func Freeze(dir string, s Selection) error {
	if existing, e := os.ReadFile(filepath.Join(dir, "selected.json")); e == nil {
		var old Selection
		if json.Unmarshal(existing, &old) != nil || old.SelectionHash != s.SelectionHash || Hash(old.Items) != Hash(s.Items) {
			return fmt.Errorf("frozen candidates changed; no model calls allowed")
		}
		return nil
	}
	if e := scheduleexperiment.Write(filepath.Join(dir, "candidates.json"), s); e != nil {
		return e
	}
	return scheduleexperiment.Write(filepath.Join(dir, "selected.json"), s)
}
