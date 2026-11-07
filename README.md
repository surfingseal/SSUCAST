# SSUCAST · SSU 공지 모아보기

숭실대학교 **슈캐치와 글로벌미디어학부 공지사항을 통합 조회하는 대학 과제용 MVP**입니다. 공고 검색·필터와 함께 AI 요약, 모집·행사 일정, 이미지 근거 및 학교 원문 링크를 제공합니다.

**배포 주소:** [ssucast.onrender.com](https://ssucast.onrender.com) · **인증:** 기존 `ADMIN_TOKEN`으로 로그인

프런트엔드는 Go 서버에 포함된 HTML/CSS/JavaScript입니다. 별도 프런트엔드 빌드나 Node 서버 없이 `cmd/web` 하나로 실행합니다. 배포된 웹은 Atlas에 저장된 데이터를 조회하며, 접속할 때 크롤링하거나 Gemini를 호출하지 않습니다.

## 주요 기능

- 제목·AI 요약·본문 검색, 출처·공식 카테고리·원본 상태 필터
- 최신순·확인된 모집 마감순·행사 예정순 정렬과 서버 페이지네이션
- 공고 상세보기: 요약, 확인된 참여 대상, 전체 일정, 원문 근거, 출처 링크
- 본문·이미지·병합 일정 구분 및 검토 필요 표시
- 날짜·시각·연도 미상·월 단위 정보를 보존하는 schedules v2
- 확인된 모집 마감과 학교 원본 완료 상태에 따른 기본 목록 숨김
- 일정이 없는 이유, 상대 신청기한, 상담 운영시간을 설명하는 선택적 점검 안내
- 375px·768px·1440px에서 검증한 반응형 화면

회원가입·개인별 알림은 구현하지 않았습니다. 현재 인증은 단일 접근 토큰과 웹 세션을 사용합니다.

## 현재 데이터 현황

**2026-11-07 확인한 스냅샷**입니다. 시각 경과와 이후 수집·분석에 따라 달라질 수 있습니다.

| 항목 | 건수 |
|---|---:|
| 전체 공고 | 226 |
| 슈캐치 / 글로벌미디어학부 | 212 / 14 |
| 요약·일정 분석 완료 공고 | 100: 슈캐치 86 + 학부 14 |
| 기본 목록에서 보이는 공고 | 77 |
| 학교 원본 상태 `완료`로 제외 | 126 |
| 확인된 모집·신청·제출·이수 마감으로 제외 | 23 |
| 전체 저장 일정 | 299 |
| 이미지 분석이 반영된 공고 | 31 |
| 이미지 근거가 연결된 일정 | 62 |
| 사용자 승인 정책으로 정상 취급하는 이미지 일정 | 22 |
| 연도 미상·충돌 등 검토가 남은 이미지 일정 | 40 |
| 분석 이력 `schedule_analyses` | 144: 기존 110 + 이미지 분석·검토 34 |

최신 분석 공고 100건의 모델은 `gemini-3.8-flash`입니다. 과거 Flash-Lite 이력 10건도 보존합니다. 이미지 근거 연결 일정 62개에는 신규 일정 59개와 기존 텍스트 일정에 근거를 병합한 3개가 포함됩니다.

기본 목록 77건 중 모집·행사 일정이 모두 없는 공고는 13건입니다. 일반 안내 4건, 다른 종류의 일정이 있는 공고 2건, 첨부파일 확인 1건, 연결 공지 확인 3건, 미분석 포스터 1건, 상대 기한·운영시간 안내 2건으로 구분했습니다. 날짜를 임의로 채우지 않았습니다.

## 로컬 실행

필수 환경은 **Go 1.27.3**과 접근 가능한 MongoDB입니다. Docker는 선택 사항이며 Node.js는 브라우저 테스트에만 사용합니다.

새로 시작하는 경우 저장소를 받은 뒤 `.env.example`을 참고해 `.env`를 준비합니다. 기존 프로젝트의 `.env`가 있다면 인증정보를 유지하고 덮어쓰지 않습니다. 실제 데이터와 인증정보는 저장소에 포함되지 않습니다.

```sh
git clone https://github.com/surfingseal/SSUCAST.git
cd SSUCAST
# 신규 환경에서 .env.example을 참고해 .env 준비

go mod download
WEB_ADDR=127.0.0.1:8088 go run ./cmd/web
```

접속: [http://127.0.0.1:8088](http://127.0.0.1:8088). 기존 작업 경로는 `/Users/surfing_seal/Documents/ChatGPT/HCI`입니다.

### 웹서비스 환경변수

| 변수 | 설명 |
|---|---|
| `MONGODB_URI` | MongoDB 연결 문자열. 백엔드에서만 사용 |
| `MONGODB_DATABASE` | 현재 데이터베이스는 `ssu_crawler` |
| `ADMIN_TOKEN` | 24바이트 이상의 접근 토큰. 웹 로그인·Bearer 인증에 사용 |
| `WEB_ADDR` | 로컬 기본 `127.0.0.1:8080`. 위 실행 예시는 `8088` |
| `WEB_DEPLOYMENT` | 로컬에서는 생략. Render에서만 `render` |
| `WEB_PUBLIC_ORIGIN` | 선택. 커스텀 도메인을 사용할 때 정확한 HTTPS Origin |
| `WEB_TIMING_REVIEW_FILE` | 선택. 일정 점검 안내 JSON의 비공개 파일 경로 |

현재 작업 디렉터리의 `.env`를 읽으며 **이미 설정된 프로세스 환경변수가 우선**합니다. 셸 명령 치환을 실행하지 않습니다.

`cmd/web`은 읽기 전용 Repository로 연결합니다. 수집기·Runner·AI·cron을 초기화하지 않고, 공고·분류·분석·Source·인덱스를 변경하지 않습니다.

## Render 배포

[배포 설정](render.yaml)과 [웹 전용 Dockerfile](Dockerfile.web)을 제공합니다. 실제 배포는 수동으로 실행합니다.

| Render 항목 | 설정 |
|---|---|
| Branch / Language | `main` / `Docker` |
| Region / Instance | `Singapore` / `Free` |
| Root Directory | 비워두기 |
| Docker Build Context Directory | `.` |
| Dockerfile Path | `Dockerfile.web` |
| Docker Command | 비워두기 |
| Health Check Path | `/healthz` |
| Auto-Deploy | `Off`: 현재 Blueprint 기준 |

Render의 비공개 환경변수에는 다음 4개를 설정합니다.

| Key | Value |
|---|---|
| `WEB_DEPLOYMENT` | `render` |
| `MONGODB_DATABASE` | `ssu_crawler` |
| `MONGODB_URI` | 기존 Atlas URI |
| `ADMIN_TOKEN` | 기존 웹 접근 토큰 |

**Render에는 로컬 `WEB_ADDR`를 설정하지 않습니다.** `PORT`와 `RENDER_EXTERNAL_URL`은 플랫폼 값을 사용합니다. Render 모드에서만 `0.0.0.0:$PORT`로 바인딩하고 HTTPS Origin 검사 및 Secure/HttpOnly/SameSite 쿠키를 사용합니다.

웹 조회에는 **Vertex AI 서비스 계정 키가 필요 없습니다.** `.env` 전체, Google 서비스 계정 JSON, 실험 결과와 DB 백업을 업로드하지 않습니다. 기본 `Dockerfile`은 기존 수집 서버용이므로 웹 배포에서는 반드시 `Dockerfile.web`을 선택합니다.

일정 점검 안내 13건도 표시하려면 로컬 `annotations.json`을 Render Secret File `timing-reviews.json`으로 등록하고 `WEB_TIMING_REVIEW_FILE=/etc/secrets/timing-reviews.json`을 지정합니다. 이 파일이 없어도 공고·요약·이미지 일정 조회는 가능합니다.

Free 인스턴스는 휴면 후 첫 접속이 느릴 수 있고, 재시작 후에는 다시 로그인해야 합니다. 자세한 설정과 Atlas 연결 확인은 [Render 배포 문서](docs/deployment.md)를 참고하세요.

## 일정과 숨김 정책

원본 공고 상태, 공식 카테고리, AI 분류, 일정 검토 상태와 만료 상태는 구분합니다.

- 슈캐치 원본 `status=완료`는 기본 목록에서 제외합니다.
- 확인된 모든 모집·신청·제출·이수 범위가 종료됐다고 판단할 때 만료 공고를 기본 목록에서 숨깁니다. 다른 차수나 미래 신청기간이 남아 있으면 유지합니다.
- 행사일만 지났다는 이유로 모집 마감으로 처리하지 않습니다.
- 연도·마감일이 불명확하거나 일정이 충돌하면 임의로 만료 확정하지 않습니다.
- 날짜만 있는 기한은 Asia/Seoul 기준 그날이 끝날 때까지 유효합니다. 원본에 없는 시각을 일정 필드에 생성하지 않습니다.
- 웹의 **만료 공고 포함** 옵션 또는 `include_expired=true`로 만료 공고를 다시 조회합니다. 학교 완료 상태 제외는 별도이며 `status=완료`로 조회할 수 있습니다.

숨김은 삭제가 아닙니다. 원본 `status`, `deadline`, `lifecycle_status`, 공고 본문과 기존 분석 이력은 유지합니다.

### 이미지 일정도 정상 일정으로 활용

이미지라는 출처만으로 검토 대상으로 남기지 않습니다. 명확한 달력 날짜·유효한 시각·정상적인 범위와 근거를 확인한 일정은 사용자 승인 정책을 통해 본문 일정과 같이 표시하고 마감 판단에 사용합니다. 현재 22개가 해당합니다.

연도 미상, 부분 날짜, 본문과 충돌, 낮은 판독 신뢰, 조건부·잠정 일정 등은 계속 검토 대상으로 구분합니다. 이미지에 없는 연도를 게시일이나 올해에서 임의 확정하지 않습니다.

정책은 `notice_policy` 파생 정보로 반영합니다. 원본 AI 응답과 검증 결과를 정상 결과로 위장해 덮어쓰지 않습니다. [이미지 출처 승인 정책](docs/image-source-policy.md)을 참고하세요.

### 빈 일정 안내

일반 안내, 이수·선발 일정, 첨부/연결 공지 확인, 운영시간 및 상대 기한을 구분해 표시합니다. 예를 들어 유고결석 이미지의 10일·보건결석 7일 안내는 개인별 기산점 없이 고정 마감일로 계산하지 않습니다.

점검 안내는 선택적인 로컬 파일에서 읽으며, 원문 해시와 최신 분석ID가 일치할 때만 적용합니다. 이 안내 자체는 마감 정렬·알림·만료 판정에 사용하지 않습니다. [동작과 파일 형식](docs/timing-review.md)을 참고하세요.

## 조회 API

`/api/notices`, `/api/sources`, `/api/categories`는 Bearer 토큰 또는 웹 로그인 세션이 필요합니다. 토큰을 URL에 넣거나 프런트엔드 코드에 저장하지 않습니다.

| 메서드 | 경로 | 설명 |
|---|---|---|
| GET | `/healthz` | 공개 프로세스 상태 확인 |
| GET | `/api/notices` | 검색·필터·정렬·페이지네이션 |
| GET | `/api/notices/{id}` | 요약·본문·대상·일정·근거·원문 링크 |
| GET | `/api/sources` | 출처와 현재 노출·저장 건수 |
| GET | `/api/categories` | 공통 카테고리 목록 |
| GET / POST / DELETE | `/api/session` | 로그인 상태 / 로그인 / 로그아웃 |

목록 매개변수는 `query`, `source`, `category`, `status`, `has_schedule`, `upcoming`, `include_expired`, `sort`, `page`, `limit`입니다. 페이지 크기는 최대 50개, `sort`는 `latest`, `deadline`, `event`입니다. 확인된 날짜만 일정 정렬과 upcoming 계산에 사용합니다.

상세 API는 원문 AI `raw_response`, DB 접속정보, 내부 분석 해시를 반환하지 않습니다. 선택적 `timing_review`는 날짜가 아닌 점검 설명입니다. [API 문서](docs/api.md)를 참고하세요. 해당 문서의 과거 실행 건수보다 이 README의 최신 스냅샷과 실제 API 응답을 우선합니다.

## 수집·AI 분석 구조

| 출처 | 수집 방식 | 카테고리 |
|---|---|---|
| 슈캐치 `ssu_catch` | 실제 목록·상세 HTML 파싱 | 공식 카테고리 정규화, `source` 전략 |
| 글로벌미디어학부 `ssu_global_media` | Next.js가 사용하는 공개 목록·상세 JSON 및 Lexical 본문 파싱 | 공식 분류가 불충분하면 AI를 사용하는 `hybrid` 전략 |

공통 카테고리는 `academic`, `scholarship`, `international`, `international_student`, `extracurricular`, `teaching`, `volunteer`, `other`이며 미확정은 `unknown`으로 구분합니다. 원본 카테고리와 AI 결과를 별도로 보존합니다. 채용·교원채용은 수집 제외 대상이며 단순 ‘모집’ 표현만으로 제외하지 않습니다.

슈캐치의 **카테고리 분류**에는 AI를 호출하지 않습니다. 공고 **요약·일정 분석**에는 Gemini를 사용합니다. 현재 모델은 `gemini-3.8-flash`이며 다른 유료 모델로 자동 대체하지 않습니다.

분석은 Structured Output → Go 검증 → Preview → 백업·복원 확인 → CAS 적용 → 재조회 순서로 진행합니다. 원본 응답과 검증 결과는 `schedule_analyses`에서 구분하고, `crawled_contents`에는 최신 조회용 결과를 연결합니다. 같은 입력·결과를 중복 적용하지 않고 구버전 덮어쓰기를 차단합니다.

이미지 보완은 저장된 공개 본문 이미지에만 조건부 적용합니다. 공고당 최대 3장, 이미지 다운로드 10MiB·픽셀/메모리 제한, 압축·필요시 긴 포스터 분할, 요청 간격 2초 이상·동시성 1을 사용합니다. 근거 URL·원본/처리 이미지 해시를 보존합니다. S3 이미지는 접근 가능 여부와 수집 정책을 구분해 정책 불명확 시 보류합니다.

유료 분석과 실제 수집은 명시적인 수동 명령에서만 실행합니다. Source별 접근 승인, robots, 페이지·공고·요청 수, 호출·비용 예산과 성공 체크포인트를 확인합니다. 기본 테스트와 웹 접속으로 유료 호출이 발생하지 않습니다.

## 프로젝트 구조와 주요 도구

```text
cmd/web/                       학생용 읽기 전용 웹 서버
internal/web/static/           Go embed로 제공하는 HTML/CSS/JavaScript
internal/notices/              조회 DTO·필터·일정 상태·점검 안내
internal/crawler/              Source별 파서·공통 Fetcher·접근/요청 제한
internal/classification/       Source 전략·Vertex AI 분류
internal/schedules/            schedules v2 변환·근거·날짜·품질 검증
internal/imageanalysis/        이미지 준비·모델·검증·병합·체크포인트
internal/imageapply/            이미지 분석 이력·CAS 적용·격리 저장 검증
internal/noticepolicy/          연도·이미지 출처 승인·만료 조회 정책
internal/repository/            MongoDB 저장·조회·분류/일정 이력
internal/model/                 BSON/JSON 데이터 모델
cmd/server/                    기존 관리자 API·Runner 수집 서버
Dockerfile.web / render.yaml    Render 웹 배포
Dockerfile / docker-compose.yml 기존 수집 서버·로컬 MongoDB 개발 환경
```

주요 컬렉션은 `crawled_contents`, `crawl_sources`, `crawl_jobs`, `schedule_analyses`, `content_classifications`입니다. 분석/정책 적용은 허용된 필드만 갱신하며 원본 문서를 `ReplaceOne`으로 교체하거나 만료 데이터를 자동 영구 삭제하지 않습니다.

기존 `cmd/server`의 관리자 `/api/v1/*`와 Runner는 유지합니다. `cmd/web`과 달리 관리자 서버는 초기화·수집·저장 기능이 있으므로 웹 배포에는 사용하지 않습니다. 수동 도구는 `cmd/ssu-recent`, `collect-media`, `analyze-image-schedules`, `prepare-schedule-apply`, `apply-schedules`, `notice-policy`, `verify-image-schedules`, `deadlines` 등에 있습니다. 각 명령의 도움말과 관련 문서에서 Preview·실행 옵션을 확인하세요.

## 개발·검증

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
```

실제 MongoDB 쓰기 테스트는 명시적인 loopback 임시 DB에만 실행합니다. 운영 Atlas를 테스트 DB로 지정하면 보호 검사에서 거부합니다.

```sh
TEST_MONGODB_URI=mongodb://127.0.0.1:27030 go test -count=1 ./...
```

실제 캡처·모델 응답이 없는 clone에서는 관련 회귀 테스트가 사유와 함께 SKIP되며 합성·Mock 테스트는 실행됩니다. 해당 로컬 자료를 반드시 요구하려면 `REQUIRE_LOCAL_FIXTURES=true`를 사용합니다.

선택적인 실제 Chrome E2E는 실행 중인 웹 서버와 `.env`, 현재 MVP 데이터를 전제로 합니다.

```sh
npm install --prefix /tmp/ssu-web-e2e --no-audit --no-fund playwright-core@1.65.0
NODE_PATH=/tmp/ssu-web-e2e/node_modules node scripts/web-e2e.cjs
```

`WEB_BASE_URL`, `CHROME_PATH`, `E2E_OUTPUT`으로 접속 주소·Chrome 위치·결과 경로를 지정합니다. Node.js는 웹서비스 실행 의존성이 아닙니다. 다른 데이터셋에서는 E2E의 데이터별 기대값을 조정해야 합니다.

## 보안과 공개 저장소 범위

- DB 연결과 AI 호출은 백엔드에서만 수행하며, 웹 조회에는 Vertex 서비스 계정 키를 제공하지 않습니다.
- API 입력·페이지 크기·타임아웃·외부 링크를 검증하고 원문 HTML은 실행하지 않습니다.
- 기본 인증과 TLS 검증을 유지하고 로그인·CAPTCHA·접근 제한을 우회하지 않습니다.
- `.env`, 자격 증명, 실제 수집 원문·이미지, AI raw 응답, BSON 백업과 `outputs/`는 Git에 포함하지 않습니다. Docker 빌드에서도 제외합니다.
- 운영 데이터와 비밀값은 clone에 제공하지 않습니다. 학교 접근 정책 확인은 포괄적인 재수집 허가로 해석하지 않습니다.

## 상세 문서

| 내용 | 문서 |
|---|---|
| Render·Docker·환경변수·Secret File | [배포](docs/deployment.md) |
| 웹 구조·조회 계약 | [웹 구조](docs/web-architecture.md) · [API](docs/api.md) |
| 수집·검증·저장 파이프라인 | [데이터 파이프라인](docs/data-pipeline.md) |
| 글로벌미디어 실제 공개 API·파서·정책 | [학부 크롤러](docs/global-media-crawler.md) |
| Source별 카테고리 전략 | [분류 전략](docs/classification-strategies.md) |
| Gemini 분석·이력 | [AI 분석](docs/ai-analysis.md) |
| 일정 정밀도·부분 날짜·근거 | [schedules v2](docs/schedules-v2.md) |
| 이미지 준비·분석·수동 적용 | [이미지 일정](docs/image-schedule-analysis.md) |
| 사용자 승인 이미지 출처 정책 | [이미지 정상 취급](docs/image-source-policy.md) |
| 빈 일정 사유·상대 기한·운영시간 | [일정 점검 안내](docs/timing-review.md) |
| 일정 저장 Preview·CAS | [저장 적용](docs/apply-schedules.md) |
| 기한 관리·백업·dry-run | [만료 관리](docs/lifecycle.md) |

과거 단계의 문서는 당시 검증 범위를 포함할 수 있습니다. 최신 실행 방법은 이 README와 현재 코드를 기준으로 확인하세요. 실제 실행 보고서·스크린샷·백업은 로컬 `outputs/`에 보관합니다.
