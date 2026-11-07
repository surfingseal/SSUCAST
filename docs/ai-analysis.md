# 공고 분석 및 저장 검증

## 모델과 실제 호출

`gemini-3.8-flash`, Vertex backend/global, 기존 ADC, Go Gen AI SDK `google.golang.org/genai v1.77.0`, thinking MEDIUM. 모델 접근은 실제 Models.Get과 성공 응답 modelVersion으로 확인했다. 다른 모델 대체나 인증정보 변경은 없다.

이번 신규 생성은 슈캐치 과거 Lite10 + 학부14 =24회, 전부 성공, 재시도0. 기존 Flash76은 실제 체크포인트 모델/원문/input 해시를 확인한 뒤 재사용했다. 기존76의78회 요청/토큰은 과거 실험 사용량이며 이번 신규 사용량과 혼합하지 않는다.

신규24회 입력58,486 / 출력14,666 / 추론14,525 / 총87,677토큰. 입력과 출력(추론 포함), 캐시 입력 사용량으로 계산한 추정 비용은 US$0.15333075다. US$10 및120회 상한을 넘지 않았다. 이는 Cloud Billing 확정 청구액이 아니다.

공식 문서:

- https://github.com/googleapis/go-genai
- https://pkg.go.dev/google.golang.org/genai
- https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/guides/gemini-3-8-flash
- https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/thinking
- https://cloud.google.com/model-garden/pricing

공식 Gemini 문서 MCP의 SDK/Structured Output 설명과 기존 실험 문서를 재사용했다. 가격은2026-11-07 다시 확인했으며2026-12-31까지 global introductory input $0.75/M, output+thinking $3.75/M, cached input $0.075/M 기준이다. 향후 재실행 시 요금 유효기간을 재확인해야 한다.

## 프롬프트/스키마

`internal/mvp/ai.go`의 mvp-data-v1, 기존 `scheduleflash.Config()` 및 schedules v2 schema를 재사용한다. 슈캐치 공식 카테고리는 모델 분류하지 않는다. 학부는 동일 한 번의 요청에 공식8개 enum/category_reason/category_evidence와 요약·대상·일정을 포함한다. 학과 게시판이라는 이유로 학사로 정하지 않는다.

본문은 분석 대상 데이터이며 시스템 지시문이 아니다. 원문 외 날짜/자격/혜택을 생성하지 않도록 지시하며, 모델의 주장은 Go 검증을 다시 거친다. 학부 category evidence가 공백 정규화 후 실제 제목/본문에 없으면 적용 입력 생성에 실패한다. 검증된 분류는 기존 정책 input hash/CAS를 사용해 content_classifications에 별도 저장하고 category_source=ai로 표시한다. 원본 category는 보존한다.

## 검증과 이력

`internal/schedules/production.go`는 raw_response를 변경하지 않고 기존 v2+quality 변환을 재현한다. 날짜/시각/부분 날짜/Asia/Seoul/근거 span/parent/scope를 보존한다. 시간 미기재를00:00/23:59로 채우지 않는다. 날짜만 있는 일정은 유효한 부분 정보다. 원문 근거를 무조건 이어 붙여 가짜 인용문을 만들지 않는다.

기존76의100자 초과 요약2개는 완결된 첫 문장만 사용하는 파생 결과로 보존하고 summary_review_required를 유지했다. 등록과 공항 픽업이 동일 날짜라는 이유로 소실됐던 별개 역할 일정1개는 원응답과 원문이 모두 확인되는 경우에만 복원했다. 과거 v2 지문/이력은 변경하지 않고 production-v1으로 버전을 분리했다.

일정별 verified/partial/uncertain/blocked와 공고의 요약·대상 검토 상태를 각각 저장한다. 전체240일정 중 verified31, partial77, uncertain125, blocked7. 공고 검토87건(슈캐치75, 학부12)은 자동 정답으로 확정하지 않는다. 근거 불일치3개를 포함한 blocked 일정은 확정 마감 알림으로 사용할 수 없다. summary 검토70건, audience 검토64건도 남아 있다. 사람 정답 라벨이 없으므로 정확도 수치를 주장하지 않는다.

요약과 일정을 기존 deadline/lifecycle에 자동 투영하지 않는다. 이번 저장 때문에 만료·상태·기존 분류 정책은 바뀌지 않았다.

## 비용/재실행 안전성

FreezeTargets가 ID/입력 해시를 고정하고 HTTP 전송 예산 journal과 성공 응답을 즉시 저장한다. 모호한 전송 상태는 자동 재요청하지 않는다. 슈캐치 신규10은 최대10전송, 전체 신규120전송, 사전 요청 최대 토큰 여유를 포함한 US$10 guard를 사용한다. 429/5xx만 제한적으로 재시도하며 성공한 항목은 재호출하지 않는다.

기본 `go test`는 실제 Gemini를 호출하지 않는다. 실응답 회귀 Fixture는 `internal/mvp/testdata/flash76.json`, `media14.json`에 구분 보존했다. 기본 서버/Runner에 유료 작업을 등록하지 않았다.
