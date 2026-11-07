package crawler

import (
	"fmt"
	"net/url"
	"time"
)

type SSUCategorySpec struct {
	Label string `json:"label"`
	Query string `json:"query"`
	Code  string `json:"code"`
}

// Query spelling is taken from actual list HTML links, not display labels.
var SSUOfficialCategories = []SSUCategorySpec{{"학사", "학사", "academic"}, {"장학", "장학", "scholarship"}, {"국제교류", "국제교류", "international"}, {"외국인 유학생", "외국인유학생", "international_student"}, {"비교과·행사", "비교과·행사", "extracurricular"}, {"교직", "교직", "teaching"}, {"봉사", "봉사", "volunteer"}, {"기타", "기타", "other"}}

func SSUCategoryURL(base string, spec SSUCategorySpec) (string, error) {
	u, e := url.Parse(base)
	if e != nil {
		return "", e
	}
	if u.Scheme != "https" && u.Scheme != "http" || u.Host == "" || u.User != nil {
		return "", fmt.Errorf("invalid category origin")
	}
	u.Path = ssuNoticePath
	u.RawPath = ""
	u.Fragment = ""
	u.RawQuery = url.Values{"f": {""}, "category": {spec.Query}, "keyword": {""}}.Encode()
	return u.String(), nil
}
func SSURecentRange(now time.Time) (time.Time, time.Time) {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.FixedZone("Asia/Seoul", 9*3600)
	}
	now = now.In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return day.AddDate(0, -3, 0), now
}
