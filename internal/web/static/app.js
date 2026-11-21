'use strict';
const $=id=>document.getElementById(id);
const state={page:1,limit:8,sort:'latest',source:'',categories:[],query:'',status:'',include_expired:false};
let homeSearchUI=null;
const INTEREST_STORAGE_KEY='ssu_cast_interests_v1';
let currentView=location.hash==='#simulation'?'simulation':location.hash==='#calendar'?'calendar':location.hash==='#explore'?'explore':'home';
let sources=[],categories=[],pageData=null,listController=null,detailController=null,weatherNotices=[],weatherAsOf=null,selectedForecastDate=null,userInterests=loadSavedInterests();
const reasons={image_evidence_not_independently_verified:'이미지 판독 내용 확인 필요',text_image_conflict:'본문과 이미지의 일정이 다름',image_model_review_required:'이미지 정보 검토 필요',low_image_confidence:'작거나 흐린 이미지 판독 확인 필요',missing_year:'연도 확인 필요',unsupported_datetime:'날짜·시각 근거 확인 필요',evidence_mismatch:'원문 근거 불일치',tentative_schedule:'변경될 수 있는 일정',conditional_deadline:'조건부 마감',source_conflict:'원문 내용 확인 필요',approximate_date:'대략적인 날짜',missing_date:'날짜 미상',missing_time:'시각 미기재',no_fixed_calendar:'확정 날짜 없음'};
function el(tag,cls,text){const n=document.createElement(tag);if(cls)n.className=cls;if(text!==undefined)n.textContent=text;return n;}
function sourceLabel(id){return sources.find(s=>s.name===id)?.label||id;}
function categoryLabel(id){return categories.find(c=>c.id===id)?.label||(id==='unknown'?'분류 확인 필요':id||'기타');}
function date(value){if(!value)return '게시일 미상';const d=new Date(value);return Number.isNaN(d.valueOf())?'게시일 미상':new Intl.DateTimeFormat('ko-KR',{timeZone:'Asia/Seoul',year:'numeric',month:'2-digit',day:'2-digit'}).format(d);}
async function api(path,options={}){const r=await fetch(path,{credentials:'same-origin',cache:'no-store',...options});let v;try{v=await r.json();}catch{throw new Error('서버 응답을 읽지 못했습니다. 다시 시도해 주세요.');}if(r.status===401){showLogin();throw new Error(v.error||'로그인이 필요합니다.');}if(!r.ok)throw new Error(v.error||'공지를 불러오지 못했습니다.');return v;}
function showLogin(){homeSearchUI?.close();if($('senior-detail').open)$('senior-detail').close();if($('detail').open)$('detail').close();if($('calendar-day-dialog').open)$('calendar-day-dialog').close();$('login').classList.remove('hidden');$('app').classList.add('hidden');$('logout').classList.add('hidden');}
function showApp(){$('login').classList.add('hidden');$('app').classList.remove('hidden');$('logout').classList.remove('hidden');}
function parameters(){const p=new URLSearchParams();if(state.query)p.set('search_in','title');for(const [k,v]of Object.entries(state)){if(k!=='categories'&&v!==''&&v!==false)p.set(k,String(v));}if(state.categories.length===1)p.set('category',state.categories[0]);else if(state.categories.length>1)p.set('categories',state.categories.join(','));return p;}
function remember(){const p=parameters();p.delete('limit');history.replaceState(null,'','/?'+p+(currentView==='simulation'?'#simulation':currentView==='calendar'?'#calendar':currentView==='explore'?'#explore':'#home'));}
function feedback(title,message,retry=false){const box=$('feedback');box.replaceChildren(el('strong','',title),el('p','',message));if(retry){const b=el('button','primary','다시 불러오기');b.type='button';b.onclick=typeof retry==='function'?retry:()=>load();box.append(b);}box.classList.remove('hidden');}
function filters(){
 const nav=$('categories');const categoryScroll=nav.scrollLeft;nav.replaceChildren();for(const c of [{id:'',label:'전체'},...categories]){
  const active=c.id?state.categories.includes(c.id):state.categories.length===0;
  const b=el('button','category-button'+(active?' active':''));b.type='button';b.dataset.category=c.id;b.setAttribute('aria-pressed',String(active));b.append(el('span','',c.label));
  b.onclick=()=>{state.categories=c.id?(active?state.categories.filter(id=>id!==c.id):[...state.categories,c.id]):[];state.page=1;filters();[...nav.querySelectorAll('button')].find(item=>item.dataset.category===c.id)?.focus({preventScroll:true});load();};nav.append(b);
 }
 nav.scrollLeft=categoryScroll;
 const tabs=$('sources');tabs.replaceChildren();for(const s of [{name:'',label:'전체',count:sources.reduce((a,s)=>a+s.count,0)},...sources]){const b=el('button','source-button'+(state.source===s.name?' active':''),(s.name==='ssu_global_media'?'글로벌미디어학부':s.label));b.type='button';b.setAttribute('aria-pressed',String(state.source===s.name));b.append(el('span','tab-count',String(s.count)));b.onclick=()=>{state.source=s.name;state.page=1;filters();load();};tabs.append(b);}
 $('query').value=state.query;if(document.activeElement!==$('home-query'))$('home-query').value=state.query;homeSearchUI?.sync();$('sort').value=state.sort;
}
function range(s){const a=s.start_label,b=s.end_label;if(a&&b&&a!==b)return a+' ~ '+b;if(b)return b;if(a)return a;return '날짜 미상';}
function isDeadlineType(type){return ['application','submission','completion'].includes(type);}
function important(schedules){return [...schedules].sort((a,b)=>{const rank=s=>(isDeadlineType(s.type)?0:2)+(s.review_required?1:0);return rank(a)-rank(b);})[0];}
function scheduleStateLabel(s){
 if(s.type==='submission')return {'신청 중':'제출 가능','신청 예정':'제출 예정','신청 마감':'제출 마감'}[s.state]||s.state;
 return s.state;
}
function schedulePrefix(s){return s.type==='submission'?'제출 마감 · ':s.type==='completion'?'완료 기한 · ':s.type==='application'?'신청 마감 · ':'행사·발표 · ';}
function imageAsset(path,cls=''){const img=el('img',cls);img.src=path;img.alt='';return img;}
function card(n){
 const a=el('article','card');a.dataset.noticeId=n.id;
 const meta=el('div','card-meta');meta.append(el('span','category-tag',categoryLabel(n.category)),el('span','card-source',sourceLabel(n.source)));
 const dInfo=window.SSUWeather?.noticeDeadlineInfo(n,weatherAsOf||Date.now());
 if(dInfo)meta.append(el('span','card-weather-pill '+dInfo.level,dInfo.remainingLabel));
 a.append(meta);
 const h=el('h2');const open=el('button','card-open',n.title);open.type='button';open.title=n.title;open.onclick=()=>detail(n.id);h.append(open);a.append(h);
 const summary=n.analyzed?n.summary||'요약이 없습니다. 원문을 확인해 주세요.':'아직 분석하지 않은 공고입니다. 원문을 확인해 주세요.';
 const description=el('p','card-summary',summary);description.title=summary;a.append(description);
 const bottom=el('div','card-footer');const preview=el('div','card-deadline');
 const selected=window.SSUWeather?.selectNoticeSchedule(n,weatherAsOf||Date.now())||important(n.schedules||[]);
 let deadline='확인된 일정 없음';
 if(selected){
  const label=selected.dates?.end_date||selected.dates?.start_date;
  const clock=selected.dates?.end_time||selected.dates?.start_time;
  const prefix=selected.type==='submission'?'제출 마감':selected.type==='completion'?'이수 마감':selected.type==='application'?'신청 마감':'행사 일정';
  deadline=prefix+'  '+(label?label.replaceAll('-','.')+(clock?' '+clock:' · 시각 미기재'):range(selected));
  if(selected.review_required)deadline+=' · 확인 필요';
 }
 preview.append(imageAsset('/figma/982c3.svg'),el('span','',deadline));preview.title=deadline;
 const more=el('button','card-details','상세보기');more.type='button';more.setAttribute('aria-label',n.title+' 상세보기');more.append(el('span','','›'));more.onclick=()=>detail(n.id);
 bottom.append(preview,more);a.append(bottom);return a;
}
function renderPagination(data){
 const box=$('page-numbers');box.replaceChildren();
 const list=window.SSUDashboard?.paginationPages(data.page,data.pages)||[1];
 for(const page of list){
  const b=el('button','page-number'+(data.page===page?' current':''),String(page));b.type='button';b.setAttribute('aria-label',page+'페이지');if(data.page===page)b.setAttribute('aria-current','page');
  b.onclick=()=>{state.page=page;load();$('explore-results').scrollTop=0;};box.append(b);
 }
}
async function load(){
 $('explore-results').scrollTop=0;
 if(listController)listController.abort();listController=new AbortController();const controller=listController;remember();$('cards').replaceChildren();$('cards').setAttribute('aria-busy','true');feedback('소식을 불러오고 있어요','잠시만 기다려 주세요.');$('prev').disabled=true;$('next').disabled=true;
 try{const data=await api('/api/notices?'+parameters(),{signal:controller.signal});if(controller!==listController)return;pageData=data;$('feedback').classList.add('hidden');$('cards').replaceChildren(...data.items.map(card));const count=$('result-count');count.replaceChildren(el('b','',String(data.total)),document.createTextNode('개의 공고'));
 const notes=[];if(state.query)notes.push(`“${state.query}” 검색 결과`);if(state.sort==='deadline')notes.push('확인된 신청·제출·이수 마감부터 표시합니다. 날짜가 불확실한 공지는 뒤에 표시해요.');if(state.sort==='event')notes.push('확인된 행사·면접·발표 일정부터 표시합니다.');$('filter-note').textContent=notes.join(' · ');$('filter-note').classList.toggle('hidden',notes.length===0);
 renderPagination(data);$('page-label').textContent=`${data.page} / ${Math.max(1,data.pages)}`;$('prev').disabled=data.page<=1;$('next').disabled=data.page>=data.pages;
 if(!data.items.length){feedback('찾으시는 공지가 없어요','다른 검색어를 입력하거나 필터를 초기화해 보세요.');const clear=el('button','primary','필터 초기화');clear.type='button';clear.onclick=resetFilters;$('feedback').append(clear);}
 }catch(e){if(e.name==='AbortError')return;$('result-count').textContent='조회 실패';feedback('공지를 불러오지 못했어요',e.message,true);}finally{if(controller===listController)$('cards').setAttribute('aria-busy','false');}
}
function section(title){const s=el('section','detail-section');s.append(el('h3','',title));return s;}
function schedule(s){
 const a=el('article','schedule-item'+(s.parent_id?' child':''));const heading=el('div','schedule-heading');heading.append(el('h4','',s.label),el('span','schedule-state'+(s.review_required?' review':''),scheduleStateLabel(s)));a.append(heading);
 const badges=el('div','schedule-badges');
 const kind=isDeadlineType(s.type);
 badges.append(el('span','schedule-kind-tag'+(kind?'':' event'),kind?'모집·신청 마감':'행사·발표 일정'));
 badges.append(el('span','image-origin',s.image_derived?(s.origin==='merged_text_image'?'본문·이미지 병합':'이미지에서 추출됨'):'본문 기반 일정'));
 a.append(badges);
 a.append(el('p','schedule-dates',range(s)));if(s.year_assumed)a.append(el('div','schedule-scope','연도는 사용자 기준연도 정책으로 보완했습니다.'));if(s.scope)a.append(el('div','schedule-scope','구분 · '+s.scope));if(s.review_required){const r=s.review_reasons.map(v=>reasons[v]||'추가 원문 검토 필요');a.append(el('p','review-note','확정 일정으로 사용하지 마세요.'+(r.length?' '+[...new Set(r)].join(' · '):'')));}
 if(s.image_derived){const imageProof=el('details');imageProof.append(el('summary','','이미지 판독 근거 보기'));for(const evidence of s.image_evidence||[]){imageProof.append(el('blockquote','',evidence.evidence_text));if(evidence.image_url){const link=el('a','','원본 이미지 보기');link.href=evidence.image_url;link.target='_blank';link.rel='noopener noreferrer';imageProof.append(link);}}imageProof.append(el('p','review-note','모델이 이미지에서 읽은 문구입니다. 판독 정확성은 원본 이미지에서 확인해 주세요.'));a.append(imageProof);}
 const quotes=(s.evidence_spans||[]).filter(v=>v.location?.matched&&v.location.original_text).map(v=>v.location.original_text);if(!quotes.length&&s.evidence&&!s.image_derived)quotes.push(s.evidence);if(quotes.length){const evidence=el('details');evidence.append(el('summary','','원문 근거 보기'));for(const q of [...new Set(quotes)])evidence.append(el('blockquote','',q));a.append(evidence);}return a;
}
function timingReview(r){
 const box=section(r.label);box.classList.add('timing-review');box.append(el('p','',r.message));
 for(const rule of r.rules||[])box.append(el('p','timing-rule',rule));
 if(r.review_required)box.append(el('p','review-note','고정 마감일이 확인되지 않았습니다. 신청 전에 원문을 확인해 주세요.'));
 if(r.evidence?.length){const proof=el('details');proof.append(el('summary','','점검 근거 보기'));for(const e of r.evidence){proof.append(el('p','timing-source',e.source==='image'?'저장 이미지':e.source==='existing_schedule'?'기존 일정':'저장 본문'),el('blockquote','',e.text));if(e.url){const a=el('a','','근거 이미지 보기');a.href=e.url;a.target='_blank';a.rel='noopener noreferrer';proof.append(a);}}box.append(proof);}
 return box;
}
async function detail(id){
 if(detailController)detailController.abort();detailController=new AbortController();const controller=detailController;const dlg=$('detail');$('detail-content').replaceChildren(el('h2','detail-title','공지를 불러오는 중'));$('detail-content').firstChild.id='detail-title';if(!dlg.open)dlg.showModal();dlg.scrollTop=0;
 try{const n=await api('/api/notices/'+encodeURIComponent(id),{signal:controller.signal});if(controller!==detailController)return;const box=$('detail-content');box.replaceChildren();const meta=el('div','detail-meta');meta.append(el('span','category-tag '+n.category,categoryLabel(n.category)),el('span','',sourceLabel(n.source)),el('span','',date(n.published_at)),el('span','',n.category_method==='ai'?'AI 분류':'학교 공식 분류'));const h=el('h2','detail-title',n.title);h.id='detail-title';box.append(meta,h);
 const summary=el('section','detail-summary');summary.append(el('h3','',n.analyzed?'AI 핵심 요약':'원문 공지'),el('p','',n.analyzed?n.summary||'제공된 요약이 없습니다.':'아직 AI 분석이 완료되지 않은 공지입니다. 아래 원문 내용을 확인해 주세요.'));if(n.summary_review_required)summary.append(el('p','review-note','요약의 대상·조건 등은 원문 확인이 필요합니다.'));box.append(summary);
 const audience=section('누가 참여할 수 있나요?');audience.append(el('p','audience',n.audience_review_required?'참여 대상과 지원 자격은 원문에서 확인해 주세요.':n.target_audience||'확인된 참여 대상 정보가 없습니다.'));box.append(audience);
 if(n.timing_review)box.append(timingReview(n.timing_review));
 const schedules=section(`일정 ${n.schedules.length}개`);if(n.schedules.length)for(const s of n.schedules)schedules.append(schedule(s));else schedules.append(el('p','audience',n.timing_review?n.timing_review.message:(n.analyzed?'저장된 본문에서 일정을 추출하지 못했습니다. 첨부파일이나 원문에 추가 정보가 있을 수 있어요.':'일정이 아직 분석되지 않았습니다.')));box.append(schedules);
 const original=el('details','original-body');original.append(el('summary','','저장된 원문 본문 보기'),el('pre','',n.content||'저장된 본문이 없습니다.'));box.append(original);
 if(n.original_url){const link=el('a','primary original-link','학교 원문 페이지 열기');link.href=n.original_url;link.target='_blank';link.rel='noopener noreferrer';box.append(link);}else box.append(el('p','review-note','안전한 원문 링크가 제공되지 않았습니다.'));
 box.append(el('p','detail-footnote','AI가 정리한 정보가 포함되어 있습니다. 일정과 지원 자격은 변경될 수 있으니 신청 전 원문을 확인해 주세요.'));
 }catch(e){if(e.name!=='AbortError'){$('detail-content').replaceChildren(el('h2','detail-title','상세보기를 불러오지 못했어요'),el('p','audience',e.message));$('detail-content').firstChild.id='detail-title';}}
}
async function boot(){
 try{showApp();$('logout').classList.add('hidden');const [meta,cats]=await Promise.all([api('/api/sources'),api('/api/categories')]);sources=meta.sources;categories=cats.items;$('visible-count').replaceChildren(document.createTextNode(String(meta.visible_count)),el('small','','건'));$('updated').textContent=meta.updated_at?date(meta.updated_at)+' 업데이트':'업데이트 시각 미상';renderInterestChips();filters();await Promise.all([load(),loadWeatherData()]);if(currentView==='calendar'){loadCalendar();loadCalendarPredictions();}}catch(e){briefingError();feedback('서비스를 불러오지 못했어요',e.message,()=>boot());$('weather-count-label').textContent='조회 실패';$('weather-count').textContent='—';$('weather-summary').textContent='서비스를 불러오지 못했어요. 잠시 후 다시 확인해 주세요.';$('deadline-weather').setAttribute('aria-busy','false');$('urgent-spotlight').replaceChildren(el('p','spotlight-empty','공고 정보를 불러오지 못했습니다.'));const retry=el('button','quiet','다시 불러오기');retry.type='button';retry.onclick=boot;$('urgent-spotlight').append(retry);}
}
function loadSavedInterests(){
 try{const raw=localStorage.getItem(INTEREST_STORAGE_KEY);if(!raw)return [];const parsed=JSON.parse(raw);return Array.isArray(parsed)?parsed.filter(v=>typeof v==='string'):[];}catch{return [];}
}
function saveInterests(list){
 userInterests=list;
 try{localStorage.setItem(INTEREST_STORAGE_KEY,JSON.stringify(list));}catch{}
}

function buildSmallWeatherSvg(level){return imageAsset(window.SSUDashboard.weatherAsset(level),'weather-asset '+level);}
function buildHeroWeatherSvg(level){return buildSmallWeatherSvg(level);}

function renderInterestChips(){
 const box=$('interest-chips');if(!box)return;
 box.replaceChildren();
 const allActive=userInterests.length===0;
 const allBtn=el('button','interest-chip'+(allActive?' active':''),'전체 공고');
 allBtn.type='button';allBtn.setAttribute('aria-pressed',String(allActive));
 allBtn.onclick=()=>{saveInterests([]);renderInterestChips();renderWeather();};
 box.append(allBtn);
 for(const c of categories){
  const active=userInterests.includes(c.id);
  const b=el('button','interest-chip'+(active?' active':''),c.label);
  b.type='button';b.setAttribute('aria-pressed',String(active));
  b.onclick=()=>{
   const next=active?userInterests.filter(id=>id!==c.id):[...userInterests,c.id];
   saveInterests(next.length===categories.length?[]:next);
   renderInterestChips();
   renderWeather();
  };
  box.append(b);
 }
}
function renderSelectedDayPanel(){
 const panel=$('weather-day-panel');if(!panel)return;
 panel.classList.add('hidden');panel.replaceChildren();
}
function renderSpotlight(report){
 const grid=$('urgent-spotlight');grid.replaceChildren();
 const activeDate=selectedForecastDate||report.todayDate;
 const day=report.forecast.find(f=>f.date===activeDate);
 const isToday=!day||day.isToday||day.date===report.todayDate;
 const deadlines=isToday?window.SSUDashboard.todayDeadlines(report):(day.deadlines||[]);
 const dayPrefix=isToday?'오늘':`${day.shortDate} (${day.weekday})`;
 $('spotlight-title').textContent=isToday?'오늘 마감되는 공고':`${dayPrefix} 마감되는 공고`;
 $('today-count').textContent=deadlines.length+'건';
 grid.dataset.count=String(deadlines.length);grid.classList.toggle('few-notices',deadlines.length<=2);
 if(!deadlines.length){grid.append(el('p','spotlight-empty',isToday?'오늘 마감되는 공고가 없습니다.':`${dayPrefix}에 마감되는 공고가 없습니다.`));return;}
 for(const d of deadlines){
  const btn=el('button','spotlight-item');btn.type='button';btn.onclick=()=>detail(d.noticeId);
  const time=d.endTime||'시각 미기재';
  const title=el('span','spotlight-item-title');title.append(el('span','spotlight-title-text',d.noticeTitle));
  btn.append(el('span','category-tag',categoryLabel(d.category)),title,el('span','spotlight-item-date',`${dayPrefix} 마감 · `+time));grid.append(btn);
 }
}
function briefingError(){
 const box=$('briefing-content');box.replaceChildren(el('p','muted','마감 정보를 불러오지 못했어요.'));const retry=el('button','briefing-action','다시 불러오기 →');retry.type='button';retry.onclick=loadWeatherData;box.append(retry);
}
function renderBriefing(report){
 const b=window.SSUDashboard.weekOverview(report),short=d=>d.slice(5).replace('-','.');$('briefing-scope').textContent=short(b.startDate)+' — '+short(b.endDate);
 const box=$('briefing-content');box.replaceChildren();const stats=el('div','briefing-week-stats');
 const total=el('div','briefing-week-stat');total.append(el('span','briefing-week-label','7일 내 마감 공고'));const count=el('p','briefing-week-total');count.append(el('strong','',String(b.total)),el('span','','건'));total.append(count);stats.append(total);
 const peak=el('button','briefing-week-stat briefing-week-peak');peak.type='button';peak.append(el('span','briefing-week-label','마감이 가장 몰린 날'));
 if(b.peakDays.length){const day=b.peakDays[0];peak.append(el('strong','briefing-peak-date',short(day.date)+' ('+day.weekday+')'),el('span','briefing-peak-count',b.peakCount+'건'+(b.peakDays.length>1?' · 외 '+(b.peakDays.length-1)+'일':'')));peak.setAttribute('aria-label','마감이 가장 몰린 날 '+day.date+' '+b.peakCount+'건, 공고 보기');peak.onclick=()=>{selectedForecastDate=day.date===report.todayDate?null:day.date;renderWeather();$('spotlight-section').scrollIntoView({block:'nearest',behavior:'smooth'});};}
 else{peak.append(el('strong','briefing-peak-date','없어요'));peak.disabled=true;}
 stats.append(peak);box.append(stats);const footer=el('div','briefing-week-footer');footer.append(el('span','muted',report.isPersonalized?'선택 분야 기준':'전체 공고 기준'));box.append(footer);
}
function renderWeather(){
 if(!window.SSUWeather)return;
 const board=$('deadline-weather');if(!board)return;
 const report=window.SSUWeather.evaluateDeadlineWeather({notices:weatherNotices,interests:userInterests,now:weatherAsOf||Date.now()});
 const today=window.SSUDashboard.todayForecast(report);
 if(!today)throw new Error('오늘 마감 예보가 없습니다.');
 board.dataset.tone=today.tone;
 $('weather-scope-badge').textContent=report.isPersonalized?`선택 분야 ${userInterests.length}개`:'전체 공고';
 updateClock();

 const iconWrap=$('weather-icon');
 iconWrap.className="weather-hero-illust";iconWrap.setAttribute('aria-label','오늘 마감 긴급도: '+today.label);
 iconWrap.replaceChildren(buildHeroWeatherSvg(today.level));

 $('weather-headline').textContent=today.label;
 const todayCount=today.dayCount;
 $('weather-count-label').textContent='오늘 마감';$('weather-count').textContent=todayCount+'건';
 $('weather-summary').textContent=todayCount?`오늘 마감되는 모집 공고가 ${todayCount}건 있어요.`:'오늘 마감되는 모집 공고가 없어요.';
 renderSpotlight(report);
 renderBriefing(report);

 const activeHighlightDate=selectedForecastDate||report.todayDate;
 const strip=$('weather-forecast');strip.replaceChildren();
 for(const day of report.forecast){
  const isHighlighted=activeHighlightDate===day.date;
  const b=el('button','forecast-day'+(day.isToday?' is-today':'')+(isHighlighted?' selected':''));
  b.type='button';b.setAttribute('aria-pressed',String(isHighlighted));
  const countText=day.dayCount+'건';
  const countCls=day.dayCount>0?'':day.dataState==='unverified_only'?' unverified':' empty';
  b.setAttribute('aria-label',`${day.isToday?'오늘':day.shortDate+' '+day.weekday+'요일'} ${countText}, 날씨 ${day.label}`);
  const dateBox=el('span','forecast-day-date');dateBox.append(el('span','forecast-date',day.isToday?'오늘':day.weekday),el('span','forecast-shortdate',day.shortDate));b.append(dateBox);
  const iconBox=el('span','forecast-icon');iconBox.append(buildSmallWeatherSvg(day.level));
  b.append(iconBox,el('span','forecast-count'+countCls,countText));
  b.onclick=()=>{selectedForecastDate=(day.date===report.todayDate||selectedForecastDate===day.date)?null:day.date;renderWeather();};
  strip.append(b);
 }

 renderSelectedDayPanel(report);
}
async function loadWeatherData(){
 $('deadline-weather').setAttribute('aria-busy','true');
 try{
  const first=await api('/api/notices?sort=deadline&limit=50&page=1');
  const items=[...(first.items||[])];
  weatherAsOf=first.as_of||null;
  const totalPages=Math.min(first.pages||1,5);
  if(totalPages>1){
   const rest=await Promise.all(Array.from({length:totalPages-1},(_,i)=>api(`/api/notices?sort=deadline&limit=50&page=${i+2}`)));
   for(const p of rest)items.push(...(p.items||[]));
  }
  weatherNotices=items;
  renderWeather();
  if(pageData&&pageData.items)$('cards').replaceChildren(...pageData.items.map(card));
 }catch{
  briefingError();
  $('weather-count-label').textContent='조회 실패';$('weather-count').textContent='—';
 $('weather-headline').textContent='확인 실패';
  $('weather-summary').textContent='마감 정보를 불러오지 못했어요. 잠시 후 다시 확인해 주세요.';
 $('today-count').textContent='—';$('urgent-spotlight').replaceChildren(el('p','spotlight-empty','마감 정보 조회 실패'));const retry=el('button','quiet','다시 불러오기');retry.type='button';retry.onclick=loadWeatherData;$('urgent-spotlight').append(retry);
 }finally{$('deadline-weather').setAttribute('aria-busy','false');}
}
$('open-policy-btn').onclick=()=>$('policy-dialog').showModal();$('close-policy').onclick=()=>$('policy-dialog').close();
$('login-form').addEventListener('submit',async e=>{e.preventDefault();const button=e.currentTarget.querySelector('button');button.disabled=true;$('login-error').textContent='';try{await api('/api/session',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({token:$('token').value})});$('token').value='';await boot();}catch(e){$('login-error').textContent=e.message;}finally{button.disabled=false;}});
$('logout').onclick=async()=>{try{await api('/api/session',{method:'DELETE'});showLogin();}catch(e){feedback('로그아웃에 실패했어요',e.message);}};
$('search-form').addEventListener('submit',e=>{e.preventDefault();state.query=$('query').value.trim();$('home-query').value=state.query;state.page=1;load();});
for(const [id,key]of [['sort','sort']])$(id).addEventListener('change',e=>{state[key]=e.target.type==='checkbox'?e.target.checked:e.target.value;state.page=1;load();});
function resetFilters(){Object.assign(state,{page:1,sort:'latest',source:'',categories:[],query:'',status:'',include_expired:false});filters();load();}
$('reset').onclick=resetFilters;
$('prev').onclick=()=>{if(state.page>1){state.page--;load();$('explore-results').scrollTop=0;}};$('next').onclick=()=>{if(pageData&&state.page<pageData.pages){state.page++;load();$('explore-results').scrollTop=0;}};
$('close-detail').onclick=()=>$('detail').close();$('detail').addEventListener('close',()=>detailController?.abort());$('detail').addEventListener('click',e=>{if(e.target===$('detail')){const r=e.target.getBoundingClientRect();if(e.clientX<r.left||e.clientX>r.right||e.clientY<r.top||e.clientY>r.bottom)e.target.close();}});
function switchView(){
 const hash=location.hash;
 currentView=hash==='#simulation'?'simulation':hash==='#calendar'?'calendar':hash==='#explore'?'explore':'home';document.body.dataset.view=currentView;if(currentView!=='home')homeSearchUI?.close();
 $('home-view').classList.toggle('hidden',currentView!=='home');$('explore-section').classList.toggle('hidden',currentView!=='explore');$('calendar-section').classList.toggle('hidden',currentView!=='calendar');$('simulation-view').classList.toggle('hidden',currentView!=='simulation');$('home-search').classList.toggle('hidden',currentView!=='home');
 for(const link of document.querySelectorAll('.rail-links a')){const active=link.hash==='#'+currentView;link.classList.toggle('active',active);if(active)link.setAttribute('aria-current','page');else link.removeAttribute('aria-current');}
 if(hash==='#forecast')$('forecast-section').scrollIntoView({block:'start'});else window.scrollTo({top:0,left:0,behavior:'instant'});
 document.title=({home:'홈',explore:'공고 탐색',calendar:'캘린더',simulation:'시뮬레이션'}[currentView])+' · 숭실대학교 공고 모아보기';
 if(currentView==='calendar'){renderCalendar();loadCalendar();loadCalendarPredictions();}
}
window.addEventListener('hashchange',switchView);
homeSearchUI=window.SSUHomeSearch.attach({form:$('home-search'),input:$('home-query'),api,onNotice:id=>detail(id),onSearch:query=>{Object.assign(state,{query,page:1,source:'',categories:[],status:'',include_expired:false,sort:'latest'});filters();location.hash='explore';switchView();load();$('query').focus();}});
let clockDate='';
function updateClock(){const c=window.SSUDashboard.seoulClock(Date.now());$('greeting-message').textContent=c.greeting;$('weather-weekday').textContent=c.weekday;$('weather-date').textContent=c.date;const changed=clockDate&&clockDate!==c.date;clockDate=c.date;return changed;}
updateClock();
setInterval(()=>{updateClock();if(weatherNotices.length){weatherAsOf=new Date().toISOString();renderWeather();}},60000);
document.addEventListener('visibilitychange',()=>{if(!document.hidden){updateClock();weatherAsOf=new Date().toISOString();if(weatherNotices.length)renderWeather();}});
const calendarToday=()=>window.SSUCalendar.seoulDate();
let calendarMonth=calendarToday().slice(0,7),calendarEvents=[],calendarLoaded=false,calendarLoading=false,calendarFailed=false;
const deadlineTypeLabel={application:'신청',submission:'제출',completion:'이수',prediction:'AI 예상'};
const calendarPredictionCache=new Map(),calendarPredictionLoading=new Set(),calendarPredictionFailed=new Set();
let calendarMode='deadline',seniorPersona=window.SSUSeniors.personas[0].id,seniorCategory='all',seniorSelected='';
function calendarDisplayEvents(){if(calendarMode==='senior')return window.SSUSeniors.events(seniorPersona,seniorCategory);return [...calendarEvents,...($('calendar-predictions-toggle').checked?(calendarPredictionCache.get(calendarMonth)||[]):[])].sort((a,b)=>a.date.localeCompare(b.date)||Number(!!a.synthetic)-Number(!!b.synthetic));}
function openCalendarEvent(event){if(event.senior){seniorDetail(event);return;}if(event.synthetic)predictionDetail(event.prediction);else detail(event.noticeId);}
function predictionDetail(p){
 const forecast=p.data_origin==='historical_forecast';
 const box=$('detail-content');box.replaceChildren();const h=el('h2','detail-title',p.title);h.id='detail-title';
 box.append(h,el('span','prediction-badge',forecast?'AI 예상 공고 · 작년 공고 기반':'AI 예상 공고 · 합성 데이터'),el('p','prediction-disclaimer',forecast?'작년 공고를 바탕으로 예상한 지원 기회입니다. 올해 실제 모집 여부와 마감일은 학교 공고에서 확인해 주세요.':'실제 게시 공고나 확정 마감일이 아닙니다.'));
 const summary=section('예상 공고 내용');summary.append(el('p','',p.summary));box.append(summary);
 const date=section(forecast?'예상 마감 기간':'캘린더 표시일');
 date.append(el('p','',forecast?p.expected_from.replace(/-/g,'.')+' ~ '+p.expected_to.replace(/-/g,'.'):p.date.replace(/-/g,'.')+' · 합성 데이터 배정일'),el('p','muted',forecast?'달력은 작년 마감일의 1년 뒤를 기준일로 표시합니다. ±7일은 표시를 위한 추정 범위이며 통계적으로 검증한 구간이 아닙니다.':'실제 학교 일정에서 확인한 날짜가 아닙니다.'));box.append(date);
 if(forecast&&p.date<window.SSUCalendar.seoulDate())box.append(el('p','review-note','이미 지난 날짜의 추정입니다. 사전에 검증한 예측 결과가 아닙니다.'));
 const reason=section(forecast?'예측 근거':'생성 설명');reason.append(el('p','',p.reason));box.append(reason);
 if(forecast){
  const prep=section('미리 준비할 것');prep.append(el('p','',p.preparation));box.append(prep);
  const proof=section('작년 원문');proof.append(el('strong','',p.evidence.title),el('p','muted','게시일 '+p.evidence.published_date+' · 당시 마감일 '+p.evidence.deadline),el('blockquote','',p.evidence.quote));
  const link=el('a','primary original-link','작년 학교 공고 보기');link.href=p.evidence.url;link.target='_blank';link.rel='noopener noreferrer';proof.append(link);box.append(proof);
 }
 box.append(el('p','detail-footnote','AI 예상 공고는 실제 공고 수와 홈 마감 날씨에 포함되지 않습니다.'));if(!$('detail').open)$('detail').showModal();$('detail').scrollTop=0;
}

function showCalendarDay(date,events){
 $('calendar-day-title').textContent=date.replace(/-/g,'.')+(calendarMode==='senior'?' 준비 기록':' 마감 공고');
 const list=$('calendar-day-events');list.replaceChildren();
 if(!events.length)list.append(el('p','muted',calendarMode==='senior'?'이 날짜의 준비 기록이 없습니다.':'이 날짜에 확인된 마감 공고가 없습니다.'));
 for(const event of events){const button=el('button','calendar-day-event');button.type='button';button.append(el('span','category-tag',event.senior?window.SSUSeniors.labels[event.category]:categoryLabel(event.category)),el('strong','',event.title),el('span','muted',event.senior?event.label:event.synthetic?'AI 예상 · 실제 공고 아님':deadlineTypeLabel[event.type]+' 마감 · '+(event.time||'시각 미기재')));button.onclick=()=>{$('calendar-day-dialog').close();openCalendarEvent(event);};list.append(button);}
 $('calendar-day-dialog').showModal();
}
function renderCalendar(){
 const [year,month]=calendarMonth.split('-').map(Number);const label=year+'년 '+month+'월';$('calendar-month').textContent=label;$('calendar-caption').textContent=label+(calendarMode==='senior'?' 선배의 준비 기록':' 신청·제출·이수 마감 공고');
 const senior=calendarMode==='senior';$('calendar-section').dataset.mode=calendarMode;$('calendar-deadline-legend').classList.toggle('hidden',senior);$('senior-controls').classList.toggle('hidden',!senior);$('senior-calendar-note').classList.toggle('hidden',!senior);$('calendar-status').classList.toggle('calendar-status-hidden',senior);$('calendar-status').setAttribute('aria-hidden',String(senior));$('calendar-deadline-mode').setAttribute('aria-pressed',String(!senior));$('calendar-senior-mode').setAttribute('aria-pressed',String(senior));
 const displayEvents=calendarDisplayEvents();const monthEvents=calendarEvents.filter(e=>e.date.startsWith(calendarMonth));const predictedCount=displayEvents.filter(e=>e.synthetic&&e.date.startsWith(calendarMonth)).length;
 $('calendar-status').textContent=calendarLoading?'마감 일정을 불러오고 있어요.':calendarFailed?'마감 일정을 불러오지 못했어요. 다시 시도해 주세요.':calendarLoaded?`실제 마감 ${monthEvents.length}건 · AI 예상 ${predictedCount}건`+(calendarPredictionLoading.has(calendarMonth)?' · AI 예상 공고 조회 중':calendarPredictionFailed.has(calendarMonth)?' · AI 예상 공고 조회 실패':''):'마감 일정을 불러오고 있어요.';
 $('calendar-retry').classList.toggle('hidden',!calendarFailed);$('calendar-prediction-retry').classList.toggle('hidden',!calendarPredictionFailed.has(calendarMonth));
 $('calendar-prev').disabled=year<=1970&&month===1;$('calendar-next').disabled=year>=2100&&month===12;
 const days=$('calendar-days');days.replaceChildren();const byDate=new Map();for(const e of displayEvents){if(!byDate.has(e.date))byDate.set(e.date,[]);byDate.get(e.date).push(e);}
 let row;window.SSUCalendar.monthDays(year,month-1).forEach((day,index)=>{
  if(index%7===0){row=el('tr');days.append(row);}
  const events=byDate.get(day.date)||[];const cell=el('td','calendar-cell'+(!day.inMonth?' outside-month':'')+(day.date===calendarToday()?' today':'')+(day.date===seniorSelected&&senior?' senior-selected':''));
  const dateButton=el('button','calendar-date',String(day.day));dateButton.type='button';dateButton.dataset.date=day.date;dateButton.setAttribute('aria-label',senior?day.date+` 준비 기록 ${events.length}건`:day.date+` 실제 마감 ${events.filter(e=>!e.synthetic).length}건, AI 예상 ${events.filter(e=>e.synthetic).length}건`);if(day.date===calendarToday())dateButton.setAttribute('aria-current','date');dateButton.onclick=()=>showCalendarDay(day.date,events);cell.append(dateButton);
  const tags=el('div','calendar-tags');for(const event of events.slice(0,2)){const tag=el('button','calendar-event '+event.type,event.senior?window.SSUSeniors.labels[event.category]+' · '+event.label:deadlineTypeLabel[event.type]+' · '+event.title);tag.type='button';tag.title=event.senior?event.title+' · '+event.label:event.title+(event.time?' · '+event.time:(event.synthetic?' · AI 예상':' · 시각 미기재'));tag.onclick=()=>openCalendarEvent(event);tags.append(tag);}
  if(events.length>2){const more=el('button','calendar-more','+'+(events.length-2)+'건 더 보기');more.type='button';more.onclick=()=>showCalendarDay(day.date,events);tags.append(more);}cell.append(tags);row.append(cell);
 });
}
async function loadCalendar(){
 if(calendarLoaded||calendarLoading)return;calendarLoading=true;calendarFailed=false;renderCalendar();
 try{
  const byId=new Map();for(const status of ['', '완료']){
   let pages=1;for(let page=1;page<=pages;page++){
    const q=new URLSearchParams({page:String(page),limit:'50',sort:'latest',include_expired:'true'});if(status)q.set('status',status);
    const response=await api('/api/notices?'+q);pages=response.pages||1;if(pages>10000)throw new Error('일정 조회 범위를 초과했습니다.');for(const n of response.items||[])byId.set(n.id,n);
   }
  }
  calendarEvents=window.SSUCalendar.deadlineEvents([...byId.values()]);calendarLoaded=true;
 }catch{calendarFailed=true;}finally{calendarLoading=false;renderCalendar();}
}
async function loadCalendarPredictions(){
 if(calendarMode==='senior')return;const monthKey=calendarMonth;if(calendarPredictionCache.has(monthKey)||calendarPredictionLoading.has(monthKey))return;calendarPredictionLoading.add(monthKey);calendarPredictionFailed.delete(monthKey);renderCalendar();
 try{const [year,month]=monthKey.split('-').map(Number);const days=window.SSUCalendar.monthDays(year,month-1);const response=await api('/api/notice-predictions?'+new URLSearchParams({from:days[0].date,to:days[days.length-1].date}));calendarPredictionCache.set(monthKey,window.SSUCalendar.predictionEvents(response.items||[]));}catch{calendarPredictionFailed.add(monthKey);}finally{calendarPredictionLoading.delete(monthKey);if(calendarMonth===monthKey)renderCalendar();}
}

function seniorDetail(event){
 seniorSelected=event.date;renderCalendar();const a=event.activity,p=event.persona;
 $('senior-detail-title').textContent=event.date.slice(5).replace('-','월 ')+'일의 준비 기록';
 const box=$('senior-detail-content');box.replaceChildren(el('h3','senior-activity-title',a.title),el('p','muted',p.name+' 선배 · '+p.major+' '+p.grade));
 const timeline=el('ol','senior-timeline');a.timeline.forEach((step,i)=>{const row=el('li',i===event.stepIndex?'current':'');row.append(el('time','',step.date.slice(5).replace('-','.')),el('span','',step.label));if(i===event.stepIndex)row.setAttribute('aria-current','step');timeline.append(row);});
 box.append(timeline);const preparation=section('미리 준비한 것');const list=el('ul','senior-prepared');for(const item of a.prepared)list.append(el('li','',item));preparation.append(list);box.append(preparation);const advice=section('올해 준비해볼 일');advice.classList.add('senior-advice');advice.append(el('p','',a.advice));box.append(advice,el('p','senior-source-date','작년 기록 · '+event.sourceDate.replace(/-/g,'.')));
 if(!$('senior-detail').open)$('senior-detail').showModal();$('senior-detail').scrollTop=0;
}
function seniorFilters(){
 const p=window.SSUSeniors.personas.find(p=>p.id===seniorPersona);$('senior-initial').textContent=p.name[0];
 const group=$('senior-categories');group.replaceChildren();for(const [id,label] of Object.entries({all:'전체',...window.SSUSeniors.labels})){const b=el('button','',label);b.type='button';b.setAttribute('aria-pressed',String(id===seniorCategory));b.onclick=()=>{seniorCategory=id;seniorSelected='';seniorFilters();renderCalendar();};group.append(b);}
}
for(const p of window.SSUSeniors.personas){const option=el('option','',p.name+' 선배 · '+p.major);option.value=p.id;$('senior-persona').append(option);}
$('senior-persona').onchange=()=>{seniorPersona=$('senior-persona').value;seniorSelected='';seniorFilters();renderCalendar();};
function setCalendarMode(mode){calendarMode=mode;seniorSelected='';renderCalendar();if(mode==='deadline'){loadCalendar();loadCalendarPredictions();}}
$('calendar-deadline-mode').onclick=()=>setCalendarMode('deadline');$('calendar-senior-mode').onclick=()=>setCalendarMode('senior');
$('close-senior-detail').onclick=()=>$('senior-detail').close();
$('senior-detail').addEventListener('close',()=>{const selected=seniorSelected;seniorSelected='';renderCalendar();document.querySelector('.calendar-date[data-date="'+selected+'"]')?.focus();});
$('senior-detail').addEventListener('click',e=>{if(e.target===$('senior-detail')){const r=e.target.getBoundingClientRect();if(e.clientX<r.left||e.clientX>r.right||e.clientY<r.top||e.clientY>r.bottom)e.target.close();}});
$('calendar-senior-mode').parentElement.addEventListener('keydown',e=>{if(!['ArrowLeft','ArrowRight','Home','End'].includes(e.key))return;e.preventDefault();const mode=e.key==='Home'?'deadline':e.key==='End'?'senior':calendarMode==='deadline'?'senior':'deadline';setCalendarMode(mode);$(mode==='senior'?'calendar-senior-mode':'calendar-deadline-mode').focus();});
seniorFilters();

$('calendar-predictions-toggle').onchange=renderCalendar;
$('calendar-prediction-retry').onclick=()=>{calendarPredictionCache.delete(calendarMonth);loadCalendarPredictions();};
function moveCalendarMonth(offset){const [year,month]=calendarMonth.split('-').map(Number);const d=new Date(Date.UTC(year,month-1+offset,1));calendarMonth=d.toISOString().slice(0,7);renderCalendar();loadCalendarPredictions();}
$('calendar-prev').onclick=()=>moveCalendarMonth(-1);$('calendar-next').onclick=()=>moveCalendarMonth(1);
$('calendar-today').onclick=()=>{calendarMonth=calendarToday().slice(0,7);renderCalendar();loadCalendarPredictions();};$('calendar-retry').onclick=loadCalendar;
$('calendar-day-close').onclick=()=>$('calendar-day-dialog').close();
// Keep the home list scrollbar quiet until its content is actually scrolling.
let spotlightScrollTimer;
$('urgent-spotlight').addEventListener('scroll',()=>{const list=$('urgent-spotlight');clearTimeout(spotlightScrollTimer);if(list.scrollHeight<=list.clientHeight){list.classList.remove('is-scrolling');return;}list.classList.add('is-scrolling');spotlightScrollTimer=setTimeout(()=>list.classList.remove('is-scrolling'),700);},{passive:true});

switchView();
const params=new URLSearchParams(location.search);for(const k of ['query','source','status','sort'])if(params.has(k))state[k]=params.get(k);state.categories=params.get('categories')?[...new Set(params.get('categories').split(','))]:params.get('category')?[params.get('category')]:[];state.include_expired=params.get('include_expired')==='true';const p=Number(params.get('page'));if(Number.isInteger(p)&&p>=1&&p<=10000)state.page=p;
boot();
