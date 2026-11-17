# AI 생성 공고 캘린더 구현 및 실행 기록

2026-11-17~2027-02-17(한국 시간, 양 끝 포함)에 합성 공고 36건을 생성해 연결된 Atlas DB의 `notice_predictions`에 저장했다. `notice_prediction_runs`에는 모델·프롬프트·원본 응답·사용량·참고 제목 및 실행 해시를 기록했다. 배포된 웹 코드는 이번 작업에서 변경하지 않았다. 로컬 `http://localhost:8080/#calendar`에 반영했다.

## 설계와 변경 파일

- `internal/predictions`: 합성 모델, 검증, 3개월 날짜 배정, 콘텐츠 해시. 날짜는 AI가 추론한 확정 기한이 아닌 시나리오용 배정일이다.
- `cmd/generate-notice-predictions`: 준비/유료 생성/DB 적용을 명시적 플래그로 구분한다. 3개 배치로 36건 생성, 재시도 포함 최대 6회, 요청 전 예상 비용 US$1 한도. 같은 체크포인트에서 성공 응답 재사용. 최초 파일 부재와 손상 파일을 구분하는 회귀 테스트 포함.
- `internal/repository/predictions.go`: 별도 컬렉션만 조회·저장한다. 실행 및 항목의 `_id`와 해시를 사용해 재실행 중복을 방지하고 변경 내용은 덮어쓰지 않는다. 일부 저장 후 중단되어도 같은 실행으로 재개할 수 있다. 트랜잭션에 의한 전체 원자적 저장은 제공하지 않는다.
- `internal/web/predictions.go`, `web.go`: 기존 인증 규칙을 따르는 읽기 API. 최대 100일 범위만 허용한다. 기존 Reader 인터페이스는 확장하지 않고 별도 선택 인터페이스로 연결했다.
- `internal/web/static/calendar.js`, `app.js`, `index.html`, `app.css`: 실제 일정과 합성 일정을 분리한 채 병합 표시한다. 보라색 점선, AI 생성 라벨, 표시 토글, 별도 상세 설명을 제공한다. 합성 조회 실패는 실제 캘린더 조회를 막지 않는다.
- README: 생성 및 재개 방법, 컬렉션, 데이터 의미와 한도를 설명한다.

## 실행 결과

- Gemini `gemini-3.8-flash` 요청 3회, 총 9,532토큰.
- 예상 비용 US$0.021786. 최종 청구 금액이 아니다.
- 저장 및 해시 재검증 36건: 11월 6건, 12월 12건, 1월 12건, 2월 6건.
- 적용 전후 `crawled_contents`, `crawl_sources`, `schedule_analyses`, `content_classifications`의 전체 해시 동일.
- 브라우저에서 11월 실제 마감 34건 유지, AI 생성 표시 6건/숨김 0건, 12월~2월 생성 항목 및 합성 설명 상세 확인. 1280×720 화면 바깥 넘침 없음.

## 검증

- `TEST_MONGODB_URI=mongodb://127.0.0.1:27030 go test -count=1 ./internal/... ./cmd/...` 통과. 로컬 임시 MongoDB의 별도 테스트 DB로 통합 검증하고 테스트 프로세스를 종료했다. 새 명령의 체크포인트 회귀 테스트는 추가 후 별도 재실행해 통과했다.
- 신규 MongoDB 통합 테스트: 저장/재조회, 재적용 삽입 0건, 날짜 필터, 동일 ID 변경 거부, 실제 공고 불변.
- `go test -race ./internal/predictions ./internal/web` 통과.
- `go vet ./internal/predictions ./internal/repository ./internal/web ./cmd/generate-notice-predictions` 통과.
- 생성 명령 및 `cmd/web` 빌드 통과.
- calendar/dashboard JavaScript 단위 테스트 통과. 캘린더 실제 브라우저 동작 확인.
- 최초 `go test ./...`는 무시된 로컬 산출물 `outputs/image-trust-policy/source-before/internal/model/notice_policy.go`의 불완전한 과거 코드 사본도 패키지로 탐색해 실패했다. 소스 디렉터리 `internal` 및 `cmd` 전체를 명시한 위 명령으로 검증했다. 로컬 포트 제한에 걸린 실행은 허용된 환경에서 재실행했다.

이 데이터는 검증된 공고 예측이나 실제 학교 공고가 아니다. 현재 목표는 데이터가 부족한 상태에서 DB 저장과 캘린더 경험을 검증하는 것이다. 웹 접속으로 AI를 호출하거나 데이터를 자동 생성하지 않는다. 원본 공고 검색·개수·마감 날씨는 합성 데이터를 읽지 않는다.
