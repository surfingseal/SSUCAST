package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"example.com/crawlserver/internal/classification"
	"example.com/crawlserver/internal/model"
)

type Annotation struct {
	Origin     string                     `json:"origin"`
	Approved   bool                       `json:"approved"`
	Reviewer   string                     `json:"reviewer"`
	ApprovedAt string                     `json:"approved_at"`
	InputHash  string                     `json:"input_hash"`
	Fields     []string                   `json:"fields"`
	Labels     model.ClassificationLabels `json:"labels"`
}
type Prediction struct {
	Provider     string                     `json:"provider"`
	Model        string                     `json:"model"`
	Labels       model.ClassificationLabels `json:"labels"`
	ErrorCode    string                     `json:"error_code,omitempty"`
	ModelCalls   int                        `json:"model_calls"`
	UsageKnown   bool                       `json:"usage_known"`
	InputTokens  int32                      `json:"input_tokens"`
	OutputTokens int32                      `json:"output_tokens"`
	InputHash    string                     `json:"input_hash"`
}
type Case struct {
	InputVersion   string        `json:"input_version"`
	ID             string        `json:"id"`
	Provenance     string        `json:"provenance"`
	SourceArtifact string        `json:"source_artifact"`
	Input          model.Content `json:"input"`
	Reference      *Annotation   `json:"reference,omitempty"`
	Human          *Annotation   `json:"human_gold,omitempty"`
	Mock           *Prediction   `json:"mock_prediction,omitempty"`
	Replay         *Prediction   `json:"replay_prediction,omitempty"`
}
type CaseResult struct {
	ID               string                      `json:"id"`
	Provenance       string                      `json:"provenance"`
	InputHash        string                      `json:"input_hash"`
	OriginalCategory string                      `json:"original_category"`
	Raw              model.ClassificationLabels  `json:"raw_prediction"`
	Validated        model.ClassificationLabels  `json:"validated_prediction"`
	Quality          model.ClassificationQuality `json:"quality"`
	HumanGold        *Annotation                 `json:"human_gold,omitempty"`
	ErrorCode        string                      `json:"error_code,omitempty"`
}
type Usage struct {
	Cases           int     `json:"cases"`
	ModelCalls      int     `json:"model_calls"`
	Failures        int     `json:"failures"`
	FailureRate     float64 `json:"failure_rate"`
	KnownUsageCases int     `json:"known_usage_cases"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
}
type Report struct {
	Mode                      string              `json:"mode"`
	NewVertexCalls            int                 `json:"new_vertex_calls"`
	HumanMetrics              map[string]*Metrics `json:"human_gold_raw_metrics"`
	HumanValidatedMetrics     map[string]*Metrics `json:"human_gold_validated_metrics"`
	ReferenceMetrics          map[string]*Metrics `json:"agent_reference_raw_mock_diagnostics"`
	ReferenceValidatedMetrics map[string]*Metrics `json:"agent_reference_validated_mock_diagnostics"`
	Usage                     map[string]*Usage   `json:"usage_by_provider"`
	Selected                  int                 `json:"selected_cases"`
	NeedsReview               int                 `json:"needs_review_cases"`
	NeedsReviewRate           float64             `json:"needs_review_rate"`
	AcceptedCoverage          float64             `json:"classified_coverage"`
	RejectedClaims            int                 `json:"rejected_claims"`
	Results                   []CaseResult        `json:"results"`
	Limitations               []string            `json:"limitations"`
}

func Load(path string) ([]Case, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []Case
	if err = json.Unmarshal(raw, &cases); err != nil {
		return nil, err
	}
	if err := validateCases(cases); err != nil {
		return nil, err
	}
	return cases, nil
}
func validateCases(cases []Case) error {
	seen := map[string]bool{}
	for _, c := range cases {
		if c.InputVersion != "prompt1" && c.InputVersion != "prompt2" {
			return fmt.Errorf("unsupported input version")
		}
		if c.ID == "" || seen[c.ID] || c.Input.Title == "" || (c.Provenance != "synthetic" && c.Provenance != "real_html") {
			return fmt.Errorf("invalid or duplicate evaluation case")
		}
		seen[c.ID] = true
		if c.Human != nil {
			a := c.Human
			if a.Origin != "human" || !a.Approved || a.Reviewer == "" || a.InputHash != InputHash(c) {
				return fmt.Errorf("human approval/hash missing for %s", c.ID)
			}
			if _, err := time.Parse(time.RFC3339, a.ApprovedAt); err != nil {
				return fmt.Errorf("invalid human approval time")
			}
		}
		for _, a := range []*Annotation{c.Human, c.Reference} {
			if a == nil {
				continue
			}
			if a.InputHash != InputHash(c) {
				return fmt.Errorf("annotation input hash mismatch")
			}
			if len(a.Fields) == 0 {
				return fmt.Errorf("annotation has no approved fields")
			}
			fieldSeen := map[string]bool{}
			for _, field := range a.Fields {
				if fieldSeen[field] {
					return fmt.Errorf("duplicate annotation field")
				}
				fieldSeen[field] = true
				allowed, ok := dimensions[field]
				if !ok {
					return fmt.Errorf("unsupported annotation field")
				}
				seen := map[string]bool{}
				for _, v := range labels(a.Labels, field) {
					if seen[v] || !contains(allowed, v) {
						return fmt.Errorf("invalid annotation label")
					}
					seen[v] = true
				}
			}
		}
		for _, p := range []*Prediction{c.Mock, c.Replay} {
			if p != nil && (p.InputHash != InputHash(c) || p.ModelCalls < 0 || p.InputTokens < 0 || p.OutputTokens < 0) {
				return fmt.Errorf("prediction hash/telemetry mismatch for %s", c.ID)
			}
		}
	}
	return nil
}

var dimensions = map[string][]string{"categories": classification.LegacyPrimary, "activity_types": classification.Activities, "topics": classification.Topics}

func contains(a []string, v string) bool {
	for _, x := range a {
		if x == v {
			return true
		}
	}
	return false
}
func labels(v model.ClassificationLabels, field string) []string {
	switch field {
	case "categories":
		return v.Categories
	case "activity_types":
		return v.ActivityTypes
	case "topics":
		return v.Topics
	}
	return nil
}
func Evaluate(cases []Case, mode string) (Report, error) {
	r := Report{Mode: mode, HumanMetrics: map[string]*Metrics{}, HumanValidatedMetrics: map[string]*Metrics{}, ReferenceMetrics: map[string]*Metrics{}, ReferenceValidatedMetrics: map[string]*Metrics{}, Usage: map[string]*Usage{}, Results: []CaseResult{}, Limitations: []string{"Mock/reference diagnostics and their zero-token telemetry are simulated fixture diagnostics, not real Gemini accuracy or billing.", "Human labels apply only to explicitly approved fields and matching input hashes.", "needs_review predictions still count in metrics; failed predictions count as empty predictions, not dropped samples.", "Evidence guards verify text spans and lexical support; they do not prove semantic correctness or analyse images/attachments."}}
	if err := validateCases(cases); err != nil {
		return r, err
	}
	if mode != "mock" && mode != "replay" {
		return r, fmt.Errorf("mode must be mock or replay; evaluation never calls Vertex")
	}
	humanRaw, humanClean, refRaw, refClean := map[string][]Pair{}, map[string][]Pair{}, map[string][]Pair{}, map[string][]Pair{}
	for _, c := range cases {
		p := c.Mock
		if mode == "replay" {
			p = c.Replay
		}
		if p == nil {
			continue
		}
		if mode == "mock" && p.Provider != "mock" || mode == "replay" && p.Provider != "vertex_replay" {
			return r, fmt.Errorf("prediction provenance/provider mismatch")
		}
		r.Selected++
		u := r.Usage[p.Provider]
		if u == nil {
			u = &Usage{}
			r.Usage[p.Provider] = u
		}
		u.Cases++
		u.ModelCalls += p.ModelCalls
		if p.UsageKnown {
			u.KnownUsageCases++
			u.InputTokens += int64(p.InputTokens)
			u.OutputTokens += int64(p.OutputTokens)
		}
		raw := p.Labels
		errCode := p.ErrorCode
		if errCode == "" {
			if err := classification.ValidateLegacy(raw); err != nil {
				errCode = "invalid_prediction"
			}
		}
		cleaned, q := classification.ApplyQuality(c.Input, raw)
		if errCode != "" {
			u.Failures++
			raw = model.ClassificationLabels{}
			cleaned = model.ClassificationLabels{}
			q = model.ClassificationQuality{Version: classification.QualityVersion, ReviewReasons: []string{"model_failure"}, RejectedClaims: []model.RejectedClaim{}}
		}
		if cleaned.ClassificationStatus == "needs_review" {
			r.NeedsReview++
		}
		if cleaned.ClassificationStatus == "classified" {
			r.AcceptedCoverage++
		}
		r.RejectedClaims += len(q.RejectedClaims)
		r.Results = append(r.Results, CaseResult{ID: c.ID, Provenance: c.Provenance, InputHash: InputHash(c), OriginalCategory: c.Input.OriginalCategory, Raw: p.Labels, Validated: cleaned, Quality: q, HumanGold: c.Human, ErrorCode: errCode})
		score := func(a *Annotation, toRaw, toClean map[string][]Pair) {
			if a == nil {
				return
			}
			for _, field := range a.Fields {
				toRaw[field] = append(toRaw[field], Pair{Gold: labels(a.Labels, field), Predicted: labels(raw, field)})
				toClean[field] = append(toClean[field], Pair{Gold: labels(a.Labels, field), Predicted: labels(cleaned, field)})
			}
		}
		score(c.Human, humanRaw, humanClean)
		if c.Provenance == "synthetic" && mode == "mock" {
			score(c.Reference, refRaw, refClean)
		}
	}
	for field, universe := range dimensions {
		r.HumanMetrics[field] = Score(humanRaw[field], universe)
		r.HumanValidatedMetrics[field] = Score(humanClean[field], universe)
		r.ReferenceMetrics[field] = Score(refRaw[field], universe)
		r.ReferenceValidatedMetrics[field] = Score(refClean[field], universe)
	}
	for _, u := range r.Usage {
		if u.Cases > 0 {
			u.FailureRate = float64(u.Failures) / float64(u.Cases)
		}
	}
	if r.Selected > 0 {
		r.NeedsReviewRate = float64(r.NeedsReview) / float64(r.Selected)
		r.AcceptedCoverage /= float64(r.Selected)
	}
	return r, nil
}

// Legacy replay retains the exact input format used by the original paid call.
// Media metadata enriches quality analysis but must not silently change which
// text/model input is associated with the historical prediction.
func InputHash(c Case) string {
	if c.InputVersion != "prompt1" {
		return classification.InputHash(c.Input)
	}
	v := c.Input
	b, _ := json.Marshal(map[string]any{"title": model.NormalizeText(v.Title), "original_category": model.NormalizeText(v.OriginalCategory), "content": model.NormalizeText(v.Content), "source": v.SourceName, "original_url": v.OriginalURL, "published_at": v.PublishedAt, "department": model.NormalizeText(v.Department)})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ApplyAnnotations imports only explicit human approvals. Pending template rows
// never become gold. A matching hash binds each approval to its exact input.
func ApplyAnnotations(cases []Case, path string) ([]Case, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		CaseID string `json:"case_id"`
		Annotation
	}
	if err = json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	index := map[string]int{}
	for i, c := range cases {
		index[c.ID] = i
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if seen[row.CaseID] {
			return nil, fmt.Errorf("duplicate human annotation")
		}
		seen[row.CaseID] = true
		i, ok := index[row.CaseID]
		if !ok {
			return nil, fmt.Errorf("unknown annotated case")
		}
		if !row.Approved {
			continue
		}
		a := row.Annotation
		cases[i].Human = &a
	}
	if err := validateCases(cases); err != nil {
		return nil, err
	}
	return cases, nil
}
