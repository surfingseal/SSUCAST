# 글로벌미디어학부 Source — 2026-11-06 검증

## 현재 상태

`ssu_global_media` Crawler 인터페이스 구현과 레지스트리 등록, hybrid 분류, 작성자와 첨부파일 확장자 저장, 고정 공지 중복 제거, 제외 사유 로깅, MongoDB·Mock·REST API 테스트를 추가했다. **실제 목록/상세 데이터 API 응답은 아직 확보하지 않았다. 실사이트용 상세 파서 검증은 미완료이며 수집은 차단한다.**

기존 Fetcher의 robots, 동일 출처, SSRF, 재시도/요청 예산과 속도 제한을 재사용하고 Accept 헤더에 application/json을 추가했다. 기본 테스트와 서버 시작 경로에는 유료 모델 호출을 추가하지 않았다. 기존 인증·모델·분류 CAS·입력 해시·분류 이력을 유지한다.

## 실제 관찰한 구조

- URL: https://media.ssu.ac.kr/board/notices
- 서버 HTML은 Next.js App Router 화면과 `#board-skeleton-container`만 포함한다. 실제 게시물 행은 자바스크립트에서 생성한다. `main h1` 텍스트는 `학부 공지`다.
- 목록/상세 **게시물 CSS 선택자는 확정하지 않았다.** 위 두 선택자는 화면 제목과 로딩 영역일 뿐이다. 합성 상세 CSS `article[data-synthetic-notice] .synthetic-body`는 실사이트에 사용하지 않는다.
- HTML의 메뉴 정보: ID `136`, `link=/board/notices`, `layout=MULTIPLE`.
- 실제 공개 페이지 스크립트의 목록 훅: `GET /v1/board/`; UI `page` 기본 1, API `page`는 UI 값에서 1을 뺀 값; `size=15`, `menuId=136`, 검색은 `content` 쿼리다. 화면 검색 쿼리는 `search_keyword`다.
- 클라이언트가 참조하는 응답: `data.boards`, `data.totalPage`; 행 필드: `id`, `title`, `userName`, `createdAt`, `content`, `isPinned`, `hasAttachment`. 이는 **클라이언트 코드 관찰**이며 실제 API 응답 검증과 구분한다.
- 행 클릭의 상세 라우트: `/board/notices/{id}?previous_page=...&previous_search_keyword=...`. 저장 URL은 ID 경로만 남긴다. 게시물 ID로 페이지 간 중복을 제거하고 `isPinned`를 메타데이터에 보존한다. 실제 서비스가 고정 공지를 어느 페이지에 반복 배치하는지는 아직 확인하지 않았다.
- `<redacted>` 내용은 클라이언트에서도 비공개 처리한다. 수집기도 해당 행의 상세 요청을 생략한다.
- 공개 스크립트에 일부 에디터 내용의 이중 JSON 디코딩 로직이 있으나 실제 상세 본문 및 첨부파일 스키마는 확인하지 않았다. category 필드 부재도 단정하지 않는다. 따라서 hybrid 전략으로 등록한다.
- API **서버 주소**, 상세 API 엔드포인트, 상세 DOM/JSON 필드와 첨부파일 필드는 미확인이다. `/v1/board/`를 학교 웹 호스트에 임의로 요청하지 않았다. 쿠키/토큰 없는 API 접근 허용 여부 역시 미확인이다.

## 접근 정책과 요청 기록

robots.txt는 `User-Agent: * / Allow: /`이다. 이용약관·개인정보·쿠키 링크가 모두 `/board/service_policy`를 가리키지만 요청하면 HTTP 307로 홈페이지로 이동했다. 저장된 `policy.actual.html`은 **약관 문서가 아니라 리다이렉트된 홈페이지**다. robots 허용을 수집 승인으로 취급하지 않는다.

이번 구조 조사 직접 HTTP 요청은 **7회**: robots 3회, 목록 HTML 1회, 약관 링크 1회, 리다이렉트 홈페이지 1회, 공개 목록 JS 1회. 게시물 API/상세/이미지/첨부 다운로드는 모두 0회다. 별도 웹 검색 1회는 검색 서비스 요청이며 이 직접 HTTP 장부에 포함하지 않는다. 테스트 중 학교 요청과 실제 Vertex 요청은 모두 0회다.

원본은 `internal/crawler/testdata/global_media/`에 저장했다. `requests.json`에 요청별 URL·UTC 시작 시각·상태가 있고 `provenance.json`에 크기·SHA256·확인/미확인 항목이 있다. 원본 JS를 실행하거나 브라우저 보안을 우회하지 않는다.

## Source와 실행 차단

`configs/sources.university.json`은 기존 SSU 설정 그대로와 신규 Source를 함께 담는다. 기존 `configs/sources.ssu-catch.json`과 `.env`는 변경하지 않았다. 새 설정을 선택하려면 원본 프로젝트 디렉터리에서 실행한다.

```sh
CRAWL_SOURCES_FILE=configs/sources.university.json go run ./cmd/server
```

이 명령은 Source 등록용 설정 선택이며 실제 수집을 승인하지 않는다. 서버 시작 시 기존 ApplySources가 두 Source를 upsert한다. 환경의 `DEMO_MODE=false`, 인증 및 MongoDB 설정은 기존 값을 사용한다. 신규 설정은 `name=ssu_global_media`, `display_name=숭실대학교 글로벌미디어학부`, `classification_strategy=hybrid`, `enabled=false`, `auto_enabled=false`, `policy_approved=false`, 페이지 1·공고 1·총 요청 4·동시성 1·간격 2초다. 자동 AI 설정도 기존 false를 유지한다. 기존 SSU는 계속 source 전략이다.

기본 내장 `internal/crawler/global_media_review.json`은 `reviewed=false`다. 정책 미승인이면 `policy_unreviewed`, 정책만 승인하고 실 응답 검토가 없으면 `schema_unverified`를 반환하며 **네트워크 요청은 0회**다.

실제 응답 확보 후 API 주소와 상세 엔드포인트, JSON Pointer, 본문 포맷/선택자, 첨부 필드, 카테고리 존재 여부를 검토 JSON에 기록하고 `evidence_kind=actual`로 검증해야 한다. 현재 Fetcher는 동일 출처만 허용하므로 별도 API 호스트가 발견되면 호스트 정책과 robots 적용을 별도로 검토해야 하며 이 구현은 교차 출처를 자동 허용하지 않는다. `synthetic` 검토 계약은 명시적인 로컬 Fixture 서버에서만 실행할 수 있다.

## 파싱·제외·분류

- `author`를 Content에 선택 필드로 추가했다. 작성자 변경은 content hash에 반영한다. 기존 작성자 없는 문서의 해시 직렬화는 유지한다.
- 첨부파일은 이름·URL·확장자만 저장한다. 본문 HTML은 지정된 좁은 영역에서 메뉴·스크립트·푸터를 제거한다. 잘못된 포맷, ID 불일치, 누락/중복 본문은 오류로 처리한다.
- 기존 공식 `채용`·`교원채용` 제외를 유지한다. 원본 분류가 없거나 일반적인 게시판에서는 명시적인 직원 채용/교원 초빙 표현을 제목·본문에서 탐지한다. `모집` 자체는 제외 근거가 아니다. 대학원 신입생, 연수/행사 참가자, 채용설명회, 현장실습은 유지하는 테스트가 있다. 이 규칙은 보수적인 텍스트 규칙이며 모든 문맥을 의미적으로 판정하는 모델이 아니다.
- 제외 로그는 URL과 `code/field/rule`만 남기고 제외 본문을 별도 컬렉션에 저장하지 않는다. 목록에서 확인되면 상세 요청도 생략하며, 본문에서 확인되면 저장·AI 분류 전에 제외한다. 공통 Runner/Repository/분류 결정도 동일 정책을 사용한다.
- 공식 카테고리가 유효하면 hybrid는 기존 원본 정규화를 사용한다. 없거나 불충분하면 기존 수동 분류 경로를 사용한다. 공식 원문은 AI 결과로 덮어쓰지 않는다.
- 기존 input_hash/taxonomy_version/model_version 중복 예약과 정책/본문 CAS를 재사용한다. 이미지·첨부파일을 분석하지 않은 상태와 짧은 본문은 기존 품질 게이트에서 needs_review 사유로 보존한다.

## Fixture와 검증 범위

`testdata/global_media/synthetic/`는 **전부 합성 데이터**다. `/synthetic/list`, `/synthetic/detail/{id}`, `fixture` 응답 필드와 상세 CSS는 테스트를 위해 만든 값이다. 실학교의 실제 응답처럼 소개하면 안 된다. 실제 정적 문서와 합성 게시물은 서로 다른 디렉터리/검토 상태로 관리한다.

합성 2페이지에서 고정 공지 1건이 반복되고 일반 공지, 채용/교원 초빙, 비공개 글, 본문 채용 공지를 포함한다. 3건 파싱, 제외 3건, 비공개 상세 생략, 반복 고정 상세 1회, 파일 다운로드 0회를 검증한다. 이 테스트의 로컬 HTTP 7회는 실학교 조사 HTTP 7회와 별개의 수치다.

MongoDB 테스트는 임시 `crawltest_*` DB에 합성 공지 2건을 저장하고 중복 문서 0건을 검증한 뒤 삭제한다. Mock 2회를 실행한다. 미분석 이미지/파일이 있는 공지는 raw `extracurricular` 후보를 이력에 남기고 최종 unknown/needs_review로 저장한다. 충분한 텍스트가 있는 공지는 international/classified로 저장하고 API 카테고리 조회 200을 검증한다. Mock 토큰은 실제 사용량이 아니다. 실제 운영 DB 문서/분류 이력은 삭제하지 않는다.

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
```

MongoDB 통합 테스트는 로컬 테스트 DB를 가리키는 `TEST_MONGODB_URI`가 필요하다. 없으면 해당 테스트는 skip한다. 이번 검증에서는 URI를 명시하여 실제 로컬 MongoDB 저장까지 실행했다. 기본 테스트는 외부 학교/유료 모델을 호출하지 않는다.

## 실제 Vertex 1회 수동 테스트

기존 `cmd/classify-one`을 그대로 재사용한다. 기존 인증/프로젝트/모델/사전 점검과 최대 1회·재시도 0 제한을 적용한다. 먼저 이미 저장된 **승인된 학과 공고 하나**를 미리 본다.

```sh
go run ./cmd/classify-one -content-id <CONTENT_ID> -source-id <SOURCE_OBJECT_ID>
```

이 명령은 정책 미리보기만 수행하고 Vertex 클라이언트를 만들지 않는다. 확인 후에만 다음 명령으로 실제 호출한다. 이번 작업에서는 아래 명령을 실행하지 않았다.

```sh
go run ./cmd/classify-one -content-id <CONTENT_ID> -source-id <SOURCE_OBJECT_ID> -execute
```

원본 source가 needsAI인 경우에만 Gemini를 호출하고, 중복 예약이면 호출을 재사용/거절한다. SSU source 전략이면 AI를 호출하지 않는다. 서비스 계정·키·프로젝트 ID를 바꾸거나 출력하지 않는다. 합성 Fixture 결과를 실제 Gemini 성공으로 저장하지 않는다.

## 실제 수집 전 필요한 자료

수집 허용 근거와 읽기 전용 공개 API 여부를 먼저 확인해야 한다. 본인이 정상 접근 가능한 브라우저에서 DevTools → Network → Fetch/XHR로 목록과 상세 한 건의 요청 URL·응답을 확인할 수 있다. 로그인/제한 표시가 있으면 우회하지 않는다. 공개 읽기 요청만 대상으로 한다.

필요 시 **응답 본문만** 저장해 `internal/crawler/testdata/global_media/list.api.actual.json`, `detail.api.actual.json`에 넣는다. 쿠키·Authorization·Set-Cookie·계정 정보가 들어 있는 HAR 전체는 저장하지 않는다. 응답에 개인정보가 있으면 공유 전에 검토한다. 원본 URL, 저장 시각, 게시글 ID, 원본 파일 해시와 예상 제목/본문/작성자/날짜/첨부 메타데이터를 검토 JSON에 함께 기록한다.

DOM으로만 확인할 수 있다면 렌더링 완료 후 DevTools Elements에서 실제 게시물 컨테이너를 확인하고 `document.documentElement.outerHTML` 결과를 로컬 `detail.rendered.actual.html`로 저장한다. 브라우저의 일반 '페이지 소스 보기'나 HTML 저장만으로는 Next.js 로딩 화면만 남을 수 있다. 자료가 확보되기 전에는 실제 상세 선택자를 추측해 채우지 않는다.
