'use strict';

const assert = require('node:assert/strict');
const { evaluateDeadlineWeather, noticeDeadlineInfo, selectNoticeSchedule, toKSTDateParts } = require('./weather.js');

const NOW_KST = '2026-11-16T13:00:00+09:00';

function makeNotice(id, category, schedules, extra = {}) {
  return {
    id,
    title: `공고 ${id}`,
    category,
    categories: [category],
    source: 'ssu_catch',
    status: '진행',
    schedules,
    ...extra
  };
}

function makeSchedule(id, endDate, endTime, overrides = {}) {
  return {
    id,
    type: 'application',
    label: '신청 기간',
    timezone: 'Asia/Seoul',
    quality: 'verified',
    review_required: false,
    dates: {
      start_date: '2026-11-01',
      start_time: null,
      end_date: endDate,
      end_time: endTime
    },
    ...overrides
  };
}

// 1. 마감 예정 공고가 없는 경우 -> 맑음 + 전체 공고 기준(isPersonalized=false)
{
  const res = evaluateDeadlineWeather({
    notices: [],
    interests: [],
    now: NOW_KST
  });
  assert.equal(res.isPersonalized, false);
  assert.equal(res.today.level, 'clear');
  assert.equal(res.today.label, '맑음');
  assert.ok(res.today.reason.includes('맑음'));
  assert.equal(res.today.confirmedTotal, 0);
  assert.equal(res.nearestDeadline, null);
  assert.equal(res.forecast.length, 7);
  assert.equal(res.forecast[0].dataState, 'empty');
}

// 2. 3일 뒤 마감 공고가 1건인 경우 -> 오늘 전체 날씨는 구름, 7일 예보는 해당 날짜(11/19)에만 분포 표시
{
  const res = evaluateDeadlineWeather({
    notices: [
      makeNotice('n1', 'scholarship', [makeSchedule('s1', '2026-11-19', '18:00')])
    ],
    interests: ['scholarship'],
    now: NOW_KST
  });
  assert.equal(res.isPersonalized, true);
  assert.equal(res.today.level, 'cloudy');
  assert.equal(res.today.within3dCount, 1);
  assert.equal(res.today.urgent24hCount, 0);
  assert.ok(res.today.reason.includes('구름'));
  // 11/16 has 0 deadlines on that specific date -> clear ('empty')
  assert.equal(res.forecast[0].dayCount, 0);
  assert.equal(res.forecast[0].level, 'clear');
  // 11/19 is forecast[3] -> 1 deadline on that date -> cloudy ('has_deadlines')
  assert.equal(res.forecast[3].date, '2026-11-19');
  assert.equal(res.forecast[3].dayCount, 1);
  assert.equal(res.forecast[3].level, 'cloudy');
  assert.equal(res.forecast[3].dataState, 'has_deadlines');
}

// 3. 오늘 마감 공고가 1건인 경우 -> 비
{
  const notice = makeNotice('n1', 'academic', [makeSchedule('s1', '2026-11-16', '17:00')]);
  const res = evaluateDeadlineWeather({
    notices: [notice],
    interests: ['academic'],
    now: NOW_KST
  });
  assert.equal(res.today.level, 'rain');
  assert.equal(res.today.urgent24hCount, 1);
  assert.equal(res.nearestDeadline.remainingLabel, '4시간 남음');
  assert.equal(res.forecast[0].level, 'rain');
  const cardBadge = noticeDeadlineInfo(notice, NOW_KST);
  assert.equal(cardBadge.kind, 'confirmed');
  assert.equal(cardBadge.level, 'rain');
}

// 4. 오늘 여러 공고가 마감되는 경우 -> 폭풍
{
  const res = evaluateDeadlineWeather({
    notices: [
      makeNotice('n1', 'academic', [makeSchedule('s1', '2026-11-16', '17:00')]),
      makeNotice('n2', 'academic', [makeSchedule('s2', '2026-11-16', '23:00')])
    ],
    interests: ['academic'],
    now: NOW_KST
  });
  assert.equal(res.today.level, 'storm');
  assert.equal(res.today.urgent24hCount, 2);
  assert.ok(res.today.reason.includes('폭풍'));
  assert.equal(res.forecast[0].level, 'storm');
}

// 5. 관심 분야를 변경한 경우 -> 해당 분야만 반영
{
  const notices = [
    makeNotice('n1', 'scholarship', [makeSchedule('s1', '2026-11-16', '17:00')]),
    makeNotice('n2', 'volunteer', [makeSchedule('s2', '2026-11-16', '18:00')])
  ];
  const r1 = evaluateDeadlineWeather({ notices, interests: ['scholarship'], now: NOW_KST });
  assert.equal(r1.today.level, 'rain');
  assert.equal(r1.today.urgent24hCount, 1);

  const r2 = evaluateDeadlineWeather({ notices, interests: ['academic'], now: NOW_KST });
  assert.equal(r2.today.level, 'clear');
  assert.equal(r2.today.urgent24hCount, 0);
}

// 6. 마감 시간이 없는 경우 -> 임의로 23:59를 만들지 않음
{
  const res = evaluateDeadlineWeather({
    notices: [
      makeNotice('n1', 'scholarship', [makeSchedule('s1', '2026-11-16', null)])
    ],
    interests: ['scholarship'],
    now: NOW_KST
  });
  assert.equal(res.today.level, 'rain');
  assert.equal(res.nearestDeadline.hasTime, false);
  assert.ok(!res.nearestDeadline.displayLabel.includes('23:59'));
  assert.equal(res.nearestDeadline.remainingLabel, '오늘 마감 (시각 미기재)');
}

// 7. 불확실한 AI 추출 일정만 존재하는 경우 -> 확정 산정 제외 + 날짜별 unverified_only 구분
{
  const res = evaluateDeadlineWeather({
    notices: [
      makeNotice('n1', 'academic', [
        makeSchedule('s1', '2026-11-16', '17:00', {
          quality: 'uncertain',
          review_required: true
        })
      ]),
      makeNotice('n2', 'academic', [
        makeSchedule('s2', '2026-11-17', '18:00', {
          quality: 'blocked',
          review_required: false
        })
      ])
    ],
    interests: ['academic'],
    now: NOW_KST
  });
  assert.equal(res.today.level, 'clear');
  assert.equal(res.today.confirmedTotal, 0);
  assert.equal(res.uncertainCount, 2);
  assert.equal(res.forecast[0].dataState, 'unverified_only');
  assert.equal(res.forecast[1].dataState, 'unverified_only');
}

// 8. 같은 공고의 마감이 중복 저장된 경우 -> 1회만 계산
{
  const res = evaluateDeadlineWeather({
    notices: [
      makeNotice('n1', 'academic', [
        makeSchedule('s1', '2026-11-16', '17:00'),
        makeSchedule('s2', '2026-11-16', '17:00')
      ])
    ],
    interests: ['academic'],
    now: NOW_KST
  });
  assert.equal(res.today.urgent24hCount, 1);
  assert.equal(res.today.level, 'rain');
}

// 9. 시작일/행사일/발표일 및 완료 공고는 마감에서 제외
{
  const res = evaluateDeadlineWeather({
    notices: [
      makeNotice('n1', 'academic', [
        makeSchedule('s1', '2026-11-16', '17:00', { type: 'event' }),
        makeSchedule('s2', '2026-11-16', '18:00', { type: 'announcement' })
      ]),
      makeNotice('n2', 'academic', [makeSchedule('s3', '2026-11-16', '17:00')], {
        status: '완료'
      })
    ],
    interests: ['academic'],
    now: NOW_KST
  });
  assert.equal(res.today.level, 'clear');
  assert.equal(res.today.confirmedTotal, 0);
}

// 10. 서버/브라우저 시간대가 다르거나 자정 전후인 경우 (Asia/Seoul 고정 검증)
{
  const beforeMidnight = evaluateDeadlineWeather({
    notices: [
      makeNotice('n1', 'academic', [makeSchedule('s1', '2026-11-16', null)])
    ],
    interests: ['academic'],
    now: '2026-11-16T14:50:00Z'
  });
  assert.equal(beforeMidnight.todayDate, '2026-11-16');
  assert.equal(beforeMidnight.today.level, 'rain');

  const afterMidnight = evaluateDeadlineWeather({
    notices: [
      makeNotice('n1', 'academic', [makeSchedule('s1', '2026-11-16', null)])
    ],
    interests: ['academic'],
    now: '2026-11-16T15:05:00Z'
  });
  assert.equal(afterMidnight.todayDate, '2026-11-17');
  assert.equal(afterMidnight.today.level, 'clear');
  assert.equal(afterMidnight.today.confirmedTotal, 0);
  assert.equal(toKSTDateParts(Date.parse('2026-11-16T15:05:00Z')).hour, 0);
}

// 11. 순수 함수 결정성 검증
{
  const input = {
    notices: [
      makeNotice('n1', 'scholarship', [makeSchedule('s1', '2026-11-17', '15:00')])
    ],
    interests: ['scholarship'],
    now: NOW_KST
  };
  assert.deepEqual(evaluateDeadlineWeather(input), evaluateDeadlineWeather(input));
}

// 12. A future submission badge and the card preview select the same schedule.
{
 const notice=makeNotice('mixed','extracurricular',[
  makeSchedule('application','2026-11-02',null),
  makeSchedule('preliminary','2026-11-12','13:00',{type:'submission',label:'예선 제출'}),
  makeSchedule('final','2026-11-25','13:00',{type:'submission',label:'본선 제출'})
 ]);
 const badge=noticeDeadlineInfo(notice,NOW_KST);
 const selected=selectNoticeSchedule(notice,NOW_KST);
 assert.equal(badge.scheduleId,'final');
 assert.equal(selected.id,badge.scheduleId);
 assert.equal(selected.label,'본선 제출');
 assert.equal(selected.dates.end_date,'2026-11-25');
}
// 13. Older schedules without IDs still match the actual selected date/type.
{
 const notice=makeNotice('legacy','academic',[
  makeSchedule(undefined,'2026-11-02',null),
  makeSchedule(undefined,'2026-11-25',null,{type:'submission'})
 ]);
 assert.equal(selectNoticeSchedule(notice,NOW_KST).dates.end_date,'2026-11-25');
}
// 14. Past notices retain a preview, but never get a false future badge.
{
 const notice=makeNotice('past','academic',[makeSchedule('past','2026-11-02',null)]);
 assert.equal(noticeDeadlineInfo(notice,NOW_KST),null);
 assert.equal(selectNoticeSchedule(notice,NOW_KST).id,'past');
}
// 15. Missing schedules have no preview.
assert.equal(selectNoticeSchedule({id:'empty'},NOW_KST),null);
console.log('ALL 15 DEADLINE WEATHER UNIT TESTS PASSED');
