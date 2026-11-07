# Render 배포

이 배포는 `cmd/web` 조회 전용 앱만 실행한다. 기존 `Dockerfile`/Compose의 수집 서버는 로컬 개발용으로 유지하며 Render는 `Dockerfile.web`을 사용한다. 크롤러·Runner·Gemini·인덱스 생성·데이터 변경은 웹 프로세스에서 실행하지 않는다.

## 설정

`render.yaml`은 Docker, Singapore, Free, main, `/healthz`, 수동 배포를 정의한다. 대시보드에서 직접 생성할 경우 동일 값을 선택하고 Dockerfile Path를 `Dockerfile.web`으로 지정한다.

Render의 비공개 환경변수:

| 키 | 값 |
|---|---|
| WEB_DEPLOYMENT | render |
| MONGODB_DATABASE | ssu_crawler |
| MONGODB_URI | 기존 Atlas 연결 문자열. Git/로그에 넣지 않는다. |
| ADMIN_TOKEN | 기존 웹 로그인 토큰. 프런트엔드에 포함하지 않는다. |

`PORT`와 HTTPS 주소 `RENDER_EXTERNAL_URL`은 Render가 제공한다. 앱은 명시적인 Render 모드에서만 `0.0.0.0:$PORT`로 바인딩한다. 로컬 기본값은 계속 loopback이다. 프록시 환경 로그인은 지정된 HTTPS Origin을 확인하고 Secure/HttpOnly/SameSite 쿠키를 사용한다. 임의 X-Forwarded-Proto 값으로 인증 정책을 완화하지 않는다. 커스텀 도메인은 `WEB_PUBLIC_ORIGIN=https://your-domain`으로 명시한다.

## 선택적 일정 점검 파일

로컬의 `outputs/missing-schedules-review/annotations.json`은 Git과 Docker 이미지에서 제외한다. 동일 안내가 필요하면 Render Environment의 Secret Files에 `timing-reviews.json`으로 비공개 등록하고 `WEB_TIMING_REVIEW_FILE=/etc/secrets/timing-reviews.json`을 설정한다. 실제 공고 해시와 분석ID가 일치할 때만 표시한다. 이 파일이 없어도 공고·요약·이미지 일정 조회는 정상 동작한다.

서비스 계정/Vertex 키, 로컬 .env 전체, BSON 백업, 실험 raw_response, 이미지 원본은 배포하지 않는다. 빌드 컨텍스트는 .dockerignore로 제외하고 실행 이미지는 웹 바이너리와 CA 인증서만 포함한다.

## 배포 후 확인

1. `/healthz`200, 로그인 화면200 확인.
2. 비인증 `/api/notices`401 확인.
3. 기존 ADMIN_TOKEN으로 로그인 후 실제 Atlas 공고 수·검색·출처 필터·일정 상세 확인.
4. 로그인 쿠키의 Secure/HttpOnly와 다른 Origin의 로그인 차단 확인.
5. Atlas IP 접근 제한이 있다면 Render Connect에 표시되는 해당 서비스의 outbound 범위를 검토한다. 임의로 전체 인터넷 허용으로 바꾸지 않는다.

Free는 휴면과 cold start가 있고 세션은 단일 프로세스 메모리에 있어 재시작 후 재로그인이 필요하다. 유료 플랜 전환이나 자동 실행 작업은 별도로 명시하지 않는 한 활성화하지 않는다. Atlas 접근 권한도 현재 웹 코드의 읽기 전용 경로를 유지한다.

## 공식 문서

- https://render.com/docs/web-services
- https://render.com/docs/blueprint-spec
- https://render.com/docs/environment-variables
- https://render.com/docs/configure-environment-variables
- https://render.com/docs/free
