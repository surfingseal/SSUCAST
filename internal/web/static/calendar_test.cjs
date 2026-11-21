'use strict';
const assert=require('node:assert/strict');
const ui=require('./calendar.js');
assert.equal(ui.seoulDate('2026-11-16T15:01:00Z'),'2026-11-17');
assert.equal(ui.validDate('2026-02-30'),false);
assert.equal(ui.validDate('2024-02-29'),true);
assert.equal(ui.validDate('2026-02-29'),false);
const november=ui.monthDays(2026,10);
assert.equal(november.length,42);
assert.equal(november[0].date,'2026-10-26');
assert.equal(november[6].date,'2026-11-01');
assert.equal(november.at(-1).date,'2026-12-06');
assert.equal(november.filter(d=>d.inMonth).length,30);
assert.equal(ui.monthDays(2027,1).length,28); // Four full Monday-start weeks.
const schedule=(type,date,time=null,extra={})=>({type,timezone:'Asia/Seoul',review_required:false,quality:'partial',dates:{end_date:date,end_time:time},...extra});
const notices=[{id:'a',title:'신청 공고',category:'scholarship',schedules:[schedule('application','2026-11-18'),schedule('application','2026-11-18'),schedule('submission','2026-11-18','18:00'),schedule('event','2026-11-19'),schedule('application','2026-11-19',null,{review_required:true}),schedule('application','2026-11-20',null,{quality:'blocked'}),schedule('application','2026-02-30'),schedule('application','2026-11-21','24:00')]},{id:'b',title:'다른 공고',category:'other',schedules:[schedule('completion','2026-10-01')]}];
const events=ui.deadlineEvents(notices);
assert.equal(events.length,3); // Same notice/date/type is deduplicated; distinct roles retained.
assert.equal(events[0].date,'2026-10-01'); // Historical dates must remain available.
assert.equal(events.filter(e=>e.date==='2026-11-18').length,2);
assert.equal(events.find(e=>e.type==='application').time,''); // Never invent a clock.
console.log('CALENDAR MONTH, KST, GROUNDING AND DEDUPLICATION TESTS PASSED');
const generated={id:'synthetic-1',title:'겨울방학 가상 공고',date:'2026-12-01',data_origin:'synthetic',date_origin:'simulation_slot',event_type:'deadline',timezone:'Asia/Seoul',horizon_from:'2026-11-17',horizon_to:'2027-02-17'};
assert.equal(ui.predictionEvents([generated])[0].synthetic,true);
assert.equal(ui.predictionEvents([{...generated,data_origin:'real'}]).length,0);
assert.equal(ui.predictionEvents([{...generated,date:'2027-02-18'}]).length,0);
assert.equal(ui.deadlineEvents([{id:'real',title:'실제 공고',schedules:[]}]).length,0);
const forecast={...generated,data_origin:'historical_forecast',date_origin:'anniversary_estimate',expected_from:'2026-11-24',expected_to:'2026-12-08',evidence:{id:'history-1',url:'https://scatch.ssu.ac.kr/공지사항/?slug=old'}};
assert.equal(ui.predictionEvents([forecast])[0].forecast,true);
assert.equal(ui.predictionEvents([{...forecast,evidence:{id:'x',url:'https://evil.invalid/'}}]).length,0);
assert.equal(ui.predictionEvents([{...forecast,expected_to:'2026-11-30'}]).length,0);
assert.equal(ui.predictionEvents([{...forecast,evidence:null}]).length,0);
