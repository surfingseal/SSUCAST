// Shared presentation rules; clock and data remain injectable for verification.
(function(root){
 'use strict';
 function seoulClock(now){
  const parts=new Intl.DateTimeFormat('ko-KR',{timeZone:'Asia/Seoul',year:'numeric',month:'numeric',day:'numeric',weekday:'long',hour:'numeric',hourCycle:'h23'}).formatToParts(new Date(now));
  const part=t=>parts.find(p=>p.type===t)?.value;
  const hour=Number(part('hour'));
  return {weekday:part('weekday'),date:`${part('year')}년 ${part('month')}월 ${part('day')}일`,greeting:hour>=5&&hour<11?'좋은 아침입니다.':hour>=11&&hour<17?'맛있는 점심 되세요':'편안한 저녁 되세요'};
 }
 function weatherAsset(level){return '/figma/'+({clear:'b0372.png',cloudy:'001c7.png',rain:'1022f.png',storm:'8e098.png'}[level]||'001c7.png');}
 function todayDeadlines(report){return report.forecast.find(d=>d.date===report.todayDate)?.deadlines||[];}
 function paginationPages(page,totalPages){
  const pages=Math.max(1,Number(totalPages)||1);
  const current=Math.max(1,Math.min(pages,Number(page)||1));
  const start=Math.floor((current-1)/5)*5+1;
  const list=[];
  for(let p=start;p<=Math.min(pages,start+4);p++)list.push(p);
  return list;
 }
 const api={seoulClock,weatherAsset,todayDeadlines,paginationPages};
 if(typeof module!=='undefined'&&module.exports)module.exports=api;
 else root.SSUDashboard=api;
})(typeof window!=='undefined'?window:globalThis);
