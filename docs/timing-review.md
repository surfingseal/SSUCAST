# 일정이 비어 있는 공고의 오프라인 점검

2026-11-07 기본 목록77건 중 application/submission과 event가 모두 없는13건을 저장 본문·이미지·기존 이력으로 점검했다. 추가 AI·학교 HTTP·첨부 다운로드·Atlas 변경은 모두0회다.

- 일반 안내4: 학자금 중복지원, 통학로, 외국인 유학생 불법취업 주의, AI윤리 가이드라인.
- 이미 다른 일정이 있는2: 폭력예방교육(completion), 정부포상 심사(selection). 기존 일정을 event/application으로 바꾸지 않았다.
- 첨부파일 확인1: 특별장학금. 본문은 붙임을 안내하며 신청기간이 없다.
- 연결 공지 확인3: 대학별 학점교류, 글로벌미디어 AI학업장려 대출, 주거안정장학금 우선지원 기준.
- 이미지 미분석1: 생성형AI 교육 포스터는 현재 허용 범위 밖 startup.ssu.ac.kr 자산. 이번에 새 요청/허용 목록 변경을 하지 않았다.
- 상대 기한/운영시간2: 유고결석 포스터의10일·보건결석7일, 전공설계상담의 학기 중09:00~17:30.

저장된 자료만으로 추가 확정할 달력 날짜는0개다. 안내문 작성일·정책 시행일·자격 연령 기준일·이전 공지 작성일을 신청/행사 날짜로 오인하지 않았다. 유고결석 본문은 결석 종료일 이후 신청을, 이미지는 결석일로부터10일/7일을 안내하므로 개인별 기산점을 임의 결정하지 않고 두 근거를 함께 보여준다.

## 조회 전용 웹 안내

`internal/notices/timing_review.go`가 로컬 JSON 안내 파일을 읽고 API의 선택적 `timing_review` 필드를 채운다. 날짜·schedules·만료 상태·분류 이력은 수정하지 않는다. 내용 해시와 최신 분석ID가 모두 일치하는 공고에만 표시해 원문 또는 분석 변경 뒤 오래된 안내를 적용하지 않는다. 해시와 내부 분석ID는 응답에 노출하지 않는다. 안내문은 textContent로 렌더링하고 근거 링크는 HTTPS로 정제한다.

기본 파일은 프로젝트 `outputs/missing-schedules-review/annotations.json`이다. 실제 공고 근거를 포함하므로 로컬 전용이며 Git에 넣지 않는다. 이 파일이 없는 clone은 기존 조회 기능을 그대로 제공한다. 명시적 다른 경로는 서버 실행 시만 설정할 수 있으며 `.env` 변경은 필요 없다.

```sh
WEB_ADDR=127.0.0.1:8088 go run ./cmd/web
# 선택적 로컬 파일 경로
WEB_TIMING_REVIEW_FILE=/absolute/annotations.json WEB_ADDR=127.0.0.1:8088 go run ./cmd/web
```

JSON은 `id`, `content_hash`, `analysis_id`, `review` 객체의 배열이다. review에는 status/label/message/review_required/rules/evidence가 있으며 달력 날짜 필드를 받지 않는다. 상태는 informational, other_schedule_available, attachment_required, linked_notice_required, image_unavailable, relative_or_recurring, insufficient_information이다. 이는 일정 점검 사유이며 공식 category나 lifecycle_status를 대체하지 않는다. 파일은 최대2MiB/500건이며 중복ID·알 수 없는 키·상태는 거부한다.

이번13건의 확인 결과는 로컬 `outputs/missing-schedules-review/audit.json`과 `annotations.json`에 있다. image source quote는 에이전트가 저장 이미지를 직접 대조한 결과이며 사람 정답 라벨로 표시하지 않는다. 웹에서는 근거를 펼쳐 확인할 수 있고, 개인별 날짜 없이 상대 기한을 마감 정렬·알림·만료 판정에 사용하지 않는다.
