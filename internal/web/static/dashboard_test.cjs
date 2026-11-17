'use strict';
const assert=require('node:assert/strict');
const ui=require('./dashboard.js');
for(const [utc,expected] of [
 ['2026-11-15T19:59:00Z','편안한 저녁 되세요'],
 ['2026-11-15T20:00:00Z','좋은 아침입니다.'],
 ['2026-11-16T01:59:00Z','좋은 아침입니다.'],
 ['2026-11-16T02:00:00Z','맛있는 점심 되세요'],
 ['2026-11-16T07:59:00Z','맛있는 점심 되세요'],
 ['2026-11-16T08:00:00Z','편안한 저녁 되세요']
])assert.equal(ui.seoulClock(utc).greeting,expected);
assert.deepEqual(ui.seoulClock('2026-11-15T15:01:00Z'),{weekday:'월요일',date:'2026년 11월 16일',greeting:'편안한 저녁 되세요'});
const report={todayDate:'2026-11-16',forecast:[{date:'2026-11-16',deadlines:[{noticeId:'today'}]},{date:'2026-11-17',deadlines:[{noticeId:'tomorrow'}]}]};
assert.deepEqual(ui.todayDeadlines(report),[{noticeId:'today'}]);
assert.deepEqual(ui.todayDeadlines({todayDate:'2026-11-18',forecast:report.forecast}),[]);
for(const level of ['clear','cloudy','rain','storm'])assert.match(ui.weatherAsset(level),/^\/figma\/\w+\.png$/);
assert.deepEqual(ui.paginationPages(1,1),[1]);
assert.deepEqual(ui.paginationPages(1,3),[1,2,3]);
assert.deepEqual(ui.paginationPages(1,9),[1,2,3,4,5]);
assert.deepEqual(ui.paginationPages(5,9),[1,2,3,4,5]);
assert.deepEqual(ui.paginationPages(6,9),[6,7,8,9]);
assert.deepEqual(ui.paginationPages(9,9),[6,7,8,9]);
assert.deepEqual(ui.paginationPages(6,12),[6,7,8,9,10]);
console.log('DASHBOARD CLOCK, DATE, TODAY FILTER AND ASSET TESTS PASSED');

// The hero uses today's daily forecast, not the aggregate three-day urgency.
const differingReport={todayDate:'2026-11-17',today:{level:'storm'},forecast:[
 {date:'2026-11-18',level:'storm',dayCount:3,deadlines:[{noticeId:'future'}]},
 {date:'2026-11-17',level:'clear',dayCount:0,deadlines:[]}
]};
assert.equal(ui.todayForecast(differingReport).level,'clear');
assert.equal(ui.todayForecast(differingReport).dayCount,0);
assert.deepEqual(ui.todayDeadlines(differingReport),[]);
assert.equal(ui.todayForecast({todayDate:'2026-11-19',forecast:differingReport.forecast}),null);

const briefingReport={todayDate:'2026-11-17',forecast:[
 {date:'2026-11-17',dayCount:0,deadlines:[]},
 {date:'2026-11-18',dayCount:3,deadlines:[{noticeId:'late',endDate:'2026-11-18',endTime:null,boundaryMs:300},{noticeId:'early',noticeTitle:'먼저 마감',endDate:'2026-11-18',endTime:'09:00',boundaryMs:100},{noticeId:'middle',endDate:'2026-11-18',endTime:'17:00',boundaryMs:200}]}
]};
const briefing=ui.briefing(briefingReport);
assert.equal(briefing.todayCount,0);assert.equal(briefing.tomorrowCount,3);
assert.equal(briefing.next.noticeId,'early');assert.equal(briefing.targetDate,'2026-11-18');
assert.equal(ui.briefing({todayDate:'2026-11-30',forecast:[]}).tomorrowDate,'2026-12-01');
assert.equal(ui.briefing({todayDate:'2026-11-30',forecast:[]}).next,null);
assert.equal(ui.briefing({todayDate:'2026-11-30',forecast:[]}).targetDate,null);
assert.equal(briefingReport.forecast[1].deadlines[0].noticeId,'late'); // Sorting must not mutate the shared report.

const weekly=ui.weekOverview({todayDate:'2026-11-17',forecast:[
 {date:'2026-11-16',dayCount:9,deadlines:[{noticeId:'past'}]},
 {date:'2026-11-17',dayCount:1,deadlines:[{noticeId:'a'}]},
 {date:'2026-11-18',dayCount:2,deadlines:[{noticeId:'a'},{noticeId:'b'}]},
 {date:'2026-11-20',dayCount:2,deadlines:[{noticeId:'c'},{noticeId:'d'}]},
 {date:'2026-11-24',dayCount:8,deadlines:[{noticeId:'outside'}]}
]});
assert.equal(weekly.total,4);assert.equal(weekly.endDate,'2026-11-23');
assert.equal(weekly.peakCount,2);assert.equal(weekly.peakDays[0].date,'2026-11-18');assert.equal(weekly.peakDays.length,2);
assert.equal(ui.weekOverview({todayDate:'2026-12-29',forecast:[]}).endDate,'2027-01-04');
assert.deepEqual(ui.weekOverview({todayDate:'2026-11-17',forecast:[]}).peakDays,[]);
