# 웹서비스 구조

## 범위와 기술 선택

기존 Go net/http + MongoDB Driver v2를 재사용했다. 별도 프런트엔드가 없어 HTML/CSS/JavaScript를 Go embed로 제공한다. 상태는 작은 필터 객체와 Fetch/AbortController로 관리한다. Node/React 빌드 의존성이나 새 외부 UI 라이브러리가 필요하지 않다. 선택적인 Playwright E2E만 Node를 사용한다.

`cmd/web` → `internal/web` (인증·HTTP·정적 UI) → `internal/repository/notices.go` → 기존 Atlas.

- `internal/notices/`: 필터 검증, 공개 DTO, Asia/Seoul 일정 상태
- `internal/web/web.go`: 인증된 GET API, 세션, 보안 헤더
- `internal/web/static/`: index.html/app.css/app.js/favicon.svg
- `internal/repository/notices.go`: 서버측 검색/필터/집계/정렬/페이지네이션
- `cmd/web-audit`: 수동 읽기 전용 체크섬·인덱스·쿼리 계획 감사
- `scripts/web-e2e.cjs`: 실제 Chrome UI 검증

기존 Crawler/AI/Runner와 `internal/api`의 관리자 API는 수정하지 않았다. `cmd/web`는 기존 서버 시작 경로의 인덱스 생성/Source 조정/스케줄러 등록을 실행하지 않는다. `OpenReadOnly`로 연결하며 UI에는 쓰기·수집·생성 API가 없다.

## 쿼리와 성능

검색어는 최대100자, regexp.QuoteMeta로 이스케이프하여 제목/요약/본문에서 한글 부분 문자열을 검색한다. Source·category·status·has_schedule 조건과 기존 완료/만료 노출 정책을 합성한 `$match`가 먼저 적용된다. 필요한 경우만 일정에서 신뢰 가능한 미래 정렬 키를 계산하고 `$facet`에서 count와 sort/skip/limit를 실행한다. 응답은 최대50건, 목록에는 본문을 포함하지 않는다. 모든 공고를 브라우저에 내려 보내 필터링하지 않는다.

기존 source_name+crawled_at, category+published_at, categories+published_at, original_url unique 인덱스를 조사해 재사용했다. 실제 source 조회 explain에서는 IXSCAN과 서버측 SORT를 확인했다. 새 인덱스 쓰기는 수행하지 않았다. 현재226건 규모에서 한글 부분 문자열 regex 검색은 적절하지만 임의 부분 문자열은 일반 B-tree로 가속되지 않는다. 데이터가 커지면 별도의 한국어 검색/Atlas Search 및 source_name+published_at 인덱스를 검토해야 한다. 현재 컬렉션을 매 요청 클라이언트 메모리로 읽는 구현은 없다.

HTTP 요청 context10초, Mongo client timeout10초, 페이지1~10000/limit1~50, HTTP read/write timeout, DB projection 허용 목록을 사용한다. 소스/업데이트 metadata는 서버측 그룹 집계로 반환한다.

## 인증과 보안

기존 ADMIN_TOKEN의 Bearer 인증을 유지하고 웹 로그인 후에는 난수32바이트 기반의 서버 메모리 세션을 발급한다. 쿠키는 HttpOnly/SameSiteStrict, TLS에서는 Secure다. 세션8시간/서버 재시작 시 재로그인하며 브라우저 저장소에 토큰을 쓰지 않는다. 로그인/로그아웃은 동일 Origin을 요구하고 실패 횟수를 제한한다. 웹 명령은 loopback 주소만 허용하고 외부 배포는 하지 않는다.

HTML/JS는 자격 증명과 raw_response를 포함하지 않는다. API는 사용자용 DTO만 반환한다. 본문과 AI 문장은 textContent/pre로 렌더링해 HTML을 실행하지 않는다. https·비자격증명 URL만 원문 링크로 허용하고 noopener/noreferrer를 설정한다. CSP self, object-src none, base-uri none, frame-ancestors none, nosniff와 no-referrer를 사용한다. DB 오류·URI·스택은 응답에 포함하지 않는다.

이 로컬 MVP는 기존 관리자 토큰으로 접근한다. 외부 학생 서비스로 운영할 때에는 관리자 토큰을 학생에게 공유하지 않고 별도 읽기 전용 인증/사용자 체계와 HTTPS를 설계해야 한다.

## 일정 상태와 품질

공고의 원본 status/classification/lifecycle와 일정 상태는 독립이다. 현재 Asia/Seoul 날짜·시각을 사용해 신청 예정/중/마감, 행사 예정/진행 중/당일/종료, 일정 미상, 원문 확인 필요를 표시한다. 범위 없는 단일 행사일은 해당 달력 날짜가 경과했는지로 구분한다.

원본의 날짜/시각/부분 날짜/precision/timezone/parent/evidence는 응답에 보존한다. 날짜만 있는 신청 기한은 그 날짜가 끝나기 전까지 유효하지만 화면/DB에00:00이나23:59를 추가하지 않는다. 내부 비교/정렬에서만 날짜 경계를 사용한다. confirmed full-date이며 quality verified/partial이고 review_required=false인 일정만 미래/마감 정렬의 확정 후보로 사용한다. 연도 미상/월 정밀도/조건부/blocked 일정은 확정 마감 정렬이나 upcoming 후보에서 제외한다. 정렬 정보가 없는 공고도 결과 끝에 유지한다.

공고 검토 플래그가 있어도 전체 공고를 숨기지 않는다. 일정·요약·참여 대상 각각의 검토 상태를 표시하고, 대상이 불확실하면 지원 자격을 확정 표시하지 않는다. 분석 이력 ID/input/model/extraction 버전이 없으면 분석 미완료로 표시한다. 기존 deadline/lifecycle를 새 일정으로 변경하지 않는다.

## 테스트

Go 단위/회귀 및 loopback 격리 MongoDB 테스트, 실제 Atlas를 사용하는 인증 HTTP 테스트, Chrome Playwright E2E를 분리했다. 375/768/1440px에서 overflow 검사와 스크린샷을 남겼다. 오류503은 테스트 브라우저 route에서만 주입한다. 학교 요청·Gemini 호출·Atlas 데이터/인덱스 쓰기는0회다. 자세한 실행 결과는 outputs/phase2-report.md를 참고한다.
