package model

import "testing"

func TestDepartmentCollectionExclusion(t *testing.T) {
	for _, tc := range []struct{ title, body, want string }{
		{"계약직 직원 채용 공고", "", "employment"},
		{"비전임교원 초빙 안내", "", "faculty_recruitment"},
		{"학부 안내", "연구원 공개채용 안내입니다.", "employment"},
		{"대학원 신입생 모집", "전형 일정 안내", ""},
		{"해외 연수 참가자 모집", "지원 방법 안내", ""},
		{"행사 참가자 모집", "창작 워크숍입니다.", ""},
		{"채용설명회 참가자 모집", "기업 채용설명회를 개최합니다.", ""},
		{"현장실습 모집", "채용 연계형 현장실습 프로그램입니다.", ""},
		{"교직과정 신청", "교원 자격증 취득 절차", ""},
	} {
		t.Run(tc.title, func(t *testing.T) {
			v := Content{Title: tc.title, Content: tc.body}
			r := CollectionExclusion(v)
			if tc.want == "" {
				if r != nil {
					t.Fatal(r)
				}
			} else if r == nil || r.Code != tc.want {
				t.Fatal(r)
			}
			if IsExcludedNotice(v) != (tc.want != "") {
				t.Fatal("shared exclusion disagrees")
			}
		})
	}
}
func TestAuthorAndAttachmentMetadataRevision(t *testing.T) {
	v := Content{Title: "title", Content: "body", Author: "작성자"}
	before := ContentDigest(v)
	v.Author = "다른 작성자"
	if before == ContentDigest(v) {
		t.Fatal("author changes missed")
	}
}
