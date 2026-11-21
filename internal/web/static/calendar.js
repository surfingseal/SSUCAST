// Pure month/deadline presentation; no requests or database writes.
(function(root){
 'use strict';
 const pad=n=>String(n).padStart(2,'0');
 const key=d=>`${d.getUTCFullYear()}-${pad(d.getUTCMonth()+1)}-${pad(d.getUTCDate())}`;
 function seoulDate(now=Date.now()){return new Date(new Date(now).getTime()+9*3600000).toISOString().slice(0,10);}
 function validDate(value){
  if(typeof value!=='string'||!/^\d{4}-\d{2}-\d{2}$/.test(value))return false;
  const d=new Date(value+'T00:00:00Z');return !Number.isNaN(d.valueOf())&&key(d)===value;
 }
 function monthDays(year,month){
  const first=new Date(Date.UTC(year,month,1));
  const offset=(first.getUTCDay()+6)%7;
  const count=new Date(Date.UTC(year,month+1,0)).getUTCDate();
  const length=Math.ceil((offset+count)/7)*7;
  return Array.from({length},(_,i)=>{const d=new Date(Date.UTC(year,month,1-offset+i));return {date:key(d),day:d.getUTCDate(),inMonth:d.getUTCMonth()===month};});
 }
 function deadlineEvents(notices){
  const by=new Map();
  for(const n of notices){
   for(const s of n.schedules||[]){
    if(!['application','submission','completion'].includes(s.type)||s.review_required!==false||!['verified','partial'].includes(s.quality)||s.timezone!=='Asia/Seoul')continue;
    const date=s.dates?.end_date;if(!validDate(date))continue;
    const time=s.dates.end_time||'';
    if(time&&!/^([01]\d|2[0-3]):[0-5]\d$/.test(time))continue;
    const identity=[n.id,date,time,s.type].join('|');
    if(!by.has(identity))by.set(identity,{noticeId:n.id,title:n.title,date,time,type:s.type,category:n.category,label:s.label||''});
   }
  }
  return [...by.values()].sort((a,b)=>a.date.localeCompare(b.date)||(a.time||'24:00').localeCompare(b.time||'24:00')||a.title.localeCompare(b.title,'ko'));
 }
 function predictionEvents(rows){
  return rows.filter(p=>{
   const provenance=p.data_origin==='synthetic'&&p.date_origin==='simulation_slot'||p.data_origin==='historical_forecast'&&p.date_origin==='anniversary_estimate'&&p.evidence?.id&&/^https:\/\/scatch\.ssu\.ac\.kr\//.test(p.evidence.url||'')&&validDate(p.expected_from)&&validDate(p.expected_to)&&p.expected_from<=p.date&&p.expected_to>=p.date;
   return provenance&&p.event_type==='deadline'&&p.timezone==='Asia/Seoul'&&validDate(p.date)&&validDate(p.horizon_from)&&validDate(p.horizon_to)&&p.date>=p.horizon_from&&p.date<=p.horizon_to&&p.id&&p.title;
  }).map(p=>({noticeId:p.id,title:p.title,date:p.date,time:'',type:'prediction',category:p.category,synthetic:true,forecast:p.data_origin==='historical_forecast',prediction:p}));
 }

 const api={seoulDate,validDate,monthDays,deadlineEvents,predictionEvents};
 if(typeof module!=='undefined'&&module.exports)module.exports=api;else root.SSUCalendar=api;
})(typeof window!=='undefined'?window:globalThis);
