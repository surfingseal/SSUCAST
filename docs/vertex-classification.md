# Vertex AI 공고 1건 분류

## 최신 결과: Vertex 직접 호출 성공

사용자의 직접 호출 요청에 따라 관리 API 조회를 필수 조건으로 두지 않는 명시적
`-execute -direct-vertex` 모드를 추가했다. ADC·프로젝트 일치·입력 검토·한도·MongoDB
중복 예약 검사는 유지하며 Vertex가 API 활성/권한/모델 오류를 직접 반환하도록 했다.
일반 실행과 기본 테스트는 계속 유료 API를 호출하지 않는다.

실제 생성 요청은 정확히 **1회** 성공했다. gemini-3.5-flash/global의 결과는
primary_category=student_support, categories=[student_support], activity_types=[other],
topics=[general], target_audience=[청년], classification_status=needs_review다.
입력 토큰 870, 출력 토큰 88이다. 사유는 청년지원정보 플랫폼 안내이지만 본문이
짧아 추가 검토가 필요하다는 것이다. 원본 학교 분류 장학/scholarship은 그대로 두었다.

`ssu_vertex_validation.content_classifications`에 1건 저장했으며 기존 인증 REST API
`GET /api/v1/contents/6aed4fea6abc9a0b35c77076/classifications`가 HTTP 200/1건을 반환했다.
실제 결과는 출력의 `vertex/direct-result.json`에 있다. 학교 HTTP 요청은 0회다.
관리 API의 403은 아래 초기 이력에만 해당하며 Vertex 호출의 실패가 아니었다.

```sh
# 사용자가 명시적으로 직접 1회 실행을 요청한 경우의 명령:
go run ./cmd/classify-one -execute -direct-vertex -report /tmp/vertex-direct-result.json
```

동일 입력 해시·taxonomy_version·모델의 예약이 있으면 추가 생성하지 않는다. 이후 품질 평가에서 입력/evidence 버전이 prompt2로 바뀌었으므로, 새 버전의 명시적 실행은 별도 유료 호출이 될 수 있다. 품질 평가 작업에서는 새 버전의 실제 호출을 하지 않았다.
실제 결과가 있는 DB는 삭제하지 않고 별도 검증용 MongoDB 데이터 경로에 보존한다.

## 초기 구현 및 관리 API 사전 점검 이력

기존 Crawler/Fetcher 인터페이스, Repository 저장 경로, Job Runner, REST API 인증/조회 동작을 유지했다. 별도 수동 명령 `cmd/classify-one`과 분류 전용 Repository/API를 추가했다. 시작 시 또는 일반 테스트에서 유료 생성 API를 호출하지 않는다. 자동 분류는 이 MVP에서 비활성이며 true 설정은 거절한다.

현재 요청 모델은 `gemini-3.5-flash`, 리전은 `global`이다. 공식 모델 문서에서 모델 ID, global 및 Structured Output/GenerateContent 지원을 확인했다. 초기에는 사전 점검 차단으로 모델 접근을 확인하지 못했으나, 이후 직접 생성 성공으로 확인했다. 모델을 임의 대체하지 않았다.

프로젝트 설정은 `graduation-exhibition-510404`다. 서비스 계정 파일이 존재하며 JSON 형식, service_account 타입, 프로젝트 일치, 소유자 전용 0600 권한을 확인했다. 실제 개인키나 전체 JSON은 출력하거나 프로젝트에 복사하지 않았다. 기존 ADC 파일도 있으나 이번 설정에서는 GOOGLE_APPLICATION_CREDENTIALS가 지정한 서비스 계정을 사용한다.

읽기 전용 사전 점검 결과:

- 최초 Service Usage 상태 조회는 HTTP 403, `AUTH_PERMISSION_DENIED`였다. 이 결과를 Vertex API가 꺼졌다는 증거로 해석하지 않는다.
- 공식 Service Usage 리소스의 프로젝트 번호 형식에 맞춰 Resource Manager로 프로젝트 ID→번호를 먼저 확인하도록 보완했다.
- 최종 Resource Manager 조회는 HTTP 403, `SERVICE_DISABLED`였다. 따라서 API 활성 상태/예측 권한/실제 모델 접근 검증은 완료되지 않았다.
- 초기 사전 점검 시점에는 유료 Gemini GenerateContent 요청 **0회**, 실제 AI 분류 성공 **0건**, 실제 AI 결과 저장 **0건**이었다. 성공한 결과를 만들어 대신 저장하지 않았다.

관리 API 조회 경로를 사용하려면 프로젝트 관리자가 Cloud Resource Manager API(`cloudresourcemanager.googleapis.com`) 활성 상태와 `resourcemanager.projects.get`, `serviceusage.services.get` 조회 권한을 확인해야 한다. 이 관리 API 조회는 직접 GenerateContent 호출의 필수 조건이 아니다. 초기 경로에서는 그 뒤 Vertex API(`aiplatform.googleapis.com`) 활성화, `aiplatform.endpoints.predict`, 모델 접근 조건을 확인한다. 이번 작업은 API 활성화나 IAM 변경을 하지 않았다. 모델의 문서상 지원과 프로젝트에서의 실제 접근은 별개다.

## 공식 문서와 MCP

Google Cloud 전용 문서 MCP는 노출된 도구 목록에서 발견하지 못했다. 연결된 Gemini 공식 개발 문서 MCP(`gemini_search_docs`, `gemini_get_doc`)와 Context7(`/googleapis/go-genai`)을 실제 조회했다. 별도 MCP 설치는 필요하지 않았으며 설치 명령을 만들어 사용하지 않았다.

검증한 SDK는 **google.golang.org/genai v1.77.0**, ADC 라이브러리는 **cloud.google.com/go/auth v0.9.3**이며 go.mod/go.sum에 고정했다. Context7/main 문서와 함께 실제 고정 버전 소스를 확인했다.

- [Google Gen AI Go SDK v1.77.0](https://pkg.go.dev/google.golang.org/genai@v1.77.0): NewClient, ClientConfig, BackendVertexAI, Models.GenerateContent, Models.Get, GenerateContentConfig, Schema, HTTPRetryOptions, APIError, UsageMetadata.
- [공식 SDK 저장소](https://github.com/googleapis/go-genai): codegen_instructions.md의 Vertex 초기화 및 Structured Output 예제를 MCP로 확인.
- [Gemini 3.5 Flash](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/3-5-flash): 모델 ID, global, GenerateContent 및 Structured Output 지원.
- [Structured Output](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/capabilities/control-generated-output).
- [Service Usage services.get](https://docs.cloud.google.com/service-usage/docs/reference/rest/v1/services/get): 프로젝트 번호 및 serviceusage.services.get.
- [Resource Manager projects.get](https://docs.cloud.google.com/resource-manager/reference/rest/v3/projects/get): 프로젝트 조회 권한.

MCP의 일부 예제에는 다른 모델 ID가 포함되어 있으나 구현은 사용자가 요청한 gemini-3.5-flash를 사용한다.

## 환경 변수와 인증

기존 설정은 os.Getenv를 읽었고 Compose의 .env 주입 외에 로컬 dotenv 로더는 없었다. LoadDotEnv를 추가하여 로컬 실행에서도 .env를 읽는다. 프로세스에 이미 존재하는 값은 빈 값이어도 덮어쓰지 않는다. 쉘 실행·변수 보간·명령 치환을 하지 않는다. .env는 0600이며 기존 .gitignore의 .env 규칙으로 제외된다. .env.example에는 프로젝트/사용자/키 실값이 없다.

SDK 표준 변수:

```dotenv
GOOGLE_GENAI_USE_VERTEXAI=true
GOOGLE_CLOUD_PROJECT=YOUR_PROJECT_ID
GOOGLE_CLOUD_LOCATION=global
GOOGLE_APPLICATION_CREDENTIALS=/absolute/path/to/service-account.json
```

프로젝트 자체 변수:

```dotenv
VERTEX_MODEL=gemini-3.5-flash
CLASSIFICATION_PROVIDER=vertexai
CLASSIFICATION_AUTO_ENABLED=false
CLASSIFICATION_BATCH_SIZE=1
CLASSIFICATION_CONCURRENCY=1
CLASSIFICATION_MAX_ITEMS=1
CLASSIFICATION_TIMEOUT_SECONDS=60
CLASSIFICATION_MAX_RETRIES=0
```

NewVertex는 BackendVertexAI를 명시하고 ADC로 자격 증명을 읽는다. 프로젝트 일치 및 파일 권한을 확인한 뒤 클라이언트를 만든다. 인증된 HTTP 클라이언트와 공식 Google 엔드포인트를 명시한다. 환경 변수의 임의 기본 URL로 학교 공고나 자격 증명이 전달되지 않도록 한다.

SDK HTTPRetryOptions.Attempts=1 및 AutomaticContinuation=false로 재시도/이어생성을 끄고, HTTP 전송 계층에서도 :generateContent 요청을 클라이언트당 한 번으로 제한한다. 배치/동시성/최대 처리 수 1, 재시도 0, 타임아웃 최대 60초를 검증한다. 명령에 -execute를 주지 않으면 사전 점검만 한다.

## 입력과 분류 검증

ReadSSULocalFixture는 기존 실제 상세/목록 HTML과 검토 JSON을 읽어 SHA256과 같은 slug를 검사하고 기존 ParseSSUCatchList/ParseSSUCatchDetail을 호출한다. 기존 정책 차단을 바꾸거나 외부 HTTP 요청을 하지 않는다. 제목, 원본 분류, 추출 가능한 본문, 원래 URL, 게시일, 부서를 사용한다. 이미지/첨부파일 다운로드 및 OCR은 없다.

같은 정규화 JSON을 입력 해시 계산과 Gemini 입력에 사용한다. 공고 데이터는 사용자 메시지의 JSON이며 시스템 지침과 분리한다. 공고 안의 명령을 따르지 말고 불명확한 활동/관심 분야/대상을 추측하지 않도록 지시한다. 정보가 부족하면 unknown/needs_review, 근거 없는 배열은 빈 배열을 허용한다.

ResponseMIMEType=application/json과 ResponseSchema를 지정하고 반환 값을 Go에서 다시 검사한다. 필수 필드, null, 타입, 허용 enum, 중복 배열, 대표 분류의 categories 포함, unknown→needs_review, 짧은 근거 문자열을 검증한다. 완료되지 않거나 차단된 응답도 성공으로 저장하지 않는다. API 오류는 인증/403/404/429/400/시간초과를 구분한다.

## 저장과 조회

content_classifications의 고유 인덱스는 `(content_id, input_hash, taxonomy_version, model_version)`다. taxonomy_version은 `ssu-v1-prompt1`이다. 생성 전에 processing 예약을 삽입하여 동시 요청과 재실행의 중복 과금을 막는다. 성공 시 요청된 필드와 token usage 및 실제 응답 model version을 저장한다. model_version은 중복 방지에 사용한 요청 모델 ID이고 resolved_model_version은 공급자가 반환한 버전이다.

실패 시 failed/error_code로 기록하며 성공 label이나 classified_at을 만들지 않는다. 중단되어 processing에 남거나 실패한 동일 예약도 자동으로 재호출하지 않는다. 실패 후 재실행을 원하면 원인과 기존 예약을 사람이 확인해야 한다. 모델 alias의 공급자 내부 업데이트를 자동으로 새 분류로 간주하지 않으므로 재분류가 필요하면 taxonomy/prompt 버전을 명시적으로 갱신한다.

기존 Bearer 인증 아래 다음 GET을 추가했다.

```text
GET /api/v1/contents/{content_id}/classifications
```

최대 100개의 저장된 분류/시도 기록을 반환하며 존재하지 않는 공고는 404, 인증 없이는 401이다. 이 조회는 Gemini를 호출하지 않는다. 기존 Reader 인터페이스는 변경하지 않고 추가 조회 기능을 가진 Store만 확장 동작을 제공한다.

## 수동 실행

프로젝트 루트 `/Users/surfing_seal/Documents/ChatGPT/HCI`에서 실행한다. 이 명령은 실제 Google 읽기 API에 접속하므로 기본 테스트와 분리한다.

```sh
go run ./cmd/classify-one -report /tmp/vertex-preflight.json
# 사전 조건을 모두 해결한 뒤, 명시적으로 1회 생성을 허용할 때만 실행:
go run ./cmd/classify-one -execute -report /tmp/vertex-one-result.json
```

실행 전 MongoDB가 .env의 URI에 있어야 한다. 이번 테스트용 URI는 127.0.0.1:27029이며 다른 DB와 분리했다. API 접속 실패를 분류 성공으로 꾸미지 않는다. 명령의 안전한 보고서에 실제 생성 요청 수와 실패 단계가 들어간다.

## 검증 구분

Mock SDK 테스트는 실제 Go SDK를 가짜 HTTP transport에 연결하여 Structured Output 요청, 토큰 처리, 할당량 오류 무재시도, 2회차 전송 차단과 사전 점검 차단을 확인한다. 일반 테스트는 Google API를 호출하지 않는다.

TestActualNoticeMockClassificationMongoAPI는 실제 학교 HTML에서 파싱한 1문서를 실제 로컬 MongoDB에 저장하고 **가짜 분류 결과**로 저장·8개 경쟁자 중 1회 분류·실패 보존·인증 API 200/401을 검증한다. 모델 ID와 근거에 MOCK을 표시하며 임시 crawltest_... DB는 종료 시 삭제한다. 이것은 실제 Gemini 분류 성공 결과가 아니다.

기존 전체/race/vet/build 검사 및 실제 HTML→MongoDB→원본 카테고리 API 회귀도 유지한다. 초기 GCP 사전 점검 보고서는 outputs/vertex/preflight-final.json이고, 이후 성공한 실제 생성 결과는 outputs/vertex/direct-result.json이다.

## 현재 정책 변경

위 슈캐치 실제 AI 실험은 과거 검증 이력이다. 현재 슈캐치 Source는 source 전략으로
AI를 호출하지 않는다. 학과의 ai/hybrid 수동 실행은 저장된 -content-id를 요구하며
Source 정책을 SDK 생성보다 먼저 검사한다. 현재 8개 분류/제외 대상/새 명령은
[classification-strategies.md](classification-strategies.md)를 참조한다.
