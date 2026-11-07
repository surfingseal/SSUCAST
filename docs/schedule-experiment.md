# 슈캐치 일정·요약 10건 수동 실험

기존 Source 분류 전략·크롤러·Repository·Runner·REST API·환경변수는 변경하지 않는다. 슈캐치 공식 카테고리 분류에 AI를 연결하는 기능이 아니다. 이 명령은 이미 저장된 공고의 일정과 요약만 분석하며 MongoDB 쓰기 메서드를 호출하지 않는다.

모델은 `gemini-3.5-flash-lite`로 고정한다. `.env`의 `VERTEX_MODEL`을 변경하지 않으며 SDK는 Vertex AI Backend/ADC, 기존 project/global 설정을 사용한다. 자동 분류 제한은 기존 1건 설정 그대로이며 실험만 별도의 10건·12회 제한을 갖는다.

1. 읽기 전용 스냅샷에서 `source_name=ssu_catch`, `status in [진행,unknown]`의 실제 후보 86건을 확인한다.
2. 10건의 ID·원문·제목·게시일·원본상태·선정사유·스냅샷 체크섬을 `selected.json`에 기록한다. 나머지 76건은 모델 입력으로 보내지 않는다.
3. 실행 전 스냅샷과 개별 입력을 다시 대조한다. 서비스 계정 타입·권한·project 일치 검증, ADC 인증, 모델 메타데이터 GET을 수행한다. 관리 API의 403과 생성 권한은 구분하며 첫 표본 생성으로 실제 접근을 확인한다.
4. 기본 실행은 사전 검사만 수행한다. 유료 생성은 아래 `-execute`를 직접 지정해야 한다.

```sh
go run ./cmd/analyze-schedules -dir /absolute/path/to/experiment
# 명시적인 실제 유료 호출: 10건 / 총 최대 12회
go run ./cmd/analyze-schedules -dir /absolute/path/to/experiment -execute
# 저장된 응답 재검증: .env·MongoDB·Vertex 접근 없음
go run ./cmd/analyze-schedules -dir /absolute/path/to/experiment -validate-only
```

SDK 재시도는 1회 시도로 고정한다. 429/5xx에 대해서만 전역 추가 재시도 최대 2회, 총 12회 상한을 전송 계층에서도 검사한다. 정상·형식 오류 응답을 다시 요청하거나 다른 모델로 대체하지 않는다. 400/401/403/404 및 통신/인증 실패는 후속 생성을 중단한다. `execution.lock`은 중단 후 자동 재실행도 막는다. 생성 전 `attempts.json`을 디스크에 동기화하며 개별 응답과 사용량을 순차 저장한다.

응답의 JSON 속성·필수 필드·자료형·nullable·enum과 실제 날짜 파싱, UTC+09:00, 시작/종료 순서, 근거의 정확한 원문 일치, 인용문의 날짜·시각 숫자, 중복을 Go에서 검증한다. 요약의 100자 제한과 원문에 없는 숫자 및 대상 서술의 재검토 여부를 검사한다. 의미·완전성·자격조건은 자동 검사만으로 증명할 수 없으므로 원문 대조 검토를 별도로 기록한다.

`output`/`raw_response`는 모델 원응답이다. `validated_output`은 근거에서 검증되지 않은 날짜·시각을 null로 제거하고 검토 사유를 추가한 보수적 결과다. 원응답을 정확한 값으로 덮어쓰지 않는다. 날짜만 있는 표현을 처리하기 위해 현재 RFC3339 필드에 임의의 00:00이나 23:59를 채우지 않는다. 그 날짜는 evidence에 남으며 자동 상태 반영에는 사용할 수 없다. 후속 운영 설계에서는 date-only 필드와 시간 정밀도 필드를 추가할 수 있다.

기존 규칙 추출기는 단일 마감일 및 일말 추정 정책을 갖는다. 복수 일정 추출기와 동일한 정답 범위를 가진 것으로 취급하지 않는다. 실험 보고서는 실제 입력·원응답·보수적 결과·규칙 결과·검토 의견을 구분한다. 학교 HTTP/이미지·첨부 다운로드/OCR/DB apply/delete는 수행하지 않는다.

공식 근거(2026-11-06 확인):
- Google Gemini 문서 MCP의 Go SDK `advanced-capabilities` / Structured Outputs·SystemInstruction 예제.
- SDK `google.golang.org/genai@v1.77.0` 및 [공식 SDK](https://pkg.go.dev/google.golang.org/genai).
- [모델과 global 지원](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/3-5-flash-lite?hl=en).
- [Google Cloud Standard Global 가격](https://cloud.google.com/model-garden/pricing): 백만 토큰당 입력 $0.30, 출력·추론 $2.50, 캐시 입력 $0.03. Flex/Batch/Priority 요금과 구분한다.
