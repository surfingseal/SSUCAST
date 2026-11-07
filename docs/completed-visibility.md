# 슈캐치 원본 status=완료 목록 노출 정책

기본 공고 목록에서 슈캐치 Source에 속한 `status=완료` 문서만 제외한다. 진행/unknown은 유지한다. 다른 Source의 완료 문서는 이 정책으로 숨기지 않는다. `status`, 카테고리, lifecycle_status, deadline, content_hash와 원본 문서는 변경하지 않는다.

## 조회 옵션

```text
GET /api/v1/contents
GET /api/v1/contents?include_completed=true
GET /api/v1/contents?source=ssu_catch&category=scholarship&include_completed=true
GET /api/v1/contents?include_completed=true&include_expired=true
```

`include_completed`는 true/false만 받으며 반복/잘못된 값은 400이다. 기존 Bearer 인증은 유지한다. Job 조회에는 이 옵션을 허용하지 않는다.

학교의 완료 상태와 확인된 만료 상태는 별개다. `include_completed=true`는 완료 숨김만 해제하며 기존 lifecycle/deadline 만료 필터는 해제하지 않는다. `include_expired=true`만으로는 완료 숨김을 해제하지 않는다. 둘 다 포함하려면 두 옵션을 모두 지정한다. `lifecycle_status=expired`만 지정해도 슈캐치 완료 숨김은 그대로이며 완료도 보려면 include_completed=true를 추가한다.

이번 요구는 **목록 정책**이다. 단건 URL과 분류 이력은 기존 규칙을 유지하고, 완료 공고라는 이유만으로 직접 접근을 막지 않는다. 기존 lifecycle 단건 만료 제한과 include_expired 옵션은 유지한다.

unknown은 원본 unknown을 그대로 보존한다. unknown이라는 이유만으로 만료·분류 실패 상태를 새로 저장하지 않는다. 기존 lifecycle의 미평가 needs_review 표시와 학교 원본 status를 혼동하지 않는다.

## Source 범위와 페이지네이션

`crawl_sources`의 실제 Source ID를 기준으로 적용한다. 명시적인 crawler_type=ssu_catch와 crawler_type이 없는 레거시 name=ssu_catch를 인식한다. 명시적인 다른 crawler_type은 이름보다 우선한다. 콘텐츠의 source_name/제목/URL 문자열만으로 판정하지 않는다. Source를 확인할 수 없는 문서는 임의로 숨기지 않는다.

MongoDB의 `$nor` 조건에 완료 제외를 추가하여 기존 카테고리 `$or`, lifecycle 및 날짜 필터를 보존한다. skip/limit 전에 필터하므로 숨겨진 문서 때문에 페이지가 비거나 누락되지 않는다. 내부 스냅샷 이관·보존 감사·분류 재조정은 IncludeCompleted=true를 명시해 전체 데이터 접근을 유지한다.

## 읽기 전용 Preview

원본 프로젝트 .env의 연결을 사용하며 URI/비밀번호는 출력하지 않는다.

```sh
go run ./cmd/preview-completed --output /absolute/completed-preview.json
```

실제 Source·공고를 읽고 숨길 공고의 ID, 제목, 카테고리와 원본 status를 기록한다. 실제 Atlas Reader에 연결한 기존 REST Handler를 인프로세스로 호출해 기본/완료 포함/만료 조합과 8개 카테고리를 검증한다. 인증 없는 요청의 401도 검사한다. 웹 서버를 시작하거나 재시작하지 않는다.

읽기 전용 Store 생성은 인덱스/컬렉션을 생성하지 않는다. 공고 전체 BSON의 전후 SHA256을 비교하고 다르면 성공으로 보고하지 않는다. 정리 dry-run도 읽기만 수행하여 완료 상태 자체가 삭제 사유로 추가되지 않았음을 확인한다.

보고서의 hidden_items가 원본 숨김 대상 목록이며 categories는 필터 후 실제 API 건수다. 여러 공식 카테고리를 갖는 문서가 있다면 카테고리별 합계와 전체 문서 수는 다를 수 있다.

백업·확정 만료 CAS 삭제·dry-run 및 TTL 미사용 정책은 docs/lifecycle.md대로 유지한다. 이번 정책은 소프트 노출 필터이며 데이터를 수정/삭제하거나 자동 정리하지 않는다.
