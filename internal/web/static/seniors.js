// AI-authored fictional examples. Not interviews, verified outcomes, or school deadlines.
(function(root){
'use strict';
const personas=[
  {
    "id": "jimin",
    "name": "지민",
    "major": "컴퓨터학부",
    "grade": "3학년",
    "data_origin": "synthetic",
    "generation_method": "ai_authored_example",
    "activities": [
      {
        "id": "jimin-1",
        "category": "scholarship",
        "title": "장학금 지원 준비",
        "timeline": [
          {
            "date": "2025-11-05",
            "label": "지원서 초안 작성"
          },
          {
            "date": "2025-11-20",
            "label": "서류 제출"
          }
        ],
        "prepared": [
          "활동 및 학업 경험 정리",
          "지원서 초안 작성",
          "제출 서류 목록 확인"
        ],
        "advice": "관심 장학금의 올해 지원 자격과 제출 서류를 확인해보세요."
      },
      {
        "id": "jimin-2",
        "category": "campus",
        "title": "교내 기획단 지원",
        "timeline": [
          {
            "date": "2025-11-10",
            "label": "기획단 지원"
          },
          {
            "date": "2025-11-27",
            "label": "면접 준비"
          }
        ],
        "prepared": [
          "참여하고 싶은 역할 정리",
          "팀 프로젝트 경험 정리",
          "예상 질문으로 면접 연습"
        ],
        "advice": "지난 프로젝트에서 맡았던 역할과 배운 점을 정리해보세요."
      },
      {
        "id": "jimin-3",
        "category": "internship",
        "title": "동계 인턴십 준비",
        "timeline": [
          {
            "date": "2025-11-03",
            "label": "공고 탐색"
          },
          {
            "date": "2025-11-17",
            "label": "자기소개서 준비"
          },
          {
            "date": "2025-11-24",
            "label": "지원 완료"
          }
        ],
        "prepared": [
          "수업 프로젝트 경험 정리",
          "자기소개서와 포트폴리오 초안",
          "지원 서류와 제출 형식 확인"
        ],
        "advice": "관심 직무를 정하고 프로젝트 경험을 정리해보세요."
      },
      {
        "id": "jimin-4",
        "category": "campus",
        "title": "겨울방학 학습 프로그램",
        "timeline": [
          {
            "date": "2025-12-04",
            "label": "프로그램 비교"
          },
          {
            "date": "2025-12-15",
            "label": "학습 계획 작성"
          },
          {
            "date": "2025-12-22",
            "label": "참여 신청"
          }
        ],
        "prepared": [
          "방학 중 가능한 시간 확인",
          "배우고 싶은 주제 정리",
          "개인 학습 계획 작성"
        ],
        "advice": "방학 시간표를 정리하고 관심 있는 교내 프로그램을 찾아보세요."
      },
      {
        "id": "jimin-5",
        "category": "scholarship",
        "title": "새 학기 장학금 준비",
        "timeline": [
          {
            "date": "2026-01-08",
            "label": "지원 조건 확인"
          },
          {
            "date": "2026-01-19",
            "label": "학업 계획 정리"
          },
          {
            "date": "2026-02-05",
            "label": "신청 서류 점검"
          }
        ],
        "prepared": [
          "학업 목표 정리",
          "지원 조건과 서류 점검",
          "제출 전 파일 형식 확인"
        ],
        "advice": "올해 공식 공고에서 지원 대상과 필요한 증빙을 확인하세요."
      },
      {
        "id": "jimin-6",
        "category": "internship",
        "title": "새 학기 직무 경험 준비",
        "timeline": [
          {
            "date": "2026-01-13",
            "label": "관심 직무 조사"
          },
          {
            "date": "2026-01-27",
            "label": "포트폴리오 정리"
          },
          {
            "date": "2026-02-12",
            "label": "지원서 검토"
          }
        ],
        "prepared": [
          "관심 직무의 업무 조사",
          "프로젝트 결과물 정리",
          "지원서 오탈자 점검"
        ],
        "advice": "지원하고 싶은 분야의 요구 역량과 본인 경험을 연결해보세요."
      }
    ]
  },
  {
    "id": "seoyeon",
    "name": "서연",
    "major": "경영학부",
    "grade": "2학년",
    "data_origin": "synthetic",
    "generation_method": "ai_authored_example",
    "activities": [
      {
        "id": "seoyeon-1",
        "category": "scholarship",
        "title": "교내 장학금 지원 준비",
        "timeline": [
          {
            "date": "2025-11-06",
            "label": "지원서 초안 작성"
          },
          {
            "date": "2025-11-21",
            "label": "서류 제출"
          }
        ],
        "prepared": [
          "활동 및 학업 경험 정리",
          "지원서 초안 작성",
          "제출 서류 목록 확인"
        ],
        "advice": "관심 장학금의 올해 지원 자격과 제출 서류를 확인해보세요."
      },
      {
        "id": "seoyeon-2",
        "category": "campus",
        "title": "학생 홍보단 지원",
        "timeline": [
          {
            "date": "2025-11-11",
            "label": "기획단 지원"
          },
          {
            "date": "2025-11-28",
            "label": "면접 준비"
          }
        ],
        "prepared": [
          "참여하고 싶은 역할 정리",
          "팀 프로젝트 경험 정리",
          "예상 질문으로 면접 연습"
        ],
        "advice": "지난 프로젝트에서 맡았던 역할과 배운 점을 정리해보세요."
      },
      {
        "id": "seoyeon-3",
        "category": "internship",
        "title": "동계 기업 실무 경험 준비",
        "timeline": [
          {
            "date": "2025-11-04",
            "label": "공고 탐색"
          },
          {
            "date": "2025-11-18",
            "label": "자기소개서 준비"
          },
          {
            "date": "2025-11-25",
            "label": "지원 완료"
          }
        ],
        "prepared": [
          "수업 프로젝트 경험 정리",
          "자기소개서와 포트폴리오 초안",
          "지원 서류와 제출 형식 확인"
        ],
        "advice": "관심 직무를 정하고 프로젝트 경험을 정리해보세요."
      },
      {
        "id": "seoyeon-4",
        "category": "campus",
        "title": "방학 학습동아리 참여",
        "timeline": [
          {
            "date": "2025-12-05",
            "label": "프로그램 비교"
          },
          {
            "date": "2025-12-16",
            "label": "학습 계획 작성"
          },
          {
            "date": "2025-12-23",
            "label": "참여 신청"
          }
        ],
        "prepared": [
          "방학 중 가능한 시간 확인",
          "배우고 싶은 주제 정리",
          "개인 학습 계획 작성"
        ],
        "advice": "방학 시간표를 정리하고 관심 있는 교내 프로그램을 찾아보세요."
      },
      {
        "id": "seoyeon-5",
        "category": "scholarship",
        "title": "새 학기 학업 지원 준비",
        "timeline": [
          {
            "date": "2026-01-09",
            "label": "지원 조건 확인"
          },
          {
            "date": "2026-01-20",
            "label": "학업 계획 정리"
          },
          {
            "date": "2026-02-06",
            "label": "신청 서류 점검"
          }
        ],
        "prepared": [
          "학업 목표 정리",
          "지원 조건과 서류 점검",
          "제출 전 파일 형식 확인"
        ],
        "advice": "올해 공식 공고에서 지원 대상과 필요한 증빙을 확인하세요."
      },
      {
        "id": "seoyeon-6",
        "category": "internship",
        "title": "마케팅 직무 경험 준비",
        "timeline": [
          {
            "date": "2026-01-14",
            "label": "관심 직무 조사"
          },
          {
            "date": "2026-01-28",
            "label": "포트폴리오 정리"
          },
          {
            "date": "2026-02-13",
            "label": "지원서 검토"
          }
        ],
        "prepared": [
          "관심 직무의 업무 조사",
          "프로젝트 결과물 정리",
          "지원서 오탈자 점검"
        ],
        "advice": "지원하고 싶은 분야의 요구 역량과 본인 경험을 연결해보세요."
      }
    ]
  },
  {
    "id": "doyun",
    "name": "도윤",
    "major": "글로벌미디어학부",
    "grade": "3학년",
    "data_origin": "synthetic",
    "generation_method": "ai_authored_example",
    "activities": [
      {
        "id": "doyun-1",
        "category": "scholarship",
        "title": "창작 활동 장학금 준비",
        "timeline": [
          {
            "date": "2025-11-07",
            "label": "지원서 초안 작성"
          },
          {
            "date": "2025-11-22",
            "label": "서류 제출"
          }
        ],
        "prepared": [
          "활동 및 학업 경험 정리",
          "지원서 초안 작성",
          "제출 서류 목록 확인"
        ],
        "advice": "관심 장학금의 올해 지원 자격과 제출 서류를 확인해보세요."
      },
      {
        "id": "doyun-2",
        "category": "campus",
        "title": "교내 미디어 기획단 지원",
        "timeline": [
          {
            "date": "2025-11-12",
            "label": "기획단 지원"
          },
          {
            "date": "2025-11-29",
            "label": "면접 준비"
          }
        ],
        "prepared": [
          "참여하고 싶은 역할 정리",
          "팀 프로젝트 경험 정리",
          "예상 질문으로 면접 연습"
        ],
        "advice": "지난 프로젝트에서 맡았던 역할과 배운 점을 정리해보세요."
      },
      {
        "id": "doyun-3",
        "category": "internship",
        "title": "콘텐츠 분야 인턴십 준비",
        "timeline": [
          {
            "date": "2025-11-05",
            "label": "공고 탐색"
          },
          {
            "date": "2025-11-19",
            "label": "자기소개서 준비"
          },
          {
            "date": "2025-11-26",
            "label": "지원 완료"
          }
        ],
        "prepared": [
          "수업 프로젝트 경험 정리",
          "자기소개서와 포트폴리오 초안",
          "지원 서류와 제출 형식 확인"
        ],
        "advice": "관심 직무를 정하고 프로젝트 경험을 정리해보세요."
      },
      {
        "id": "doyun-4",
        "category": "campus",
        "title": "겨울방학 창작 프로그램",
        "timeline": [
          {
            "date": "2025-12-06",
            "label": "프로그램 비교"
          },
          {
            "date": "2025-12-17",
            "label": "학습 계획 작성"
          },
          {
            "date": "2025-12-24",
            "label": "참여 신청"
          }
        ],
        "prepared": [
          "방학 중 가능한 시간 확인",
          "배우고 싶은 주제 정리",
          "개인 학습 계획 작성"
        ],
        "advice": "방학 시간표를 정리하고 관심 있는 교내 프로그램을 찾아보세요."
      },
      {
        "id": "doyun-5",
        "category": "scholarship",
        "title": "새 학기 장학 지원 준비",
        "timeline": [
          {
            "date": "2026-01-10",
            "label": "지원 조건 확인"
          },
          {
            "date": "2026-01-21",
            "label": "학업 계획 정리"
          },
          {
            "date": "2026-02-07",
            "label": "신청 서류 점검"
          }
        ],
        "prepared": [
          "학업 목표 정리",
          "지원 조건과 서류 점검",
          "제출 전 파일 형식 확인"
        ],
        "advice": "올해 공식 공고에서 지원 대상과 필요한 증빙을 확인하세요."
      },
      {
        "id": "doyun-6",
        "category": "internship",
        "title": "디자인 직무 경험 준비",
        "timeline": [
          {
            "date": "2026-01-15",
            "label": "관심 직무 조사"
          },
          {
            "date": "2026-01-29",
            "label": "포트폴리오 정리"
          },
          {
            "date": "2026-02-14",
            "label": "지원서 검토"
          }
        ],
        "prepared": [
          "관심 직무의 업무 조사",
          "프로젝트 결과물 정리",
          "지원서 오탈자 점검"
        ],
        "advice": "지원하고 싶은 분야의 요구 역량과 본인 경험을 연결해보세요."
      }
    ]
  }
];

const labels={scholarship:'장학금',campus:'교내 활동',internship:'인턴십'};
function displayDate(source){
 if(!/^\d{4}-\d{2}-\d{2}$/.test(source))return null;
 const original=new Date(source+'T00:00:00Z');if(Number.isNaN(original.valueOf())||original.toISOString().slice(0,10)!==source)return null;
 const date=String(original.getUTCFullYear()+1)+source.slice(4);
 const shifted=new Date(date+'T00:00:00Z');return shifted.toISOString().slice(0,10)===date?date:null;
}
function events(personaId,category='all'){
 const p=personas.find(p=>p.id===personaId&&p.data_origin==='synthetic');if(!p)return [];
 return p.activities.filter(a=>category==='all'||a.category===category).flatMap(a=>a.timeline.flatMap((step,index)=>{
  const date=displayDate(step.date);return date?[{date,sourceDate:step.date,title:a.title,label:step.label,type:'senior',senior:true,persona:p,activity:a,stepIndex:index,category:a.category}]:[];
 })).sort((a,b)=>a.date.localeCompare(b.date)||a.title.localeCompare(b.title,'ko'));
}
const api={personas,labels,events,displayDate};
if(typeof module!=='undefined'&&module.exports)module.exports=api;else root.SSUSeniors=api;
})(typeof window!=='undefined'?window:globalThis);
