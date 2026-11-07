# 실제 사이트 연결을 위한 기반 작업

검증일: 2026-10-30, Asia/Seoul. Go 1.27.3, 로컬 MongoDB 8.3.4 사용.

## 변경 파일과 이유

| 파일 | 변경과 이유 |
|---|---|
| internal/crawler/fetcher.go | 공통 Fetcher와 실행별 Session. Context/Timeout, robots, origin 공유 rate limit·동시성, 요청 예산, 안전한 redirect, 지수 재시도와 Retry-After, FetchError |
| internal/crawler/url.go | 기존 SSRF URL/IP 검사를 이동. DNS 검증 후 확인한 IP로 접속하며 host 대소문자·기본 포트를 정규화 |
| internal/crawler/crawler.go | 기존 Crawler 인터페이스와 Example 유지. 네트워크 처리를 Session에 위임하고 목록 URL·페이지/상세 한도 적용 |
| internal/crawler/registry.go, builtins.go | 타입별 Factory 등록과 선택. source 이름과 구현 타입 분리. 공유 Fetcher를 모든 기본 Factory에 전달 |
| internal/config/config.go, sources.go | CRAWL_SOURCES_FILE JSON 로드, ID/name/URL/cron/한도 검증, 기존 환경 변수 유지 |
| internal/model/model.go | optional source 설정 필드, auto_enabled 포인터로 누락과 false 구분, legacy 기본값, 선택적인 작업 오류 필드 |
| internal/service/runner.go, scheduler.go | Registry 주입, 기존 map 생성자 호환, 수동/자동 진입점 분리. cron 콜백도 최신 source의 자동 실행 여부 재확인 |
| internal/repository/sources.go | 설정 Upsert, 고유 name/명시 ID 처리, 기존 ID·생성 시각·마지막 실행 시각 보존 |
| internal/repository/jobs.go | 구조화된 오류 코드·단계를 작업 기록에 저장 |
| cmd/server/main.go | 공유 Fetcher/Registry 구성, 다중 설정 반영, 기존 데모 seed 유지 |
| configs/sources.example.json, .env.example, docker-compose.yml | 비활성 source 예제와 환경 변수, Compose 읽기 전용 설정 마운트 |
| README.md | 설정·호환성·등록 방법·실제 사이트 연결 정보와 제한 문서화 |
| 각 계층의 테스트 및 internal/integration/server_test.go | 기존 검증 유지, 신규 기능 및 실제 MongoDB를 포함한 API 연동 검증 |

기존 REST 경로, 작업 실행 요청 본문, Crawler 인터페이스, 콘텐츠 저장 컬렉션/URL 인덱스/Upsert는 유지했습니다. source와 job 응답에는 선택적인 필드만 추가했습니다. 외부 라이브러리와 go.mod/go.sum 의존성은 추가·변경하지 않았습니다. 기존 fixture HTML과 데모 smoke 스크립트도 유지했습니다. 이전 테스트에서 제거된 Example의 private 네트워크 helper 호출은 Fetcher/Session 호출로 옮겼으며, 기존 시나리오는 유지했습니다.

## 검증 결과

다음 전체 검증은 실제 MongoDB 연결 설정을 제공하여 수행했습니다.

```sh
TEST_MONGODB_URI=mongodb://127.0.0.1:27028 go test -race -cover ./...
go vet ./...
go build -o .cache/server ./cmd/server
```

모두 통과했습니다. 실제 DB 테스트는 별도의 crawltest_<ObjectID> 데이터베이스를 만들고 종료 시 삭제했습니다. 이번 최종 실행에서 실제 DB 테스트를 skip하지 않았습니다.

| 검증 영역 | 확인한 동작 | 결과 |
|---|---|---|
| 기존 기능 | 예제 목록·상세·페이지네이션·정규화, 기존 API 인증·검증, Runner와 Repository 기존 시나리오 | 통과 |
| Fetcher | 요청/대기 중 Context 취소, Timeout, 응답 크기, robots 동시 초기화와 대기자 취소 | 통과 |
| SSRF/redirect | 비공개 IP·localhost DNS 주소 차단, 다른 origin 요청 없음, 동일 origin redirect 성공, redirect 대상 robots 차단, redirect 루프 제한 | 통과 |
| robots | 금지 경로 요청 없음, 404 처리, 403/redirect fail-closed, Crawl-delay 반영 | 통과 |
| 재시도 | 503 재시도, 429 Retry-After 2초 실제 대기, HTTP 날짜 형식 해석, 긴 대기의 취소, 본문이 잘린 403도 재시도하지 않음 | 통과 |
| 요청 제한 | robots/재시도 포함 예산, 두 Session 간 origin 동시성·요청 간격 공유, origin 정규화, 소량 제한과 상대 목록 URL | 통과 |
| 설정/Registry | 다중 source, 기본 자동 실행 false, 잘못된 값·중복 이름·미등록/중복 타입·nil Factory 결과 거부, 타입 선택과 동시 접근 | 통과 |
| 자동/수동 | 자동 비활성 source의 수동 실행 성공, cron 제외 및 자동 진입점 거부, enabled=false의 수동 실행 거부 | 통과 |
| BSON/DB 호환성 | 신규 필드 없는 기존 문서 조회·기본값·일정 유지, false 포인터 roundtrip, 명시 ID/이름 Upsert 재실행, 생성 시각·기존 이력 유지, ID 충돌 거부 | 통과 |
| DB 포함 종단간 | API → 두 source의 같은 crawler_type 선택 → httptest → MongoDB 저장 → source별 API 조회, 요청 한도 오류의 코드·단계 저장 | 통과 |
| 실제 서버 시작 | JSON 파일과 DEMO_MODE 동시 로드, 추가 source의 enabled/auto_enabled=false 확인, 기존 scripts/smoke.py의 두 번 수집·ID 유지·조회 | 통과 |
| 종료 | 위 로컬 데모 서버에 SIGTERM, exit code 0 | 통과 |
| Compose 설정 | docker compose config --quiet | 통과; 이번 작업에서 이미지 재빌드·Compose 실행 테스트는 수행하지 않음 |

커버리지는 crawler 87.1%, config 92.2%, model 90.5%, repository 81.1%, service 90.3%, API 76.9%입니다. main과 fixture의 패키지 단위 statement coverage는 0%이며, 실제 서버 smoke 테스트로 실행 경로를 별도 확인했습니다.

## 수집 범위와 제한

이번 작업에서 실제 외부 웹사이트를 요청하지 않았습니다. HTTP 검증은 httptest와 로컬 데모 fixture를 사용했고, private DNS 차단 검사는 localhost만 사용했습니다. 설정의 .invalid 주소는 파싱만 하며 DNS 조회/HTTP 요청을 하지 않았습니다. 실제 사이트의 robots·이용 정책·HTML 파서는 아직 대상이 없으므로 검토·구현하지 않았습니다.

같은 프로세스의 공유 Fetcher를 통과하는 origin 제한만 제공하며 여러 프로세스 간 rate limit은 없습니다. robots.txt의 redirect는 따라가지 않고 중단합니다. 설정 파일은 시작 시 반영하므로 변경 뒤 재시작해야 하며 파일에서 빠진 DB source를 자동 삭제하지 않습니다. 이름을 바꾸면서 ID를 유지하려면 명시적인 id가 필요합니다. 수집 도중 HTTP 예산을 소진하면 실패로 기록하고 그 실행의 수집 결과는 저장하지 않습니다.

테스트용 MongoDB 데이터/프로세스는 검증 후 정리했습니다. 시스템 MongoDB 설정이나 Go 설치는 변경하지 않았습니다.
