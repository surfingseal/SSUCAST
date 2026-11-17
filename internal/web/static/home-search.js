// Home search: bounded local history, cancellable title suggestions and input-focused keyboard navigation.
(function(root){
 'use strict';
 const storageKey='ssu_cast_recent_searches_v1';
 function cleanHistory(value){return Array.isArray(value)?[...new Set(value.filter(s=>typeof s==='string').map(s=>s.trim()).filter(s=>s&&s.length<=100))].slice(0,5):[];}
 function remember(history,query){return cleanHistory([query,...cleanHistory(history)]);}
 function attach({form,input,api,onSearch,onNotice}){
  const popup=form.querySelector('.home-search-popup'),list=form.querySelector('[role="listbox"]'),heading=form.querySelector('.home-search-heading'),status=form.querySelector('[role="status"]'),clear=form.querySelector('.home-search-clear'),clearHistory=form.querySelector('.home-search-history-clear');
  let options=[],active=-1,timer=null,controller=null,generation=0,composing=false,opened=false;
  const node=(tag,cls,text)=>{const e=document.createElement(tag);e.className=cls;if(text!==undefined)e.textContent=text;return e;};
  function history(){try{return cleanHistory(JSON.parse(localStorage.getItem(storageKey)||'[]'));}catch{return [];}}
  function save(query){if(!query.trim())return;try{localStorage.setItem(storageKey,JSON.stringify(remember(history(),query)));}catch{}}
  function sync(){clear.classList.toggle('hidden',!input.value);}
  function cancel(){generation++;clearTimeout(timer);controller?.abort();controller=null;}
  function close(){cancel();status.textContent='';opened=false;popup.classList.add('hidden');input.setAttribute('aria-expanded','false');input.removeAttribute('aria-activedescendant');active=-1;}
  function open(){opened=true;popup.classList.remove('hidden');input.setAttribute('aria-expanded','true');}
  function select(index){active=index;Array.from(list.children).forEach((e,i)=>{e.setAttribute('aria-selected',String(i===active));});if(active>=0){input.setAttribute('aria-activedescendant',list.children[active].id);list.children[active].scrollIntoView({block:'nearest'});}else input.removeAttribute('aria-activedescendant');}
  function choose(option){const query=input.value.trim();close();if(option.kind==='notice'){save(query);onNotice(option.id);}else{input.value=option.title;sync();save(option.title);onSearch(option.title);}}
  function render(rows,label,message){
   options=rows;active=-1;input.removeAttribute('aria-activedescendant');list.replaceChildren();heading.textContent=label;clearHistory.classList.toggle('hidden',label!=='최근 검색어'||!rows.length);
   rows.forEach((option,index)=>{const item=node('div','home-search-option');item.id='home-search-option-'+index;item.setAttribute('role','option');item.setAttribute('aria-selected','false');const icon=node('span','home-search-option-icon',option.kind==='recent'?'↶':'↗');icon.setAttribute('aria-hidden','true');item.append(icon,node('span','home-search-option-title',option.title));item.addEventListener('pointerdown',e=>e.preventDefault());item.onclick=()=>choose(option);item.addEventListener('pointermove',()=>select(index));list.append(item);});
   const hint=form.querySelector('.home-search-empty');hint.textContent=message||'';hint.classList.toggle('hidden',!!rows.length);status.textContent=rows.length?label+' '+rows.length+'개. 위아래 방향키로 선택할 수 있습니다.':message||'';
  }
  function suggest(){
   cancel();sync();if(document.activeElement!==input)return;open();const query=input.value.trim();
   if(!query){const rows=history().map(title=>({kind:'recent',title}));render(rows,'최근 검색어','공고명을 입력하고 Enter를 눌러 검색하세요.');return;}
   render([],'공고명 검색','공고명을 찾고 있어요.');const request=generation;
   timer=setTimeout(async()=>{
    controller=new AbortController();
    try{const response=await api('/api/notices?'+new URLSearchParams({query,search_in:'title',sort:'latest',limit:'5',page:'1'}),{signal:controller.signal});if(request!==generation||!opened)return;render((response.items||[]).map(n=>({kind:'notice',id:n.id,title:n.title})),'공고명 검색','일치하는 공고명이 없어요. 다른 검색어를 입력해보세요.');}
    catch(e){if(request!==generation||!opened)return;render([],'공고명 검색','후보를 불러오지 못했어요. Enter를 눌러 검색해보세요.');}
   },250);
  }
  input.addEventListener('focus',suggest);
  input.addEventListener('input',()=>{sync();if(!composing)suggest();});
  input.addEventListener('compositionstart',()=>{composing=true;cancel();});
  input.addEventListener('compositionend',()=>{composing=false;suggest();});
  input.addEventListener('keydown',e=>{
   if(e.isComposing||composing||e.keyCode===229)return;
   if(e.key==='Escape'){e.preventDefault();close();return;}
   if(e.key==='ArrowDown'||e.key==='ArrowUp'){e.preventDefault();if(!opened)suggest();if(options.length)select(e.key==='ArrowDown'?(active+1)%options.length:(active<0?options.length-1:(active-1+options.length)%options.length));}
   if(e.key==='Enter'&&opened&&active>=0){e.preventDefault();choose(options[active]);}
  });
  form.addEventListener('submit',e=>{e.preventDefault();if(composing)return;const query=input.value.trim();save(query);close();onSearch(query);});
  clear.onclick=()=>{input.value='';sync();input.focus();suggest();};
  clearHistory.onclick=()=>{try{localStorage.removeItem(storageKey);}catch{}input.focus();suggest();};
  document.addEventListener('pointerdown',e=>{if(!form.contains(e.target))close();});
  form.addEventListener('focusout',e=>{if(!form.contains(e.relatedTarget))close();});
  sync();return {close,sync};
 }
 const api={cleanHistory,remember,attach};if(typeof module!=='undefined'&&module.exports)module.exports=api;else root.SSUHomeSearch=api;
})(typeof window!=='undefined'?window:globalThis);
