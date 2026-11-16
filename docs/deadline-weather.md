# SSU:CAST — Deadline Weather 디자인 시스템 및 산정 정책

## 1. 핵심 콘셉트 및 디자인 원칙

**SSU:CAST (숭실대학교 공고 일기예보)**는 공고 마감의 긴급도와 밀집도를 기상 예보 메타포로 시각화하여 학생이 중요한 신청·제출 기한을 놓치지 않도록 돕는 정보 서비스다.

- **Information First**: 날씨 시각화는 공고 탐색을 돕는 보조 수단이며, 항상 실제 마감 건수·날짜·남은 시간을 함께 표기한다.
- **기상예보 혼동 방지**: 화면 상단에 "실제 기상 정보가 아닌 모집 마감 긴급도 예보(Asia/Seoul 기준)"임을 명시한다.
- **개인화 정직성**: 기본값은 **전체 공고 기준 마감 예보**로 동작하며, 사용자가 분야 필터를 선택한 경우에만 선택 분야 기준으로 표시한다.
- **불안 조장 금지**: 과도한 적색 점멸이나 경고음을 배제하고 차분하고 명확한 상태 색상을 사용한다.

---

## 2. 디자인 시스템 (Design System)

### 2.1 색상 토큰 (Color Palette — WCAG AA 4.5:1 이상 확보)
- **Brand & Surface**:
  - `--navy`: `#13294B` (Primary Brand / Heading)
  - `--blue`: `#1D4ED8` (Interactive Accent / Link)
  - `--bg`: `#F6F8FB` (App Canvas)
  - `--surface`: `#FFFFFF` (Card / Dialog Surface)
  - `--surface-subtle`: `#F1F5F9` (Secondary Panel / Filter Bar)
  - `--text`: `#0F172A` (Primary Body, contrast 17.4:1)
  - `--muted`: `#475569` (Secondary Meta, contrast 7.5:1 on white)
  - `--line`: `#DCE4EE` (Hairline Border)
- **Weather Semantic Tokens**:
  - **맑음 (`clear` / `calm`)**: Surface `#F4FBF8`, Border `#BFE5D8`, Badge `#0F5132` on `#D1FAE5`, Icon `#D97706`
  - **구름 (`cloudy` / `watch`)**: Surface `#F6F8FC`, Border `#CBD5E1`, Badge `#334155` on `#E2E8F0`, Icon `#64748B`
  - **비 (`rain` / `alert`)**: Surface `#EFF6FF`, Border `#BFDBFE`, Badge `#1E40AF` on `#DBEAFE`, Icon `#2563EB`
  - **폭풍 (`storm` / `critical`)**: Surface `#FFF8F3`, Border `#F5D0FE`, Badge `#9A3412` on `#FFEDD5`, Icon `#D97706`

### 2.2 타이포그래피 및 수치 위계
- **Font Stack**: `Pretendard Variable`, `-apple-system`, `BlinkMacSystemFont`, `"Apple SD Gothic Neo"`, `"Malgun Gothic"`, `sans-serif`
- **Scale & Measure**:
  - Hero / Weather Title: `clamp(1.35rem, 2.2vw, 1.75rem)`, `font-weight: 750`, `letter-spacing: -0.025em`
  - Card Title: `1rem (16px)`, `line-height: 1.55`, `font-weight: 680`, `word-break: keep-all`
  - Body / Summary: `0.875rem (14px)` ~ `0.9375rem (15px)`, `line-height: 1.75`, 최대 읽기 폭 `68ch`
  - Meta / Badge: 최소 `0.75rem (12px)` 이상 유지, 모든 날짜·카운트·남은 시간에 `font-variant-numeric: tabular-nums` 적용
- **Icon System**: 이모지나 임의 유니코드 기호 대신 일관된 스트로크 두께(`1.8px~2px`)의 인라인 SVG 아이콘만 사용한다.

### 2.3 반응형 및 터치 기준
- **Desktop (`1440px`)**: 2열 대시보드 날씨 보드 + `220px` 사이드바 + 2열 공고 카드 그리드
- **Tablet (`768px~960px`)**: 단일 열 날씨 보드 + 가로 스크롤 카테고리 바 + 2열 카드 그리드
- **Mobile (`375px~550px`)**: 모바일 우선 단일 열 카드, 터치 타깃 최소 `40×40px`~`44×44px`, 가로 오버플로(`scrollWidth > innerWidth`) 0건 보장

---

## 3. 마감 날씨 산정 정책 (결정적 순수 함수)

### 3.1 전체(오늘의) 마감 날씨 — 향후 긴급도 기준
현재 시각(`now`, `Asia/Seoul`) 기준 진행 중인 공고의 **확정 모집 마감(`application`, `submission`, `completion`)**에 대해 계산한다.

- **`urgent24hCount`**: 남은 시간이 `0 < remaining <= 24시간`인 확정 마감 건수 (시각 미기재 일정은 `end_date === todayKST`인 경우 포함)
- **`within3dCount`**: `Asia/Seoul` 달력 기준 `0 <= calendarDays(todayKST, end_date) <= 3`인 확정 마감 건수 (오늘~D-3)

| 단계 (`level`) | 레이블 | 오늘의 마감 날씨 조건 (상위 우선) | 결정 사유(`reason`) 출력 예시 |
|---|---|---|---|
| `storm` | **폭풍** | `urgent24hCount >= 2` **OR** `within3dCount >= 4` | 24시간 내 확정 마감 N건이 몰려 있어 폭풍 단계로 산정되었습니다. |
| `rain` | **비** | `urgent24hCount === 1` **OR** `within3dCount >= 2` | 24시간 내 확정 마감 1건이 있어 비 단계로 산정되었습니다. |
| `cloudy` | **구름** | `within3dCount === 1` | 3일 이내 확정 마감 1건이 있어 구름 단계로 산정되었습니다. |
| `clear` | **맑음** | `urgent24hCount === 0` **AND** `within3dCount === 0` | 향후 3일 이내 확정된 모집 마감이 없어 맑음 단계로 산정되었습니다. |

### 3.2 7일간의 마감 예보 — 날짜별 마감 분포 기준
7일 예보(`today + 0` ~ `today + 6`)는 오늘의 누적 긴급도와 혼동되지 않도록 **해당 날짜(`D`) 당일에 마감되는 확정 공고 분포(`dayCount`)**를 나타낸다.

| 단계 (`level`) | 레이블 | 날짜별(`D`) 분포 조건 | 데이터 상태(`dataState`) 구분 |
|---|---|---|---|
| `storm` | **폭풍** | 해당 날짜 마감 `dayCount >= 3` (오늘인 경우 `dayCount >= 2`) | `has_deadlines` (`마감 N건`) |
| `rain` | **비** | 해당 날짜 마감 `dayCount === 2` (오늘인 경우 `dayCount === 1`) | `has_deadlines` (`마감 N건`) |
| `cloudy` | **구름** | 해당 날짜 마감 `dayCount === 1` (미래 날짜) | `has_deadlines` (`마감 1건`) |
| `clear` | **맑음** | 해당 날짜 확정 마감 `dayCount === 0` | 확정 0건 & 미검증 일정 존재 시 `unverified_only` (`확인 필요`), 완전 0건 시 `empty` (`마감 없음`) |

### 3.3 필수 데이터 검증 원칙
1. **행사·발표 제외**: `event`, `interview`, `announcement`, `selection` 및 시작일(`start_date`)은 마감 계산에서 제외한다.
2. **완료·만료 제외**: `status === "완료"`이거나 마감 경계가 현재 시각 이하인 일정은 제외한다.
3. **불확실 일정 분리**: `uncertain`, `blocked`, `review_required=true`, 날짜 미상(`end_date=null`) 일정은 확정 날씨에서 제외하고 하단 확인 필요 목록(`uncertainNotices`)과 상세 화면 경고 배지로 별도 안내한다.
4. **시각 미기재 보존**: `end_date`만 있고 `end_time`이 없으면 `23:59`를 임의로 표기하지 않고 `"오늘 마감 (시각 미기재)"` / `"D-N (시각 미기재)"`로 표시한다.
5. **중복 제거**: 동일 공고(`notice.id`)의 동일 `(end_date, end_time)` 마감은 1건으로 집계한다.

## 2026-11-16 UI 반영

날씨 산정 정책은 유지하면서 헤더·예보 보드·모바일 배치와 카드 일정 선택을 개선했다. 실제 CSS에 반영한 최신 색상·레이아웃 결정, 변경 파일 및 검증 결과는 [UI 개선 기록](ui-refresh-2026-11-16.md)을 기준으로 한다. 카드의 남은 시간과 일정 미리보기는 같은 미래 확정 일정을 사용한다.
