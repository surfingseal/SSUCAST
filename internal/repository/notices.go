package repository

import (
	"context"
	"example.com/crawlserver/internal/model"
	"example.com/crawlserver/internal/notices"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"regexp"
	"time"
)

func noticeProjection(detail bool) bson.M {
	p := bson.M{}
	for _, k := range []string{"title", "summary", "source_name", "category", "categories", "classification_method", "published_at", "original_url", "status", "target_audience", "target_audience_review_required", "summary_review_required", "schedule_analysis_id", "schedule_input_hash", "schedule_model_version", "schedule_extraction_version", "schedule_review_required", "schedules", "content_hash", "notice_policy"} {
		p[k] = 1
	}
	if detail {
		p["content"] = 1
	}
	return p
}
func (s *ContentRepository) noticeMatch(ctx context.Context, f notices.Filter) (bson.M, error) {
	base, e := s.ListVisibilityQuery(ctx, model.Filter{SourceName: f.Source, Category: f.Category, IncludeCompleted: f.Status == "완료", IncludeExpired: f.IncludeExpired})
	if e != nil {
		return nil, e
	}
	and := bson.A{base}
	if f.Status != "" {
		and = append(and, bson.M{"status": f.Status})
	}
	if f.Query != "" {
		r := bson.Regex{Pattern: regexp.QuoteMeta(f.Query), Options: "i"}
		and = append(and, bson.M{"$or": bson.A{bson.M{"title": r}, bson.M{"summary": r}, bson.M{"content": r}}})
	}
	if f.HasSchedule != nil {
		if *f.HasSchedule {
			and = append(and, bson.M{"schedules.0": bson.M{"$exists": true}})
		} else {
			and = append(and, bson.M{"schedules.0": bson.M{"$exists": false}})
		}
	}
	return bson.M{"$and": and}, nil
}
func op(k string, v any) bson.M { return bson.M{k: v} }
func eq(a, b any) bson.M        { return op("$eq", bson.A{a, b}) }
func all(v ...any) bson.M       { return op("$and", v) }
func anyOf(v ...any) bson.M     { return op("$or", v) }
func dateKey(date, clock string) any {
	return op("$cond", bson.A{op("$regexMatch", bson.M{"input": op("$ifNull", bson.A{date, ""}), "regex": `^\d{4}-\d{2}-\d{2}$`}), op("$concat", bson.A{date, " ", op("$ifNull", bson.A{clock, "24:00"}), ":00"}), nil})
}

// Read-only aggregation: match before schedule computation, database-side sort,
// facet pagination, and a positive projection. No stored dates or indexes change.
func noticesPipeline(match bson.M, f notices.Filter, now time.Time) mongo.Pipeline {
	nowKey := now.In(notices.Seoul).Format("2006-01-02 15:04:05")
	trusted := all(eq(op("$ifNull", bson.A{"$$s.review_required", true}), false), op("$in", bson.A{"$$s.quality.state", bson.A{"verified", "partial"}}), eq("$$s.timezone", "Asia/Seoul"))
	dates := func(kind string) any {
		key := dateKey("$$s.end_date", "$$s.end_time")
		if kind == "event" {
			key = op("$let", bson.M{"vars": bson.M{"start": dateKey("$$s.start_date", "$$s.start_time"), "end": dateKey("$$s.end_date", "$$s.end_time")}, "in": op("$cond", bson.A{all(op("$ne", bson.A{"$$start", nil}), op("$gte", bson.A{"$$start", nowKey})), "$$start", "$$end"})})
		}
		types := bson.A{"application", "submission", "completion"}
		if kind == "event" {
			types = bson.A{"event", "interview", "announcement", "selection"}
		}
		if kind == "all" {
			types = bson.A{"application", "submission", "completion", "event", "interview", "announcement", "selection"}
		}
		mapped := op("$map", bson.M{"input": op("$ifNull", bson.A{"$schedules", bson.A{}}), "as": "s", "in": op("$cond", bson.A{all(trusted, op("$in", bson.A{"$$s.type", types})), key, nil})})
		if kind == "all" { // point events with only a start date also qualify
			mapped = op("$map", bson.M{"input": op("$ifNull", bson.A{"$schedules", bson.A{}}), "as": "s", "in": op("$cond", bson.A{trusted, op("$ifNull", bson.A{dateKey("$$s.end_date", "$$s.end_time"), dateKey("$$s.start_date", "$$s.start_time")}), nil})})
		}
		return op("$min", op("$filter", bson.M{"input": mapped, "as": "d", "cond": all(op("$ne", bson.A{"$$d", nil}), op("$gte", bson.A{"$$d", nowKey}))}))
	}
	p := mongo.Pipeline{bson.D{{Key: "$match", Value: match}}, bson.D{{Key: "$set", Value: bson.M{"schedules": bson.M{"$cond": bson.A{noticePolicyValid(), "$notice_policy.schedules", "$schedules"}}}}}}
	if f.Sort != "latest" || f.Upcoming != nil {
		p = append(p, bson.D{{Key: "$set", Value: bson.M{"__deadline": dates("deadline"), "__event": dates("event"), "__upcoming": dates("all")}}})
	}
	if f.Upcoming != nil {
		condition := bson.M{"$ne": nil}
		if !*f.Upcoming {
			condition = bson.M{"$eq": nil}
		}
		p = append(p, bson.D{{Key: "$match", Value: bson.M{"__upcoming": condition}}})
	}
	sort := bson.D{{Key: "published_at", Value: -1}, {Key: "_id", Value: -1}}
	if f.Sort != "latest" {
		key := "__deadline"
		if f.Sort == "event" {
			key = "__event"
		}
		p = append(p, bson.D{{Key: "$set", Value: bson.M{"__missing": op("$cond", bson.A{eq("$"+key, nil), 1, 0})}}})
		sort = bson.D{{Key: "__missing", Value: 1}, {Key: key, Value: 1}, {Key: "published_at", Value: -1}, {Key: "_id", Value: -1}}
	}
	p = append(p, bson.D{{Key: "$facet", Value: bson.M{"items": mongo.Pipeline{bson.D{{Key: "$sort", Value: sort}}, bson.D{{Key: "$skip", Value: (f.Page - 1) * f.Limit}}, bson.D{{Key: "$limit", Value: f.Limit}}, bson.D{{Key: "$project", Value: noticeProjection(false)}}}, "count": mongo.Pipeline{bson.D{{Key: "$count", Value: "n"}}}}}})
	return p
}
func (s *ContentRepository) Notices(ctx context.Context, f notices.Filter, now time.Time) (notices.Page, error) {
	result := notices.Page{Items: []notices.Notice{}, Page: f.Page, Limit: f.Limit, AsOf: now}
	q, e := s.noticeMatch(ctx, f)
	if e != nil {
		return result, e
	}
	cur, e := s.contents.Aggregate(ctx, noticesPipeline(q, f, now))
	if e != nil {
		return result, e
	}
	defer cur.Close(ctx)
	var rows []struct {
		Items []model.Content `bson:"items"`
		Count []struct {
			N int64 `bson:"n"`
		} `bson:"count"`
	}
	if e = cur.All(ctx, &rows); e != nil {
		return result, e
	}
	if len(rows) == 1 {
		if len(rows[0].Count) > 0 {
			result.Total = rows[0].Count[0].N
		}
		for _, v := range rows[0].Items {
			result.Items = append(result.Items, notices.View(v, false, now))
		}
	}
	result.Pages = int((result.Total + int64(f.Limit) - 1) / int64(f.Limit))
	return result, nil
}
func (s *ContentRepository) Notice(ctx context.Context, id bson.ObjectID, now time.Time) (notices.Notice, error) {
	q := query(model.Filter{IncludeCompleted: true, IncludeExpired: true}, "crawled_at")
	q["_id"] = id
	var v model.Content
	e := s.contents.FindOne(ctx, q, options.FindOne().SetProjection(noticeProjection(true))).Decode(&v)
	return notices.View(v, true, now), e
}
func (s *ContentRepository) NoticeMetadata(ctx context.Context) (notices.Metadata, error) {
	result := notices.Metadata{Sources: []notices.Source{}}
	q, e := s.ListVisibilityQuery(ctx, model.Filter{})
	if e != nil {
		return result, e
	}
	cur, e := s.contents.Aggregate(ctx, mongo.Pipeline{bson.D{{Key: "$match", Value: q}}, bson.D{{Key: "$group", Value: bson.M{"_id": "$source_name", "n": bson.M{"$sum": 1}}}}})
	if e != nil {
		return result, e
	}
	defer cur.Close(ctx)
	var counts []struct {
		Name string `bson:"_id"`
		N    int64  `bson:"n"`
	}
	if e = cur.All(ctx, &counts); e != nil {
		return result, e
	}
	by := map[string]int64{}
	for _, v := range counts {
		by[v.Name] = v.N
		result.VisibleCount += v.N
	}
	cur, e = s.contents.Database().Collection("crawl_sources").Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"name": 1, "display_name": 1}).SetSort(bson.D{{Key: "name", Value: 1}}))
	if e != nil {
		return result, e
	}
	defer cur.Close(ctx)
	var sources []model.Source
	if e = cur.All(ctx, &sources); e != nil {
		return result, e
	}
	for _, v := range sources {
		label := v.DisplayName
		if label == "" {
			label = v.Name
		}
		if v.Name == "ssu_catch" {
			label = "슈캐치"
		}
		result.Sources = append(result.Sources, notices.Source{ID: v.ID.Hex(), Name: v.Name, Label: label, Count: by[v.Name]})
	}
	result.StoredCount, e = s.contents.CountDocuments(ctx, bson.M{})
	if e != nil {
		return result, e
	}
	cur, e = s.contents.Aggregate(ctx, mongo.Pipeline{bson.D{{Key: "$group", Value: bson.M{"_id": nil, "at": bson.M{"$max": bson.M{"$max": bson.A{"$schedule_analyzed_at", "$crawled_at"}}}}}}})
	if e != nil {
		return result, e
	}
	defer cur.Close(ctx)
	var ts []struct {
		At *time.Time `bson:"at"`
	}
	e = cur.All(ctx, &ts)
	if len(ts) > 0 {
		result.UpdatedAt = ts[0].At
	}
	return result, e
}
