# SSU:catch 로컬 MVP 검증

## 완료 범위와 중단 사유

기존 프로젝트 `/Users/surfing_seal/Documents/ChatGPT/HCI`의 Crawler 인터페이스, 공통 Fetcher, Repository, Job Runner, Bearer 인증 REST API를 유지하여 SSU 전용 구현과 로컬 MongoDB 통합 테스트를 추가했다.

**실제 슈캐치 상세 게시글 수집·DB 저장 성공 건수는 0건이다.** 학교의 자동 수집 및 본문 재이용 허용 범위를 확인하지 못해 외부 수집을 중단했다. 공개 목록 HTML 1개와 robots.txt만 직접 요청했다. 목록 메타데이터 15행 파싱은 실제 HTML로 검증했으며, 상세 본문 추출은 합성 Fixture만으로 검증했다. 실제 상세 본문 선택자는 아직 확인하지 않았다.

사용자의 후속 지시에 따라 이 상태를 유지했다. 소스 설정은 `enabled=false`, `auto_enabled=false`, `policy_approved=false`, `detail_body_selector=""`이다. 정책 미검토 또는 상세 선택자 미설정 시 HTTP 요청 전에 구조화된 오류로 종료한다. SSU는 코드에서도 자동 스케줄을 차단한다.

## 접근 근거와 정책 검토

- robots: https://scatch.ssu.ac.kr/robots.txt
- 목록: https://scatch.ssu.ac.kr/공지사항/
- 사용자가 제공한 상세 예시(요청하지 않음): https://scatch.ssu.ac.kr/공지사항/?category=학사&slug=증명서-발급-서비스-일시-중단-안내-5
- 실제 robots 응답: `Allow: /`, `Disallow: /wp-content/plugins/`, `Disallow: /wp-admin`, `Disallow: /download.php`.
- 실제 목록 메뉴·푸터에서는 별도 수집/재이용 정책 링크를 확인하지 못했고, `Copyright Soongsil University All Rights Reserved.` 문구를 확인했다. robots 경로 허용을 학교의 저작권·서비스 이용 승인으로 해석하지 않았다.
- 추가 정책 검색: `site:ssu.ac.kr 저작권 정책 홈페이지 무단 수집`, `site:scatch.ssu.ac.kr 이용약관 저작권 자동수집`, 도메인 제한 검색 `저작권 정책 이용약관`(ssu.ac.kr), `SSU catch 이용 정책 자동 수집`(scatch.ssu.ac.kr). 2회의 검색 도구 호출, 총 4개 검색어였다. 관련된 공식 자동 수집/재이용 허용 조건을 확보하지 못했다. **정책이 존재하지 않는다고 단정한 것이 아니다.** 검색 결과의 다른 대학·다른 기관 정책은 판단 근거로 사용하지 않았다.
- 검색서비스 호출은 아래 Fetcher 원장과 별도다. 검색서비스 내부 HTTP 동작/요청 수는 도구가 공개하지 않는다. 검색 결과를 통해 상세 HTML을 확보한 것으로 간주하지 않았다.
- 학교 담당부서에 연락하거나 메시지를 보내지 않았다.

## 실제 대상 도메인 직접 HTTP 원장

한국 시간 2026-11-06. `internal/crawler/testdata/ssu_catch/requests.json`에 UTC 원장이 있다.

| 순서 | 한국 시간 | URL | 상태 | 시도 |
|---|---|---|---|---|
| 1 | 00:30:20 | https://scatch.ssu.ac.kr/robots.txt | 200 | 1 |
| 2 | 00:30:39 | https://scatch.ssu.ac.kr/%EA%B3%B5%EC%A7%80%EC%82%AC%ED%95%AD/ | 200 | 1 |

직접 요청 합계 **2회**. 재시도·리다이렉트 0회, 상세 0회, 페이지 2 요청 0회, 첨부파일/금지 경로 요청 0회. 요청 사이 약 18.35초. 같은 공통 Fetcher Session에서 최대 5회 예산과 최소 2초 간격으로 검사했고 정책 불명확 확인 후 닫았다. 이후 실제 수집 Job은 실행하지 않았다.

## 실제 DOM에서 검증한 선택자

근거는 변경하지 않은 `internal/crawler/testdata/ssu_catch/list.actual.html`이다.

| 데이터 | 선택자 / 처리 | 근거 |
|---|---|---|
| 컨테이너 | `ul.notice-lists` 1개 필수 | 캡처 344행 |
| 게시글 | `ul.notice-lists > li:not(.notice_head)` | 표제 행 제외, 실제 15행 |
| 작성일 | `.notice_col1 .h2` | `2026.11.05` 등, 모바일용 숨김 행에도 값 존재 |
| 상태 | `.notice_col2`, 내부 `.tag` | `진행`, `완료`; 실제 빈 상태 칸도 있어 `unknown`으로 보존 |
| 링크 | `.notice_col3 a[href]` | 게시글 식별에 `slug` 사용 |
| 원본 분류 | 링크 내부 `.label` | `채용`, `봉사`, `장학`, `비교과·행사` 등 |
| 제목 | 링크 복제에서 `.label` 제거 후 텍스트 | 분류 문자열이 제목 앞에 섞이지 않도록 처리 |
| 등록부서 | `.notice_col4` | 교수학습혁신센터 등 |
| 다음 페이지 | `nav.board-pagination a.next-btn` | `/공지사항/page/2/`, 마지막 페이지 버튼 제외 |

필수 컨테이너·행·날짜·제목·부서 칸의 누락/중복, 날짜 오류, 빈 제목/부서, 잘못된 URL은 정상 빈 결과로 처리하지 않고 `parse` 오류와 단계·안전한 원인을 기록한다. 분류 미식별은 원본 문자열을 유지하고 `unknown`으로 매핑한다. 상태 칸의 누락은 오류지만 칸이 존재하고 값만 없으면 `unknown`이다.

**실제 상세 본문 선택자는 미확인**이다. `detail.synthetic.html`의 `.fixture-notice-body`는 합성 Fixture의 DOM에서만 검증했다. 학교 상세 페이지의 선택자로 추정하여 하드코딩하지 않았다. `detail_body_selector`에는 실제 캡처를 검토한 좁은 선택자만 설정해야 한다. 비어 있거나 `body`, `html`, `main`, `article`, `*` 같은 전체 페이지 대체 선택자는 거절한다. 본문이 0개/여러 개/비어 있으면 오류다. 이미지뿐이고 추출 가능한 본문 텍스트가 없는 경우도 현재는 오류이며 OCR은 수행하지 않는다.

## 구현과 저장 규칙

- `SSUCatchCrawler.Crawl(context.Context, model.Source) ([]model.Content, error)`로 기존 인터페이스 구현, `ssu_catch` 타입 등록.
- 목록 페이지 최대 1, 상세 최대 3, 총 요청 최대 5(robots·실패·재시도·리다이렉트 포함), 동시성 1, 요청 시작 간격 최소 2초. 직접 호출의 과도한 한도도 코드에서 축소한다. 다음 페이지 URL을 파싱/검증하지만 MVP에서는 추가 목록을 요청하지 않는다.
- 공통 HTTP Timeout, Context 취소, robots, 동일 origin, DNS/IP 기반 SSRF, 리다이렉트 대상 검사 유지. robots가 HTML/challenge 응답이면 실패하도록 보강했다. 로그인·CAPTCHA 요소 발견 시 중단하며 우회하지 않는다.
- 제목·본문·원본 URL·원본 분류·표준 분류·부서·상태·게시일·첨부 메타데이터·해시·시각 필드를 모델에 추가했다. 기존 `source_id`는 기존 의미인 source의 BSON ObjectID를 유지한다. 게시글의 식별자는 URL의 slug다. `source_name`은 `ssu_catch`, `deadline`은 null.
- 게시일은 `YYYY.MM.DD`를 한국 시간 자정으로 해석하여 UTC 저장한다.
- 상세 엔진은 지정 영역만 복제하고 script/style/nav/header/footer/aside/form 등을 제거한다. 파일 확장자, download 속성, download.php 링크의 파일명/URL만 저장하며 링크를 요청하지 않는다. 실제 SSU 첨부 DOM은 미확인이다.
- SSU 상세 URL은 같은 origin 및 공지 경로만 허용하고, 분류·검색·페이지·추적 파라미터와 fragment를 없애며 **slug만 보존**한다. 공통 저장소는 utm_*, fbclid, gclid를 제거하고 다른 사이트의 식별용 쿼리와 %2F 같은 인코딩된 경로 경계는 보존한다.
- 기존 `original_url` Unique Index를 유지하고 정규화한 URL로 원자적 upsert한다. `_id`는 유지한다. 같은 해시일 때 본문/updated_at/최초 crawled_at을 바꾸지 않고 last_seen_at을 갱신한다. 해시가 바뀌면 변경 필드 및 updated_at을 갱신한다. `crawled_at`은 최초 성공 수집 시각이다.
- 해시는 공백 정규화한 제목·본문·부서·상태, 게시일, 정렬한 첨부 메타데이터를 대상으로 한다. 수집 시각과 DB ID는 제외한다.
- 여러 분류 등장에 대비해 기존 scalar `category`, `original_category`를 유지하면서 `categories`, `original_categories` 배열을 추가했다. 동일 URL의 분류 멤버십은 `$setUnion`으로 병합한다. 멤버십은 본문 해시에서 제외하므로 분류만 달라져도 같은 글을 불필요하게 내용 수정으로 처리하지 않는다. 같은 해시에서도 **새 분류 멤버십 추가**는 반영한다. 분류가 사라졌다고 추론하여 삭제하지 않는다. 서로 다른 slug는 제목/본문이 같더라도 합치지 않는다.
- 저장 실패 Job은 `error_code=storage`, `error_stage=save`; 파싱 실패와 접근 제한은 각각의 단계/코드 기록. BulkWrite 전체가 트랜잭션은 아니므로 배치 도중 실패 시 일부 저장될 수 있다는 기존 제약은 유지한다.

## REST API와 실제 MongoDB 로컬 검증

기존 Bearer 인증 유지. 아래 결과는 **로컬 HTTP Fixture → 실제 MongoDB → 기존 Job Runner → 실제 HTTP API** 통합 테스트 결과이며 실사이트 수집 결과가 아니다.

| 항목 | 결과 |
|---|---|
| 실제 서버 목록 파싱 | 캡처된 15행 메타데이터 파싱 성공; DB 적재 안 함 |
| 실사이트 상세/완전한 공지 수집 | 0건 |
| 실사이트 공지 MongoDB 저장 | 0건 |
| 로컬 통합 테스트 신규 저장 | 2문서 |
| 로컬 category=scholarship | 200, 1문서 |
| 로컬 category=academic | 200, 1문서 |
| 로컬 source=ssu_catch | 200, 2문서 |
| 인증 없음 | 401 |
| 동일 Fixture 재수집 | 문서 수 2 유지, ID/updated_at 유지, last_seen_at 증가 |
| 수정 Fixture 재수집 | 문서 수 2 유지, 본문·해시·updated_at 변경 |
| 분류 중복 저장 별도 테스트 | 1문서에 academic+scholarship 병합; 두 필터에서 모두 조회 |
| 동시 저장 별도 테스트 | 동일 URL 8개 동시 upsert 후 1문서 |
| 정책 미승인 Job | failed, policy_unreviewed/policy, 추가 HTTP 0회 |
| 금지 링크 | download.php URL은 메타데이터로만 저장; 다운로드 0회 |

로컬 통합 Fixture는 실제 목록에서 2행을 추출한 후 API 검증을 위해 분류를 장학/학사로 바꾸고 **본문은 합성**했다. 이는 해당 실제 글의 분류/본문 수집 성과가 아니다. 저장된 필드 예시는 별도 `local-api-result.json`에 `mode=local_fixture_only`로 표시했다. 테스트 MongoDB는 `127.0.0.1:27028`에 별도 데이터 디렉터리로 실행했고 DB 이름은 매번 `crawltest_<ObjectID>`를 생성했다. 테스트 종료 시 해당 임시 DB만 Drop했다. 기존 데이터베이스/계정/컨테이너는 수정하지 않았다. 최종 결과는 테스트 로그에 따른다.

## 테스트 실행

기존 프로젝트의 캐시를 사용했다. 비밀값 없이 테스트 전용 URI만 설정했다.

```sh
export TEST_MONGODB_URI=mongodb://127.0.0.1:27028
export GOMODCACHE="$PWD/.cache/mod"
export GOCACHE="$PWD/.cache/build"
go test -count=1 -v ./...
go test -race -count=1 -v ./...
go vet ./...
go build ./...
```

상세 결과는 전달한 `go-test.log`, `go-test-race.log`, `go-vet.log`, `go-build.log`를 참조한다. MongoDB 통합 테스트는 URI 미설정 시 skip하지만 이번 검증에는 URI를 설정했다. 외부 도메인 자동 수집을 하는 테스트는 없다. SSU 크롤러 로컬 네트워크 한도 테스트는 최대 5회/최소 2초를 실측한다. End-to-end 3회 실행의 로컬 요청은 회당 4회, 총 12회이며 실사이트 요청 원장과 별도다.

## 변경 파일

- `internal/crawler/ssu_catch.go`, `ssu_catch_test.go`: 전용 수집기, 실제 목록 파서, 검증 선택자 기반 상세 엔진, 정책/범위/카테고리/날짜/URL/본문/취소 테스트.
- `internal/crawler/testdata/ssu_catch/`: 실제 목록·robots, HTTP 원장, 합성 상세 HTML, 출처/미검증 범위 설명.
- `internal/crawler/builtins.go`: Registry 등록.
- `internal/crawler/fetcher.go`, `fetcher_test.go`: robots 재사용 조회, 실행별 요청 원장, HTML robots fail-closed 및 테스트.
- `internal/model/model.go`, `content.go`, `content_test.go`: 호환 필드·분류 배열·해시·URL·수동 전용 동작.
- `internal/config/sources.go`, `sources_test.go`, `configs/sources.ssu-catch.json`: 정책/선택자와 MVP 제한 설정, 기본 비활성.
- `internal/repository/contents.go`, `mongo.go`, `sources.go`, `ssu_mongo_integration_test.go`: 해시 비교 원자적 upsert, 시각 및 분류 병합, 조회/인덱스, 정책 설정 보존, 실제 MongoDB 검증.
- `internal/api/api.go`, `api_test.go`: source 이름 필터와 인증/입력 검증.
- `internal/service/runner.go`, `runner_test.go`: 저장 실패 단계/코드.
- `internal/integration/ssu_test.go`: Fixture 기반 실제 MongoDB·Job Runner·인증 API 전체 흐름.
- `README.md`, `docs/ssu-catch.md`: 동작 변경 및 검증/정책 기록.

시작 시 Git에는 기존 프로젝트 파일 전체가 untracked였다. 이번 작업에서 저장소 초기 커밋·추가·커밋·원격 푸시는 하지 않았다.

## 실제 수집 재개 전 남은 확인

1. 학교의 공식 이용 조건 또는 담당부서가 확인한 자동 수집·본문 저장/재제공 허용 범위, 보존 기간, 개인정보 처리 범위를 확보한다. robots 허용만으로 승인되었다고 표시하지 않는다.
2. 해당 범위 안에서 실제 상세 HTML을 캡처하고 본문/첨부 선택자를 검증한다. 사용자 제공 slug 예시도 실제 응답은 아직 확인하지 않았다.
3. 검증한 실제 상세 Fixture와 회귀 테스트를 추가한 뒤 정책 근거를 기록하고 `policy_approved`, `detail_body_selector`, `enabled`를 설정한다. `.fixture-notice-body`를 실사이트 설정에 넣지 않는다. `auto_enabled`는 계속 false로 유지한다.
4. 실제 소량 Job에서 1페이지·최대 3상세·총 5요청을 재확인하고 저장/API를 검증한다. 현시점에는 이 단계가 완료되지 않았다.
5. 기존 운영 데이터에 추적 쿼리 포함 URL이 이미 저장된 경우 정규화 후 충돌을 사전 조사해야 한다. 이번 작업은 운영 DB에 연결하거나 기존 문서를 대량 마이그레이션하지 않았다. 새로 저장되는 정규화 URL에는 고유 인덱스가 적용되지만 이전 비정규 URL과의 소급 중복 정리는 별도 작업이다.
6. 이미지 중심 본문 처리, 실제 첨부 컨테이너 검증, 분류 삭제/정정 정책, 여러 프로세스의 통합 rate limit 및 대규모 HTML 변경 대응은 후속 과제다.

## 최종 실행 결과

모든 명령의 종료 코드가 0이었다. go test와 race 검사 모두 TEST_MONGODB_URI를 설정하여 실행했으며 SKIP 없이 MongoDB 테스트를 통과했다. 테스트용 MongoDB 8.3.4(darwin/arm64)와 Go 1.27.3을 사용했다.

| 명령 | 결과 |
|---|---|
| go test ./... (실제 실행: -count=1 -v 추가) | PASS |
| go test -race ./... (실제 실행: -count=1 -v 추가) | PASS, data race 없음 |
| go vet ./... | PASS |
| go build ./... | PASS |

## 후속: 사용자 제공 상세 HTML 검증 준비

기존 위 기록은 유지한다. 2026-11-06 후속 작업 복사본에 실제 HTML과 검토한 기대값을
받는 테스트 구조를 추가했다. 상세 내용은 [Fixture 안내](detail-fixture.md)를 참조한다.
실제 상세 HTML이 없어 선택자를 확정하지 않았고 실제 상세 회귀 테스트와 실제 상세
MongoDB 검증은 아직 대기 상태다. 새로운 전체 검증 결과는 작업 출력의
`validation/`에 있으며 기존 실행 로그는 그 아래 `previous/`에 보존했다.

## 후속 완료: 사용자 제공 실제 상세 Fixture

사용자가 제공한 HTML의 제목·본문·첨부 영역을 실제 DOM으로 검증했다.
등록부서는 상세에 없으므로 기존 실제 목록의 같은 slug에서 학생지원팀으로 확인했다.
본문은 이미지 2개와 안내 링크 중심이며 OCR 없이 추출 가능한 텍스트만 저장한다.
검토 선택자, 원본 파일 해시, 실제 Fixture 회귀 테스트와 MongoDB 테스트의 범위는
[현재 상세 Fixture 결과](detail-fixture.md)에 기록했다. 이전의 '실제 HTML 미제공'
설명은 이 파일 앞부분에 남긴 작업 이력이다. 정책 및 실사이트 비활성 설정은 유지한다.
