package evaluation

import (
 "example.com/crawlserver/internal/testutil/localfixture"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func closeTo(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %.12f want %.12f", got, want)
	}
}
func TestMultiLabelMetricsHandCalculated(t *testing.T) {
	m := Score([]Pair{{[]string{"academic", "scholarship"}, []string{"academic", "career"}}, {[]string{"career"}, []string{"career", "career"}}}, []string{"academic", "scholarship", "career"})
	if m.Micro.TP != 2 || m.Micro.FP != 1 || m.Micro.FN != 1 {
		t.Fatal(m)
	}
	closeTo(t, m.Micro.Precision, 2.0/3)
	closeTo(t, m.Micro.Recall, 2.0/3)
	closeTo(t, m.Micro.F1, 2.0/3)
	closeTo(t, m.MacroF1, (1+0+2.0/3)/3)
	closeTo(t, m.ExactMatch, .5)
}
func TestUndefinedAndNoSamplesAreExplicit(t *testing.T) {
	if Score(nil, []string{"academic"}) != nil {
		t.Fatal("no samples scored")
	}
	m := Score([]Pair{{nil, nil}}, []string{"academic"})
	if m.Micro.F1 != 0 || m.ExactMatch != 1 || m.ZeroDivision == "" {
		t.Fatal(m)
	}
}
func TestHumanGoldAndMockAreSeparateAndFailuresCount(t *testing.T) {
	localfixture.Require(t,"testdata/notices.json")
 cases, err := Load("testdata/notices.json")
	if err != nil {
		t.Fatal(err)
	}
	mock, err := Evaluate(cases, "mock")
	if err != nil {
		t.Fatal(err)
	}
	if mock.Selected != 13 || mock.HumanMetrics["categories"] != nil || mock.ReferenceMetrics["categories"].Samples != 13 || mock.Usage["mock"].Failures != 1 {
		t.Fatal(mock.Selected, mock.Usage)
	}
	replay, err := Evaluate(cases, "replay")
	if err != nil {
		t.Fatal(err)
	}
	m := replay.HumanMetrics["categories"]
	if replay.Selected != 1 || m == nil || m.Samples != 1 || m.Micro.TP != 0 || m.Micro.FP != 1 || m.Micro.FN != 1 || m.Micro.F1 != 0 || replay.HumanMetrics["activity_types"] != nil || replay.NewVertexCalls != 0 {
		t.Fatal(replay)
	}
	if replay.Usage["vertex_replay"].InputTokens != 870 || replay.Usage["vertex_replay"].OutputTokens != 88 {
		t.Fatal(replay.Usage)
	}
	if _, err := Evaluate(cases, "vertex"); err == nil {
		t.Fatal("implicit paid mode accepted")
	}
}
func TestTamperedLabelsOrInputsAreRejected(t *testing.T) {
	localfixture.Require(t,"testdata/notices.json")
 cases, err := Load("testdata/notices.json")
	if err != nil {
		t.Fatal(err)
	}
	cases[0].Input.Title = "changed"
	p := filepath.Join(t.TempDir(), "cases.json")
	raw, _ := json.Marshal(cases)
	if err := os.WriteFile(p, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("mismatched input accepted")
	}
}

func TestPendingHumanTemplatesDoNotBecomeGold(t *testing.T) {
	localfixture.Require(t,"testdata/notices.json")
 cases, err := Load("testdata/notices.json")
	if err != nil {
		t.Fatal(err)
	}
	cases, err = ApplyAnnotations(cases, "testdata/human-annotations.template.json")
	if err != nil {
		t.Fatal(err)
	}
	report, err := Evaluate(cases, "mock")
	if err != nil {
		t.Fatal(err)
	}
	if report.HumanMetrics["categories"] != nil {
		t.Fatal("pending human labels scored as gold")
	}
}

func TestPublicSyntheticEvaluation(t *testing.T){cases,e:=Load("testdata/notices.synthetic.json");if e!=nil{t.Fatal(e)};r,e:=Evaluate(cases,"mock");if e!=nil||r.Selected!=13||r.NewVertexCalls!=0{t.Fatal(r,e)}}
