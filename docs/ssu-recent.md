# 슈캐치 공식 8개 카테고리 · 달력 기준 최근 3개월

기존 `ssu_catch` Source와 Fetcher, goquery 파서, Repository, Runner, REST API를 재사용한다. `collection_mode=recent_three_months`를 선택한 경우에만 확장 수집 경로를 실행한다. 기존 소량 MVP 경로의 1페이지·3건·5요청 한도와 기존 설정 파일은 유지한다.

## 실행

프로젝트 디렉터리에서 기본 Preview를 실행한다. 실제 HTTP 요청, MongoDB 연결/변경, Vertex 클라이언트 생성은 없다.

```sh
go run ./cmd/ssu-recent
```

실제 수동 수집:

```sh
go run ./cmd/ssu-recent -execute \
  -output-dir /absolute/new/capture-directory \
  -mongo-uri mongodb://127.0.0.1:27030 \
  -database ssu_recent_20261106
```

사용자가 2026-11-06에 앞으로 요청 범위의 실제 수집을 진행하도록 명시적으로 승인했다. 이를 신규 `configs/sources.ssu-recent.json`의 `policy_approved=true`와 `policy_approval_basis`에 기록했다. 이 값은 학교의 별도 재이용 허가를 취득했다는 주장이 아니다. 학교 robots의 금지 경로, 로그인·접근 제한은 계속 지킨다. 승인 false인 Source는 수동 실행에서도 HTTP/DB 작업 전에 차단한다.

기존 `sources.ssu-catch.json`, `sources.university.json`의 승인 false와 `.env`는 변경하지 않는다. 새 프로필은 `enabled=false`, `auto_enabled=false`, `classification_strategy=source`다. 수동 명령은 지정 DB의 해당 Source만 실행 중 enabled=true로 두고 종료 시 false로 돌린다. cron 등록/자동 스케줄은 하지 않으며 모델의 기존 SSU 자동 수집 차단도 유지한다.

수동 실행의 기본 MongoDB는 원본 프로젝트 `.env`의 `MONGODB_URI`/`MONGODB_DATABASE`를 사용한다. 명시한 `-mongo-uri`/`-database` 플래그가 최우선이고, 프로세스 환경 변수, `.env`, 표준 로컬 기본값 순서다. Preview는 연결 설정을 로드하거나 출력하지 않는다. 테스트/별도 DB를 선택하려면 URI/이름을 명시한다. 임의 DB 삭제, 오래된 데이터 삭제, 과거 전체 공고 재조정은 하지 않는다.

## 날짜와 페이지 순회

- 실행 시작 시 `Asia/Seoul`로 변환한 날짜의 00:00에서 `AddDate(0,-3,0)`을 계산한다. 90일 고정, 월말 자동 클램프를 추가하지 않는다.
- 시작일 포함, 시작일 이전 제외, 미래 게시일 제외. 실제 슈캐치 게시일은 일 단위이며 시각을 추측하지 않는다.
- 각 카테고리를 순차적으로 순회하고 목록의 날짜·공식 제외 카테고리를 **상세 요청 전에** 검사한다. 잘못된 날짜는 URL과 원문 날짜를 검토 이슈에 남긴다.
- 실제 카테고리 링크의 query 값은 `외국인유학생`이다. 표시명 `외국인 유학생`과 구분한다. URL은 net/url.Values로 인코딩한다.
- 실제 필터 HTML에서 확인한 다음 링크 `/공지사항/page/N/?category=...&f&keyword`를 따라간다. 카테고리가 사라지거나 바뀐 링크, 다른 호스트, 미확인 파라미터는 오류로 중단하며 임의 페이지 URL을 합성하지 않는다.
- 반복 URL, 동일 게시글 ID/날짜 목록의 반복 페이지, 순환 링크를 검출한다. Source `max_pages`는 카테고리당 한도이고 `max_contents`, `max_requests`는 전체 실행 한도다. 한도에 걸리면 전체 완료로 보고하지 않는다.
- 두 페이지 이상에서 정상 날짜가 내림차순임을 관찰하고 현재 일반 행이 모두 시작일 이전일 때 조기 종료한다. 정렬이 한 번이라도 어긋나면 조기 종료를 사용하지 않는다. 날짜 경계 페이지는 전 행 검사한다.
- 실제 HTML에서는 명시적인 고정 공지 표식을 발견하지 못했으므로 `.start` 등을 고정 공지로 추측하지 않는다. 기본 경로는 미확인 행 전체를 보수적으로 날짜/순서 검사한다. 검토한 고정 행 어댑터가 제공되면 해당 행은 정렬/일반 행 종료 판정에서 분리하되 각 고정 공지에도 날짜 조건을 적용한다. 합성 테스트에서 오래된 고정 공지와 최신 일반 공지가 섞인 경우를 검증한다.

## 파서·분류·저장

기존 검증 본문 선택자 `#contents .bg-white > hr + div`를 사용한다. 공식 카테고리는 목록 HTML 라벨에서만 추출하며 검색 카테고리 값을 정답으로 주입하지 않는다. 상세 헤더에서 확인되지 않은 category CSS를 추측하지 않는다. 추후 실제로 검토한 상세 라벨 선택자가 제공되면 원문 라벨을 추가 보존하고 불일치를 로그로 기록한다. 같은 URL의 여러 공식 라벨은 기존 멤버십 병합 규칙을 적용한다. 유효하지 않은 분류는 unknown으로 남긴다.

채용/교원채용은 목록에서 제외하고 교직은 유지한다. 본문 제외 판정에도 기존 정책을 적용한다. `모집`만으로 제외하지 않는다. Source 전략이며 Vertex/Gemini 요청은 없다.

본문이 빈 공지는 성공한 수집 항목에 포함하지 않는다. 이미지/첨부만 있고 텍스트가 없는 경우도 본문을 만들어내지 않고 원본 HTML과 검토 이슈를 남긴다. 정상 본문의 이미지 URL은 `image_urls`에 보존하며 OCR/이미지·첨부 다운로드를 하지 않는다. 첨부는 기존 이름/URL 메타데이터만 저장한다.

8개 카테고리 탐색이 끝나면 기존 Runner가 Repository.Save로 검증된 최종 항목을 저장한다. ObjectID, 최초 수집일, 최종 확인일, 본문 해시 및 공식 분류 멤버십 병합을 그대로 사용한다. 새 이미지 URL도 변경 감지 해시에 포함하되 이미지 필드가 비어 있는 기존 데이터의 해시 직렬화는 유지한다. API의 조회 기간은 변경하지 않는다.

## 요청 관리와 산출물

새 프로필 기본값: 동시성 1, 최소 간격 2초, 추가 jitter 0~200ms, 타임아웃 20초, 재시도 최대 1회, 카테고리당 최대 30페이지, 전체 공고 최대 1000건·HTTP 1500회. 429/5xx와 Retry-After는 공통 Fetcher에서 처리한다. 401/403, 재시도 후 429, 예산 소진은 중단한다. 자동 요청은 추가하지 않는다.

기존 Source는 jitter 0과 기존 재시도 횟수를 유지한다. 새 설정은 `request_jitter`, `max_retries`로 조정한다. 8개 카테고리를 병렬로 요청하지 않는다.

출력 디렉터리:

- `report.json`: 날짜 범위, 카테고리별 페이지/행/날짜 통과·제외/공식 제외/중복/수집/신규·업데이트, 오류와 중단 사유, 요청 수, Vertex=0.
- `artifacts.json`, `raw/`: 실제 응답 URL·저장 시각·SHA256·원본 HTML.
- `checkpoint.ndjson`: 진행 중 후보/멤버십 갱신 기록. 최종 제외가 나중에 발견될 수 있으므로 직접 DB에 가져오는 용도로 쓰지 않는다.
- `items.json`: 최종 정상 항목(저장 성공 시).
- `job.json`: 기존 Job Runner 결과. 누락·파싱 실패 등 검토 이슈가 있으면 partial로 기록하고 정상 공고 수와 오류 수를 분리한다.

`date_eligible`은 목록에서 날짜 조건을 통과한 행 수다. `duplicates`는 날짜 조건을 통과했지만 이미 발견되어 상세 요청을 생략한 건수다. `collected/new_stored/updated`는 최초 발견 카테고리에 귀속한 고유 문서 수이며, 복수 카테고리 API 멤버십 수와 다를 수 있다. `updated`는 기존 URL 문서의 최종 확인 갱신을 포함한다.

## 로컬 사후 감사

```sh
go run ./cmd/audit-ssu-recent -dir /absolute/completed/capture-directory \
  -mongo-uri mongodb://127.0.0.1:27030 -database ssu_recent_20261106
```

학교 HTTP 없이 원본 해시, DB 날짜/카테고리/중복, 분류 이력 0건, 카테고리 API를 검증한다. `reviews.json`에 이미지 본문만 있는 공지의 이미지 URL과 첨부 정보를 별도로 보존한다. 실제 저장 결과와 검토 대상을 분리한다.

## 테스트

실제 카테고리별 HTML 8개는 `internal/crawler/testdata/ssu_catch/recent_categories/`와 provenance.json에 보존한다. 여러 페이지·날짜·고정 공지·정렬 오류·미래 날짜·중복·채용/교원채용·교직 처리는 로컬 httptest의 **합성 게시판**으로 검증한다. MongoDB 테스트는 임시 DB를 사용하며 과거 데이터 보존·멤버십·API를 검증한다. 기본 테스트에서 실학교와 유료 모델은 호출하지 않는다.

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
```

## Atlas 연결 및 기존 스냅샷 이관

접속 정보는 Git 제외 `.env`에만 보관하며 출력/보고서/패치에는 포함하지 않는다.

```sh
go run ./cmd/check-mongo
go run ./cmd/migrate-ssu-snapshot -execute -report /absolute/migration.json
```

첫 명령은 Ping/기존 컬렉션 건수 확인만 수행하고 인덱스나 데이터를 생성하지 않는다. 두 번째는 명시적으로 승인한 로컬 `ssu_recent_20261106` 스냅샷을 환경의 대상 DB로 이관한다. 신규 문서의 ObjectID와 최초 수집일을 유지하며, 같은 URL이 이미 있으면 기존 대상 ID를 보존한다. 내용이 다른 대상 문서는 충돌로 중단하고 자동 덮어쓰기/삭제하지 않는다. 기존 분류 이력도 삭제하지 않으며 학교/Vertex 요청을 하지 않는다.
