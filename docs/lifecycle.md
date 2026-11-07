# 마감일과 만료 관리

공식 카테고리, 학교 상태(status), AI classification_status와 별도로 lifecycle_status를 사용한다. 상태는 active / expired / needs_review / no_deadline이다. 기존 deadline=null은 만료/삭제 근거가 아니다.

## 기본 Preview — 읽기 전용

```sh
go run ./cmd/deadlines -output /absolute/preview.json
```

기본 대상은 이번 요청의 로컬 `mongodb://127.0.0.1:27030`, `ssu_recent_20261106`이다. Atlas .env를 읽지 않는다. 다른 대상은 --uri/--database를 명시한다. 원본 BSON 전체의 ID 정렬 SHA256을 조회 전후 비교하며, Preview는 필드·인덱스를 변경하지 않는다. --at RFC3339 옵션으로 판정 시각을 고정할 수 있다.

## 추출 규칙과 불확실성

신청/접수/지원/모집/설문/이수/제출 등 기한 문맥에 연결된 날짜만 후보로 본다. YYYY.MM.DD, YYYY-MM-DD, MM.DD, MM/DD, 한글 년월일, 기간 ~, 날짜+시각을 지원한다. 행사일·발표일·교육일 등은 신청 마감으로 사용하지 않는다. 복수 대학/기관/차수와 서로 다른 기한은 단순히 가장 늦은 날짜로 합치지 않고 검토한다.

연도가 있는 기간 시작일은 끝 날짜로 이어받되 12월→1월의 명시적 기간은 다음 해로 해석한다. 연도가 없는 단독 날짜는 게시일이 있어 같은 해의 게시일 이후~달력 3개월 이내인 좁은 문맥에서만 추론하고 year_policy를 기록한다. 불확실한 연말·연초나 게시일 이전 추론은 needs_review다. 제목의 학년도 숫자를 날짜의 연도로 주입하지 않는다.

단일 기한이 유일하게 확인된 경우만 confirmed=true다. 같은 날짜의 날짜 전용 제목과 명시 시각 본문은 시각이 더 구체적인 근거를 사용한다. 명시적인 변경/연장 후 날짜는 이전 기한과 구분하지만 상충하면 검토한다. 선착순·조기 마감·상시 등 조건부 종료, 자격 기준일, 날짜 오류, 참조 이미지의 미확인 기한은 자동 확정하지 않는다.

날짜만 있는 기한은 **Asia/Seoul 해당 날짜 23:59:59.999까지 유효**하다는 정책을 사용한다. 명시 시각은 그 시각까지 포함한다. UTC BSON Date로 저장하며 Evidence의 time_policy/year_policy와 판정 근거 문장을 보존한다. 날짜가 명확해도 이 정책이 실제 학교 종료 시각을 확인했다는 뜻은 아니다.

이미지/첨부/OCR/학교 HTTP/Vertex 요청은 하지 않는다. 이미지 전용 본문은 needs_review이며 마감일을 생성하지 않는다. 과거 수집의 이미지 전용 24건은 정상 본문 DB 212건과 별도의 검토 자료에 있으므로 이번 212건 통계에 합치지 않는다.

## 상태 반영 — 명시적인 옵션에서만

```sh
go run ./cmd/deadlines --apply --output /absolute/applied.json
```

deadline / lifecycle_status / lifecycle 메타데이터만 변경한다. content_hash, 원본 URL, Source ID, 게시일·최초 수집일·업데이트 시각·분류 필드는 바꾸지 않는다. 전체 원본 BSON 스냅샷 비교 CAS를 사용해 Preview 이후 어느 필드든 변경된 문서는 건너뛴다. 각 판정의 버전·입력 해시·근거·확정 여부를 저장한다.

본문/제목 등이 바뀌어 content_hash가 바뀌는 기존 재수집 저장에서는 오래된 deadline/lifecycle을 무효화하고 needs_review로 돌린다. hash가 같은 재저장은 확정 결과를 유지한다. 기간 연장 후 오래된 expired 상태가 남아 문서가 감춰지지 않도록 한다. 자동 판정 작업/스케줄은 추가하지 않는다.

## REST API

기본 목록은 확인된 만료 공지를 제외한다. 아직 평가하지 않은 공지와 no_deadline/needs_review는 유지한다. 저장 당시 active였어도 확인된 deadline이 현재 시각보다 과거가 되면 조회에서 만료로 취급한다. 목록의 기존 기간 필터/카테고리 정책은 유지한다.

- `GET /api/v1/contents?include_expired=true`: 만료 포함.
- `GET /api/v1/contents?lifecycle_status=expired`: 만료만.
- `GET /api/v1/contents?lifecycle_status=active|needs_review|no_deadline`: 해당 상태.
- 단건 및 `/classifications` 조회도 기본 만료 제외이며 `?include_expired=true`로 명시한다.

위 옵션은 기존 Bearer 인증을 유지하며 Job 조회 옵션으로는 허용하지 않는다. 내부 Content(id) 조회는 관리/분류 작업을 위해 문서를 유지하고 API 계층에서 만료 공개 여부를 검사한다.

## 수동 정리 — 기본 dry-run, TTL 없음

```sh
go run ./cmd/deadlines --cleanup --output /absolute/cleanup-preview.json
```

저장된 confirmed 기한이 지난 공지만 후보로 삼는다. null deadline, 미평가/불확실/오래된 추출 버전, 입력 해시 불일치 등은 제외한다. Source 참조가 없거나 실행 중 수집 Job이 있거나 분류 이력이 참조하는 문서는 삭제 대상에서 제외한다. AI/hybrid Source도 별도 보존 검토 대상으로 건너뛴다. Source 문서나 분류 이력을 연쇄 삭제하지 않는다.

실제 삭제 명령(이번 작업에서는 실행하지 않음):

```sh
go run ./cmd/deadlines --cleanup --delete \
  --backup-dir /absolute/new/backup-directory \
  --output /absolute/deletion-audit.json
```

실행 전에 대상 ID/개수 Preview를 출력 파일에 기록하고 새 0700 디렉터리에 원본 공고·Source·관련 이력을 canonical Extended JSON으로 백업한다. 파일은 0600, fsync 후 SHA256 검증을 통과해야 삭제할 수 있다. 기존 백업 디렉터리를 덮어쓰지 않는다.

삭제는 MongoDB 트랜잭션 안에서 원본 전체 BSON CAS, 현재 기한·버전·입력 해시, Source·진행 중 Job·이력 참조를 다시 검사한다. 실행 중 변경된 문서는 삭제하지 않는다. 삭제/제외/변경 건수를 백업 디렉터리의 감사 로그에도 기록한다. 트랜잭션을 지원하지 않는 단일 standalone MongoDB는 실패 시 삭제하지 않는다. 실제 정리에는 replica set/Atlas가 필요하다. TTL 인덱스와 자동 영구 삭제는 추가하지 않는다.

## 검증 범위

날짜·연도 생략·연말/연초·시각·EOD·신청/행사일 구분·복수 기한·조건부 마감·연장·이미지 본문을 단위 테스트한다. 임시 MongoDB에서 Preview 무변경, 상태 반영의 원본 필드 보존과 CAS, 연장 후 재수집 무효화, API 만료 필터, 백업 SHA256, 변경된 문서가 삭제 CAS에 매치되지 않음을 검증한다. 실제 삭제 함수/명령은 이번 작업에서 실행하지 않는다.
