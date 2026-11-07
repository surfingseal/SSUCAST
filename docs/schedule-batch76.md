# 남은 76건의 수동 Vertex 일정·요약 분석

Atlas ssu_crawler의 실제 212건과 진행 50/unknown 36/완료 126건을 확인한다. 기존 10건은 기존 bundle과 비교하여 projection duplicate, schedule_analysis_id/input_hash/model/extraction version, 불변 schedule_analyses 내용까지 검증한다. summary 존재만으로 완료로 세지 않는다. 충돌·수 불일치가 있으면 유료 호출 전에 중단한다.

기존 10건 전용 Budget(10건/12회)와 legacy apply bundle(정확히 10건/24+4개)은 그대로 유지한다. 이번 명령의 별도 확정 ID 76개만 허용하고, 최대 실제 생성 요청 90회/공고당 기본 1회·429/5xx 최대 추가 2회를 적용한다. 모델은 gemini-3.5-flash-lite 고정, Vertex Backend/global/기존 ADC를 사용한다. .env·인증정보·일반 자동분류 설정은 변경하지 않는다.

```sh
# 읽기 전용 후보 확정; Vertex 호출 없음
go run ./cmd/analyze-schedule-batch --directory /absolute/experiment --existing /absolute/previous-ten/apply-input.json
# 명시적인 실제 유료 분석 (Mongo 쓰기 없음)
go run ./cmd/analyze-schedule-batch --directory /absolute/experiment --existing /absolute/previous-ten/apply-input.json --execute
# 저장된 응답의 재검증만; Gemini 호출 없음
go run ./cmd/analyze-schedule-batch --directory /absolute/experiment --existing /absolute/previous-ten/apply-input.json --aggregate-only
# 일반 배치의 읽기 전용 Preview
go run ./cmd/apply-schedules --input /absolute/experiment/apply-input.json --output /absolute/experiment/preview.json
```

## 실제 호출과 복구

SDK 재시도는 Attempts=1로 고정한다. 명시적인 429/5xx만 2초/4초 지수 백오프로 재시도한다. 전송 계층이 허용 ID·고정 모델·90회 상한을 재확인하며 전송 전에 attempts.json을 파일 동기화한다. 응답 본문은 SDK에 반환하기 전에 responses/에 저장한다. 200 응답은 유효하지 않은 JSON이라도 유료 성공을 다시 호출하지 않는다. 정상 체크포인트는 그대로 재사용하고, 중간 종료로 체크포인트가 없어도 저장된 200 HTTP 응답에서 복구한다. 전송 여부가 불명확하고 응답 파일도 없는 요청은 자동 재호출하지 않고 검토를 요구한다.

OS flock으로 동시 실행을 막되 프로세스 종료 후에는 잠금이 해제된다. generation-config.json의 모델/프롬프트/Schema 해시가 달라지면 유료 이어 실행을 중단한다. 인증·403·404·비정상 통신의 경우 임의 모델 대체 없이 중단한다. 에러 응답의 본문은 저장 시 비식별화하며 인증 헤더는 저장하지 않는다. 성공 원응답·사용량은 0600 파일에 남기고 로그에는 ID와 진행 수만 출력한다.

## v2 모델 재사용

새 저장 모델은 만들지 않는다. 모델의 wire DTO는 기존 날짜/시각/부분 날짜/정밀도/timezone/scope/evidence/review 배열을 전달하고, parent_index만 Go가 안정적인 parent_id로 바꾸는 어댑터다. ID·offset·compat timestamp·provenance는 Go가 배정한다.

schedules.DecodeV2 → RevalidateV2 → 기존 FromV2/ValidateSchedule/MatchEvidence/ValidateSet/ValidateAudience를 사용한다. 원문에 없는 달력값은 규칙으로 다시 검증하고 검증 가능한 부분만 보존한다. 그래도 검증 불가능한 일정 제안은 삭제하지 않고 모든 달력값이 null인 precision=unknown 검토 항목으로 남긴다. 이 경우 SafeReviewedPlaceholder가 날짜·시각이 전혀 materialize되지 않았음을 별도 확인한다. 확인되지 않은 숫자를 데이터로 확정하지 않는다. 원응답은 별도 보존한다.

공백·줄바꿈만 정규화하고 날짜·시각·금액·기관명은 변경하지 않는다. 연도는 게시일/학년도에서 채우지 않는다. 11.2(월)~12(목) 같은 명시적 같은 범위의 월 생략은 월·일로 보존하되 누락 연도는 null이다. 날짜만 명확한 경우 missing_time 단독 검토를 하지 않는다. 부모/기관/차수별 scope를 보존하고 정확 중복은 raw를 유지한 상태에서 검증 배열만 합친다. 원문에 있는 번호 세션의 추가 규칙 추출은 origin=source_rule_supplement다.

요약은 변경하지 않고 100자·숫자·명시 대상/필수 조건/우대 구분을 검증한다. 자동 문자열 검사는 자격·혜택 함의 또는 일정 누락을 모두 입증하지 못한다. 사람 정답 라벨 기반 정확도를 주장하지 않는다. 이미지·첨부만 참조하는 원문의 정보 부족은 불확실성으로 남기고 다운로드/OCR은 하지 않는다.

## 일반 배치의 안전 경계

schedule-apply-v1은 기존 10건 제한을 유지한다. 별도 schedule-apply-batch-v1은 원래 76개 selected IDs와 기존 10개 excluded IDs, DB/source, 스냅샷·선정 해시, 성공 ID 목록/기대 개수를 요구한다. 포함 아이템은 1~76개로 제한하고 확정 목록 밖/중복/기존 제외 ID를 거부한다. 부분 API 실패 시 성공분만 입력 파일에 담으며 실패는 보고서에서 제외하지 않는다.

새 응답 포맷은 analysis.response_format=schedule-v2-direct-v1로 구분하며 기존 이력의 빈 포맷·ID·해시는 바꾸지 않는다. 검증 시 원응답과 원문에서 결과를 다시 재구성하여 임의 수정된 분석 파일을 거부한다. model.Schedule 및 기존 projection 저장 필드는 그대로다.

이번 배치 버전의 --apply는 CLI가 연결 전에 거부한다. 기존 Repository/CAS 경로와 legacy 10건 apply는 유지하되 이 실험은 Preview 전용이다. Atlas 인덱스/공고/분류·만료·이력을 수정하지 않으며 Runner에 등록하지 않는다.

공식 참고: Google Gemini 문서 MCP Go SDK Advanced Capabilities(Structured Outputs/SystemInstruction), google.golang.org/genai v1.77.0, https://cloud.google.com/model-garden/pricing (Standard Global: 입력 $0.30/백만, 출력·추론 $2.50/백만, 캐시 입력 $0.03/백만; 2026-11-06 확인).
