# 1차 데이터 파이프라인

2026-11-07 Asia/Seoul 실행. 이번 범위에는 프런트엔드가 없다.

## 현재 검증된 데이터

Atlas `ssu_crawler`: 슈캐치 원본 212건 + 글로벌미디어학부 14건 = 226건.
슈캐치 진행/unknown 86건과 학부 14건의 최신 분석은 `gemini-3.8-flash`.
일정은 슈캐치 216개, 학부 24개. `schedule_analyses` 110건은 기존 Lite10 + Flash100이며 기존 Lite 이력은 바이트 단위로 보존했다. `content_classifications`에는 학부14건을 저장했다. 슈캐치의 공식 분류에는 모델을 호출하지 않았다.

## 데이터 흐름

1. `cmd/mvp-admin --phase baseline`: 네 컬렉션 BSON 백업, 재파싱, 신규 loopback 임시DB 복원 및 바이트 비교. 운영 DB 인덱스 변경 없음.
2. 기존 Flash76 체크포인트의 모델/ID/입력 해시 검증. 원본 파일은 불변.
3. `--phase analyze-ten --execute`: 과거 Lite10의 실제 이력 연결을 확인한 뒤 Flash 신규 분석. 이미 저장된 성공 응답은 체크포인트에서 재사용한다.
4. `--phase prepare-ssu --baseline BACKUP --prior FLASH76 --ai AI_NEW`: 저장된76+신규10을 production 검증 계층으로 변환한다.
5. `--phase local-check --input SSU_PREPARED --media MEDIA_PREPARED`: 실제 원본 복사본과 분석 결과를 격리된 임시DB에서 저장/재조회/업그레이드/중복/CAS 검증. 임시DB는 소유권 확인 후 정리한다.
6. `--phase apply-ssu --input SSU_PREPARED`: 기본값은 읽기 전용 Preview. `--execute`를 명시한 경우에만 백업 검증과 CAS를 통과한 분석 필드에 한정하여 실제 적용한다.
7. 학부 수집 → 동일 요청에서 카테고리+요약+일정 분석 → `prepare-media` → 임시DB 검증 → `apply-media` Preview → 명시적 `--execute` 적용.
8. `--phase final-audit`: 운영 데이터를 읽어 원본212건 보존, 모델100건, 이력 연결, 인증 API 및 필터를 검증한다.

명령 실행 위치는 기존 `.env`가 있는 프로젝트 루트다. 모든 명령에는 `--output`이 필요하고, 백업이 필요한 단계에는 `--baseline`을 지정한다. 입력/산출물은 절대경로를 사용한다. `--execute`는 유료 분석 또는 운영 반영을 명시적으로 허용하는 옵션이다. 일반 서버 및 기본 테스트에서는 이 명령을 실행하지 않는다.

## 보존 및 CAS

`internal/repository/schedule_analyses.go`는 `_id`, source, content/input 해시, 분석 버전/시각과 관측 BSON 전체를 비교한다. `$set` 허용 목록에는 요약·대상·일정·분석 메타데이터만 있다. 제목/본문/공식 카테고리/status/lifecycle/deadline/content_hash는 수정하지 않는다. 최신 분석 뒤 구버전 덮어쓰기를 차단하며, 같은 결과는 duplicate다.

원본 AI JSON은 `schedule_analyses.raw_response`, 검증 결과는 `validated_output`으로 분리한다. 최신 공고는 `schedule_analysis_id`로 연결한다. 이력 선저장 뒤 CAS가 실패하면 적용되지 않은 이력이 남을 수 있으며 이를 저장 성공으로 보고하지 않는다. 임의 롤백이나 재호출도 하지 않는다.

신규 학부 공고는 source+canonical URL에서 결정한 ID와 `external_post_id`를 사용한다. 기존 unique original_url 인덱스를 재사용한다. Preview는 URL/ID/source+게시물ID 중 하나라도 충돌하면 중단한다. 기존 문서 전체를 ReplaceOne으로 교체하지 않는다.

## 2차 작업에서 재사용할 경로

- 모델: `internal/model/model.go`, `schedule.go`, `schedule_analysis.go`, `classification.go`
- 수집: `internal/crawler/global_media.go`, `global_media_live.go`, `global_media_review.json`
- 검증: `internal/schedules/response_v2.go`, `quality.go`, `production.go`
- 저장: `internal/repository/contents.go`, `schedule_analyses.go`, `prepared_classification.go`
- 오케스트레이션: `internal/mvp/`, `cmd/mvp-admin/`, `cmd/collect-media/`
- 기존 API: `internal/api/api.go`, 인증 유지. `/api/v1/contents?source=ssu_global_media&category=scholarship`, `/api/v1/contents/{id}`, `/api/v1/sources`.

기본 조회는 슈캐치 완료 상태와 기존 expired 정책을 유지한다. 실제 인증 핸들러+Atlas 읽기 조회에서 슈캐치86, 학부14, 학부 장학3을 확인했다. 검색 UI나 새 프런트엔드는 이번에 추가하지 않았다.
