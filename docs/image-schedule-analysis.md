# 본문 이미지 일정 분석 MVP

2026-11-07 사용자 이미지 출처 승인으로 운영 조회의22개 일정은 정상 취급한다. 아래 uncertain61/blocked1은 보존된 원본 검증 결과이며, 최신 파생 조회 정책은 [이미지 출처 승인](image-source-policy.md)을 참고한다.

모델은 `gemini-3.8-flash`, Vertex AI global, 기존 ADC와 Go Gen AI SDK v1.77.0을 사용한다. 텍스트 분석·카테고리·원문 수집 경로는 그대로 두고, 필요한 공고의 공개 본문 이미지만 수동으로 보완한다. Runner나 자동 분류 작업은 등록하지 않는다.

## 실제 반영 상태 — 2026-11-07

- 원본 공고226건 유지: 슈캐치212 / 글로벌미디어14.
- 기존 이미지 결과16건 재사용·반영, 근거5개를 공고3건에서 시각 대조 후 보완, 미완료15건 신규 분석·반영.
- 고유 이미지 분석 공고31건. S3 정책 불명확1건은 보류.
- 새 일정59개 추가, 기존 텍스트 일정3개에 이미지 근거 병합. 이미지 근거 연결 일정62개 / 전체 일정299개.
- 이미지 일정61개 uncertain, 1개 blocked. 모두 review_required=true, 확정 마감/예정 일정 계산에서 제외.
- 기존 분석 이력110건을 byte 단위로 보존하고 별도 이미지 이력34건 추가. 전체144건이며 과거 Flash-Lite10건도 유지.
- 이번 추가 생성15회, 오류/재시도0회, 입력60,412·출력10,673·총71,085토큰. thinking 토큰은 별도 제공되지 않았다. 추정 US$0.085333, 확정 청구액이 아니다.

## 분석 조건과 범위

저장된 본문 image_urls와 기존에 확보한 Lexical 본문 이미지 노드만 사용한다. 첨부파일·PDF·다른 게시판·이미지 속 링크를 따라가지 않는다. URL은 HTTPS·허용 origin·검증된 자산 경로로 제한하며 서명/추적 query, 인증정보, 사설IP, off-origin redirect를 차단한다. robots 확인이 실패한 origin은 다운로드하지 않는다.

이미지가 있고 일정이 비어 있거나 핵심 신청/행사 일정이 없고, 짧은 본문·포스터 참조·불확실성이 있는 공고를 후보로 삼는다. 텍스트에서 신청/행사 모두 충분히 추출된 공고는 생략한다. 기존32개 후보에서 성공 결과가 있는 공고를 제외하고 남은 ID만 처리했다.

## 이미지 준비와 자원 제한

일반 Fetcher 기본2MiB 상한은 유지한다. 이미지 전용 호출만 `MaxResponseBytes=10<<20`을 사용한다. robots는 계속2MiB, HTTP timeout30초, origin 요청 시작 간격2초 이상, 동시성1이다. 원본 자산을 저장하고 SHA256을 보존한다.

- 입력10MiB, 최대64MP·한 변30,000px, 16bit 이미지 디코딩 거부.
- 처리 버퍼는 최대8MP, 가로2400px 이내, 비율 유지·bilinear 축소·JPEG quality92.
- 세로가 가로의3배를 넘는 포스터는 남은 슬롯 안에서 겹치는 스트립으로 분할한다. 겹침은96px이며 공고 전체 모델 입력은 최대3장이다.
- crop 좌표·원본/처리 이미지 해시·파일 위치를 별도로 기록한다. 원본 이미지를 생성형 편집하지 않는다.
- CLI의 Go heap 목표는384MiB(soft limit)다. 이는 OS RSS의 하드 제한이 아니며 별도의 바이트·픽셀·동시성 제한을 함께 사용한다.
- JPEG/PNG 디코더로 크기를 확인할 수 없는 입력은 보류한다. 동작하지 않는 형식을 크기 검사 없이 전달하지 않는다.

실제 크기 초과2건은 각각2,911,988 / 8,946,609bytes였고 920,685 / 1,302,597bytes로 압축했다. 두 포스터 모두 정상 분석했다. 이번 실제 포스터는 일반 비율이어서 분할이 필요하지 않았고, 긴 포스터 분할은 합성 테스트로 검증했다.

## 일정 중심 출력과 검증

기존 v1 응답/설정/체크포인트는 불변으로 둔다. 신규 `schedule-image-compact-v2`는 전체 이미지 전사·요약·대상 지원문을 요청하지 않고 일정 후보와 짧은 인용, 검토 사유만 요청한다. Structured Output, HIGH 이미지 해상도, LOW thinking, 최대 출력16,384토큰을 사용한다. 이전 MAX_TOKENS 공고는 새 형식1회로 완전한 JSON과 일정15개를 받았고 과거 잘린 JSON은 보존했다.

모든 날짜·시각·부분 날짜는 기존 schedules v2로 검증한다. 연도/시각을 만들거나 학년도를 달력 연도로 바꾸지 않는다. 모델의 짧은 인용은 독립 OCR 검증이 아니므로 정확한 인용처럼 보이더라도 uncertain을 유지한다. 근거 충돌은 blocked 또는 제외하며 검토 상태를 자동 해제하지 않는다.

띄어쓰기·줄바꿈은 기존 근거 정규화로 처리한다. 다른 숫자·대상·행사는 단순 유사 문자열로 허용하지 않는다. 기존 근거 불일치5개는 저장 이미지와 직접 대조한 별도 `schedule-image-visual-review-v2` 산출물로 복원했다. 떨어진 표 셀은 image_evidence에 각각의 인용으로 보존한다. 이는 에이전트의 이미지 대조이며 사람이 정답 라벨링한 결과로 표시하지 않는다.

휴학생 본 수강신청은 일반학생15시와 별도로 표에 표시된14시 마감만 보존하고 연도를 추정하지 않았다. 이 변경은 기존 AI raw를 수정하지 않고 시각 대조 사유·원본 후보 index·이미지 참조로 남긴다. 기존 summary와 target_audience는 변경하지 않는다.

## 병합·이력·CAS

`internal/imageanalysis`가 후보·다운로드·모델·검증·준비를 담당한다. 같은 type/label/scope와 호환되는 달력만 병합하며, 같은 대상이라는 이유로 다른 행사나 기관을 합치지 않는다. 원래 값은 덮어쓰지 않고 충돌/부분 날짜를 보존한다.

`internal/imageapply`는 prior analysis 연결, content/input hash, 기존 분석 ID/버전/상태를 확인한다. 적용 전에 전체 BSON 백업을 재파싱하고 격리 loopback DB에 복원해 검증한다. 읽은 BSON snapshot 전체를 원자적 CAS와 비교해 분석 필드만 `$set`한다. 기존 원문·요약·대상·카테고리·status·deadline·lifecycle·분류 이력은 유지한다. 새로운 schedule_analyses 이력을 별도 생성하고 기존 이력을 수정하지 않는다.

동일 결과 재실행은 duplicate다. 구버전은 새 분석을 덮어쓰지 못하며 경합/오류 시 후속 적용을 중단하고 결과를 기록한다. 이력 insert 후 projection CAS가 실패하면 미연결 이력이 남을 수 있으므로 이를 저장 성공으로 보고하지 않으며 임의 삭제/롤백하지 않는다.

## 실행 방법

기존 `.env`는 수정하지 않는다. 실제 원문·이미지·응답·BSON 백업은 Git 제외된 로컬 outputs에0600/0700 권한으로 보관한다. 아래 경로는 사용자가 선택하는 새 실험 디렉터리다.

```sh
# 기존 후보 중 아직 Atlas에 반영하지 않은 공고만 고정
go run ./cmd/analyze-image-schedules --phase remaining --prior /absolute/previous-run --output /absolute/new-run
# 기존 이미지 캐시 재사용, 신규 본문 이미지만 다운로드
go run ./cmd/analyze-image-schedules --phase download --prior /absolute/previous-run --output /absolute/new-run
# 명시적인 유료 호출. 일반 서버와 기본 테스트에서는 실행하지 않는다.
go run ./cmd/analyze-image-schedules --phase analyze --compact --output /absolute/new-run
# 별도 추가 배치는 같은 추가25회·US$3 예산을 합산
go run ./cmd/analyze-image-schedules --phase analyze --compact --budget-prior /absolute/completed-new-run --output /absolute/other-new-run
# 검증·읽기 전용 Preview
go run ./cmd/analyze-image-schedules --phase preview --output /absolute/new-run
# 실제 데이터 격리 저장 검증
TEST_MONGODB_URI=mongodb://127.0.0.1:27030 go run ./cmd/analyze-image-schedules --phase local-verify --output /absolute/new-run
# 운영 적용은 기본 Preview. --apply만 명시적 쓰기.
go run ./cmd/analyze-image-schedules --phase apply --output /absolute/new-run
go run ./cmd/analyze-image-schedules --phase apply --apply --output /absolute/new-run
# 초기 원문 백업과 최종 상태 읽기 전용 대조
go run ./cmd/verify-image-schedules --baseline /absolute/initial-backup --output /absolute/audit
```

성공 체크포인트를 재호출하지 않는다. 출력 잘림은 정상 결과로 저장하지 않는다.429/5xx만 제한적으로 재시도하며 요청 전송이 불분명하면 자동 재요청하지 않는다. 별도 배치의 `--budget-prior`는 앞선 추가 배치가 완료된 상태에서만 사용하고 이전 로그를 보존한다. 실험 디렉터리와 내부 파일의 절대 경로/해시를 유지한다.

`review-evidence --prior ... --input /absolute/reviews.json --output ...`는 새 생성 없이 원본 raw와 이미지 대조 산출물을 준비한다. 이후 동일 Preview/격리 검증/백업/CAS 경로를 쓴다. 외부 사용자가 전달한 임의 검토 파일을 서버 API에서 받아 적용하는 기능은 없다.

## 웹과 운영 제한

`WEB_ADDR=127.0.0.1:8088 go run ./cmd/web`로 기존 인증 웹을 실행한다. 상세보기는 본문/이미지/병합 출처, 분리된 이미지 근거, 안전한 원본 이미지 링크와 검토 안내를 표시한다. 이미지는 자동 다운로드하지 않는다. 날짜만 있으면 날짜만, 연도 미상은 부분 날짜로 표시한다. uncertain/blocked 일정은 확정 마감 정렬 키·upcoming 계산·알림 후보로 쓰지 않는다. 공고 자체는 숨기지 않고 검토 정보와 함께 조회 가능하다.

실제 Atlas 기반375/768/1440px 이미지 상세보기와 기존 검색·필터·정렬·페이지네이션을 검증했다. S3 이미지는 HEAD200이지만 robots403과 수집 허가 불명확 상태를 분리해 보류했다. 공개 접근 가능을 수집 허가로 취급하지 않는다.

## 공식 참고

- [Vertex AI 이미지 이해](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/capabilities/image-understanding)
- [Thinking 설정](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/thinking)
- [Go Gen AI SDK](https://pkg.go.dev/google.golang.org/genai)
- [공식 요금](https://cloud.google.com/model-garden/pricing): 2026-12-31까지 해당 global 입력$0.75/M·출력/추론$3.75/M. 추정 금액이며 확정 청구액은 Cloud Billing에서 확인한다.
