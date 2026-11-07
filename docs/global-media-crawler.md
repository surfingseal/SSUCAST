# 글로벌미디어학부 실제 수집 계약

조사일: 2026-11-07 Asia/Seoul. 대상: https://media.ssu.ac.kr/board/notices

## 실제 응답과 선택한 방식

초기 Next.js HTML은 목록/본문 데이터가 없는 골격이다. 실제 연결된 JS module92836에서 공개 API origin을 확인했고, 브라우저 공개 요청과 비인증 직접 HTTP 응답을 대조했다. 추측한 API나 로그인 토큰은 사용하지 않았다.

- API origin: `https://3-37-127-112.sslip.io`
- 목록: `GET /v1/board/?menuId=136&size=15&page=0&content=`
- 목록: `/data/boards`, 페이지수: `/data/totalPage`
- UI 1페이지 = API page0. 첫 목록15행 중 고정10행, 일반5행을 확인했다.
- 상세: `GET /v1/board/{id}`
- 원문 링크: `https://media.ssu.ac.kr/board/notices/{id}`. 목록 복귀/검색 쿼리는 저장하지 않는다.
- 필드: `/data/id`, `title`, `content`, `userName`, `createdAt`, `attachments`, `accessRole`.
- 공개 범위: `ROLE_GUEST`. 제한/삭제/비공개 응답은 실패로 기록한다.

본문은 HTML CSS 선택자로 추측하지 않고, JSON 문자열 안의 Lexical `editorState.root.children`을 실제 캡처에 따라 순회한다. 같은 문단의 inline text는 이어 붙이고 문단/목록/표 경계는 줄바꿈한다. 메뉴·푸터·조회수·로그인 UI는 본문에 들어가지 않는다. 이미지 노드는 미분석 상태로 구분하고 다운로드하지 않는다.

대표 실제 공고1814/1812/1810의 제목·작성자·게시일·본문 핵심 문장을 Fixture 회귀 테스트로 검증했다. 실수집 Fixture는 synthetic 계약 Fixture와 별도 경로 `internal/crawler/testdata/global_media/current/`에 있다.

## 접근 정책

웹사이트 robots는 HTTP200 `User-Agent:* / Allow:/`. 실제 연결 API robots는404였다. 서비스 정책 링크 `/board/service_policy`는 홈으로 이동해 독립된 이용약관 본문을 확인하지 못했다. 이를 명시적 포괄 허가로 해석하지 않는다. 사용자가 승인한 대학 과제 범위에서 공개 ROLE_GUEST 응답만 수동 수집했고, 인증·쿠키·CAPTCHA 우회는 하지 않았다.

신규 Source는 `ssu_global_media`, hybrid, enabled=false, auto_enabled=false이며 수동 공개 접근 조사 근거를 policy_approval_basis에 기록했다. 다른 Source나 `.env`는 변경하지 않았다. 주기 실행/대규모 재수집 전에는 접근 정책 변경을 다시 확인해야 한다.

요청 시작 간격은 2초 이상, 동시성1, 최근3개월, 상세최대100, 재시도0. robots/DNS/TLS 검증은 두 origin별 공통 Fetcher를 사용한다. 오류 반복 시 멈추며 제한을 우회하지 않는다.

## 실행 결과

기준일2026-11-07, cutoff2026-08-07 KST. 실제 수집14건, 본문 성공14, 파싱 실패0. 수집 세션 요청18회(robots/목록2페이지/상세14), 구조 조사 요청은 별도 캡처/로그에 보존했다. 고정 공지는 ID로 중복 제거하고 기간 밖 고정5건은 지속 유효 후보로 별도 기록했으며 Atlas에 새로 넣지 않았다.

API 날짜는 `YYYY-MM-DD HH:mm:ss`로 timezone 없는 문자열이다. 사이트 표시와 동일한 날짜를 확인하고 Asia/Seoul wall time으로 해석한 정책을 metadata에 남겼다. 서버 자체 timezone이 선언됐다고 주장하지 않는다.

현재14건은 모두 hasAttachment=false 및 attachments=[]로, 첨부 메타데이터 누락이 없다. 기간 밖 공고1781에서는 첨부ID가 존재했고 공개 `/v1/file/2313`가500을 반환했다. 해당 상황에서는 ID만 보존하고 `partial_attachment_metadata`로 남긴다. 이름/URL을 지어내거나 성공으로 처리하지 않는다. 브라우저 DOM에 보인 파일은 다운로드하지 않았다. 이미지/OCR/첨부 다운로드는0회다.

## 재실행

```sh
go run ./cmd/collect-media --baseline /absolute/backup --output /absolute/new-capture --as-of 2026-11-07
```

기존 contents.json이 있는 경로에서는 학교 요청을 반복하지 않는다. 새 실행은 최신 백업의 source ID를 재사용해야 한다. `--as-of` 생략 시 현재 한국 날짜, cutoff는3개월 전이다. 수집 직후에는 로컬 JSON만 생성하고 Atlas 저장은 별도 검증/적용 단계로 분리한다.
