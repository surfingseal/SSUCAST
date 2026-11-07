package integration_test

import (
	"context"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/repository"
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
)

func TestCompletedSourceIdentityLegacyAndExplicitOverride(t *testing.T) {
	for _, tc := range []struct {
		name, crawler string
		hidden        bool
	}{{"ssu_catch", "", true}, {"ssu_catch", "example", false}, {"ssu_alias", "ssu_catch", true}, {"department", "example", false}} {
		t.Run(tc.name+"/"+tc.crawler, func(t *testing.T) {
			s := strategyStore(t)
			ctx := context.Background()
			source := model.Source{ID: bson.NewObjectID(), Name: tc.name, CrawlerType: tc.crawler, BaseURL: "https://fixture.invalid", ClassificationStrategy: "source"}
			if e := s.SourceRepository.ApplySources(ctx, []model.Source{source}); e != nil {
				t.Fatal(e)
			}
			item := model.Content{SourceID: source.ID, Title: "학교 상태 완료", Content: "본문", OriginalURL: "https://fixture.invalid/item", OriginalCategory: "학사", Status: "완료"}
			if e := s.Save(ctx, []model.Content{item}); e != nil {
				t.Fatal(e)
			}
			ids, e := repository.SSUCatchSourceIDs(ctx, s.Client.Database(sourcesDatabaseName(t, s)))
			if e != nil {
				t.Fatal(e)
			}
			if (len(ids) == 1) != tc.hidden {
				t.Fatal("Source identity mismatch", ids, tc)
			}
			rows, e := s.Contents(ctx, model.Filter{Page: 1, Limit: 10})
			if e != nil {
				t.Fatal(e)
			}
			if (len(rows) == 0) != tc.hidden {
				t.Fatal("wrong Source policy", rows, tc)
			}
		})
	}
}
