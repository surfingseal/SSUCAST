package imageanalysis

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

func TestTallPosterPiecesAndHashProvenance(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 300, 2100))
	for y := 0; y < 2100; y++ {
		for x := 0; x < 300; x++ {
			img.Set(x, y, color.RGBA{uint8(y % 255), 255, 255, 255})
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	out, e := Assets(b.Bytes(), ImageInput{URL: "https://scatch.ssu.ac.kr/wp-content/uploads/synthetic.png"}, t.TempDir(), 3)
	if e != nil || len(out) != 3 {
		t.Fatal(len(out), e)
	}
	for i, v := range out {
		raw, _ := os.ReadFile(v.File)
		if v.OriginalSHA256 != digest(b.Bytes()) || v.SHA256 != digest(raw) || v.Crop == nil || v.Width != 300 {
			t.Fatal("source/piece provenance lost")
		}
		if i > 0 && v.Crop.Y >= out[i-1].Crop.Y+out[i-1].Crop.Height {
			t.Fatal("no overlap")
		}
	}
	if _, e = Assets(make([]byte, 10<<20+1), ImageInput{}, t.TempDir(), 3); e == nil {
		t.Fatal("byte cap")
	}
}
func TestCompactSchemaAndImmutablePrepared(t *testing.T) {
	r := Synthetic()
	o, _ := Decode(r.Raw)
	raw, _ := json.Marshal(compactOutput{Candidates: o.Candidates, Review: true, Reasons: []string{}})
	r.Format = CompactVersion
	r.Raw = string(raw)
	r.InputHash = InputHashFor(Candidate{Original: r.Original, Images: r.Images}, r.Format)
	c := CompactConfig()
	if c.MaxOutputTokens != 16384 || c.ResponseSchema.Properties["image_transcriptions"] != nil {
		t.Fatal("full transcription retained")
	}
	p, e := Prepare(r)
	if e != nil || ValidatePrepared(p) != nil || len(p.Merge.Schedules) != 1 {
		t.Fatal(e)
	}
	if !p.Merge.Schedules[0].ReviewRequired {
		t.Fatal("unverified quote promoted")
	}
	if _, e = decodeCompact(`{"image_schedule_candidates":[],"review_reasons":[]}`); e == nil {
		t.Fatal("missing required bool accepted")
	}
}
func TestVisualReviewSeparateQuotesAndRawPreserved(t *testing.T) {
	r := Synthetic()
	o, _ := Decode(r.Raw)
	review := VisualReview{RawHash: digest([]byte(r.Raw)), Method: "agent_visual_source_check", Candidates: []ReviewedCandidate{{OriginalIndex: 0, Value: o.Candidates[0], Quotes: []string{"행사일시: 2026년 11월 20일", "14:00~16:00"}, Reason: "synthetic separate image regions"}}}
	next := ReviewedResult(r, r.Original, review, r.At)
	p, e := Prepare(next)
	if e != nil || ValidatePrepared(p) != nil {
		t.Fatal(e)
	}
	if p.Analysis.RawResponse != r.Raw || len(p.Merge.Schedules[0].ImageEvidence) != 2 || !p.Merge.Schedules[0].ReviewRequired {
		t.Fatal("source quotes/history/review lost")
	}
	p.VisualReview.Method = "unverified"
	if ValidatePrepared(p) == nil {
		t.Fatal("tampered review accepted")
	}
}
