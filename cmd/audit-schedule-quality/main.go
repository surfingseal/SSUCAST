// Offline audit only: no database/HTTP/SDK imports and no apply option.
package main

import (
	"encoding/json"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/schedules"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

func load(p string, v any) error {
	b, e := os.ReadFile(p)
	if e != nil {
		return fmt.Errorf("input unavailable")
	}
	return json.Unmarshal(b, v)
}
func save(dir, name string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0600)
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	input := flag.String("input", "", "frozen 76-item apply-input.json")
	out := flag.String("output", "", "new directory (must not exist)")
	flag.Parse()
	var b scheduleapply.Bundle
	if e := load(*input, &b); e != nil {
		return e
	}
	if e := scheduleapply.ValidateBundle(b); e != nil {
		return e
	}
	if b.Version != scheduleapply.BatchBundleVersion || len(b.Items) != 76 {
		return fmt.Errorf("expected frozen complete batch of 76")
	}
	if *out == "" {
		return fmt.Errorf("output required")
	}
	if _, e := os.Stat(*out); !os.IsNotExist(e) {
		return fmt.Errorf("output must be new; refusing overwrite")
	}
	if e := os.MkdirAll(*out, 0700); e != nil {
		return e
	}
	refined := b
	refined.Items = []scheduleapply.Item{}
	refined.PreparedAt = time.Now().UTC()
	audits, changes, evidences, zeros, summaries, results := []any{}, []any{}, []any{}, []any{}, []any{}, []any{}
	states := map[string]int{}
	beforeReview, afterReview, changedReview, originalCount, addedCount := 0, 0, 0, 0, 0
	summaryReviews, audienceBefore, audienceAfter := 0, 0, 0
	for _, item := range b.Items {
		o, a := item.Original, item.Analysis
		doc := schedules.Document{Title: o.Title, Content: o.Content}
		q, e := schedules.RevalidateQuality(o.ID.Hex(), a.RawResponse, doc)
		if e != nil {
			return e
		}
		next, e := scheduleapply.PrepareQuality(o, a.RawResponse, a.ModelVersion, a.TokenUsage, a.AnalyzedAt)
		if e != nil {
			return fmt.Errorf("%s: %w", o.ID.Hex(), e)
		}
		refined.Items = append(refined.Items, next)
		base := map[string]model.Schedule{}
		for _, s := range a.ValidatedOutput.Schedules {
			base[s.ID] = s
			originalCount++
			if s.ReviewRequired {
				beforeReview++
			}
		}
		rows := append(append([]model.Schedule{}, q.Notice.Schedules...), q.Notice.RuleSupplements...)
		for _, s := range rows {
			states[s.Quality.State]++
			if s.ReviewRequired {
				afterReview++
			}
			prior, exists := base[s.ID]
			if !exists {
				addedCount++
			}
			if exists && prior.ReviewRequired != s.ReviewRequired {
				changedReview++
			}
			audits = append(audits, map[string]any{"notice_id": o.ID.Hex(), "title": o.Title, "schedule": s, "previous_review_required": prior.ReviewRequired, "new": !exists, "source_spans_valid": schedules.ValidSpans(s.EvidenceSpans, doc), "set_issues": schedules.ValidateSet(rows), "checks": []string{"calendar_and_clock_grounding", "year_not_inferred_from_academic_or_published_year", "range_order", "type_and_scope", "duration_not_clock", "source_conflicts", "duplicate_candidate", "partial_precision", "parent_relationship", "notification_policy"}})
			if !exists || !reflect.DeepEqual(prior, s) {
				method := "rule"
				reason := "versioned quality policy and source spans; no assertion of independent human gold verification"
				if !exists {
					reason = "explicit source calendar omitted from saved AI output; partial precision only"
				}
				if exists && prior.ConversionMethod != s.ConversionMethod {
					reason = s.ConversionMethod
				}
				changes = append(changes, map[string]any{"notice_id": o.ID.Hex(), "original_schedule_id": s.ID, "added": !exists, "before": func() any {
					if !exists {
						return nil
					}
					return prior
				}(), "after": s, "reason": reason, "method": method, "source_spans": s.EvidenceSpans, "review_change": exists && prior.ReviewRequired != s.ReviewRequired, "state_reason": s.Quality.Reasons})
			}
			if exists && !prior.EvidenceLocation.Matched {
				evidences = append(evidences, map[string]any{"notice_id": o.ID.Hex(), "title": o.Title, "schedule_id": s.ID, "type": s.Type, "original_ai_evidence": prior.Evidence, "before": prior, "after": s, "source_spans": s.EvidenceSpans, "cause": "noncontiguous parent event datetime and session heading concatenated; overall clocks not individual session clocks", "resolution": "separate source spans; parent day only; tentative session timing retained"})
			}
		}
		if q.Zero != nil {
			zeros = append(zeros, map[string]any{"id": o.ID.Hex(), "title": o.Title, "full_source": o.Content, "attachment_metadata": o.Attachments, "review": q.Zero})
		}
		if a.ValidatedOutput.TargetAudienceReviewRequired {
			audienceBefore++
		}
		if q.Notice.Audience.ReviewRequired {
			audienceAfter++
		}
		if q.Summary.State == "needs_review" {
			summaryReviews++
		}
		summaries = append(summaries, map[string]any{"id": o.ID.Hex(), "title": o.Title, "original_audience": q.Notice.Audience.Raw, "validated_audience": q.Notice.Audience.TargetAudience, "before_audience_review": a.ValidatedOutput.TargetAudienceReviewRequired, "audit": q.Summary})
		results = append(results, map[string]any{"id": o.ID.Hex(), "quality": q, "original_input_hash": a.InputHash, "content_hash": a.ContentHash, "raw_response_hash": a.RawResponseHash, "model_version": a.ModelVersion, "token_usage": a.TokenUsage})
	}
	if e := scheduleapply.ValidateBundle(refined); e != nil {
		return e
	}
	metrics := map[string]any{"version": schedules.QualityVersion, "notices": len(b.Items), "original_schedules": originalCount, "added_schedules": addedCount, "total_schedules": originalCount + addedCount, "states": states, "review_before": beforeReview, "review_after": afterReview, "review_boolean_changed_existing": changedReview, "evidence_mismatches_before": len(evidences), "zero_schedule_notices": len(zeros), "audience_reviews_before": audienceBefore, "audience_reviews_after": audienceAfter, "summary_reviews": summaryReviews, "gemini_requests": 0, "school_requests": 0, "atlas_writes": 0, "raw_responses_preserved": true, "original_ten_excluded": b.Batch.ExcludedIDs}
	files := map[string]any{"quality-audit.json": map[string]any{"metrics": metrics, "schedules": audits}, "schedule-evidence-review.json": evidences, "zero-schedule-review.json": zeros, "review-policy-report.json": map[string]any{"version": schedules.QualityVersion, "metrics": metrics, "policy": map[string]string{"verified": "grounded calendar/clock, no unresolved reasons; eligible display", "partial": "grounded but missing date/time parts; display only known information; alert requires explicit full end date AND time", "uncertain": "missing year, conditional, approximate or tentative; source review needed; no deadline alerts", "blocked": "evidence/semantic conflict or duplicate; no automatic calendar display or alerts"}, "notices": results}, "summary-audit.json": summaries, "refined-results.json": results, "changes.json": changes, "apply-input-refined.json": refined}
	for name, v := range files {
		if e := save(*out, name, v); e != nil {
			return e
		}
	}
	return save(*out, "metrics.json", metrics)
}
