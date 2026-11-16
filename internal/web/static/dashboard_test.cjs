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
