# 일정 분석 결과의 수동 저장과 읽기 전용 Preview

기존 크롤러/Runner/Source 전략/분류 이력은 변경하지 않는다. Gemini, 학교 HTTP, OCR 또는 첨부 다운로드를 호출하지 않는다. 이 기능은 기존 10건/24개 일정과 규칙 세션 4개로 검증한 수동 경로이며 자동 작업에 등록하지 않는다.

## 저장 필드

crawled_contents에는 summary, target_audience, schedules, schedule_extraction_version, schedule_extraction_method, schedule_review_required, schedule_analyzed_at, schedule_input_hash, schedule_model_version을 $set한다. 대상/요약 검토 상태 및 불확실성 배열, schedule_analysis_id/result_hash도 저장한다. schedules는 v2 24개와 rule 세션 4개를 합친 28개다. origin 및 parent_id로 원래 일정/규칙 보완을 구분한다. timezone/precision/부분 날짜/시각 null을 유지한다. 24개가 28개로 중복 증식한 것이 아니다.

schedule_analyzed_at은 기존 v2 재검증 시각이며 UTC BSON Date(밀리초)다. 날짜 전용 값은 YYYY-MM-DD 문자열, 시각은 명시된 HH:mm, timezone은 Asia/Seoul로 보존한다. 임의의 자정/일말 시각을 만들지 않는다. 검토 필요 일정 12개와 대상 검토 5개를 유지하며, 공고 단위 schedule_review_required는 일정·대상·요약 검토 중 하나라도 필요한 경우 true(이번 8건)다.

Content의 신규 필드는 optional JSON/BSON이다. 분석 없는 기존 공고 응답에는 포함되지 않는다. 기존 API 인증·카테고리·기간·만료 필터는 유지한다. Raw AI 응답과 전체 분석 이력은 content REST 응답에 넣지 않는다. 기존 ContentRepository.Save의 수집 경로를 일정 분석 적용에 사용하지 않는다.

## 입력 준비 (완전 오프라인)

```sh
go run ./cmd/prepare-schedule-apply \
  --revalidated /absolute/path/results-v2.json \
  --source-snapshot /absolute/path/candidates.json \
  --raw-results /absolute/path/results.json \
  --provenance /absolute/path/provenance.json \
  --output /absolute/path/apply-input.json
```

원본 10건을 기존 실제 스냅샷에서 찾아 content_hash와 입력 해시로 결합한다. raw_response를 재파싱하고 원문 근거의 결정적 재검증을 수행하여 저장된 v2 데이터와 비교한다. 데이터가 다르거나 10건/24+4개 범위가 다르면 실패한다. 출력은 새 파일만 허용하며 권한은 0600이다. 현재 실험의 검증된 버전은 schedule-v2 / gemini-3.5-flash-lite다. 버전 변경에는 별도 재검증이 필요하다.

## 기본 Preview (읽기 전용)

```sh
go run ./cmd/apply-schedules \
  --input /absolute/path/apply-input.json \
  --output /absolute/path/preview.json
```

설정 우선순위는 기존 방식 그대로 CLI --uri/--database > 프로세스 환경변수 > .env > 기본값이다. .env와 기존 자격 증명을 수정하지 않는다. 연결 URI/비밀번호/서비스 계정/raw_response를 로그에 출력하지 않는다. 원응답 포함 파일은 접근을 제한하고 외부 공개하지 않는다.

Preview는 OpenReadOnly를 사용하며 기존 Store.Open의 인덱스 생성도 호출하지 않는다. 원본 전체 BSON 체크섬·문서 수·schedule_analyses 및 content_classifications 개수를 전후 비교한다. eligible, duplicate, source_conflict, content_hash_conflict, input_hash_conflict, analysis_version_conflict, analysis_state_conflict, not_found를 구분한다. Preview가 eligible이어도 검토 필요 표시를 해제하지 않는다.

## 수동 적용 경계

실제 쓰기는 위 명령에 명시적 --apply를 붙인 경우에만 가능하다. **이번 검증에서는 CLI --apply를 실행하지 않았다.** 수동 반영 전에 preview.json과 검토 대상을 검토해야 한다.

CAS 필터는 _id/source_name/source_id/content_hash 및 schedule_input_hash의 absent/null/value 상태를 확인하고, 읽었던 문서 전체와 $$ROOT의 BSON equality도 비교한다. 따라서 schedules·분석 버전·분석 시각·입력 본문·last_seen_at 중 무엇이든 Preview 이후 바뀌면 쓰지 않는다. 본문이 바뀌었는데 수집 content_hash가 갱신되지 않은 경우도 별도 schedule_input_hash 재계산으로 차단한다. InputHash는 ID·출처·URL·제목·본문·게시일·원본 상태를 포함하며 분석/분류/만료 필드는 포함하지 않는다.

새로운 분석 시각이나 상위 extraction version은 오래된 결과로 덮어쓰지 않는다. 같은 시각의 다른 결과, 알 수 없는 기존 버전, 출처 또는 기존 입력 상태 충돌도 차단한다. 본문 해시가 같다는 이유만으로 수동 편집된 schedules를 덮어쓰지 않는다. 동일한 검증 결과/분석 ID는 duplicate이고, $push 대신 전체 검증 배열을 $set하므로 재적용해도 늘어나지 않는다.

수정 허용 필드 목록은 scheduleSet에 명시한다. ReplaceOne, upsert, 원본 content/title/hash/category/deadline/lifecycle 변경은 없다. 사전 Preview에 충돌이 있으면 배치 전체를 쓰기 전에 중단한다. 실행 중 CAS 충돌/쓰기 오류는 후속 처리를 중지하고 부분 결과를 보고한다. 여러 공고 전체를 원자적 트랜잭션으로 묶지는 않는다. 일부가 먼저 적용됐다면 그것을 되돌리거나 삭제하지 않으며, 재실행은 문서별 duplicate/CAS로 안전하게 처리한다.

## schedule_analyses

content_classifications와 별도다. content_id/source_id/source_name/input_hash/content_hash/model_version/extraction_version/raw_response/validated_output/validation_issues/token_usage/analyzed_at을 저장한다. 대상 자격·우대 원문 근거도 audience_validation으로 보존한다. 신규 이력의 _id는 공고·입력·버전·원응답·검증 결과의 결정적 해시이며 MongoDB 기본 _id 고유성으로 중복을 막는다. Preview 중 컬렉션이나 추가 인덱스를 생성하지 않는다.

이력은 **분석 결과**이며 **공고 반영 성공 로그가 아니다**. 불변 이력을 InsertOne으로 먼저 보존한 다음 공고 projection을 CAS UpdateOne한다. 따라서 CAS 실패/중단 시 미반영 이력이 남을 수 있지만, 반영된 공고의 원응답 이력이 유실되지는 않는다. 공고의 schedule_analysis_id로 실제 반영된 분석을 식별한다. 동일 _id의 이력이 다른 내용이면 덮어쓰지 않고 실패한다. 트랜잭션이 없는 standalone 임시 MongoDB에서도 같은 안전 규칙을 검증한다.

## 격리 테스트

기존 integration 패키지도 TestMain에서 DB 연결 전에 안전성 검사를 수행한다. TEST_MONGODB_URI는 loopback mongodb URI만 허용하며 SRV/Atlas, 비로컬 호스트, 자격 증명, URI에 내장된 DB, 생산 DB 이름을 거부한다. 새로운 ssu_schedule_apply_test_<ObjectID> 이름을 생성하고 빈 DB임을 확인한 뒤 ownership marker를 기록한다. 정리 시 동일 marker와 URI/이름을 확인한 DB만 Drop한다. 기존 로컬 ssu_recent DB나 Atlas를 정리하지 않는다.

실제 10건 저장·재조회·24+4 일정·부분 날짜 7개·대상 검토 5건·원본 BSON 필드 보존, 중복 10건, 8개 입력/버전/상태 변경 차단, 구버전에서 안전한 업그레이드, 불변 이력 충돌, 동시 8개 요청 중 1회 반영, API 인증 및 기존 응답을 테스트한다. 일반 테스트와 race 테스트에 포함된다. TEST_MONGODB_URI가 없으면 DB 통합 테스트만 건너뛴다. 현재 작업에서는 실제 loopback MongoDB로 실행했다.
