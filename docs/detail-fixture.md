# 사용자 제공 상세 HTML 검증

## 현재 결과

사용자가 제공한 36,251바이트 HTML을 변경 없이 `internal/crawler/testdata/ssu_catch/detail.actual.html`로 보존했다. `detail.actual.expected.json`에 파일 SHA256, 검토한 선택자 및 독립적으로 확인한 기대값을 기록했다. 브라우저 저장 시각은 제공되지 않았으므로 `captured_at`은 빈 문자열이며 `capture_note`에 그 사유를 남겼다. 원본 URL은 HTML의 `input#copyurl` 값으로 확인했다. 외부 HTTP 요청은 하지 않았다.

| 항목 | 확인 결과 |
|---|---|
| 제목 | `#contents .bg-white > h1` |
| 본문 | `#contents .bg-white > hr + div` |
| 첨부 목록 | `#contents .bg-white > hr + div > ul.download-list` |
| 등록부서 | 상세 HTML에는 없음. 기존 목록 Fixture의 같은 slug 행에서 `.notice_col4`로 확인 |
| 제목 기대값 | [한국장학재단] 청년지원정보 통합제공 플랫폼 ‘통학로’ 안내 |
| 등록부서 | 학생지원팀 |
| 원본/표준 분류 | 장학 / scholarship |
| 게시일 | 2026.11.03 (목록), 2026년 11월 3일 (상세) |
| 상태 | 목록의 빈 상태 칸 → unknown |
| 추출 가능한 본문 | ※ 나에게 맞는 지원정보 찾기 |
| 첨부 메타데이터 | 링크된 JPG 2개, 명시적 다운로드 목록 PDF 1개 |

본문 영역에는 이미지 2개가 들어 있고 이미지의 `alt`는 비어 있다. 이미지 내부 문구는 HTML 텍스트가 아니므로 추출하지 않는다. OCR이나 이미지 다운로드는 수행하지 않았다. 따라서 저장 본문은 텍스트 링크 한 줄이며 이미지의 정보를 완전히 텍스트화했다고 주장하지 않는다. 첨부파일과 이미지 링크는 이름/URL 메타데이터만 저장한다.

첨부 목록은 본문 안에 있다. 파일 메타데이터를 수집한 뒤 `ul.download-list`를 본문 텍스트에서 제외하여 파일명이 본문에 섞이지 않도록 했다. 기존 합성 Fixture의 동작은 유지했다. 헤더·이전/다음 글·사이드바·공유 UI·푸터는 본문 선택자 밖이다.

등록부서 선택자를 만들거나 제목의 기관명을 등록부서로 추정하지 않았다. 검토 JSON의 `department_source="list.actual.html"`과 `list_sha256`으로 목록 근거를 명시하고, 상세 URL과 같은 slug를 가진 목록 행의 제목·부서·분류·게시일·상태를 대조한다. 상세 DOM에 있는 부서를 검사하는 다른 Fixture는 기존처럼 `selectors.department`를 사용할 수 있다.

## 실행

```sh
cd /Users/surfing_seal/Documents/Codex/2026-11-06/ssucatchcrawler-mvp-1-2-html-fixture/outputs/HCI
SSU_REQUIRE_ACTUAL_DETAIL=1 go test -count=1 -v ./internal/crawler -run '^TestSSUActualDetail'
TEST_MONGODB_URI=mongodb://127.0.0.1:27029 SSU_REQUIRE_ACTUAL_DETAIL=1 go test -count=1 -v ./internal/repository -run '^TestSSUActualDetailMongo$'
```

Go 1.27.3 이상을 사용한다. MongoDB 주소는 별도 테스트 인스턴스에 맞춘다. 이번 검증용 인스턴스는 검증 후 종료했다. 저장 테스트는 임의 `crawltest_...` DB에서 저장·장학 카테고리 조회·같은 글 재저장을 검사하고 해당 테스트 DB만 삭제한다. URI가 없으면 MongoDB 테스트는 SKIP하므로 저장 검증으로 간주하지 않는다.

테스트는 파일 해시, 원본 URL, 검토 표시, CSS 문법, 노드 수, 정확한 본문, 첨부 이름/URL, 목록과 상세의 동일 공지 메타데이터를 확인한다. 실제 DOM의 본문 경계를 제거하거나 상세 문서를 중복한 경우 파서가 실패하는지도 검사한다. `SSU_REQUIRE_ACTUAL_DETAIL=1`은 두 실제 파일의 누락을 실패로 처리한다.

한 실제 페이지의 DOM을 검증했다. 이미지 전용 공지, 다른 상세 템플릿, 별도 첨부 영역 등 모든 변형을 검증한 것은 아니다. 본문 텍스트가 전혀 없으면 기존대로 파싱 오류가 발생한다.

## 설정 및 보존

원본 `/Users/surfing_seal/Documents/ChatGPT/HCI`는 변경하지 않았다. 이번 변경은 `outputs/HCI` 작업 복사본에 있다. 기존 검증 로그는 출력 `validation/previous/`와 `validation/pre-detail/`에 보존했다.

`configs/sources.ssu-catch.json`은 원본 그대로다. `policy_approved=false`, `enabled=false`, `auto_enabled=false`, `detail_body_selector=""`를 유지한다. 검토 JSON의 선택자와 `selectors_reviewed=true`는 로컬 테스트의 근거이며 네트워크 수집 승인이나 실사이트 실행 설정이 아니다.

## 추가 HTML 제공 방법

Chrome에서 다른 상세 페이지를 직접 열고 `⌘S` → **웹페이지, HTML만**으로 저장한다. 전체 HTML과 원래 페이지 URL·저장 시각을 함께 제공하면 된다. PDF/MHTML/스크린샷은 DOM 선택자 검증에 사용할 수 없다. 새 자료는 기존 실제 Fixture를 덮어쓰지 않도록 별도 이름으로 제공한다. 이미지·첨부파일을 다운로드할 필요는 없다.

## 실제 Fixture 기반 REST API 검증 추가

`TestSSUActualFixtureMongoAPI`는 위 실제 HTML을 기존 상세 파서로 처리한 문서 1개를
별도 MongoDB에 저장하고, 기존 `api.API.Handler`를 로컬 HTTP 서버에 연결하여 조회한다.
`category=scholarship`은 1개, `category=academic`은 0개, `source=ssu_catch`는 1개이며
source/category 복합 필터도 검사한다. 반환된 제목·본문·부서·분류·원본 URL·해시·첨부
메타데이터를 검토한 실제 Fixture와 대조한다. Bearer 인증이 없으면 401인지 확인한다.
실제 원본의 분류를 다른 카테고리로 바꿔 테스트하지 않는다.

검토 JSON의 `source_url`은 HTML에 들어 있던 원본 URL 그대로 보존한다.
브라우저 저장 시각은 미제공이다. 로컬 Fixture 파일의 파일시스템 저장 시각(mtime)은
`2026-11-06T10:47:51+09:00`으로 확인했다. 이 시각은 파일이 이 작업 폴더에 저장된
시각이며 브라우저 캡처 시각으로 해석하지 않는다.

```sh
TEST_MONGODB_URI=mongodb://127.0.0.1:27029 SSU_REQUIRE_ACTUAL_DETAIL=1 go test -count=1 -v ./internal/integration -run '^TestSSUActualFixtureMongoAPI$'
```

전체/race 검증의 실제 Fixture API 결과는 출력 `validation/go-test-actual-api.json` 및
`validation/go-test-race-actual-api.json`에 기록한다. 기존 Runner/API E2E 테스트는
합성 상세 본문을 사용하며 새 실제 Fixture API 테스트와 별개다. Crawler 인터페이스,
Repository, Job Runner, REST API 구현 및 정책 차단은 변경하지 않았다.
