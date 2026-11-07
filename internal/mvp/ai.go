package mvp

import (
	"context"
	"encoding/json"
	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/scheduleapply"
	"example.com/crawlserver/internal/schedulebatch"
	"example.com/crawlserver/internal/scheduleexperiment"
	"example.com/crawlserver/internal/scheduleflash"
	"fmt"
	"google.golang.org/genai"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const AIModel = "gemini-3.8-flash"
const PromptVersion = "mvp-data-v1"

func AIConfig(media bool) *genai.GenerateContentConfig {
	c := scheduleflash.Config()
	c.SystemInstruction.Parts[0].Text += "\n이번 저장 작업의 요약은 가능하면 70자 내외의 한 문장으로 작성하고 절대 100자를 넘지 않는다. 본문이 이미지/첨부 참조만 있으면 확인되지 않은 내용을 만들지 않는다."
	if media {
		c.SystemInstruction.Parts[0].Text += "\n이 출처는 공식 카테고리가 없다. category는 academic/scholarship/international/international_student/extracurricular/teaching/volunteer/other 중 하나로 분류한다. category_reason은 목적에 근거한 짧은 설명, category_evidence는 제목 또는 본문의 연속된 실제 인용 문자열 배열이다. 학과 공지라는 이유만으로 academic으로 정하지 않는다. 근거 부족이면 other 및 review_required=true로 두고 review_reasons에 사유를 넣는다. 이미지/첨부는 분석하지 않는다. 외부 URL을 열지 않는다."
		schema := c.ResponseSchema
		schema.Properties["category"] = &genai.Schema{Type: genai.TypeString, Enum: []string{"academic", "scholarship", "international", "international_student", "extracurricular", "teaching", "volunteer", "other"}}
		schema.Properties["category_reason"] = &genai.Schema{Type: genai.TypeString}
		schema.Properties["category_evidence"] = &genai.Schema{Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}}
		schema.Properties["review_required"] = &genai.Schema{Type: genai.TypeBoolean}
		schema.Properties["review_reasons"] = &genai.Schema{Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}}
		schema.Required = append(schema.Required, "category", "category_reason", "category_evidence", "review_required", "review_reasons")
	}
	return c
}
func FreezeTargets(dir string, items []model.Content) ([]model.Content, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	path := filepath.Join(dir, "targets.json")
	old := []model.Content{}
	if e := Read(path, &old); e != nil {
		if _, e := os.Stat(path); !os.IsNotExist(e) {
			return nil, fmt.Errorf("target manifest corrupt")
		}
	}
	by := map[string]int{}
	for i, v := range old {
		by[v.ID.Hex()] = i
	}
	for _, v := range items {
		if v.SourceName != "ssu_catch" && v.SourceName != "ssu_global_media" {
			return nil, fmt.Errorf("unsupported source")
		}
		if model.ContentDigest(v) != v.ContentHash {
			return nil, fmt.Errorf("target source hash mismatch")
		}
		if i, ok := by[v.ID.Hex()]; ok {
			if scheduleapply.InputHash(old[i]) != scheduleapply.InputHash(v) {
				return nil, fmt.Errorf("frozen target changed")
			}
			continue
		}
		by[v.ID.Hex()] = len(old)
		old = append(old, v)
	}
	if len(old) > 110 {
		return nil, fmt.Errorf("target cap")
	}
	if e := Write(dir, "targets.json", old); e != nil {
		return nil, e
	}
	return old, nil
}
func Analyze(ctx context.Context, dir string, requested []model.Content, cfg classification.Config) error {
	targets, e := FreezeTargets(dir, requested)
	if e != nil {
		return e
	}
	lock, e := os.OpenFile(filepath.Join(dir, "running.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return fmt.Errorf("AI worker already active")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	spec := map[string]any{"model": AIModel, "thinking": "MEDIUM", "prompt_version": PromptVersion, "ssu_config_hash": schedulebatch.Hash(AIConfig(false)), "media_config_hash": schedulebatch.Hash(AIConfig(true)), "max_requests": 120, "cost_cap_usd": 10, "fallback": false}
	path := filepath.Join(dir, "generation-config.json")
	if _, e := os.Stat(path); e == nil {
		var old map[string]any
		if Read(path, &old) != nil || schedulebatch.Hash(old) != schedulebatch.Hash(spec) {
			return fmt.Errorf("generation config changed")
		}
	} else if os.IsNotExist(e) {
		if e = Write(dir, "generation-config.json", spec); e != nil {
			return e
		}
	} else {
		return e
	}
	ids := []string{}
	for _, v := range targets {
		ids = append(ids, v.ID.Hex())
	}
	budget, e := schedulebatch.LoadBoundedBudget(dir, ids, AIModel, 120)
	if e != nil {
		return e
	}
	budget.BeforeSend = func(r *http.Request) error { return scheduleflash.GuardLimit(dir, budget, r, 10) }
	if e = os.MkdirAll(filepath.Join(dir, "checkpoints"), 0700); e != nil {
		return e
	}
	client, e := schedulebatch.NewClient(ctx, cfg, budget)
	if e != nil {
		return e
	}
	meta, e := client.SDK.Models.Get(ctx, AIModel, nil)
	if e != nil {
		return fmt.Errorf("requested model unavailable: %s", classification.ErrorCode(e))
	}
	Write(dir, "vertex-preflight.json", map[string]any{"model": AIModel, "verified_model": meta.Name, "project": cfg.Project, "region": cfg.Location, "at": time.Now().UTC()})
	failures := []map[string]any{}
	for i, v := range requested {
		saved, e := schedulebatch.Recover(dir, budget, v)
		if e != nil {
			return e
		}
		if saved != nil {
			if saved.ModelVersion != AIModel {
				return fmt.Errorf("checkpoint model conflict")
			}
			fmt.Printf("MVP checkpoint %s reused\n", v.ID.Hex())
			continue
		}
		media := v.SourceName == "ssu_global_media"
		cap := 1
		if media {
			cap = 3
		}
		budget.MaxNoticeAttempts = cap
		for {
			count := 0
			ssuCalls := 0
			ssu := map[string]bool{}
			for _, t := range targets {
				if t.SourceName == "ssu_catch" {
					ssu[t.ID.Hex()] = true
				}
			}
			for _, a := range budget.Attempts {
				if a.ID == v.ID.Hex() {
					count++
				}
				if ssu[a.ID] {
					ssuCalls++
				}
			}
			if count >= cap || len(budget.Attempts) >= 120 || !media && ssuCalls >= 10 {
				failures = append(failures, map[string]any{"id": v.ID.Hex(), "reason": "request_budget_or_retry_cap"})
				break
			}
			if count > 0 {
				timer := time.NewTimer(time.Duration(1<<uint(count)) * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
			budget.ID = v.ID.Hex()
			input := map[string]any{"title": v.Title, "content": v.Content, "published_at": v.PublishedAt, "original_status": v.Status}
			if media {
				input["source"] = v.SourceName
			}
			b, _ := json.Marshal(input)
			call, stop := context.WithTimeout(ctx, 90*time.Second)
			_, err := client.SDK.Models.GenerateContent(call, AIModel, genai.Text(string(b)), AIConfig(media))
			stop()
			if err != nil {
				status := scheduleexperiment.Status(err)
				failures = append(failures, map[string]any{"id": v.ID.Hex(), "status": status, "reason": classification.ErrorCode(err), "attempt": count + 1})
				Write(dir, "failures.json", failures)
				if status == 401 || status == 403 || status == 404 || status == 0 {
					return fmt.Errorf("AI access/ambiguous transport blocked: %s", classification.ErrorCode(err))
				}
				if media && (status == 429 || status >= 500 && status <= 599) {
					continue
				}
				break
			}
			cp, e := schedulebatch.Recover(dir, budget, v)
			if e != nil || cp == nil {
				return fmt.Errorf("paid response checkpoint recovery failed")
			}
			if cp.ModelVersion != AIModel {
				return fmt.Errorf("returned model mismatch")
			}
			if cp.Usage == nil {
				return fmt.Errorf("usage missing; paid continuation blocked")
			}
			break
		}
		if e = AggregateAI(dir, targets); e != nil {
			return e
		}
		Write(dir, "failures.json", failures)
		fmt.Printf("MVP AI %d/%d source=%s id=%s calls=%d/120 cost=%.6f\n", i+1, len(requested), v.SourceName, v.ID.Hex(), len(budget.Attempts), scheduleflash.KnownCost(dir, budget))
	}
	return AggregateAI(dir, targets)
}
func AggregateAI(dir string, items []model.Content) error {
	results := []schedulebatch.Checkpoint{}
	tokens := map[string]int64{"input": 0, "output": 0, "thinking": 0, "total": 0}
	cost := 0.0
	for _, v := range items {
		var c schedulebatch.Checkpoint
		if e := Read(filepath.Join(dir, "checkpoints", v.ID.Hex()+".json"), &c); e != nil {
			continue
		}
		if c.InputHash != scheduleapply.InputHash(v) || c.ModelVersion != AIModel {
			return fmt.Errorf("AI checkpoint/input/model conflict")
		}
		results = append(results, c)
		if u := c.Usage; u != nil {
			tokens["input"] += int64(u.PromptTokenCount)
			tokens["output"] += int64(u.CandidatesTokenCount)
			tokens["thinking"] += int64(u.ThoughtsTokenCount)
			tokens["total"] += int64(u.TotalTokenCount)
			cost += scheduleflash.Cost(u)
		}
	}
	var attempts []schedulebatch.Attempt
	Read(filepath.Join(dir, "attempts.json"), &attempts)
	if e := Write(dir, "results.json", results); e != nil {
		return e
	}
	return Write(dir, "usage.json", map[string]any{"model": AIModel, "requests": len(attempts), "completed": len(results), "tokens": tokens, "cost_estimate_usd": cost, "cap_usd": 10, "not_final_invoice": true})
}
