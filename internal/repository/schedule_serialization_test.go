package repository

import (
	"example.com/crawlserver/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
)

// Pure serialization/pipeline test. No client, server or database is opened.
func TestScheduleDatePrecisionInExistingContentPipeline(t *testing.T) {
	date := "2026-11-23"
	c := model.Content{ContentHash: "original-hash", Schedules: []model.Schedule{{EndDate: &date, Timezone: "Asia/Seoul", Precision: "date", EndPrecision: "date", ReviewReasons: []string{}}}}
	pipeline, e := contentUpdate(c)
	if e != nil {
		t.Fatal(e)
	}
	set := pipeline[0][0].Value.(bson.M)
	conditional, ok := set["schedules"].(bson.M)
	if !ok {
		t.Fatal("schedules were dropped by BSON/content pipeline")
	}
	branches := conditional["$cond"].(bson.A)
	literal := branches[1].(bson.M)["$literal"]
	raw, e := bson.Marshal(bson.M{"rows": literal})
	if e != nil {
		t.Fatal(e)
	}
	var stored struct {
		Rows []model.Schedule `bson:"rows"`
	}
	if e = bson.Unmarshal(raw, &stored); e != nil {
		t.Fatal(e)
	}
	if len(stored.Rows) != 1 || stored.Rows[0].EndDate == nil || *stored.Rows[0].EndDate != date || stored.Rows[0].EndTime != nil || stored.Rows[0].EndAt != nil || stored.Rows[0].Timezone != "Asia/Seoul" || stored.Rows[0].Precision != "date" {
		t.Fatal(stored.Rows)
	}
}
