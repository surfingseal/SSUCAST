# AI 분류 품질 평가와 근거 검증

## 범위와 실제 정답

평가 데이터는 실제 HTML 1건과 합성 13건이다. 이번 작업의 학교 HTTP 요청과 Vertex AI 신규 호출은 모두 0회다. 기존 실제 Gemini 결과는 재생(replay)해서 평가한다. Vertex 인증·모델·호출 제한·기존 Crawler/Fetcher/Repository/Job Runner/REST API는 유지했다.

실제 공지의 학교 분류는 장학/scholarship이며 저장된 AI 예측은 student_support/needs_review다. 사용자가 이 채팅에서 실제 공지의 사람 정답을 **scholarship**으로 확정했다. 이 승인은 categories 차원에만 적용한다. 활동·관심 분야·대상의 사람 정답을 만들어 채우지 않았다.

사람 정답과 원본 학교 카테고리는 별개 근거다. 학교 카테고리가 자동으로 정답이나 AI 라벨이 되지 않으며, 카테고리 차이를 해결하려고 어느 쪽도 덮어쓰지 않는다. 이번 1건은 사용자 정답 대비 FP=1(student_support), FN=1(scholarship), TP=0이다. category micro Precision/Recall/F1은 모두 0이며 이는 **이 1건의 판정**이지 모델 전체 정확도가 아니다.

## needs_review 구조

`ClassificationQuality`는 버전 `quality-v1`과 다음 사유 코드를 기록한다.

| 코드 | 발생 근거 | 주의점 |
|---|---|---|
| insufficient_information | 정규화 본문 40자(Unicode rune) 미만 | 정보 부족의 보수적 휴리스틱이며 확률 점수가 아님 |
| classification_ambiguity | 대표 분류 unknown | 다중 라벨 자체는 모호성으로 간주하지 않음 |
| images_unanalysed | 파서가 이미지 존재를 확인했고 OCR/이미지 분석을 하지 않음 | 이미지 내부 내용을 추측하지 않음 |
| attachments_unanalysed | 첨부 메타데이터가 있고 파일 내용을 분석하지 않음 | 이름/URL만으로 내용이나 자격 조건을 만들지 않음 |
| source_category_disagreement | 알려진 학교 카테고리와 AI categories가 불일치 | 학교 분류를 정답으로 강제하지 않음 |
| unsupported_claim | 활동·관심 분야·대상의 인용 근거가 없거나 검증 불가 | 누락된 근거와 거짓 주장은 같은 의미가 아님 |
| model_review_unspecified | 모델이 needs_review지만 위 사유를 특정할 근거 없음 | 기존 자유문장 근거에서 원인을 억지로 추론하지 않음 |

모델/API 실패는 분류 모호성과 구분하여 failed/error_code와 평가의 model_failure로 기록한다. `images_state`는 unanalysed/absent/unknown을 구분한다. 옛 문서에 이미지 메타데이터가 없다는 이유만으로 이미지가 없다고 표시하지 않는다.

기존 실제 결과의 파생 감사에서는 짧은 본문, 이미지 2개, 첨부파일 미분석, 학교/AI 카테고리 불일치를 확인했다. 옛 응답은 인용 evidence가 없는 형식이므로 other/general/청년 주장 3개는 새 검증 기준에서 '인용 미확인'이다. 이것만으로 그 세 주장이 사실과 다르다고 단정하지 않는다.

## 원본 보존과 근거 가드

새 분류 문서는 `raw_labels`에 모델의 원래 출력과 근거를 보존하고, 최상위 labels에는 검증 후 결과를 저장한다. `quality`에는 제거한 주장과 사유를 기록한다. 기존 실제 MongoDB 문서는 마이그레이션하거나 수정하지 않았다. 감사 명령은 읽기 전용이며 읽기 전후 BSON SHA256을 비교했다.

향후 Vertex Structured Output에는 `evidence` 배열을 요구한다. 각 항목은 field/label/source/quote로 구성하며 source는 title/content/original_category만 허용한다. Go 검증은 다음을 수행한다.

- 원문에 실제 존재하는 연속 인용문인지 확인한다.
- 활동·관심 분야는 정해진 용어의 텍스트 근거가 있는지 확인한다.
- target_audience는 원문에서 인용한 문자열 그대로만 허용한다. 원문에 없는 성적·학년·전공 조건을 생성할 수 없다.
- 같은 문장의 부정 표현, 명령 삽입 문구를 보수적으로 거절한다.
- 근거 없는 other/general 기본값도 제외한다.
- 검증 결과의 근거 문장은 규칙 기반 설명으로 생성하고 모델 자유문장은 raw_labels에 남긴다.

검증은 인용·용어·일부 부정문을 검사하는 보수적 규칙이다. 동의어·복잡한 부정·인용된 타인의 주장·모든 프롬프트 공격을 완벽하게 이해하지 못하며, 사실 확인기나 의미적 정답 보장으로 제시하지 않는다. 근거가 충분한지도 최종적으로 사람의 판단이 필요하다. 카테고리 의미의 옳고 그름은 사람 gold와 비교하여 측정한다.

입력의 미분석 미디어 상태와 evidence 지침이 바뀌어 새 버전은 `ssu-v1-prompt2-evidence`다. 옛 실제 예측은 prompt1 입력 해시를 그대로 검증한다. 새로운 미디어 메타데이터를 옛 모델이 입력으로 받았다고 표시하지 않는다. 옛 출력에 새 가드를 적용한 것은 파생 감사이며 새 프롬프트의 실제 Gemini 성능 검증이 아니다.

## Fixture와 사람 라벨링

`internal/evaluation/testdata/notices.json`에 다음을 분리했다.

- real_html: 사용자 제공 실제 HTML 1건, 기존 실제 Vertex 결과, 사용자 승인 categories 정답.
- synthetic: 장학, 학사, 진로/인턴, 비교과/AI 특강, 국제교류/교환, 봉사, 공모전, 학사+장학 다중 라벨, 정보 부족, 미디어 미분석, 명령 삽입, 부정문, Mock 실패 등 합성 13건.

합성 공지는 모두 허구이며 example.invalid URL을 사용한다. 합성 참조 라벨은 에이전트 작성(agent_reference), approved=false다. 사람이 확인한 실제 정답으로 표시하지 않는다. Mock 예측에는 의도적인 FP/FN과 실패를 넣어 평가기가 오류를 감추지 않는지 확인한다.

사람이 추가 라벨을 붙이려면 `human-annotations.template.json`을 복사하여 categories/activity_types/topics 중 실제 검토한 fields만 지정하고 labels를 작성한다. reviewer, approved_at(RFC3339), approved=true를 직접 입력한다. input_hash가 달라진 입력에는 승인을 재사용하지 못한다. 기본 템플릿의 pending 행은 gold가 되지 않는다.

```sh
cd /Users/surfing_seal/Documents/ChatGPT/HCI
go run ./cmd/evaluate-classification -mode mock -output /tmp/mock-report.json
go run ./cmd/evaluate-classification -mode replay -output /tmp/replay-report.json
go run ./cmd/evaluate-classification -mode mock -gold /path/to/reviewed-human-labels.json -output /tmp/human-evaluation.json
```

평가 명령은 mock/replay만 지원하며 Vertex 클라이언트를 생성하거나 Google API를 호출하지 않는다. 향후 실제 모델 결과를 더 얻으려면 기존 `classify-one -execute` 명령을 별도로 명시적으로 실행한 뒤, 일치하는 입력 버전/해시와 실제 예측·사용량을 replay로 가져와야 한다. 이번에는 실행하지 않았다.

## 지표 정의

각 차원(categories/activity_types/topics)의 라벨을 집합으로 비교하여 TP/FP/FN을 합산한다. 중복 라벨은 점수를 부풀리지 못한다.

- Precision = TP / (TP + FP)
- Recall = TP / (TP + FN)
- F1 = 2TP / (2TP + FP + FN)
- micro: 전체 샘플의 TP/FP/FN 합산.
- macro_all: 고정 taxonomy 전체 라벨의 F1 평균. 미등장 라벨도 0으로 포함.
- macro_observed: gold 또는 prediction에 등장한 라벨의 F1 평균.
- exact_match: 전체 예측 라벨 집합이 gold 집합과 일치한 비율.

0으로 나누는 경우는 0으로 정하며 gold 없는 차원/집단은 null로 표시한다. 실패 샘플은 빈 예측으로 포함하여 gold 라벨이 FN에 반영된다. needs_review 샘플도 제외하지 않는다. 검토 비율/분류 확정 coverage를 별도로 기록하여 보류로 점수를 높이지 못하도록 한다.

`human_gold_*` 지표와 `agent_reference_*_mock_diagnostics`는 별도다. 원래 예측과 근거 가드 적용 후 예측도 분리한다. 주관적 정답 판정 정책을 바꾸면 gold/입력/버전을 관리하고 사람이 다시 승인한다.

## 호출과 사용량

새 저장 결과는 model_requests, input_tokens, output_tokens, usage_known을 기록한다. 실패/불완전 응답도 제공된 사용량을 잃지 않으며 미제공 사용량은 0 사용으로 단정하지 않는다. failed 예약에는 요청 수와 실패 코드를 남긴다.

평가 JSON은 provider별 선택 샘플 수, 기록된 호출 수, 실패 수/비율, 사용량을 구분한다. Mock의 13회는 정해진 Fixture 응답 13개에 대한 모의 호출이며 0 토큰 사용량도 모의 데이터다. replay의 1회·870/88 토큰은 과거 실제 호출 기록이다. 평가 실행의 new_vertex_calls는 0이다. 실패율은 선택된 기록의 실패율이며 전체 서비스의 장애율로 일반화하지 않는다.

## 기존 DB 읽기 감사

```sh
go run ./cmd/audit-classification -content-id 6aed4fea6abc9a0b35c77076 -output /tmp/stored-audit.json
```

기본 URI는 별도 검증용 127.0.0.1:27029, DB는 ssu_vertex_validation이다. 해당 MongoDB가 실행 중이어야 한다. stored_classification과 derived_validated_labels/derived_quality를 함께 내보내고 기존 문서와 학교 카테고리를 수정하지 않는다. 실제 데이터 파일은 검증 후에도 보존한다.

## 검증 범위

손으로 계산한 multi-label 예제, 0 분모, gold 누락, 입력 해시 불일치, pending 라벨, 실패 샘플의 FN, 사람 gold/Mock 격리, 부정문·명령 삽입·가짜 자격조건 거절을 테스트한다. 실제 MongoDB/API에서는 raw_labels/quality/사용량과 원본 학교 카테고리 보존을 검증한다. SDK Mock 테스트와 기존 전체/race/vet/build도 유지한다. 새 프롬프트의 실제 Vertex 성능이나 대규모 일반화 성능은 아직 검증하지 않았다.
