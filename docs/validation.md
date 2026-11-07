# 검증 기록

검증일: 2026-10-30 (Asia/Seoul). 실제 실행한 결과입니다.

| 단계 | 검증 | 결과 |
|---|---|---|
| 1 설계 | HTTP/실행기/크롤러/컬렉션별 Repository 책임, 데이터 흐름·장애 의미 확인 | README 작성 |
| 2 초기화 | 전체 빌드, 설정 검사, 실제 MongoDB 연결·인덱스 자동 생성 | 통과 |
| 3 크롤러 | httptest 목록·상세·페이지네이션, 중복 URL·공백 정리, robots 차단·오류, 외부 링크·redirect 차단, 내부 IP, 재시도·취소 | 통과 |
| 4 저장 | 실제 MongoDB Bulk Upsert, ID 유지, URL 고유 인덱스, 필터·페이지, 8개 동시 실행 중 1개만 획득 | 통과 |
| 5 자동 실행 | Runner 중복 방지·포화·제한 시간·종료 취소·저장 실패, 만료 작업 복구, cron 등록·콜백 실행·잘못된 일정 거부 | 통과 |
| 6 API | httptest 인증·HTTP 상태·JSON·ID·페이지·기간·쿼리 키 검증 | 통과 |
| 7 테스트 | Go 1.27.3에서 `go test -race -cover ./...`, `go vet ./...`, `go build` | 통과 |
| 7 종단간 | 로컬 서버와 최종 Compose 서버에서 scripts/smoke.py: health·인증·수집·저장·두 번째 Upsert 시 ID 유지·조회 필터·작업 기록 | 모두 통과 |
| 7 배포 | Go 1.27.3 다단계 Docker 이미지 빌드, MongoDB 7 Compose 시작 및 DB healthcheck | 통과 |

실제 MongoDB Repository 테스트는 `TEST_MONGODB_URI=mongodb://127.0.0.1:27028`와 macOS의 MongoDB 8.3.4를 사용하여 skip 없이 통과했습니다. DB는 테스트별 임의 이름으로 생성한 뒤 삭제했습니다.

최종 패키지별 statement coverage는 API 76.9%, config 88.2%, crawler 85.5%, repository 79.8%, service 92.2%입니다. main과 fixture는 별도 단위 테스트 커버리지 0%이며, 실행 중 서버의 종단간 smoke 경로로 확인했습니다. 100% 커버리지나 대규모 부하 안정성을 주장하지 않습니다.

## 실행 중 발견·수정한 사항

초기 race 테스트는 실행 샌드박스가 httptest의 로컬 포트 바인딩을 차단해 실패했습니다. 로컬 포트가 허용된 실행에서 동일 테스트를 다시 수행하여 통과했습니다.

Docker 커널은 `7.0.12-linuxkit`입니다. 최초 `mongo:8.0` 컨테이너는 알려진 커널/TCMalloc 호환성 문제로 시작을 거부했습니다. [MongoDB 공식 호환성 표](https://jira.mongodb.org/browse/SERVER-125742)를 확인하고, 기본 이미지를 호환되는 `mongo:7.0`으로 변경하여 정상 시작을 확인했습니다. `MONGODB_IMAGE`로 서버 이미지를 선택할 수 있습니다. 호스트 커널 설정이나 시스템 Go 설치는 변경하지 않았습니다.

초기 govulncheck가 HTML 파서와 Go 1.27.1 표준 라이브러리를 포함한 호출 경로의 취약점을 보고했습니다. 프로젝트의 Go 최소 버전과 Docker 빌드를 **1.27.3**으로 올리고 goquery **1.13.0**, MongoDB Driver **2.9.2**, x/net **0.62.0**, x/text **0.43.0**, x/crypto **0.58.0** 등으로 갱신한 뒤 테스트와 검사를 다시 수행했습니다.

최종 `govulncheck -show verbose ./...` 결과는 호출 가능한 코드의 취약점 **0건**, import한 패키지의 추가 취약점 **0건**입니다. x/crypto 모듈에 포함되지만 이 프로젝트가 import하거나 호출하지 않는 openpgp 패키지의 [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932)가 모듈 수준에서 1건 보고되었습니다. 해당 패키지는 사용하지 않습니다. 이 결과는 검사 시점의 Go 취약점 DB를 기준으로 합니다.

## 범위와 한계

cron 테스트는 실제 등록된 콜백을 호출하여 Runner 연결을 확인했습니다. 몇 시간 동안 주기를 기다리는 장기 운용 시험은 수행하지 않았습니다. 실제 대상 사이트, JS 렌더링, 분산 rate limit, 다중 인스턴스 부하, 운영 TLS 배포는 검증 범위에 포함하지 않았습니다. 상세 미구현 기능과 확장 방법은 README에 있습니다.

로컬 실행 서버는 SIGTERM 후 exit code 0으로 종료했고, 최종 Docker 서버도 stop 후 exit code 0을 확인했습니다. 테스트용 컨테이너/볼륨과 로컬 임시 MongoDB는 검증 완료 후 정리했습니다.
