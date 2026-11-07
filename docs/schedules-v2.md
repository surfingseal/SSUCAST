# 일정 모델 v2와 오프라인 재검증

`cmd/revalidate-schedules`는 저장된 원문/응답 파일만 읽는다. `.env`, Google 인증, SDK/HTTP 클라이언트, MongoDB Repository, 스케줄러를 사용하지 않는다. 기존 raw_response/validated_output/comparison/report 파일은 변경하지 않는다.

```sh
go run ./cmd/revalidate-schedules \
  -input-dir /absolute/path/to/gemini35-flash-lite-10 \
  -output-dir /absolute/path/to/schedules-v2
```

입력 파일 4개가 모두 필요하다: selected.json, results.json, comparison.json, report.md. 누락 시 파일명을 보고하고 종료한다. ID·24개 일정 정렬을 확인하며 결과와 diff, 입력 체크섬을 새 디렉터리에 기록한다. 원본 10건의 요약은 수정하지 않는다. 기존 10건/24개 실험을 재현하는 명령이며 다른 데이터 범위를 암묵적으로 처리하지 않는다.

## 날짜 저장

`model.Schedule`의 date는 YYYY-MM-DD, time은 명시된 HH:mm 문자열이다. timezone은 Asia/Seoul이다. JSON과 BSON에서 같은 의미를 보존하며 날짜만 있는 값을 자정 BSON Date로 변환하지 않는다. 완전한 날짜와 시각이 모두 검증된 경우만 호환 start_at/end_at을 만든다.

- precision=date: 완전한 날짜, 시각 없음. 정상적인 추출이며 missing_time은 omissions에 기록하고 그것만으로 검토하지 않는다.
- precision=minute: 완전한 날짜와 명시 시각.
- precision=month/year: start_partial/end_partial에 확인된 year/month만 저장. 미확인 day는 null.
- precision=month_day: 월·일은 알지만 연도가 불명확함. full date는 null, partial.year=null. missing_year로 검토한다.
- 경계 정밀도가 다르면 precision=mixed이고 start_precision/end_precision을 별도로 보존한다.
- 연도는 같은 인용문의 명시된 기간에서만 공유한다. 학년도 제목·게시일·현재 연도로 채우지 않는다. 연말 범위를 넘고 종료 연도가 생략된 경우도 임의로 다음 해를 채우지 않는다.

예: 실제 김장봉사 모집은 end_date=2026-11-23, end_time=null, precision=date, review_required=false이며 기존 임의 23:59:59는 corrections에 남긴다. 실제 발표가 2026년 12월뿐이라면 start_partial={year:2026,month:12,day:null}로 보존한다.

## 원문과 추출 출처

MatchEvidence는 Unicode 공백의 축약/줄바꿈/앞뒤 공백만 정규화한다. 날짜, 시각, 기관명, 금액, 자격 조건, 구두점은 변경하지 않는다. UTF-8 byte와 Unicode rune offset을 title/content 각 원문에 대해 0 기반·끝 제외로 반환한다. 임의의 공백 삭제로 A B와 AB를 같게 만들지 않는다.

기존 AI는 일정의 범위/유형/라벨을 제안했다. v2의 달력값을 원문 근거에서 다시 구성하는 과정은 rule이며 origin=saved_ai_response로 기록한다. 기존 AI 응답에 없는 세션은 원문 목록에서 별도로 추출해 rule_supplements에 담고 origin=source_rule_supplement, extraction_method=rule, parent_id로 전체 행사와 연결한다. 기존 24개 일정 개수에 합산하지 않는다. parent의 날짜를 상속하는 자식은 해당 parent의 검증된 날짜 근거 및 실제 인접한 번호 목록이 필요하다.

Fingerprint는 type, scope, parent, 날짜/부분 날짜/시각, timezone을 포함한다. 서로 다른 기관·차수·부모 일정을 같은 시간 범위라는 이유로 합치지 않는다. 실제 동일 scope/부모/범위 중복과 잘못된 parent/ID를 검출한다.

## 검토·대상·요약

review_reasons는 missing_year, tentative_schedule, multiple_scopes, conditional_deadline, evidence_mismatch, source_conflict, unsupported_datetime 등을 배열로 보존한다. unknown legacy review 사유는 legacy_review_unresolved로 남긴다. 이미 제거한 임의 시각은 corrections에 기록하며 정밀한 날짜 자체를 무효화하지 않는다. missing_time처럼 명시적으로 없는 정보는 omissions로 분리한다.

대상 검증은 명시적 대상/자격 구간, 필수 조건, 제외 조건, 우대/가산점을 원문 인용으로 분리한다. 재단 소개에 등장한 일반 명사나 여러 조건을 축약한 대상은 실제 자격으로 확정하지 않는다. 불명확한 target_audience는 null이며 raw_target_audience는 유지한다. summary는 원문 응답 그대로이며 길이/숫자 검사를 별도로 수행한다. 문자열 검사로 자격·혜택의 모든 의미를 입증하지는 못한다.

FromLegacy는 저장된 v1 응답을 변환한다. FromV2/ValidateSchedule는 향후 새 필드로 주어진 응답도 같은 달력·원문·timezone/호환 timestamp 검증에 적용한다. 이번 단계에서는 모델에 새 요청을 보내거나 SDK 설정을 변경하지 않는다.

## 기존 시스템과 저장 경계

Content.schedules는 optional JSON/BSON 필드다. 기존 공고는 이 필드 없이도 읽힌다. content_hash 계산, original_url/source_id/published_at/status/category/classification_status, deadline/lifecycle_status, content_classifications 및 API 인증/조회, Source 전략은 변경하지 않는다. schedules는 deadline이나 만료 상태로 자동 투영하지 않는다.

기존 ContentRepository의 contentUpdate는 본문 hash가 변경된 경우에만 내용 필드를 반영한다. 이번 작업은 same-hash 공고에 schedules를 쓰는 DB 마이그레이션/관리자 apply 경로를 추가하지 않는다. 향후 같은 본문에 일정 분석 결과를 저장할 때에는 입력 hash CAS와 별도 명시적 적용 절차를 설계해야 한다. BSON roundtrip과 기존 pipeline의 literal 보존을 네트워크 없는 단위 테스트로 검증한다. 실제 저장 성공을 주장하지 않는다.

테스트에는 합성 단위 사례와 `testdata/actual10.json`의 실제 공고/응답 회귀를 분리한다. 전체 테스트는 TEST_MONGODB_URI를 제거해 쓰기 통합 테스트를 건너뛴다. DB 쓰기 금지 조건이므로 로컬 MongoDB에도 테스트 데이터를 쓰지 않는다. 기본 테스트는 Mock HTTP 또는 순수 함수만 사용하며 학교/Vertex를 호출하지 않는다.
