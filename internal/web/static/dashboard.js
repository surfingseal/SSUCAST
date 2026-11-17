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
 function todayForecast(report){return report.forecast.find(d=>d.date===report.todayDate)||null;}
 function todayDeadlines(report){return todayForecast(report)?.deadlines||[];}
 function paginationPages(page,totalPages){
  const pages=Math.max(1,Number(totalPages)||1);
  const current=Math.max(1,Math.min(pages,Number(page)||1));
  const start=Math.floor((current-1)/5)*5+1;
  const list=[];
  for(let p=start;p<=Math.min(pages,start+4);p++)list.push(p);
  return list;
 }
 function briefing(report){
  const today=report.forecast.find(d=>d.date===report.todayDate);
  const tomorrowDate=new Date(new Date(report.todayDate+'T00:00:00Z').getTime()+86400000).toISOString().slice(0,10);
  const tomorrow=report.forecast.find(d=>d.date===tomorrowDate);
  const candidates=report.forecast.flatMap(d=>d.deadlines||[]).slice().sort((a,b)=>a.boundaryMs-b.boundaryMs);
  const next=candidates[0]||null;
  return {todayCount:today?.dayCount||0,tomorrowCount:tomorrow?.dayCount||0,tomorrowDate,next,targetDate:next?.endDate||null};
 }
 function weekOverview(report){
  const endDate=new Date(new Date(report.todayDate+'T00:00:00Z').getTime()+6*86400000).toISOString().slice(0,10);
  const days=report.forecast.filter(d=>d.date>=report.todayDate&&d.date<=endDate).slice().sort((a,b)=>a.date.localeCompare(b.date));
  const unique=new Set(days.flatMap(d=>(d.deadlines||[]).map(n=>n.noticeId)).filter(Boolean));
  const peakCount=Math.max(0,...days.map(d=>d.dayCount||0));
  return {startDate:report.todayDate,endDate,total:unique.size,peakCount,peakDays:peakCount?days.filter(d=>d.dayCount===peakCount):[]};
 }
 const api={weekOverview,briefing,seoulClock,weatherAsset,todayForecast,todayDeadlines,paginationPages};
 if(typeof module!=='undefined'&&module.exports)module.exports=api;
 else root.SSUDashboard=api;
})(typeof window!=='undefined'?window:globalThis);
