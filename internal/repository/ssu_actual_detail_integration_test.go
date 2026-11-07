package repository

import (
	"context"
	"reflect"
	"testing"

	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/testutil/ssudetail"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestSSUActualDetailMongo(t *testing.T) {
	item := ssudetail.Load(t, "../crawler/testdata/ssu_catch")
	store := testStore(t)
	item.SourceID = bson.NewObjectID()
	ctx := context.Background()
	if err := store.Save(ctx, []model.Content{item}); err != nil {
		t.Fatal(err)
	}
	read := func() model.Content {
		t.Helper()
		items, err := store.Contents(ctx, model.Filter{SourceName: "ssu_catch", Category: item.Category, Page: 1, Limit: 20})
		if err != nil || len(items) != 1 {
			t.Fatalf("items=%d error=%v", len(items), err)
		}
		return items[0]
	}
	first := read()
	if first.Title != item.Title || first.Department != item.Department || first.Content != item.Content || first.ContentHash != item.ContentHash || !reflect.DeepEqual(first.Attachments, item.Attachments) {
		t.Fatal("stored fixture fields differ")
	}
	if err := store.Save(ctx, []model.Content{item}); err != nil {
		t.Fatal(err)
	}
	again := read()
	if first.ID != again.ID || !first.UpdatedAt.Equal(again.UpdatedAt) {
		t.Fatal("unchanged fixture changed identity/revision")
	}
}
