package predictions

import (
	"example.com/crawlserver/internal/crawler"
	"example.com/crawlserver/internal/model"
	"fmt"
	"time"
)

type HistoryRun struct {
	ID        string                  `bson:"_id" json:"id"`
	From      string                  `bson:"from" json:"from"`
	To        string                  `bson:"to" json:"to"`
	Completed bool                    `bson:"completed" json:"completed"`
	Collected int                     `bson:"collected" json:"collected"`
	At        time.Time               `bson:"at" json:"at"`
	Report    crawler.SSURecentReport `bson:"report" json:"report"`
}
type HistoricalNotice struct {
	ID         string        `bson:"_id" json:"id"`
	RunID      string        `bson:"run_id" json:"run_id"`
	DataOrigin string        `bson:"data_origin" json:"data_origin"`
	Content    model.Content `bson:"content" json:"content"`
}

func HistoryRecord(run HistoryRun, v model.Content) (HistoricalNotice, error) {
	a, e := ParseDate(run.From)
	if e != nil {
		return HistoricalNotice{}, e
	}
	b, e := ParseDate(run.To)
	if e != nil {
		return HistoricalNotice{}, e
	}
	if run.ID == "" || v.PublishedAt == nil || v.PublishedAt.Before(a) || !v.PublishedAt.Before(b.AddDate(0, 0, 1)) || v.Metadata["data_origin"] != "http" || v.Title == "" || v.Content == "" || v.ContentHash == "" || v.ContentHash != model.ContentDigest(v) {
		return HistoricalNotice{}, fmt.Errorf("invalid historical evidence")
	}
	u, e := crawler.SSUCatchURL(v.OriginalURL, v.OriginalURL)
	if e != nil || u != v.OriginalURL {
		return HistoricalNotice{}, fmt.Errorf("invalid historical URL")
	}
	return HistoricalNotice{Hash([]string{run.ID, u}), run.ID, "historical_http", v}, nil
}
