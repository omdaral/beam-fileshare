/* Beam web files — registry list (GET /api/shares), instant search, share download, public unshare. */
/* ---------- قائمة المشاركات: بحث فوري (محلي) + تنزيل + إلغاء مشاركة ---------- */
var lastShares=[],fileFilter="";
function searchMax(){return (typeof CONFIG!=="undefined"&&CONFIG.SEARCH_MAX)||200;}
function sh(){
  try{if(typeof shareHeaders==="function")return shareHeaders();}catch(e){}
  try{return ownerHeaders({"X-Lang":(typeof LANG!=="undefined"?LANG:"ar")});}catch(e2){return {};}
}
function shareBaseName(n){
  n=String((n==null)?"":n);
  var i=n.lastIndexOf("/");
  return i>=0?n.slice(i+1):n;
}
function sharePseudo(s){return "__share__/"+s.id;}
function shareSaveName(s){
  var n=shareBaseName((s&&s.name)||"file")||"file";
  if(s&&s.kind==="dir"&&!/\.zip$/i.test(n))n+=".zip";
  return n;
}
function refresh(){
  var box=$("files");
  if((typeof serverGone!=="undefined"&&serverGone)||!box)return {catch:function(){}};
  // Never wipe in-progress download bars (they live inside #files).
  try{if(typeof activeDownloads!=="undefined"&&activeDownloads>0)return {catch:function(){}};}catch(e){}
  return fetch("/api/shares",{headers:sh()}).then(function(r){
    if(!r.ok)throw new Error("http"+r.status);
    return r.json();
  }).then(function(j){
    lastShares=(j&&j.shares)||[];
    renderShares();
    try{if(typeof updateKeepOpen==="function")updateKeepOpen();}catch(e){}
  }).catch(function(){
    box.innerHTML="";
    var p=document.createElement("p");p.textContent=T("srv_list_fail");;box.appendChild(p);
  });
}
var SH_FILE_SVG='<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>';
var SH_DIR_SVG='<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>';
function shareRow(s,label){
  var d=document.createElement("div");d.className="file";
  if(!s.available){try{d.classList.add("is-unavail");}catch(e){}}
  var ic=document.createElement("span");ic.innerHTML=(s.kind==="dir")?SH_DIR_SVG:SH_FILE_SVG;
  var wrap=document.createElement("span");wrap.className="name";
  wrap.textContent=(label==null)?String(s.name||s.id||""):String(label);
  var ow=document.createElement("span");ow.className="path";
  ow.textContent=T("share_by")+" "+String(s.owner||"");
  wrap.appendChild(document.createElement("br"));wrap.appendChild(ow);
  var sz=document.createElement("span");sz.className="size";
  sz.textContent=(s.size>0)?fmt(s.size):((s.kind==="dir")?T("kind_dir"):T("kind_file"));
  var badge=document.createElement("span");badge.className="size";
  badge.textContent=s.available?T("avail_yes"):T("avail_no");
  try{badge.className="size "+(s.available?"badge-ok":"badge-off");}catch(e2){}
  d.appendChild(ic);d.appendChild(wrap);d.appendChild(sz);d.appendChild(badge);
  if(s.available){
    (function(ss){
      var dl=document.createElement("button");dl.className="btn-green";setBtn(dl,ICONS.dl,T("dl"));dl.title=T("dl_fast_title");
      dl.onclick=function(){smartDownload(sharePseudo(ss),ss.size||0,ss.mtime||0,d,shareSaveName(ss));};
      d.appendChild(dl);
    })(s);
  }
  (function(id){
    var un=document.createElement("button");un.className="btn-red";setBtn(un,ICONS.trash,T("unshare"));un.dataset.armed="0";
    un.onclick=function(){unshareShare(id,un);};
    d.appendChild(un);
  })(s.id);
  return d;
}
/* Folder shares carry rel paths ("dir/sub/file"). Group them into a
   collapsible tree; loose files stay flat on top. */
function buildShareTree(list){
  var root={dirs:{},files:[]};
  (list||[]).forEach(function(s){
    var nm=String((s&&s.name)||(s&&s.id)||"");
    var parts=nm.split("/");
    var node=root,ok=true;
    for(var i=0;i<parts.length-1;i++){
      var seg=parts[i];
      if(!seg){ok=false;break;}
      if(!node.dirs[seg])node.dirs[seg]={dirs:{},files:[]};
      node=node.dirs[seg];
    }
    if(!ok){root.files.push(s);return;}
    node.files.push(s);
  });
  return root;
}
function countTreeShares(node){
  var n=node.files.length;
  for(var k in node.dirs)if(node.dirs.hasOwnProperty(k))n+=countTreeShares(node.dirs[k]);
  return n;
}
function sumTreeSizes(node){
  var t=0,i;
  for(i=0;i<node.files.length;i++)t+=Number(node.files[i].size)||0;
  for(var k in node.dirs)if(node.dirs.hasOwnProperty(k))t+=sumTreeSizes(node.dirs[k]);
  return t;
}
/* One-click whole-folder download: streams the virtual folder prefix as
   a single STORE zip via GET /api/folder.zip?prefix=... */
function downloadFolderZip(prefix,baseName){
  var url="/api/folder.zip?prefix="+encodeURIComponent(prefix);
  var a=document.createElement("a");
  a.href=url;
  try{a.setAttribute("download",(baseName||prefix||"folder")+".zip");}catch(e){}
  document.body.appendChild(a);
  try{
    if(typeof a.click==="function")a.click();
    else location.href=a.href;
  }catch(e){try{location.href=a.href;}catch(e2){}}
  setTimeout(function(){try{document.body.removeChild(a);}catch(e3){}},2000);
}
function renderShareNode(node,box,prefix){
  prefix=prefix||"";
  var names=Object.keys(node.dirs).sort();
  names.forEach(function(nm){
    var sub=node.dirs[nm];
    var full=prefix?prefix+"/"+nm:nm;
    var d=document.createElement("div");d.className="dir";d.setAttribute("role","button");d.setAttribute("tabindex","0");
    var chev=document.createElement("span");chev.className="chev";chev.textContent="▸";
    var ic=document.createElement("span");ic.innerHTML=SH_DIR_SVG;
    var wrap=document.createElement("span");wrap.className="name";wrap.textContent=nm;
    var count=document.createElement("span");count.className="count";
    count.textContent=countTreeShares(sub)+" • "+fmt(sumTreeSizes(sub));
    d.appendChild(chev);d.appendChild(ic);d.appendChild(wrap);d.appendChild(count);
    // Single one-click folder button (no preflight/mode choice).
    (function(fp,bn){
      var zb=document.createElement("button");zb.className="btn-green mini";setBtn(zb,ICONS.dl,T("dl_folder")!=="dl_folder"?T("dl_folder"):"⬇ المجلد");
      zb.title=bn+".zip";
      zb.onclick=function(ev){try{if(ev)ev.stopPropagation();}catch(e){}downloadFolderZip(fp,bn);};
      zb.onkeydown=function(ev){try{if(ev)ev.stopPropagation();}catch(e2){}};
      d.appendChild(zb);
    })(full,nm);
    var kids=document.createElement("div");kids.className="dir-children";kids.style.display="none";
    renderShareNode(sub,kids,full);
    function toggle(){
      var open=kids.style.display!=="none";
      kids.style.display=open?"none":"";
      chev.textContent=open?"▸":"▾";
    }
    d.onclick=toggle;
    d.onkeydown=function(e){if(e.key==="Enter"||e.key===" "){e.preventDefault();toggle();}};
    box.appendChild(d);box.appendChild(kids);
  });
  node.files.forEach(function(s){box.appendChild(shareRow(s,shareBaseName(s.name||s.id)));});
}
function renderShares(){
  var box=$("files");
  if(!box)return;
  box.innerHTML="";
  var list=lastShares||[];
  var q=fileFilter;
  if(q){
    var ql=q.toLowerCase(),hits=[];
    for(var i=0;i<list.length&&hits.length<searchMax();i++){
      var hay=(String(list[i].name||"")+" "+String(list[i].owner||"")).toLowerCase();
      if(hay.indexOf(ql)>=0)hits.push(list[i]);
    }
    var meta=document.createElement("p");meta.className="search-meta";
    meta.textContent=hits.length+" / "+list.length;
    box.appendChild(meta);
    if(!hits.length){var p=document.createElement("p");p.textContent=T("no_match");box.appendChild(p);return;}
    hits.forEach(function(s){box.appendChild(shareRow(s));});
    if(hits.length>=searchMax()){var m2=document.createElement("p");m2.className="search-meta";m2.textContent="+"+T("search_more");box.appendChild(m2);}
    return;
  }
  if(!list.length){var p0=document.createElement("p");p0.textContent=T("no_shares");box.appendChild(p0);return;}
  renderShareNode(buildShareTree(list),box);
}
var searchTimer=null;
function bindSearch(){
  var s=$("fileSearch");
  if(!s||s.dataset.bound)return;
  s.dataset.bound="1";
  s.addEventListener("input",function(){
    if(searchTimer)clearTimeout(searchTimer);
    searchTimer=setTimeout(function(){
      fileFilter=s.value.trim();
      renderShares();
    },150);
  });
}
/* إلغاء المشاركة للجميع (public) — بخطوتين بدل النوافذ المنبثقة */
function unshareShare(id,btn){
  var m=$("msg");
  if(btn.dataset.armed!=="1"){
    btn.dataset.armed="1";setBtn(btn,ICONS.trash,T("unshare_confirm"));
    setTimeout(function(){try{if(btn.dataset.armed==="1"){btn.dataset.armed="0";setBtn(btn,ICONS.trash,T("unshare"));}}catch(e){}},4000);
    return;
  }
  btn.dataset.armed="0";setBtn(btn,ICONS.trash,T("unshare"));
  postJSON("/api/unshare",{id:id}).then(function(res){
    if(res.status===200&&res.body&&res.body.ok){
      try{if(typeof shareFiles!=="undefined")delete shareFiles[id];}catch(e){}
      show(m,T("unshared_ok"),true);refresh();
    }
    else show(m,((res.body&&res.body.error)||T("unshare_fail")),false);
  }).catch(function(){show(m,T("unshare_fail"),false);});
}
