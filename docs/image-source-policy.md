# 사용자 승인 이미지 출처 정책

2026-11-07 사용자가 이미지에서 추출한 일정도 정상 추출로 취급하도록 승인했다. 이미지라는 출처만으로 붙었던 검토 사유는 운영 조회 정책에서 해소한다. 모델 raw·검증 완료 원본 schedules·분석 이력은 수정하지 않는다.

`internal/noticepolicy/image.go`는 고신뢰/중간신뢰 이미지 근거, 명시적인 달력 날짜와 유효한 시각·Asia/Seoul·정상 순서를 확인한다. 이미지 출처 자체에만 관한 사유를 명시적 allowlist로 제거하고 사용자 승인 correction을 남긴다. 날짜만 있는 경우 시각을 만들지 않고 partial로 처리한다. 모집/제출/이수의 모든 범위가 확인돼야 만료 숨김에 사용한다. 미래 신청기간은 유지한다.

연도 미상·부분 날짜·낮은 판독 신뢰·알 수 없는 검토 사유·본문 충돌·조건부/잠정 일정·잘못된 부모/유형 등은 자동 해소하지 않는다. 기존 텍스트 올해 가정을 이미지에 적용하지 않는다. blocked는 유지한다. 이미지 출처 승인과 날짜 자체의 불확실성은 별개다.

결과는 기존 notice_policy 버전/해시/CAS와 호환되는 파생 필드에 저장하며 image_source_policy와 image_accepted_schedules로 승인 근거를 기록한다. 기존 Render 버전도 해당 파생 schedules를 읽으므로 이번 DB 정책 적용은 서비스 재배포 없이 조회에 반영된다.

수동 명령은 기존과 동일하다.

```sh
go run ./cmd/notice-policy --year 2026 --output /absolute/private/preview
go run ./cmd/notice-policy --year 2026 --apply --output /absolute/private/apply
```

실제 반영:100개 분석 공고 중16건 정책 변경,84건 동일, 이미지 일정22개 정상 취급. 전체226건·원본 AI 필드 불변을 BSON으로 확인했다. 전체 백업은 재파싱·격리 loopback 복원 후 적용했고 Gemini/학교 요청0회다. 만료 판정23건과 기본 노출77건은 이번 적용에서 변하지 않았다. 상세 일정의 정상/검토 상태만 달라질 수 있다.
