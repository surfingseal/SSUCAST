package mvp

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/schedulebatch"
	"example.com/crawlserver/internal/schedules"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"path/filepath"
	"sort"
)

type Prepared struct {
	Version          string               `json:"version"`
	Items            []scheduleapply.Item `json:"items"`
	BaselineHash     string               `json:"baseline_hash"`
	PriorHistoryHash string               `json:"prior_history_hash"`
}

func PrepareSSU(baseline, prior76, newAI, out string) error {
	var originals []model.Content
	var prior, newRows []schedulebatch.Checkpoint
	var manifest struct {
		Hashes  map[string]string `json:"hashes"`
		Restore bool              `json:"loopback_restore_exact"`
	}
	if Read(filepath.Join(baseline, "active-originals.json"), &originals) != nil || Read(filepath.Join(baseline, "baseline.json"), &manifest) != nil || !manifest.Restore {
		return fmt.Errorf("verified restored backup required")
	}
	if e := VerifyPrior76(prior76, originals); e != nil {
		return e
	}
	if Read(filepath.Join(prior76, "flash-raw-results.json"), &prior) != nil || Read(filepath.Join(newAI, "results.json"), &newRows) != nil {
		return fmt.Errorf("saved Flash responses unavailable")
	}
	by := map[string]schedulebatch.Checkpoint{}
	for _, c := range prior {
		by[c.ID] = c
	}
	newCount := 0
	for _, c := range newRows {
		if _, ok := by[c.ID]; ok {
			return fmt.Errorf("duplicate old/new paid target")
		}
		by[c.ID] = c
		newCount++
	}
	if len(originals) != 86 || len(by) != 86 || newCount != 10 {
		return fmt.Errorf("expected 86 unique: 76 reused plus ten new")
	}
	bundle := Prepared{Version: schedules.ProductionVersion, Items: []scheduleapply.Item{}, BaselineHash: manifest.Hashes["crawled_contents"], PriorHistoryHash: manifest.Hashes["schedule_analyses"]}
	for _, v := range originals {
		c, ok := by[v.ID.Hex()]
		if !ok || c.Error != "" || c.ModelVersion != AIModel || c.InputHash != scheduleapply.InputHash(v) || c.HTTPStatus != 200 {
			return fmt.Errorf("Flash identity/input mismatch")
		}
		usage := bson.M{}
		b, _ := json.Marshal(c.Usage)
		json.Unmarshal(b, &usage)
		item, e := scheduleapply.PrepareProduction(v, c.RawResponse, c.ModelVersion, usage, c.CompletedAt)
		if e != nil {
			return fmt.Errorf("%s: %w", c.ID, e)
		}
		bundle.Items = append(bundle.Items, item)
	}
	sort.Slice(bundle.Items, func(i, j int) bool { return bundle.Items[i].Original.ID.Hex() < bundle.Items[j].Original.ID.Hex() })
	return Write(out, "ssu-prepared.json", bundle)
}

var analysisKeys = map[string]bool{"summary": true, "target_audience": true, "target_audience_review_required": true, "target_audience_review_reasons": true, "summary_review_required": true, "schedules": true, "schedule_extraction_version": true, "schedule_extraction_method": true, "schedule_review_required": true, "schedule_analyzed_at": true, "schedule_input_hash": true, "schedule_model_version": true, "schedule_result_hash": true, "schedule_analysis_id": true, "schedule_validation_version": true, "schedule_quality_state": true, "summary_quality_state": true, "audience_quality_state": true}

func OriginalHash(raw bson.Raw) (string, error) {
	var d bson.D
	if bson.Unmarshal(raw, &d) != nil {
		return "", fmt.Errorf("invalid original BSON")
	}
	keep := bson.D{}
	for _, v := range d {
		if !analysisKeys[v.Key] {
			keep = append(keep, v)
		}
	}
	b, e := bson.Marshal(keep)
	return Hash(b), e
}
func ApplySSU(ctx context.Context, store *repository.Store, input, baseline, out string, apply bool) error {
	var p Prepared
	if Read(input, &p) != nil || p.Version != schedules.ProductionVersion || len(p.Items) != 86 {
		return fmt.Errorf("production input required")
	}
	var manifest struct {
		Hashes  map[string]string `json:"hashes"`
		Restore bool              `json:"loopback_restore_exact"`
	}
	if Read(filepath.Join(baseline, "baseline.json"), &manifest) != nil || !manifest.Restore || p.BaselineHash != manifest.Hashes["crawled_contents"] || p.PriorHistoryHash != manifest.Hashes["schedule_analyses"] {
		return fmt.Errorf("backup provenance mismatch")
	}
	backup, e := ReadDump(filepath.Join(baseline, "crawled_contents.bson"))
	if e != nil {
		return e
	}
	originals := map[bson.ObjectID]string{}
	for _, raw := range backup {
		var v model.Content
		bson.Unmarshal(raw, &v)
		if v.SourceName == "ssu_catch" {
			h, e := OriginalHash(raw)
			if e != nil {
				return e
			}
			originals[v.ID] = h
		}
	}
	db := store.Client.Database("ssu_crawler")
	before, _, e := Collection(ctx, db, "crawled_contents")
	if e != nil {
		return e
	}
	for _, raw := range before {
		var v model.Content
		bson.Unmarshal(raw, &v)
		if h, ok := originals[v.ID]; ok {
			actual, _ := OriginalHash(raw)
			if h != actual {
				return fmt.Errorf("original changed since backup: %s", v.ID.Hex())
			}
		}
	}
	// Upgrades require every prior immutable history still to be present before writing.
	oldHistBefore, err := ReadDump(filepath.Join(baseline, "schedule_analyses.bson"))
	if err != nil {
		return err
	}
	for _, r := range oldHistBefore {
		var v bson.M
		bson.Unmarshal(r, &v)
		var current bson.Raw
		if db.Collection("schedule_analyses").FindOne(ctx, bson.M{"_id": v["_id"]}).Decode(&current) != nil || Hash(r) != Hash(current) {
			return fmt.Errorf("prior history missing or changed; upgrade blocked")
		}
	}
	if len(originals) != 212 {
		return fmt.Errorf("expected baseline SSU212")
	}
	present := 0
	for _, r := range before {
		var v model.Content
		bson.Unmarshal(r, &v)
		if v.SourceName == "ssu_catch" {
			present++
		}
	}
	if present != len(originals) {
		return fmt.Errorf("SSU original count changed before apply")
	}
	repo := repository.NewScheduleAnalyses(db)
	plans := []repository.SchedulePlan{}
	counts := map[string]int{}
	total, review := 0, 0
	seen := map[bson.ObjectID]bool{}
	for _, item := range p.Items {
		if item.Original.SourceName != "ssu_catch" || seen[item.Original.ID] || item.Analysis.ModelVersion != AIModel || scheduleapply.ValidateItem(item) != nil {
			return fmt.Errorf("invalid production identity")
		}
		seen[item.Original.ID] = true
		plan, e := repo.Preview(ctx, item)
		if e != nil {
			return e
		}
		plans = append(plans, plan)
		counts[plan.Outcome]++
		total += plan.Schedules
		if plan.ReviewRequired {
			review++
		}
	}
	report := map[string]any{"mode": "preview", "targets": 86, "outcomes": counts, "schedules": total, "review_notices": review, "plans": plans, "atlas_writes": 0, "originals_before": len(originals)}
	if e = Write(out, "preview-ssu.json", report); e != nil {
		return e
	}
	if !apply {
		return nil
	}
	// Explicit production command only; legacy experimental --apply remains disabled.
	results := []repository.ScheduleApplyResult{}
	writes := int64(0)
	for i, item := range p.Items {
		if plans[i].Outcome != "eligible" && plans[i].Outcome != "duplicate" {
			continue
		}
		r, e := repo.Apply(ctx, item, plans[i])
		results = append(results, r)
		writes += r.Modified
		report["results"] = results
		report["atlas_writes"] = writes
		Write(out, "apply-ssu.json", report)
		if e != nil {
			return e
		}
		if r.Outcome != "updated" && r.Outcome != "duplicate" {
			return fmt.Errorf("SSU CAS conflict; stopped after partial application")
		}
	}
	rows, _, e := Collection(ctx, db, "crawled_contents")
	if e != nil {
		return e
	}
	ssu, flash, schedulesCount := 0, 0, 0
	for _, raw := range rows {
		var v model.Content
		bson.Unmarshal(raw, &v)
		if v.SourceName != "ssu_catch" {
			continue
		}
		ssu++
		h, _ := OriginalHash(raw)
		if originals[v.ID] != h {
			return fmt.Errorf("original data invariant failed")
		}
		if seen[v.ID] && v.ScheduleModelVersion == AIModel {
			flash++
			schedulesCount += len(v.Schedules)
		}
	}
	oldHist, e := ReadDump(filepath.Join(baseline, "schedule_analyses.bson"))
	if e != nil {
		return e
	}
	for _, raw := range oldHist {
		var v bson.M
		bson.Unmarshal(raw, &v)
		var now bson.Raw
		if db.Collection("schedule_analyses").FindOne(ctx, bson.M{"_id": v["_id"]}).Decode(&now) != nil || Hash(now) != Hash(raw) {
			return fmt.Errorf("prior Lite history changed")
		}
	}
	duplicates := 0
	for _, item := range p.Items {
		plan, e := repo.Preview(ctx, item)
		if e != nil {
			return e
		}
		if plan.Outcome == "duplicate" {
			duplicates++
			if e = repo.VerifyScheduleHistory(ctx, item); e != nil {
				return e
			}
		}
	}
	histories, _ := db.Collection("schedule_analyses").CountDocuments(ctx, bson.M{})
	report["mode"] = "apply"
	report["ssu_contents"] = ssu
	report["flash_applied"] = flash
	report["stored_schedules"] = schedulesCount
	report["duplicate_preview_after"] = duplicates
	report["history_count"] = histories
	report["prior_histories_preserved"] = len(oldHist)
	report["all_original_fields_unchanged"] = true
	if e = Write(out, "apply-ssu.json", report); e != nil {
		return e
	}
	if ssu != len(originals) || flash != 86 || duplicates != 86 {
		return fmt.Errorf("SSU post-apply count mismatch")
	}
	fmt.Printf("SSU applied=%d schedules=%d originals=%d histories=%d duplicate=%d\n", flash, schedulesCount, ssu, histories, duplicates)
	return nil
}

// Combined category result validation is independent of schedule confidence.
func MediaLabels(source model.Source, v model.Content, raw string) (model.Classification, error) {
	var r struct {
		Category string   `json:"category"`
		Reason   string   `json:"category_reason"`
		Evidence []string `json:"category_evidence"`
		Review   *bool    `json:"review_required"`
		Reasons  []string `json:"review_reasons"`
	}
	if json.Unmarshal([]byte(raw), &r) != nil || r.Review == nil || r.Reasons == nil {
		return model.Classification{}, fmt.Errorf("category schema missing")
	}
	valid := false
	for _, c := range model.OfficialCategories {
		if r.Category == c {
			valid = true
		}
	}
	if !valid || r.Reason == "" {
		return model.Classification{}, fmt.Errorf("invalid category enum/reason")
	}
	status := "classified"
	if *r.Review || len(r.Evidence) == 0 {
		status = "needs_review"
	}
	evidence := []model.ClassificationEvidence{}
	for _, quote := range r.Evidence {
		l := schedules.MatchEvidence(v.Title, v.Content, quote)
		if !l.Matched {
			return model.Classification{}, fmt.Errorf("category evidence mismatch")
		}
		evidence = append(evidence, model.ClassificationEvidence{Field: "categories", Label: r.Category, Source: l.Field, Quote: quote})
	}
	if v.Metadata["collection_completeness"] != "complete" {
		status = "needs_review"
	}
	labels := model.ClassificationLabels{PrimaryCategory: r.Category, Categories: []string{r.Category}, ActivityTypes: []string{}, Topics: []string{}, TargetAudience: []string{}, ClassificationStatus: status, ReasoningSummary: r.Reason, Evidence: evidence}
	if e := classification.Validate(labels); e != nil {
		return model.Classification{}, e
	}
	record := model.Classification{ContentID: v.ID, ClassificationLabels: labels, InputHash: classification.PolicyInputHash(source, v), TaxonomyVersion: classification.TaxonomyVersion, ModelVersion: AIModel, ResolvedModelVersion: AIModel, ClassificationMethod: "ai", ClassificationPolicyHash: source.ClassificationPolicyHash(), ContentHash: v.ContentHash, ApplicationStatus: "pending", UsageKnown: true, ModelRequests: 1}
	key := Hash([]byte(v.ID.Hex() + record.InputHash + record.TaxonomyVersion + AIModel))
	id, _ := bson.ObjectIDFromHex(key[:24])
	record.ID = id
	return record, nil
}
