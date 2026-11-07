# 학생용 읽기 전용 API

기본 로컬 주소: `http://127.0.0.1:8088` (실제 실행). `cmd/web`의 기본 WEB_ADDR은127.0.0.1:8080이다.

## 인증

기존 ADMIN_TOKEN을 `Authorization: Bearer …`로 전달하거나 브라우저 로그인 세션을 사용한다. 토큰을 URL에 넣지 않는다. 미인증401, 유효하지 않은 입력400, 없는 공고404, 쓰기 메서드405, 조회 장애503, 시간초과504. 오류 JSON은 `{ "error": "한국어 메시지" }`이며 내부 DB 정보는 포함하지 않는다.

브라우저 전용 `POST /api/session`에 `{ "token": "기존 토큰" }`을 보내면 HttpOnly/SameSiteStrict 쿠키를 발급한다. 동일 Origin이 필요하다. `GET /api/session`은 authenticated boolean, `DELETE /api/session`은 로그아웃이다. API 토큰을 HTML/JS/로컬스토리지에 주입하지 않는다.

## GET /api/notices

|매개변수|규칙|
|---|---|
|query|제목·요약·본문 한글 부분 문자열, 최대100자, 정규식 문법은 문자 그대로 취급|
|source|crawl_sources.name, 예 ssu_catch / ssu_global_media|
|category|기존 공식8개 enum 또는 unknown|
|status|진행 / unknown / 완료. 생략 시 기존 정책대로 슈캐치 완료 제외. 완료 지정 시 완료만 조회|
|has_schedule|true/false, 저장된 일정 존재 여부|
|upcoming|true/false, 원문 근거/품질이 확인된 앞으로의 일정 존재/부재|
|sort|latest(default), deadline, event|
|page|1(default)~10000|
|limit|12(default)~50, 최소1|

알 수 없는/중복 query 파라미터는400이다. source는 이름으로 조합 필터링하며, 존재하지 않는 이름은 빈 결과다. 정렬은 최신 게시일을 기본으로 하고 ID를 동률 기준으로 사용한다. deadline은 신청/application·제출/submission·이수/completion, event는 행사/event·면접/interview·발표/announcement·선발/selection의 확인된 미래 일정을 사용한다. 일정이 불확실하거나 지난 경우에도 공고는 결과 뒤에 남는다. upcoming=false는 '확인된 미래 일정이 없음'이며 '모든 실제 일정이 종료됨'을 의미하지 않는다.

응답: `{items:[Notice], total, page, limit, pages, as_of}`. pages는 결과0건이면0이다. 목록에는 원문 본문을 포함하지 않는다. 기존 lifecycle의 확인된 expired 제외 및 채용/교원채용 제외 정책을 유지한다.

## GET /api/notices/{id}

24자리 ObjectID. 상세보기는 기존 정책과 같이 완료/만료 공고의 직접 조회도 허용한다. Notice에는 다음이 포함된다.

- id/title/summary/content(상세만)/source/category/categories/category_method
- published_at/original_url/status/analyzed/review_required
- summary_review_required/target_audience/audience_review_required
- schedules: id/parent_id/type/label/scope/precision/timezone
- schedules.dates: start_date/end_date/start_time/end_time/start_precision/end_precision
- start_partial/end_partial: 원본의 부분 날짜
- start_label/end_label: 존재하는 값만 표시한 문자열
- evidence/evidence_spans/review_required/review_reasons/quality/state

analyzed는 분석 이력 ID·입력 해시·모델·추출 버전 존재 여부로 판정하며 summary 존재만으로 판단하지 않는다. 원본 category와 공고 status를 일정 상태로 덮어쓰지 않는다. raw_response, content_hash, credentials, 내부 Mongo 문서는 반환하지 않는다.

## GET /api/sources

`{sources:[{id,name,label,count}],updated_at,stored_count,visible_count}`.
count는 기본 완료/만료 노출 정책 기준이고 현재 필터의 결과 개수가 아니다. updated_at은 저장된 수집/분석 시각의 최댓값이다. Source 정책·연결 설정은 반환하지 않는다.

## GET /api/categories

`{items:[{id,label}]}`. academic/학사, scholarship/장학, international/국제교류, international_student/외국인 유학생, extracurricular/비교과·행사, teaching/교직, volunteer/봉사, other/기타. 기존 정의를 재사용한다.

기존 관리자 `/api/v1/*`는 `cmd/server`에서 그대로 유지한다. 학생용 `cmd/web`는 읽기 전용으로 별도 실행하며 관리자 수집 작업 API를 노출하지 않는다.
