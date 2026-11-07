# 슈캐치 실제 HTTP 수집 검증 — 2026-11-06

이번 결과는 합성 Fixture 수집이 아니라 **실제 https://scatch.ssu.ac.kr/공지사항/ 에서 가져온 HTML**의 결과다. 사용자 명시 승인 후 기존 SSUCatchCrawler와 Fetcher를 실행했다. 별도 유료 모델 호출은 하지 않았다.

## 수집 범위와 실행 결과

- 목록 1페이지, 상세 **3건** 성공. 기존 MVP 한도인 공고 최대 3건·총 HTTP 최대 5회를 유지했으며 요청하신 최대 5건 이내다.
- robots 1 + 목록 1 + 상세 3 = **직접 HTTP 5회**, 모두 200, 재시도·리다이렉트 0회. 동시성 1, 실제 요청 시작 간격 모두 2초 이상.
- 실제 서버 응답에 공지와 본문이 있어 브라우저/JavaScript 렌더링은 필요하지 않았다.
- robots의 Allow / 및 Disallow /wp-content/plugins/, /wp-admin, /download.php를 확인했다. 금지 경로, 이미지, 첨부 다운로드는 0회다. 첨부 링크는 메타데이터로만 보존했다.
- 이번 단일 수동 실행에만 사용자의 승인을 적용했다. 영구 설정의 enabled/auto_enabled/policy_approved=false와 SSU source 분류 전략, `.env`와 인증·모델은 유지했다. 학교의 별도 재이용 승인을 받았다는 뜻으로 기록하지 않았다.
- 실제 학교 목록 15행 중 공식 채용 5행을 제외하고 대상 URL 10개를 확인했다. 허용 대상 앞 3건만 상세 요청했다. 교원채용 행은 이번 페이지에 없었으며 기존 회귀 테스트에서 제외 규칙을 계속 검증한다.

| 실제 수집 제목 | 공식 분류 → 정규화 | 본문 문자 수 | 첨부 메타데이터 |
|---|---|---:|---:|
| [SIC] 2026학년도 2학기 SIC 4쿼터정기 교환학생 세미나 참가자 모집 안내 | 국제교류 → international | 1841 | 1 |
| [가양7종합사회복지관] 제 5회 가양7놀이대회 자원봉사자 모집 안내 | 봉사 → volunteer | 110 | 1 |
| [초록우산] 2026 우리함께런 with 빗썸나눔 자원봉사자 모집 안내 | 봉사 → volunteer | 998 | 1 |

등록부서는 순서대로 국제팀, 사회공헌팀, 사회공헌팀이다. 작성일은 한국 시간 기준 2026-11-06, 2026-11-06, 2026-11-04이며 목록과 상세 헤더 날짜가 모두 일치했다. 본문 길이는 Unicode 문자 수이며 이미지 OCR 내용은 포함하지 않는다.

## 실제 선택자·원본 검증

기존 실제 HTML에서 검토한 `#contents .bg-white > hr + div`를 그대로 사용했다. 세 공지 모두 본문 1개이고 비어 있지 않다. 제목은 `#contents .bg-white > h1`, 날짜는 이번 실제 DOM에서 확인한 `#contents .bg-white > h1 + .clearfix > .float-left.mr-4`로 목록 값과 대조했다. 메뉴/푸터/다운로드 UI가 본문에 섞이지 않는지 미리보기를 검토했다.

첨부파일은 각각 SIC 세미나 포스터 JPG, 가양7 자원봉사 홍보지 JPG, 초록우산 포스터 PNG 한 개씩이다. 이미지가 존재함과 미분석 상태를 보존했다. 다운로드 파일의 내용·열림 여부까지 검증한 것으로 보고하지 않는다.

`capture-1/`에는 받은 원본 HTML, robots, preview.json, URL·시각·SHA256·HTTP 상태가 있는 run.json이 있다. inspection.json은 실제 DOM 검토 결과다. 프로젝트 회귀 테스트에는 이 원본을 복사하여 오프라인으로 재검증하도록 추가했다. **원본을 활용한 회귀 테스트와 최초 실제 HTTP 실행은 구분한다.**

## 제외 공고 — 상세 요청/저장 안 함

- 2026년 숭실대학교 기초교양교육연구센터 기초교양교육연구팀 계약직 직원 모집 — 채용
- 자연과학대학 학사조교 모집 공고(물리학과) — 채용
- [교수학습혁신센터] 2026학년도 2학기 XR 콘텐츠 활용 실감형 수업 운영 보조 인력 추가 모집 — 채용
- 숭실대학교 공과대학 학사조교 (산업정보시스템공학과) 채용 공고 — 채용
- 숭실대학교 지식정보처 중앙도서관 계약직 직원 모집 — 채용

## 페이지네이션·중복 검증 범위

- 실제 첫 페이지의 다음 링크 `/공지사항/page/2/`를 파싱했다. 한 페이지 제한에 따라 실제 2페이지 요청은 하지 않았다.
- 실제 첫 페이지 URL 중복은 0건이다. 고정 공지임을 명확히 나타내는 표식은 확인하지 못했다. `.start`를 임의로 고정 공지라고 해석하지 않았다.
- 회귀 테스트에서 **실제 행을 로컬에서 의도적으로 한 번 반복**시켜도 canonical slug URL 기준 10건을 유지하는 것을 확인한다. 이는 중복 로직 테스트이며 실제 고정 공지가 여러 페이지에 반복되는 것을 실측했다고 주장하지 않는다.
- slug만 남겨 category/paged/keyword 등 목록 복귀 쿼리를 제거했다.

## MongoDB·REST API

실제 미리보기를 먼저 검토한 뒤 원본 SHA256을 다시 확인하고 상세 제목·날짜·본문·첨부를 재파싱한 결과가 동일할 때에만 저장했다. 전용 새 MongoDB 데이터 디렉터리 `work/ssu-live-mongo`, 루프백 포트 27030을 사용했다. 운영 .env를 읽거나 운영 DB에 연결하지 않았다.

`crawltest_ssulive_6aed66a273f50cc28c42e2e7` 임시 DB에 **3건** 저장했다. 같은 URL 데이터를 두 번 저장해도 3건이며 ID·본문 해시·updated_at을 유지했다. 이 재저장은 동일 실제 HTML 결과를 재사용한 것으로, 학교 HTTP를 다시 요청한 것은 아니다.

기존 ClassifyOne을 nil classifier로 실행하여 source 정규화 경로만 사용했으며 content_classifications 문서는 0개다. 실제 Gemini 호출은 0회다. 기존 인증 REST Handler에서 international=1건, volunteer=2건, HTTP 200을 검증했다. 운영 HTTP 서버를 재시작하지 않았다. 상세 DB/API 결과는 capture-1/mongo.json에 있다. 실제 수집 검증용 임시 DB는 별도 디렉터리에 보존한다.

## 실제 접근 URL 원장

1. https://scatch.ssu.ac.kr/robots.txt (HTTP 200, 2026-11-06T05:31:30.213083Z)
2. https://scatch.ssu.ac.kr/%EA%B3%B5%EC%A7%80%EC%82%AC%ED%95%AD/ (HTTP 200, 2026-11-06T05:31:32.214201Z)
3. https://scatch.ssu.ac.kr/%EA%B3%B5%EC%A7%80%EC%82%AC%ED%95%AD/?slug=sic-2026%ED%95%99%EB%85%84%EB%8F%84-2%ED%95%99%EA%B8%B0-sic-4%EC%BF%BC%ED%84%B0%EC%A0%95%EA%B8%B0-%EA%B5%90%ED%99%98%ED%95%99%EC%83%9D-%EC%84%B8%EB%AF%B8%EB%82%98-%EC%B0%B8%EA%B0%80%EC%9E%90 (HTTP 200, 2026-11-06T05:31:34.215278Z)
4. https://scatch.ssu.ac.kr/%EA%B3%B5%EC%A7%80%EC%82%AC%ED%95%AD/?slug=%EA%B0%80%EC%96%917%EC%A2%85%ED%95%A9%EC%82%AC%ED%9A%8C%EB%B3%B5%EC%A7%80%EA%B4%80-%EC%A0%9C-5%ED%9A%8C-%EA%B0%80%EC%96%917%EB%86%80%EC%9D%B4%EB%8C%80%ED%9A%8C-%EC%9E%90%EC%9B%90%EB%B4%89%EC%82%AC (HTTP 200, 2026-11-06T05:31:36.216365Z)
5. https://scatch.ssu.ac.kr/%EA%B3%B5%EC%A7%80%EC%82%AC%ED%95%AD/?slug=%EC%B4%88%EB%A1%9D%EC%9A%B0%EC%82%B0-2026-%EC%9A%B0%EB%A6%AC%ED%95%A8%EA%BB%98%EB%9F%B0-with-%EB%B9%97%EC%8D%B8%EB%82%98%EB%88%94-%EC%9E%90%EC%9B%90%EB%B4%89%EC%82%AC%EC%9E%90-%EB%AA%A8%EC%A7%91 (HTTP 200, 2026-11-06T05:31:38.217045Z)

## 재실행 명령과 자동 호출 방지

원본 프로젝트에서 다음 수동 명령을 제공한다. 기본 실행은 요청을 하지 않으며 명시적인 -execute가 필요하다. 새 빈 출력 디렉터리를 지정해야 한다. 서버/일반 테스트에서 이 명령을 호출하지 않는다.

```sh
go run ./cmd/validate-ssu -execute -output-dir /absolute/new/evidence-directory
```

미리보기 검토 후 실제 원본 해시와 필수 항목을 재검증하여 전용 임시 DB에만 저장한다. 이 모드에서는 학교 요청이 없다. 별도 로컬 MongoDB 127.0.0.1:27030이 필요하다.

```sh
go run ./cmd/validate-ssu -store-preview -output-dir /absolute/existing/evidence-directory
```

## 전체 검사와 변경 범위

전체 테스트·race·vet·build의 최종 결과는 tests.json 및 go-*.log에 기록한다. TEST_MONGODB_URI=27030과 SSU_REQUIRE_ACTUAL_DETAIL=1로 기존 MongoDB 및 실제 SSU Fixture 회귀를 포함한다. 일반 테스트는 학교/유료 모델 요청을 하지 않는다.

크롤러에는 선택적인 증거 캡처·요청 원장 콜백만 추가했고 기존 Crawler 인터페이스, 한도, 파싱 선택자, Repository, Runner 및 REST API는 재사용했다. 글로벌미디어학부 구현과 설정은 수정하지 않았다. 자동 수집도 활성화하지 않았다.

실제 접근/파싱/저장 실패는 이번 실행에서 없었다. 다만 실제 여러 페이지의 반복 고정 공지와 첨부파일 내부 내용은 검증 범위 밖이다.
