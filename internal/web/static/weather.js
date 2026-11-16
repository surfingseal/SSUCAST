(function (root, factory) {
  if (typeof module === 'object' && module.exports) {
    module.exports = factory();
  } else {
    root.SSUWeather = factory();
  }
}(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';

  const KST_OFFSET_MS = 9 * 60 * 60 * 1000;
  const HOUR_MS = 60 * 60 * 1000;
  const DAY_MS = 24 * HOUR_MS;
  const WEEKDAYS_KO = ['일', '월', '화', '수', '목', '금', '토'];
  const DEADLINE_TYPES = new Set(['application', 'submission', 'completion']);
  const EVENT_TYPES = new Set(['event', 'interview', 'announcement', 'selection']);
  const TRUSTED_QUALITIES = new Set(['verified', 'partial']);

  const WEATHER_META = {
    clear: {
      level: 'clear',
      label: '맑음',
      summary: '다가오는 마감이 없어 여유로운 상태예요.',
      tone: 'calm'
    },
    cloudy: {
      level: 'cloudy',
      label: '구름',
      summary: '가까운 모집 마감이 예정되어 있어요. 미리 일정을 살펴보세요.',
      tone: 'watch'
    },
    rain: {
      level: 'rain',
      label: '비',
      summary: '마감이 임박한 공고가 있어요. 제출 서류와 기한을 확인하세요.',
      tone: 'alert'
    },
    storm: {
      level: 'storm',
      label: '폭풍',
      summary: '마감이 여러 건 집중되어 있어요. 우선순위를 정해 바로 확인하세요.',
      tone: 'critical'
    }
  };

  function pad2(n) {
    return String(n).padStart(2, '0');
  }

  function toNowMs(now) {
    if (typeof now === 'number' && Number.isFinite(now)) return now;
    if (now instanceof Date && !Number.isNaN(now.valueOf())) return now.valueOf();
    if (typeof now === 'string') {
      const parsed = Date.parse(now);
      if (!Number.isNaN(parsed)) return parsed;
    }
    return Date.now();
  }

  function toKSTDateParts(ms) {
    const kst = new Date(ms + KST_OFFSET_MS);
    const year = kst.getUTCFullYear();
    const month = kst.getUTCMonth() + 1;
    const day = kst.getUTCDate();
    const hour = kst.getUTCHours();
    const minute = kst.getUTCMinutes();
    const second = kst.getUTCSeconds();
    const weekdayIndex = kst.getUTCDay();
    return {
      year,
      month,
      day,
      hour,
      minute,
      second,
      dateStr: `${year}-${pad2(month)}-${pad2(day)}`,
      shortDate: `${month}.${pad2(day)}`,
      weekdayIndex,
      weekdayLabel: WEEKDAYS_KO[weekdayIndex]
    };
  }

  function parseDateYMD(dateStr) {
    if (typeof dateStr !== 'string') return null;
    const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(dateStr.trim());
    if (!m) return null;
    const year = Number(m[1]);
    const month = Number(m[2]);
    const day = Number(m[3]);
    const probe = new Date(Date.UTC(year, month - 1, day));
    if (
      probe.getUTCFullYear() !== year ||
      probe.getUTCMonth() + 1 !== month ||
      probe.getUTCDate() !== day
    ) {
      return null;
    }
    return { year, month, day };
  }

  function parseTimeHM(timeStr) {
    if (typeof timeStr !== 'string' || !timeStr.trim()) return null;
    const m = /^(\d{2}):(\d{2})$/.exec(timeStr.trim());
    if (!m) return null;
    const hour = Number(m[1]);
    const minute = Number(m[2]);
    if (hour < 0 || hour > 23 || minute < 0 || minute > 59) return null;
    return { hour, minute };
  }

  function parseKSTBoundaryMs(dateStr, timeStr, endBoundary) {
    const ymd = parseDateYMD(dateStr);
    if (!ymd) return null;
    if (timeStr !== null && timeStr !== undefined && String(timeStr).trim() !== '') {
      const hm = parseTimeHM(String(timeStr));
      if (!hm) return null;
      return Date.UTC(ymd.year, ymd.month - 1, ymd.day, hm.hour, hm.minute, 0, 0) - KST_OFFSET_MS;
    }
    const dayOffset = endBoundary ? 1 : 0;
    return Date.UTC(ymd.year, ymd.month - 1, ymd.day + dayOffset, 0, 0, 0, 0) - KST_OFFSET_MS;
  }

  function calendarDaysBetween(fromDateStr, toDateStr) {
    const a = parseDateYMD(fromDateStr);
    const b = parseDateYMD(toDateStr);
    if (!a || !b) return null;
    const aMs = Date.UTC(a.year, a.month - 1, a.day);
    const bMs = Date.UTC(b.year, b.month - 1, b.day);
    return Math.round((bMs - aMs) / DAY_MS);
  }

  function addKSTDays(dateStr, deltaDays) {
    const ymd = parseDateYMD(dateStr);
    if (!ymd) return null;
    const ms = Date.UTC(ymd.year, ymd.month - 1, ymd.day + deltaDays) - KST_OFFSET_MS;
    return toKSTDateParts(ms);
  }

  function matchesInterests(notice, interests) {
    if (!Array.isArray(interests) || interests.length === 0) return true;
    const set = new Set(interests);
    if (notice.category && set.has(notice.category)) return true;
    if (Array.isArray(notice.categories)) {
      return notice.categories.some(c => set.has(c));
    }
    return false;
  }

  function formatDeadlineDisplay(endDate, endTime) {
    if (!endDate) return '날짜 미상';
    const ymd = parseDateYMD(endDate);
    const base = ymd ? `${ymd.month}/${pad2(ymd.day)}` : endDate;
    if (endTime) return `${base} ${endTime}`;
    return `${base} (시각 미기재)`;
  }

  function formatRemaining(deadline, nowMs) {
    const todayKST = toKSTDateParts(nowMs).dateStr;
    if (!deadline.hasTime) {
      const days = calendarDaysBetween(todayKST, deadline.endDate);
      if (days === null) return '일정 확인 필요';
      if (days <= 0) return '오늘 마감 (시각 미기재)';
      if (days === 1) return '내일 마감 (시각 미기재)';
      return `D-${days} (시각 미기재)`;
    }
    const diffMs = deadline.boundaryMs - nowMs;
    if (diffMs <= 0) return '마감됨';
    if (diffMs < HOUR_MS) {
      return `${Math.max(1, Math.ceil(diffMs / 60000))}분 남음`;
    }
    if (diffMs < DAY_MS) {
      const hours = Math.floor(diffMs / HOUR_MS);
      const mins = Math.floor((diffMs % HOUR_MS) / 60000);
      return mins > 0 ? `${hours}시간 ${mins}분 남음` : `${hours}시간 남음`;
    }
    const days = Math.floor(diffMs / DAY_MS);
    const hours = Math.floor((diffMs % DAY_MS) / HOUR_MS);
    return hours > 0 ? `${days}일 ${hours}시간 남음` : `${days}일 남음`;
  }

  function classifyNoticeDeadlines(notices, interests, nowMs) {
    const confirmedDeadlines = [];
    const uncertainNotices = [];
    const uncertainDates = new Set();

    for (const notice of notices || []) {
      if (!notice || notice.status === '완료') continue;
      if (!matchesInterests(notice, interests)) continue;

      const dedup = new Map();
      let hasUncertainDeadline = false;
      const uncertainReasons = [];

      for (const s of notice.schedules || []) {
        if (!s || !DEADLINE_TYPES.has(s.type)) continue;

        const dates = s.dates || {};
        const endDate = dates.end_date || null;
        const endTime = dates.end_time || null;
        const hasValidDate = parseDateYMD(endDate) !== null;
        const hasValidTime = endTime === null || endTime === '' || parseTimeHM(endTime) !== null;
        const boundaryMs = hasValidDate && hasValidTime ? parseKSTBoundaryMs(endDate, endTime, true) : null;

        const isTrusted =
          s.review_required === false &&
          TRUSTED_QUALITIES.has(s.quality) &&
          s.timezone === 'Asia/Seoul' &&
          boundaryMs !== null;

        if (isTrusted) {
          if (boundaryMs <= nowMs) {
            continue;
          }
          const key = `${notice.id}::${endDate}::${endTime || 'DATE_ONLY'}`;
          if (!dedup.has(key)) {
            dedup.set(key, {
              noticeId: notice.id,
              noticeTitle: notice.title,
              category: notice.category,
              categories: notice.categories || (notice.category ? [notice.category] : []),
              source: notice.source,
              scheduleId: s.id,
              scheduleLabel: s.label || '마감 일정',
              scheduleType: s.type,
              endDate,
              endTime: endTime || null,
              hasTime: Boolean(endTime),
              boundaryMs,
              displayLabel: formatDeadlineDisplay(endDate, endTime)
            });
          }
        } else {
          if (boundaryMs !== null && boundaryMs <= nowMs) {
            continue;
          }
          hasUncertainDeadline = true;
          if (hasValidDate) uncertainDates.add(endDate);
          if (s.label) uncertainReasons.push(s.label);
        }
      }

      for (const item of dedup.values()) {
        confirmedDeadlines.push(item);
      }

      if (
        (hasUncertainDeadline || (notice.timing_review && notice.timing_review.review_required)) &&
        dedup.size === 0
      ) {
        uncertainNotices.push({
          id: notice.id,
          title: notice.title,
          category: notice.category,
          source: notice.source,
          reasonLabel: notice.timing_review
            ? notice.timing_review.label
            : uncertainReasons[0] || '날짜·시각 원문 확인 필요'
        });
      } else if (hasUncertainDeadline && dedup.size > 0) {
        uncertainNotices.push({
          id: notice.id,
          title: notice.title,
          category: notice.category,
          source: notice.source,
          reasonLabel: uncertainReasons[0] || '일부 마감 일정 원문 확인 필요'
        });
      }
    }

    confirmedDeadlines.sort((a, b) => {
      if (a.boundaryMs !== b.boundaryMs) return a.boundaryMs - b.boundaryMs;
      return String(a.noticeId).localeCompare(String(b.noticeId));
    });

    return { confirmedDeadlines, uncertainNotices, uncertainDates };
  }

  function buildTodayReason(level, urgent24hCount, within3dCount, uncertainCount) {
    if (level === 'storm') {
      if (urgent24hCount >= 2) {
        return `24시간 내 확정 마감 ${urgent24hCount}건이 몰려 있어 폭풍 단계로 산정되었습니다.`;
      }
      return `3일 이내 확정 마감 ${within3dCount}건이 집중되어 폭풍 단계로 산정되었습니다.`;
    }
    if (level === 'rain') {
      if (urgent24hCount === 1) {
        return `24시간 내 확정 마감 1건(3일 내 총 ${within3dCount}건)이 있어 비 단계로 산정되었습니다.`;
      }
      return `3일 이내 확정 마감 ${within3dCount}건이 예정되어 비 단계로 산정되었습니다.`;
    }
    if (level === 'cloudy') {
      return `24시간 내 마감은 없으나 3일 이내 확정 마감 1건이 있어 구름 단계로 산정되었습니다.`;
    }
    if (uncertainCount > 0) {
      return `3일 이내 확정 마감은 0건(맑음)이며, 원문 확인이 필요한 일정 ${uncertainCount}건은 별도 분리되었습니다.`;
    }
    return '향후 3일 이내 확정된 모집 마감이 없어 맑음 단계로 산정되었습니다.';
  }

  function groupByNotice(deadlines, nowMs) {
    const map = new Map();
    for (const d of deadlines) {
      if (!map.has(d.noticeId)) {
        map.set(d.noticeId, {
          ...d,
          remainingLabel: formatRemaining(d, nowMs),
          extraScheduleCount: 0,
          allSchedules: [d]
        });
      } else {
        const entry = map.get(d.noticeId);
        entry.extraScheduleCount++;
        entry.allSchedules.push(d);
      }
    }
    return [...map.values()];
  }

  function evaluateWeatherWindow(deadlines, refNowMs, uncertainCount = 0) {
    const refParts = toKSTDateParts(refNowMs);
    const refDateKST = refParts.dateStr;
    const active = deadlines.filter(d => d.boundaryMs > refNowMs);

    const urgentNoticeIds = new Set();
    const within3dNoticeIds = new Set();
    const sameDayRaw = [];

    for (const d of active) {
      const dayDiff = calendarDaysBetween(refDateKST, d.endDate);
      if (d.endDate === refDateKST) {
        sameDayRaw.push(d);
      }
      if (d.hasTime) {
        const diff = d.boundaryMs - refNowMs;
        if (diff <= 24 * HOUR_MS) urgentNoticeIds.add(d.noticeId);
      } else if (dayDiff === 0) {
        urgentNoticeIds.add(d.noticeId);
      }
      if (dayDiff !== null && dayDiff >= 0 && dayDiff <= 3) {
        within3dNoticeIds.add(d.noticeId);
      }
    }

    const urgent24hCount = urgentNoticeIds.size;
    const within3dCount = within3dNoticeIds.size;
    const sameDayDeadlines = groupByNotice(sameDayRaw, refNowMs);
    const sameDayCount = sameDayDeadlines.length;

    let level = 'clear';
    if (urgent24hCount >= 2 || within3dCount >= 4) {
      level = 'storm';
    } else if (urgent24hCount === 1 || within3dCount >= 2) {
      level = 'rain';
    } else if (within3dCount === 1) {
      level = 'cloudy';
    }

    const meta = WEATHER_META[level];
    let dynamicSummary = meta.summary;
    if (level === 'storm') {
      dynamicSummary =
        urgent24hCount >= 2
          ? `24시간 내 마감하는 모집 공고 ${urgent24hCount}건이 집중되어 있어요.`
          : `3일 이내 마감하는 모집 공고 ${within3dCount}건이 몰려 있어요.`;
    } else if (level === 'rain') {
      dynamicSummary =
        urgent24hCount === 1
          ? `24시간 내 마감하는 모집 공고 1건(3일 내 총 ${within3dCount}건)이 있어요.`
          : `3일 이내 마감하는 모집 공고 ${within3dCount}건이 예정되어 있어요.`;
    } else if (level === 'cloudy') {
      dynamicSummary = `3일 이내 마감하는 모집 공고 ${within3dCount}건이 예정되어 있어요.`;
    }
    return {
      level,
      label: meta.label,
      summary: dynamicSummary,
      reason: buildTodayReason(level, urgent24hCount, within3dCount, uncertainCount),
      tone: meta.tone,
      urgent24hCount,
      within3dCount,
      sameDayCount,
      sameDayDeadlines
    };
  }

  /**
   * Evaluates a single calendar day's deadline distribution for the 7-day forecast strip.
   * Unlike Today's overall urgency window (24h + 3d), each forecast day represents
   * the deadlines closing on that specific KST date, while keeping the 4 weather tiers consistent.
   */
  function evaluateDayForecast(dayDateStr, isToday, confirmedDeadlines, uncertainDates, nowMs) {
    const dayRaw = confirmedDeadlines.filter(
      d => d.endDate === dayDateStr && d.boundaryMs > nowMs
    );
    const grouped = groupByNotice(dayRaw, nowMs);
    const dayCount = grouped.length;
    const hasUnverifiedOnDate = uncertainDates && uncertainDates.has(dayDateStr);

    let level = 'clear';
    if (dayCount >= 3 || (isToday && dayCount >= 2)) {
      level = 'storm';
    } else if (dayCount === 2 || (isToday && dayCount === 1)) {
      level = 'rain';
    } else if (dayCount === 1) {
      level = 'cloudy';
    }

    const meta = WEATHER_META[level];
    const dataState =
      dayCount > 0 ? 'has_deadlines' : hasUnverifiedOnDate ? 'unverified_only' : 'empty';

    return {
      level,
      label: meta.label,
      tone: meta.tone,
      dayCount,
      dataState,
      deadlines: grouped
    };
  }

  function evaluateDeadlineWeather(input) {
    const nowMs = toNowMs(input && input.now);
    const notices = (input && input.notices) || [];
    const interests = (input && input.interests) || [];
    const todayParts = toKSTDateParts(nowMs);

    const { confirmedDeadlines, uncertainNotices, uncertainDates } = classifyNoticeDeadlines(
      notices,
      interests,
      nowMs
    );

    const todayWeather = evaluateWeatherWindow(
      confirmedDeadlines,
      nowMs,
      uncertainNotices.length
    );
    const forecast = [];

    for (let offset = 0; offset < 7; offset++) {
      const dayParts = addKSTDays(todayParts.dateStr, offset);
      const dayEval = evaluateDayForecast(
        dayParts.dateStr,
        offset === 0,
        confirmedDeadlines,
        uncertainDates,
        nowMs
      );
      forecast.push({
        date: dayParts.dateStr,
        shortDate: dayParts.shortDate,
        weekday: dayParts.weekdayLabel,
        isToday: offset === 0,
        level: dayEval.level,
        label: dayEval.label,
        tone: dayEval.tone,
        dayCount: dayEval.dayCount,
        dataState: dayEval.dataState,
        deadlines: dayEval.deadlines
      });
    }

    const groupedAll = groupByNotice(confirmedDeadlines, nowMs);
    const nearest = groupedAll.length > 0 ? groupedAll[0] : null;
    return {
      timezone: 'Asia/Seoul',
      asOfMs: nowMs,
      isPersonalized: Array.isArray(interests) && interests.length > 0,
      todayDate: todayParts.dateStr,
      todayShortDate: `${todayParts.month}월 ${todayParts.day}일 (${todayParts.weekdayLabel})`,
      today: {
        ...todayWeather,
        confirmedTotal: groupedAll.length
      },
      nearestDeadline: nearest,
      urgentDeadlines: groupedAll.slice(0, 4),
      forecast,
      uncertainNotices,
      uncertainCount: uncertainNotices.length
    };
  }

  /**
   * Computes a concise deadline urgency descriptor for a single notice card.
   */
  function noticeDeadlineInfo(notice, now) {
    const nowMs = toNowMs(now);
    if (!notice || notice.status === '완료') return null;
    const { confirmedDeadlines, uncertainNotices } = classifyNoticeDeadlines([notice], [], nowMs);
    if (confirmedDeadlines.length > 0) {
      const first = confirmedDeadlines[0];
      const todayKST = toKSTDateParts(nowMs).dateStr;
      const dayDiff = calendarDaysBetween(todayKST, first.endDate);
      let level = 'clear';
      if ((first.hasTime && first.boundaryMs - nowMs <= 24 * HOUR_MS) || dayDiff === 0) {
        level = 'rain';
      } else if (dayDiff !== null && dayDiff <= 3) {
        level = 'cloudy';
      }
      return {
        kind: 'confirmed',
        scheduleId: first.scheduleId || null,
        scheduleType: first.scheduleType,
        endDate: first.endDate,
        endTime: first.endTime,
        level,
        label: first.scheduleLabel,
        displayDate: first.displayLabel,
        remainingLabel: formatRemaining(first, nowMs)
      };
    }
    if (uncertainNotices.length > 0) {
      return {
        kind: 'uncertain',
        level: 'watch',
        label: uncertainNotices[0].reasonLabel,
        displayDate: '원문 확인 필요',
        remainingLabel: '확정 마감 제외'
      };
    }
    return null;
  }

  // Cards and urgency badges must describe the same schedule.
  function selectNoticeSchedule(notice, now) {
    const schedules = notice?.schedules || [];
    const info = noticeDeadlineInfo(notice, now);
    if (info?.kind === 'confirmed') {
      const selected = schedules.find(s =>
        (info.scheduleId && s.id === info.scheduleId) ||
        (!info.scheduleId && s.type === info.scheduleType &&
          s.dates?.end_date === info.endDate &&
          (s.dates?.end_time || null) === info.endTime));
      if (selected) return selected;
    }
    const rank = s => (DEADLINE_TYPES.has(s.type) ? 0 : 2) + (s.review_required ? 1 : 0);
    return [...schedules].sort((a, b) => rank(a) - rank(b))[0] || null;
  }

  return {
    WEATHER_META,
    DEADLINE_TYPES,
    EVENT_TYPES,
    toKSTDateParts,
    parseKSTBoundaryMs,
    calendarDaysBetween,
    formatRemaining,
    evaluateWeatherWindow,
    evaluateDayForecast,
    evaluateDeadlineWeather,
    noticeDeadlineInfo,
    selectNoticeSchedule
  };
}));
