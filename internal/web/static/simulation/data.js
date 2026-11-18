export const fields = [
  {"k":-1,"name":"지원대학"},
  {"k":-2,"name":"프로그램 구분"},
  {"k":-3,"name":"일련번호"},
  {"k":3,"name":"1지망 수강 희망 학과","value":"Business Administration"},
  {"k":4,"name":"2지망 프로그램 구분"},
  {"k":5,"name":"2지망 지원 대학"},
  {"k":6,"name":"2지망 일련번호"},
  {"k":7,"name":"2지망 수강 희망 학과","value":"경영학 (모의 작성 예시)"},
  {"k":8,"name":"3지망 프로그램 구분"},
  {"k":9,"name":"3지망 지원 대학"},
  {"k":10,"name":"3지망 일련번호"},
  {"k":11,"name":"3지망 수강 희망 학과","value":"경영학 (모의 작성 예시)"},
  {"k":12,"name":"이름(한글)","value":"이시연"},
  {"k":13,"name":"이름(영문)","value":"Siyeon Lee"},
  {"k":14,"name":"이름(중국어/일본어)","value":"해당 없음"},
  {"k":15,"name":"성별","value":"여"},
  {"k":16,"name":"생년월일","value":"2004.01.01."},
  {"k":17,"name":"휴대전화","value":"010-0000-0000"},
  {"k":18,"name":"자택번호","value":"해당 없음"},
  {"k":19,"name":"국적","value":"대한민국"},
  {"k":20,"name":"출생지","value":"서울"},
  {"k":21,"name":"주소","value":"서울특별시 동작구 (가상 주소)"},
  {"k":22,"name":"이메일","value":"student@example.com"},
  {"k":23,"name":"학번","value":"20260000"},
  {"k":24,"name":"학과(부)","value":"경영학부"},
  {"k":25,"name":"현재학기","value":"2학년 1학기"},
  {"k":26,"name":"재학 여부","value":"재학 중"},
  {"k":27,"name":"파견학기","value":"4학기"},
  {"k":28,"name":"복수 및 부전공","value":"해당 없음"},
  {"k":29,"name":"복수 및 부전공명","value":"해당 없음"},
  {"k":30,"name":"특성화장학생 여부","value":"해당 없음"},
  {"k":31,"name":"평점평균","value":"3.75"},
  {"k":32,"name":"공인어학시험명","value":"TOEFL IBT"},
  {"k":33,"name":"총점","value":"100"},
  {"k":34,"name":"어학성적 만료일","value":"2028.02.18."},
  {"k":35,"name":"읽기 세부점수","value":"25"},
  {"k":36,"name":"듣기 세부점수","value":"25"},
  {"k":37,"name":"말하기 세부점수","value":"25"},
  {"k":38,"name":"쓰기 세부점수","value":"25"},
  {"k":39,"name":"기타 어학시험","value":"TOEIC"},
  {"k":40,"name":"기타 어학시험 점수","value":"950"},
  {"k":41,"name":"기타 어학성적 만료일","value":"2028.02.09."},
  {"k":42,"name":"택일 시험명","value":"해당 없음"},
  {"k":43,"name":"택일 시험 점수","value":"해당 없음"},
  {"k":44,"name":"택일 성적 만료일","value":"해당 없음"},
  {"k":45,"name":"국제팀 프로그램 1","value":"국제교류 설명회"},
  {"k":46,"name":"국제팀 프로그램 2","value":"해당 없음"},
  {"k":47,"name":"작성일","value":"2026년 8월 10일"},
  {"k":48,"name":"신청인과 서명","value":"이시연 (모의 서명)"},
  {"k":49,"name":"국문 자기소개서 및 학업계획서","value":"경영학을 전공하며 문화에 따라 소비자의 선택이 달라지는 이유에 관심을 갖게 되었습니다. 파견 대학에서 국제마케팅 수업을 듣고 현지 학생과 팀 프로젝트를 수행하겠습니다. 귀국 후에는 프로젝트 결과를 전공 수업과 교내 교류 활동에 활용하겠습니다."},
  {"k":50,"name":"공인어학성적 언어 자기소개서 및 학업계획서","value":"I study business and want to understand how culture shapes consumer decisions. During the exchange, I plan to take international marketing courses and work with local students on a team project. After returning, I will apply what I learned to my coursework and share it with other students."}
];
export const groups = [
  { k: "schools", name: "지원대학 정보" },
  { k: "identity", name: "인적사항" },
  { k: "academics", name: "학적사항" },
  { k: "qualifications", name: "지원자격사항" },
  { k: "signature", name: "기타 · 작성일 및 서명" },
  { k: "korean", name: "국문 자기소개서 및 학업계획서" },
  { k: "foreign", name: "외국어 자기소개서 및 학업계획서" },
];

export const universities = [
  {
    "region": "북미",
    "location": "미국 · 일반교환",
    "name": "사우스다코다 대학교",
    "english": "University of South Dakota",
    "program": "일반교환",
    "code": "E0003",
    "alternates": [
      "E0012",
      "E0006"
    ]
  },
  {
    "region": "북미",
    "location": "캐나다 · 일반교환",
    "name": "컨커디어 대학교",
    "english": "Concordia University",
    "program": "일반교환",
    "code": "E0012",
    "alternates": [
      "E0006",
      "E0003"
    ]
  },
  {
    "region": "북미",
    "location": "미국 · 일반교환",
    "name": "잭슨빌 공립 대학교",
    "english": "Jacksonville State University",
    "program": "일반교환",
    "code": "E0006",
    "alternates": [
      "E0003",
      "E0012"
    ]
  },
  {
    "region": "유럽",
    "location": "독일 · 일반교환",
    "name": "DHBW 하이덴하임 대학교",
    "english": "Baden-Wuerttemberg Cooperative State University Heidenheim (DHBW)",
    "program": "일반교환",
    "code": "E0023",
    "alternates": [
      "E0110",
      "E0075"
    ]
  },
  {
    "region": "유럽",
    "location": "프랑스 · 일반교환",
    "name": "오덴시아 경영 대학교",
    "english": "Audencia Nantes School of Management",
    "program": "일반교환",
    "code": "E0110",
    "alternates": [
      "E0075",
      "E0016"
    ]
  },
  {
    "region": "유럽",
    "location": "영국 · 일반교환",
    "name": "킬 대학교",
    "english": "University of Keele",
    "program": "일반교환",
    "code": "E0075",
    "alternates": [
      "E0016",
      "E0058"
    ]
  },
  {
    "region": "유럽",
    "location": "네덜란드 · 일반교환",
    "name": "한제 대학교",
    "english": "Hanze University of Applied Sciences",
    "program": "일반교환",
    "code": "E0016",
    "alternates": [
      "E0058",
      "E0023"
    ]
  },
  {
    "region": "유럽",
    "location": "스페인 · 일반교환",
    "name": "U-tad 대학교",
    "english": "University Center for Technology and Digital Art",
    "program": "일반교환",
    "code": "E0058",
    "alternates": [
      "E0023",
      "E0110"
    ]
  },
  {
    "region": "아시아",
    "location": "일본 · 일반교환",
    "name": "삿포로가쿠인 대학교(삿포로학원 대학교)",
    "english": "Sapporo Gakuin University(札幌学院大学)",
    "program": "일반교환",
    "code": "E0191",
    "alternates": [
      "E0208",
      "E0165"
    ]
  },
  {
    "region": "아시아",
    "location": "중국 · 일반교환",
    "name": "길림사범 대학교",
    "english": "Jilin Normal University(吉林师范大学)",
    "program": "일반교환",
    "code": "E0208",
    "alternates": [
      "E0165",
      "E0180"
    ]
  },
  {
    "region": "아시아",
    "location": "대만 · 일반교환",
    "name": "국립중앙 대학교",
    "english": "National Central University(國立中央大學)",
    "program": "일반교환",
    "code": "E0165",
    "alternates": [
      "E0180",
      "E0191"
    ]
  },
  {
    "region": "아시아",
    "location": "베트남 · 일반교환",
    "name": "똔득탕 대학교",
    "english": "Ton Duc Thang University",
    "program": "일반교환",
    "code": "E0180",
    "alternates": [
      "E0191",
      "E0208"
    ]
  },
  {
    "region": "남미",
    "location": "브라질 · 일반교환",
    "name": "바레도이타자야 대학교",
    "english": "University of Vale Do Itajai",
    "program": "일반교환",
    "code": "E0147",
    "alternates": [
      "E0003",
      "E0012"
    ]
  },
  {
    "region": "오세아니아",
    "location": "호주 · 일반교환",
    "name": "찰스 스터트 대학교",
    "english": "Charles Sturt University in Australia",
    "program": "일반교환",
    "code": "E0014",
    "alternates": [
      "E0191",
      "E0208"
    ]
  },
  {
    "region": "아프리카",
    "location": "튀니지 · 일반교환",
    "name": "카르타고 대학교",
    "english": "The University of Carthage",
    "program": "일반교환",
    "code": "E0164",
    "alternates": [
      "E0023",
      "E0110"
    ]
  }
];

export const questions = [
  {
    "title": "교환학생 프로그램에 지원한 이유는 무엇인가요?",
    "options": [
      "왜 물어보시는데요?",
      "그냥 해외여행 가고 싶어서요.",
      "친구가 지원한다길래 따라왔어요.",
      "해외 대학의 교육 방식과 전공 수업을 경험하고, 이를 통해 얻은 지식과 새로운 관점을 제 학업에 활용하고 싶기 때문입니다."
    ],
    "guide": "단순히 해외에 가고 싶다는 이유보다, 무엇을 배우고 싶은지 구체적으로 설명하는 것이 좋아요.",
    "answer": "D"
  },
  {
    "title": "파견 대학에서 어떤 학업 계획을 가지고 있나요?",
    "options": [
      "아직 생각 안 해봤는데요?",
      "일단 가서 생각하려고요.",
      "희망 대학의 전공 과목과 학점 인정 여부를 미리 확인하고, 제 전공 및 진로 목표에 맞춰 수강 계획을 세우겠습니다.",
      "수업은 꼭 들어야 하나요?"
    ],
    "guide": "교환학생도 학업을 위한 프로그램이에요. 희망 대학에서 배우고 싶은 내용과 구체적인 수강 계획을 설명하는 것이 중요해요.",
    "answer": "C"
  },
  {
    "title": "해외 생활 중 예상하지 못한 어려움이 생긴다면 어떻게 대처하겠습니까?",
    "options": [
      "그냥 한국으로 돌아오면 안 되나요?",
      "상황을 파악하고 해결 방법을 찾아보겠습니다. 혼자 해결하기 어렵다면 파견 대학의 담당 부서나 숭실대학교 국제팀에 도움을 요청하겠습니다.",
      "그건 학교에서 알아서 해결해 줘야 하는 거 아닌가요?",
      "그런 일은 안 생기겠죠."
    ],
    "guide": "낯선 환경에서 문제가 생길 수 있다는 점을 이해하고, 스스로 해결하려는 태도와 적절하게 도움을 요청하는 방법을 보여주는 것이 좋아요.",
    "answer": "B"
  }
];