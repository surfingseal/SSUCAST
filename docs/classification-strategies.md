# Source별 공식 카테고리 및 AI 전략

## 현재 정책

최종 카테고리는 academic(학사), scholarship(장학), international(국제교류), international_student(외국인 유학생), extracurricular(비교과·행사), teaching(교직), volunteer(봉사), other(기타)의 8개다. `unknown`은 별도의 검토 대기 표식이며 공식 9번째 카테고리가 아니다.

**채용·교원채용은 수집 제외 카테고리다.** 슈캐치 목록에서 원본 라벨을 확인한 즉시 제외하므로 상세 HTTP 요청을 하지 않는다. 같은 slug가 허용 라벨과 제외 라벨로 중복 등장해도 전체 공지를 제외한다. 공통 Runner/Repository에서도 원본 카테고리 또는 원본 카테고리 배열에 두 라벨이 있으면 저장·AI 처리를 차단한다. 기존에 저장된 제외 자료는 삭제하지 않고 목록/상세 조회에서 제외한다. 공고 제목의 단어를 추측하여 수집 제외로 판정하지 않는다.

실제 목록 Fixture의 원문 15행은 보존했고 채용 4행을 제외한 11행만 수집 후보로 파싱한다. 교원채용 제외는 합성 DOM 및 저장/조회 테스트로 별도 검증한다. 교직(teaching)은 제외 대상이 아니다.

| classification_strategy | 공식 원본 카테고리 | AI | 최종 상태 |
|---|---|---|---|
| source | 인식 가능한 공식 라벨을 정규화 | 호출하지 않음 | 분류 가능하면 classified, 그 외 unknown/needs_review |
| ai | 원본은 보존하고 제목/본문으로 분류 | 명시적 수동 실행 때만 필요 | 공식 8개 중 하나, 불확실하면 unknown/needs_review |
| hybrid | 유효하고 구체적인 원본이면 사용 | 누락·미식별·일반/기타 분류일 때 필요 | source 또는 ai |

원본의 기타(other)는 source 전략에서는 유효한 공식 카테고리다. hybrid에서는 포괄적인 분류로 보아 AI 대상이 된다. 제외 카테고리는 이 전략 판단보다 먼저 차단하며 unknown으로 저장하거나 AI로 보내지 않는다.

이전 Source 문서와 설정에서 전략이 없으면 안전한 `source`가 기본값이다. 슈캐치 설정 `configs/sources.ssu-catch.json`에는 source를 명시했다. 분류 전략 판단은 Source 설정을 사용하며 사이트 이름·crawler_type 분기로 AI 사용 여부를 하드코딩하지 않는다. 기존 슈캐치의 HTTP 예산·자동 수집 금지 로직은 수집 안전장치로 유지한다.

## 구성

```json
{
  "name": "my_department",
  "crawler_type": "example",
  "base_url": "https://department.example.invalid",
  "list_url": "/notices/",
  "enabled": false,
  "auto_enabled": false,
  "policy_approved": false,
  "classification_strategy": "hybrid",
  "max_pages": 1,
  "max_contents": 1,
  "max_requests": 3,
  "request_interval": "2s"
}
```

example.invalid는 합성 예시다. 실제 학과 HTML이나 검증된 선택자가 제공된 것으로 간주하지 않는다. 기존 Example 파서는 로컬 테스트용 DOM을 지원하며 모든 학과 사이트의 파서라고 주장하지 않는다. 실제 학과 수집은 해당 사이트의 검증된 파서/Fetcher 정책과 함께 설정해야 한다.

## 저장 책임

`crawled_contents`는 원본 공고와 **현재 조회에 적용되는 최종 분류**를 가진다.

- original_category / original_categories: HTML에서 얻은 원본 값과 발견한 멤버십 이력. AI가 덮어쓰지 않는다.
- category / categories: 공식 8개 체계의 현재 분류. source는 기존의 공식 분류 멤버십 병합을 유지하고, ai는 단일 대표 카테고리만 사용한다. 과거 AI 라벨을 새 카테고리 배열에 union하지 않는다.
- classification_method: source 또는 ai.
- classification_status: classified 또는 needs_review. AI가 필요한데 아직 실행하지 않았으면 unknown/needs_review다.
- model_version: 실제 AI 결과가 반영된 경우만 설정한다. source/대기 결과에서는 빈 문자열이다.
- classification_strategy / classification_policy_hash / classification_input_hash / classification_updated_at: 적용한 전략과 변경 감지 근거.

`content_classifications`는 **AI 호출 시도·후보·근거·실패·적용 이력**이다. source 전략은 이 컬렉션에 가짜 AI 문서를 만들지 않는다. 원래 모델 출력은 raw_labels, 근거 검증 결과는 quality에 남긴다. needs_review 후보는 원본을 보존하되 저장/조회되는 분류는 unknown으로 보류한다.

AI 결과는 다음 조건에서만 최종 공고에 반영한다.

1. 저장된 공고에서 Source 소유권과 현재 전략을 확인한다.
2. 제외 대상과 source/유효한 hybrid는 SDK 클라이언트를 만들지 않는 수동 CLI 경로를 제공한다. Repository도 classifier를 호출하지 않는다.
3. AI가 필요하면 입력+본문 해시+정책 해시+taxnomy/model 버전에 대해 먼저 예약한다.
4. 호출 직전 공고/정책을 다시 확인한다.
5. 응답의 8개 enum·단일 분류·스키마·근거를 검증하고 이력을 저장한다.
6. 현재 공고의 본문 해시·입력 해시·정책 해시가 같은 경우에만 원자적 CAS로 category를 반영한다.
7. 도중에 공고나 전략이 바뀌면 결과는 superseded 이력으로 남기고 최종 카테고리에 적용하지 않는다.

Standalone MongoDB에서 Source/공고/이력이 하나의 다중 문서 트랜잭션은 아니다. Source 변경 후 공고를 재투영하고 적용 전후 확인을 수행한다. AI 이력 저장 뒤 최종 반영 전에 중단되면 동일 예약의 완료 결과를 재사용해 반영할 수 있으며 모델을 다시 호출하지 않는다. 저장 실패는 성공 결과로 꾸미지 않는다.

## 변경 감지와 중복 방지

본문 해시와 URL 고유 upsert, 기존 ID/최초 crawled_at/같은 본문의 updated_at 보존을 유지했다. 분류 정책·원본 카테고리/미디어 입력의 변화는 별도 classification_input_hash로 감지한다. 과거 AI 결과를 정책 변경 후 그대로 유지하지 않는다.

Source의 strategy가 바뀔 때 MongoDB에서 classification_revision을 증가시킨다. 같은 설정을 다시 적용하면 증가하지 않는다. ai→source→ai로 돌아와도 새 정책 리비전이므로 재분류할 수 있다. 설정 적용 시 기존 공고의 최종 분류를 즉시 재투영하되 AI를 실행하지 않는다. 수정된 공고는 기존 AI 분류를 unknown/needs_review로 바꾸고 명시적 재실행 때 새 예약으로 처리한다.

같은 입력·정책·모델의 완료 결과는 재사용한다. processing/failed 예약은 자동으로 재시도하지 않는다. 한 실제 명령의 batch/concurrency/max_items는 1, retries는 0이며 전송 계층의 1회 제한도 유지한다.

새 AI taxonomy는 `official-eight-v1-prompt3`이다. 과거의 career/student_support 등이 들어 있는 실험 이력과 평가 데이터는 역사적 데이터로 보존하고 평가기에서는 legacy taxonomy로 읽는다. 그 결과를 새 최종 category에 다시 적용하지 않는다.

## 수동 테스트

자동 AI 분류는 기존처럼 기본 비활성화이며 현재 구현은 수동 전용이다. 수집 Job은 공고를 저장하고 전략별 source/대기 분류까지만 처리한다. AI의 유료 호출을 시작하지 않는다.

```sh
cd /Users/surfing_seal/Documents/ChatGPT/HCI
# 저장된 공고의 정책만 확인. Vertex 클라이언트를 만들지 않는다.
go run ./cmd/classify-one -content-id CONTENT_ID
# source이면 이 명령도 모델 없이 공식 카테고리만 반영한다.
# ai/필요한 hybrid인 학과 공고 1건을 직접 호출할 때만 명시적으로 실행한다.
go run ./cmd/classify-one -content-id CONTENT_ID -source-id SOURCE_ID -execute -direct-vertex -report /tmp/department-one.json
```

`source-id`는 저장된 소유권을 확인하는 값이며 소유권을 바꾸지 않는다. 등록되지 않은 Source는 안전한 source 전략으로 처리한다. Source 설정을 ai/hybrid로 명시하기 전에는 강제로 AI로 우회할 수 없다. 실제 학과 데이터는 이번에 제공되지 않았으므로 실제 학과 Vertex 호출은 수행하지 않았다. 합성 학과 JSON은 `internal/fixture/department/notice.synthetic.json`이며 Mock 테스트 전용이다.

Vertex 인증·프로젝트·모델/리전·.env는 변경하지 않았다. source 또는 유효한 hybrid에서는 자격 증명을 읽거나 Vertex 사전 점검을 실행할 필요도 없다. 실제 모델 호출은 명시적 실행에 한정한다.

## 기존 데이터의 최종 분류 정리

```sh
# 기본은 dry-run. 분류 후보와 적용 전략만 출력한다.
go run ./cmd/reconcile-classification -report /tmp/projection-plan.json
# 최종 분류 메타데이터를 정리한다. AI 호출/AI 예측 라벨 수정/삭제는 없다.
go run ./cmd/reconcile-classification -apply -report /tmp/projection-applied.json
```

분류의 활성 상태(applied/application_status)는 바뀔 수 있지만 AI의 예측·근거·모델 기록은 보존한다. 기존의 제외 카테고리는 일반 목록 및 상세 조회에서 감춘다. 명령이 원본 파일/DB의 제외 공고나 AI 이력을 파괴적으로 삭제하지 않는다.

## 검증

실제 슈캐치 Fixture의 source 분류 및 AI 0회, 채용·교원채용의 상세 요청/저장/AI/조회 제외, 모든 공식 8개와 unknown의 조회, hybrid 분기, 동시 중복 예약, 수정 및 전략 순환, 생성 도중 전략 변경, raw/근거 보존을 MongoDB/인증 API와 Mock으로 검증한다. 기존 전체/race/vet/build도 실행한다. 실제 웹 또는 Gemini 추가 호출은 하지 않는다.
