/* Beam web files — folder tree, local search, zip/download/delete rows. */
/* ---------- قائمة الملفات: شجرة مجلدات + بحث خفيف (محلي، بدون طلبات) ---------- */
var lastFiles=[],lastDirs=[],lastTruncated=false,expandedDirs={},fileFilter="";
function searchMax(){return (typeof CONFIG!=="undefined"&&CONFIG.SEARCH_MAX)||200;}
function baseName(p){var i=p.lastIndexOf("/");return i>=0?p.slice(i+1):p;}
function dirName(p){var i=p.lastIndexOf("/");return i>=0?p.slice(0,i):"";}
function dirInfo(path){
  for(var i=0;i<lastDirs.length;i++)if(lastDirs[i].path===path)return lastDirs[i];
  return {path:path,files:0,over:""};
}
function overReason(over,n){
  if(over==="files")return T("over_files")+" ("+n+")";
  if(over==="depth")return T("over_depth");
  return T("over_list");
}
function refresh(){
  var box=$("files");
  if(serverGone||!box)return {catch:function(){}};
  return fetch("/files").then(function(r){
    if(!r.ok)throw new Error("http"+r.status);
    return r.json();
  }).then(function(j){
    lastFiles=(j.files||[]).map(function(f){
      var p=f.path||f.name||"";
      return {path:String(p),name:baseName(String(p)),size:f.size||0,mtime:f.mtime||0};
    });
    lastDirs=j.dirs||[];
    lastTruncated=!!j.truncated;
    renderFiles();
  }).catch(function(){
    box.innerHTML="";
    var p=document.createElement("p");p.textContent=T("srv_list_fail");;box.appendChild(p);
  });
}
function fileRow(f,showPath){
  var d=document.createElement("div");d.className="file";
  var wrap=document.createElement("span");wrap.className="name";
  wrap.textContent=f.name;
  if(showPath){
    var pp=document.createElement("span");pp.className="path";pp.textContent=f.path;
    wrap.appendChild(document.createElement("br"));wrap.appendChild(pp);
  }else{wrap.title=f.path;}
  var s=document.createElement("span");s.className="size";s.textContent=fmt(f.size);
  var a=document.createElement("a");a.className="btn-green";a.style.minHeight="48px";setBtn(a,ICONS.dl,T("dl"));a.href="/download?file="+encodeURIComponent(f.path);a.setAttribute("download",f.name);
  var fast=document.createElement("button");fast.className="btn-green";fast.style.minHeight="48px";setBtn(fast,ICONS.bolt,T("fast"));fast.title=T("dl_fast_title");
  fast.onclick=function(){fastDownload(f.path,f.size,f.mtime,d);};
  var del=document.createElement("button");del.className="btn-red";setBtn(del,ICONS.trash,T("del"));del.dataset.armed="0";
  del.onclick=function(){delFile(f.path,del);};
  d.appendChild(wrap);d.appendChild(s);d.appendChild(a);d.appendChild(fast);d.appendChild(del);
  return d;
}
function dirRow(info){
  var d=document.createElement("div");d.className="dir"+(info.over?" zip-row":"");
  var open=!!expandedDirs[info.path]&&!info.over;
  var chev=document.createElement("span");chev.className="chev";chev.textContent=info.over?"⇩":(open?"▾":"▸");
  var ic=document.createElement("span");ic.innerHTML='<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>';
  var nm=document.createElement("span");nm.className="name";nm.textContent=baseName(info.path);
  var c=document.createElement("span");c.className="count";c.textContent=info.files+" "+T("dir_files");
  d.appendChild(chev);d.appendChild(ic);d.appendChild(nm);d.appendChild(c);
  var zip=document.createElement("button");zip.className="btn-gray mini";setBtn(zip,ICONS.dl,T("dir_zip"));
  zip.onclick=function(ev){if(ev)ev.stopPropagation();zipDl(info.path);};
  d.appendChild(zip);
  var del=document.createElement("button");del.className="btn-red mini";setBtn(del,ICONS.trash,T("dir_del"));del.dataset.armed="0";
  del.onclick=function(ev){if(ev)ev.stopPropagation();delDir(info.path,del);};
  d.appendChild(del);
  if(info.over){
    var note=document.createElement("p");note.className="over-note";note.textContent=overReason(info.over,info.files);
    d.appendChild(note);
  }
  if(!info.over){
    d.onclick=function(){expandedDirs[info.path]=!expandedDirs[info.path];renderFiles();};
    if(open){
      var kids=document.createElement("div");kids.className="dir-children";
      lastDirs.forEach(function(sub){
        if(dirName(sub.path)===info.path)kids.appendChild(dirRow(sub));
      });
      lastFiles.forEach(function(f){
        if(dirName(f.path)===info.path)kids.appendChild(fileRow(f,false));
      });
      var out=document.createElement("div");
      out.appendChild(d);out.appendChild(kids);
      return out;
    }
  }
  return d;
}
function renderFiles(){
  var box=$("files");
  if(!box)return;
  box.innerHTML="";
  var q=fileFilter;
  if(q){
    var ql=q.toLowerCase(),hits=[];
    for(var i=0;i<lastFiles.length&&hits.length<searchMax();i++){
      if(lastFiles[i].path.toLowerCase().indexOf(ql)>=0)hits.push(lastFiles[i]);
    }
    var meta=document.createElement("p");meta.className="search-meta";
    meta.textContent=hits.length+" / "+lastFiles.length;
    box.appendChild(meta);
    if(!hits.length){var p=document.createElement("p");p.textContent=T("no_match");box.appendChild(p);return;}
    hits.forEach(function(f){box.appendChild(fileRow(f,true));});
    if(hits.length>=searchMax()){var m2=document.createElement("p");m2.className="search-meta";m2.textContent="+"+T("search_more");box.appendChild(m2);}
    return;
  }
  if(lastTruncated){
    var tn=document.createElement("p");tn.className="search-meta";tn.textContent=T("over_list");
    box.appendChild(tn);
  }
  var roots=lastDirs.filter(function(x){return dirName(x.path)==="";});
  var rootFiles=lastFiles.filter(function(f){return dirName(f.path)==="";});
  if(!roots.length&&!rootFiles.length){var p0=document.createElement("p");p0.textContent=T("no_files");box.appendChild(p0);return;}
  roots.forEach(function(x){box.appendChild(dirRow(x));});
  rootFiles.forEach(function(f){box.appendChild(fileRow(f,false));});
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
      renderFiles();
    },150);
  });
}
/* تنزيل مجلد كـ zip مباشر */
function zipDl(dir){
  var a=document.createElement("a");
  a.href="/download_zip?dir="+encodeURIComponent(dir);
  a.setAttribute("download","");
  document.body.appendChild(a);
  try{a.click();}catch(e){location.href=a.href;}
  setTimeout(function(){try{document.body.removeChild(a);}catch(e2){}},2000);
}
/* حذف مجلد بكل ما فيه — بخطوتين بدل النوافذ المنبثقة */
function delDir(dir,btn){
  var m=$("msg");
  if(btn.dataset.armed!=="1"){
    btn.dataset.armed="1";setBtn(btn,ICONS.trash,T("dir_confirm_del"));
    setTimeout(function(){setBtn(btn,ICONS.trash,T("dir_del"));},4000);
    return;
  }
  setBtn(btn,ICONS.trash,T("dir_del"));
  fetch("/delete",{method:"POST",headers:{"Content-Type":"application/json","X-Lang":LANG},body:JSON.stringify({dir:dir})}).then(function(r){
    return r.json().then(function(j){return {ok:r.ok,body:j};});
  }).then(function(res){
    if(res.ok){show(m,T("deleted_ok"),true);refresh();}
    else{show(m,(res.body&&res.body.error)||T("del_fail"),false);}
  }).catch(function(){show(m,T("del_conn_fail"),false);});
}
/* حذف بخطوتين بدل النوافذ المنبثقة */
function delFile(name,btn){
  var m=$("msg");
  if(btn.dataset.armed!=="1"){
    btn.dataset.armed="1";setBtn(btn,ICONS.trash,T("confirm_del"));
    setTimeout(function(){setBtn(btn,ICONS.trash,T("del"));},4000);
    return;
  }
  setBtn(btn,ICONS.trash,T("del"));
  fetch("/delete",{method:"POST",headers:{"Content-Type":"application/json","X-Lang":LANG},body:JSON.stringify({file:name})}).then(function(r){
    return r.json().then(function(j){return {ok:r.ok,body:j};});
  }).then(function(res){
    if(res.ok){show(m,T("deleted_ok"),true);refresh();}
    else{show(m,(res.body&&res.body.error)||T("del_fail"),false);}
  }).catch(function(){show(m,T("del_conn_fail"),false);});
}

