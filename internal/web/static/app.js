'use strict';
const $=id=>document.getElementById(id);
const state={page:1,limit:12,sort:'latest',source:'',category:'',query:'',status:'',has_schedule:false,upcoming:false};
let sources=[],categories=[],pageData=null,listController=null,detailController=null;
const icons={all:'▦',academic:'▤',scholarship:'◇',international:'◎',international_student:'↗',extracurricular:'✦',teaching:'▧',volunteer:'♡',other:'···'};
const reasons={missing_year:'연도 확인 필요',unsupported_datetime:'날짜·시각 근거 확인 필요',evidence_mismatch:'원문 근거 불일치',tentative_schedule:'변경될 수 있는 일정',conditional_deadline:'조건부 마감',source_conflict:'원문 내용 확인 필요',approximate_date:'대략적인 날짜',missing_date:'날짜 미상',missing_time:'시각 미기재',no_fixed_calendar:'확정 날짜 없음'};
function el(tag,cls,text){const n=document.createElement(tag);if(cls)n.className=cls;if(text!==undefined)n.textContent=text;return n;}
function sourceLabel(id){return sources.find(s=>s.name===id)?.label||id;}
function categoryLabel(id){return categories.find(c=>c.id===id)?.label||(id==='unknown'?'분류 확인 필요':id||'기타');}
function date(value){if(!value)return '게시일 미상';const d=new Date(value);return Number.isNaN(d.valueOf())?'게시일 미상':new Intl.DateTimeFormat('ko-KR',{timeZone:'Asia/Seoul',year:'numeric',month:'2-digit',day:'2-digit'}).format(d);}
async function api(path,options={}){const r=await fetch(path,{credentials:'same-origin',cache:'no-store',...options});let v;try{v=await r.json();}catch{throw new Error('서버 응답을 읽지 못했습니다. 다시 시도해 주세요.');}if(r.status===401){showLogin();throw new Error(v.error||'로그인이 필요합니다.');}if(!r.ok)throw new Error(v.error||'공지를 불러오지 못했습니다.');return v;}
function showLogin(){if($('detail').open)$('detail').close();$('login').classList.remove('hidden');$('app').classList.add('hidden');$('logout').classList.add('hidden');}
function showApp(){$('login').classList.add('hidden');$('app').classList.remove('hidden');$('logout').classList.remove('hidden');}
function parameters(){const p=new URLSearchParams();for(const [k,v]of Object.entries(state)){if(v!==''&&v!==false)p.set(k,String(v));}return p;}
function remember(){history.replaceState(null,'','/?'+parameters());}
function feedback(title,message,retry=false){const box=$('feedback');box.replaceChildren(el('strong','',title),el('p','',message));if(retry){const b=el('button','primary','다시 불러오기');b.type='button';b.onclick=typeof retry==='function'?retry:()=>load();box.append(b);}box.classList.remove('hidden');}
function filters(){
 const nav=$('categories');nav.replaceChildren();for(const c of [{id:'',label:'전체 공지'},...categories]){const b=el('button','category-button'+(state.category===c.id?' active':''));b.type='button';b.setAttribute('aria-pressed',String(state.category===c.id));b.append(el('span','category-icon',icons[c.id||'all']||'·'),el('span','',c.label));b.onclick=()=>{state.category=c.id;state.page=1;filters();load();};nav.append(b);}
 const tabs=$('sources');tabs.replaceChildren();for(const s of [{name:'',label:'전체',count:sources.reduce((a,s)=>a+s.count,0)},...sources]){const b=el('button','source-button'+(state.source===s.name?' active':''),s.label);b.type='button';b.setAttribute('aria-pressed',String(state.source===s.name));b.append(el('span','tab-count',String(s.count)));b.onclick=()=>{state.source=s.name;state.page=1;filters();load();};tabs.append(b);}
 $('query').value=state.query;$('sort').value=state.sort;$('status').value=state.status;$('has-schedule').checked=state.has_schedule;$('upcoming').checked=state.upcoming;
}
function range(s){const a=s.start_label,b=s.end_label;if(a&&b&&a!==b)return a+' ~ '+b;if(b)return b;if(a)return a;return '날짜 미상';}
function important(schedules){return [...schedules].sort((a,b)=>{const rank=s=>(['application','submission','completion'].includes(s.type)?0:2)+(s.review_required?1:0);return rank(a)-rank(b);})[0];}
function card(n){
 const a=el('article','card');a.dataset.noticeId=n.id;
 const meta=el('div','card-meta');meta.append(el('span','category-tag '+n.category,categoryLabel(n.category)),el('span','card-source',sourceLabel(n.source)));a.append(meta);
 const h=el('h2');const open=el('button','card-open',n.title);open.type='button';open.onclick=()=>detail(n.id);h.append(open);a.append(h);
 a.append(el('p','card-summary'+(!n.analyzed?' no-analysis':''),n.analyzed?n.summary||'요약이 없습니다. 원문을 확인해 주세요.':'아직 분석하지 않은 공지입니다. 상세보기에서 원문을 확인할 수 있어요.'));
 const preview=el('div','schedule-preview');const s=important(n.schedules);
 if(s){preview.append(el('strong','',s.label),el('span','',range(s)),el('span','state'+(s.review_required?' needs-review':''),s.state));if(n.schedules.length>1){const more=el('button','more-schedules',`일정 ${n.schedules.length-1}개 더 보기 ↗`);more.type='button';more.onclick=()=>detail(n.id);preview.append(more);}}
 else{preview.append(el('span','',n.analyzed?'추출된 일정 없음 · 필요 시 원문 확인':'일정 분석 전'));}
 a.append(preview);const bottom=el('div','card-bottom');bottom.append(el('span','',date(n.published_at)));if(n.review_required||n.summary_review_required||n.audience_review_required)bottom.append(el('span','review-pill','일부 정보 확인 필요'));else bottom.append(el('span','',n.analyzed?'AI 요약 제공':'원문 공지'));a.append(bottom);return a;
}
async function load(){
 if(listController)listController.abort();listController=new AbortController();const controller=listController;remember();$('cards').replaceChildren();$('cards').setAttribute('aria-busy','true');feedback('소식을 불러오고 있어요','잠시만 기다려 주세요.');$('prev').disabled=true;$('next').disabled=true;
 try{const data=await api('/api/notices?'+parameters(),{signal:controller.signal});if(controller!==listController)return;pageData=data;$('feedback').classList.add('hidden');$('cards').replaceChildren(...data.items.map(card));const count=$('result-count');count.replaceChildren(el('b','',String(data.total)),document.createTextNode('개의 공지'));
 const notes=[];if(state.query)notes.push(`“${state.query}” 검색 결과`);if(state.sort==='deadline')notes.push('확인된 신청·제출·이수 마감부터 표시합니다. 날짜가 불확실한 공지는 뒤에 표시해요.');if(state.sort==='event')notes.push('확인된 행사·면접·발표 일정부터 표시합니다.');if(state.upcoming)notes.push('원문 근거가 확인된 앞으로의 일정만 표시합니다.');$('filter-note').textContent=notes.join(' · ');$('filter-note').classList.toggle('hidden',notes.length===0);
 $('page-label').textContent=`${data.page} / ${Math.max(1,data.pages)}`;$('prev').disabled=data.page<=1;$('next').disabled=data.page>=data.pages;
 if(!data.items.length)feedback('찾으시는 공지가 없어요','다른 검색어를 입력하거나 필터를 초기화해 보세요.');
 }catch(e){if(e.name==='AbortError')return;$('result-count').textContent='조회 실패';feedback('공지를 불러오지 못했어요',e.message,true);}finally{if(controller===listController)$('cards').setAttribute('aria-busy','false');}
}
function section(title){const s=el('section','detail-section');s.append(el('h3','',title));return s;}
function schedule(s){
 const a=el('article','schedule-item'+(s.parent_id?' child':''));const heading=el('div','schedule-heading');heading.append(el('h4','',s.label),el('span','schedule-state'+(s.review_required?' review':''),s.state));a.append(heading,el('p','schedule-dates',range(s)));if(s.scope)a.append(el('div','schedule-scope','구분 · '+s.scope));if(s.review_required){const r=s.review_reasons.map(v=>reasons[v]||'추가 원문 검토 필요');a.append(el('p','review-note','확정 일정으로 사용하지 마세요.'+(r.length?' '+[...new Set(r)].join(' · '):'')));}
 const quotes=(s.evidence_spans||[]).filter(v=>v.location?.matched&&v.location.original_text).map(v=>v.location.original_text);if(!quotes.length&&s.evidence)quotes.push(s.evidence);if(quotes.length){const evidence=el('details');evidence.append(el('summary','','원문 근거 보기'));for(const q of [...new Set(quotes)])evidence.append(el('blockquote','',q));a.append(evidence);}return a;
}
async function detail(id){
 if(detailController)detailController.abort();detailController=new AbortController();const controller=detailController;const dlg=$('detail');$('detail-content').replaceChildren(el('h2','detail-title','공지를 불러오는 중'));$('detail-content').firstChild.id='detail-title';if(!dlg.open)dlg.showModal();dlg.scrollTop=0;
 try{const n=await api('/api/notices/'+encodeURIComponent(id),{signal:controller.signal});if(controller!==detailController)return;const box=$('detail-content');box.replaceChildren();const meta=el('div','detail-meta');meta.append(el('span','category-tag '+n.category,categoryLabel(n.category)),el('span','',sourceLabel(n.source)),el('span','',date(n.published_at)),el('span','',n.category_method==='ai'?'AI 분류':'학교 공식 분류'));const h=el('h2','detail-title',n.title);h.id='detail-title';box.append(meta,h);
 const summary=el('section','detail-summary');summary.append(el('h3','',n.analyzed?'✦ AI 요약':'원문 공지'),el('p','',n.analyzed?n.summary||'제공된 요약이 없습니다.':'아직 AI 분석이 완료되지 않은 공지입니다. 아래 원문 내용을 확인해 주세요.'));if(n.summary_review_required)summary.append(el('p','review-note','요약의 대상·조건 등은 원문 확인이 필요합니다.'));box.append(summary);
 const audience=section('누가 참여할 수 있나요?');audience.append(el('p','audience',n.audience_review_required?'참여 대상과 지원 자격은 원문에서 확인해 주세요.':n.target_audience||'확인된 참여 대상 정보가 없습니다.'));box.append(audience);
 const schedules=section(`일정 ${n.schedules.length}개`);if(n.schedules.length)for(const s of n.schedules)schedules.append(schedule(s));else schedules.append(el('p','audience',n.analyzed?'저장된 본문에서 일정을 추출하지 못했습니다. 첨부파일이나 원문에 추가 정보가 있을 수 있어요.':'일정이 아직 분석되지 않았습니다.'));box.append(schedules);
 const original=el('details','original-body');original.append(el('summary','','저장된 원문 본문 보기'),el('pre','',n.content||'저장된 본문이 없습니다.'));box.append(original);
 if(n.original_url){const link=el('a','primary original-link','학교 원문에서 자세히 보기 ↗');link.href=n.original_url;link.target='_blank';link.rel='noopener noreferrer';box.append(link);}else box.append(el('p','review-note','안전한 원문 링크가 제공되지 않았습니다.'));
 box.append(el('p','detail-footnote','AI가 정리한 정보가 포함되어 있습니다. 일정과 지원 자격은 변경될 수 있으니 신청 전 원문을 확인해 주세요.'));
 }catch(e){if(e.name!=='AbortError'){$('detail-content').replaceChildren(el('h2','detail-title','상세보기를 불러오지 못했어요'),el('p','audience',e.message));$('detail-content').firstChild.id='detail-title';}}
}
async function boot(){
 try{const session=await api('/api/session');if(!session.authenticated){showLogin();return;}showApp();const [meta,cats]=await Promise.all([api('/api/sources'),api('/api/categories')]);sources=meta.sources;categories=cats.items;$('visible-count').replaceChildren(document.createTextNode(String(meta.visible_count)),el('small','','건'));$('updated').textContent=meta.updated_at?date(meta.updated_at)+' 업데이트':'업데이트 시각 미상';filters();await load();}catch(e){if(!$('app').classList.contains('hidden'))feedback('서비스를 불러오지 못했어요',e.message,()=>boot());else{$('login-error').textContent=e.message;showLogin();}}
}
$('login-form').addEventListener('submit',async e=>{e.preventDefault();const button=e.currentTarget.querySelector('button');button.disabled=true;$('login-error').textContent='';try{await api('/api/session',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({token:$('token').value})});$('token').value='';await boot();}catch(e){$('login-error').textContent=e.message;}finally{button.disabled=false;}});
$('logout').onclick=async()=>{try{await api('/api/session',{method:'DELETE'});showLogin();}catch(e){feedback('로그아웃에 실패했어요',e.message);}};
$('search-form').addEventListener('submit',e=>{e.preventDefault();state.query=$('query').value.trim();state.page=1;load();});
for(const [id,key]of [['sort','sort'],['status','status'],['upcoming','upcoming'],['has-schedule','has_schedule']])$(id).addEventListener('change',e=>{state[key]=e.target.type==='checkbox'?e.target.checked:e.target.value;state.page=1;load();});
$('reset').onclick=()=>{Object.assign(state,{page:1,sort:'latest',source:'',category:'',query:'',status:'',has_schedule:false,upcoming:false});filters();load();};
$('prev').onclick=()=>{if(state.page>1){state.page--;load();$('sources').scrollIntoView({block:'start'});}};$('next').onclick=()=>{if(pageData&&state.page<pageData.pages){state.page++;load();$('sources').scrollIntoView({block:'start'});}};
$('close-detail').onclick=()=>$('detail').close();$('detail').addEventListener('close',()=>detailController?.abort());$('detail').addEventListener('click',e=>{if(e.target===$('detail')){const r=e.target.getBoundingClientRect();if(e.clientX<r.left||e.clientX>r.right||e.clientY<r.top||e.clientY>r.bottom)e.target.close();}});
const params=new URLSearchParams(location.search);for(const k of ['query','source','category','status','sort'])if(params.has(k))state[k]=params.get(k);for(const k of ['upcoming','has_schedule'])state[k]=params.get(k)==='true';const p=Number(params.get('page'));if(Number.isInteger(p)&&p>=1&&p<=10000)state.page=p;
boot();
