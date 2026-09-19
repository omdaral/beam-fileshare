/* Beam web shares — registry model: guest announce + chunked publish, host browse/share, relay + presence ticker, temp panel. */
/* lastShares lives in files.js (single source); shares.js only reads it. */
var shareFiles={};
var sharePending=0;
/* Owner identity: random hex in localStorage "beam-owner" (created if absent).
   NOTE: api.js also uses "beam-owner" as the X-Beam-Owner admin token. A 64-hex
   admin token doubles as the relay owner_id, so guest shares stay attributable
   after the host pairs; guests just keep their random id (unknown token => non-admin). */
function beamOwnerId(){
  var t="";
  try{t=localStorage.getItem("beam-owner")||"";}catch(e){t="";}
  if(!/^[0-9a-fA-F]{8,128}$/.test(t)){
    try{t=String(genUuid()).replace(/-/g,"");}catch(e2){t="";}
    if(!/^[0-9a-fA-F]{8,128}$/.test(t)){
      t="";
      for(var k=0;k<32;k++)t+="0123456789abcdef"[Math.floor(Math.random()*16)];
    }
    try{localStorage.setItem("beam-owner",t);}catch(e3){}
  }
  return t;
}
/* Display name: "beam-name" or a device default. No editor UI (not in contract). */
function beamName(){
  var n="";
  try{n=localStorage.getItem("beam-name")||"";}catch(e){n="";}
  n=String(n||"").replace(/^\s+|\s+$/g,"");
  if(n)return n.slice(0,64);
  try{return "Beam-"+String(beamOwnerId()).slice(0,6);}catch(e2){return "Beam";}
}
function shareHeaders(h){
  h=h||{};
  try{h["X-Lang"]=(typeof LANG!=="undefined"?LANG:"ar");}catch(e){}
  try{return ownerHeaders(h);}catch(e2){return h;}
}
function shareUrl(id){return "/r/"+encodeURIComponent(id);}
function myGuestShares(){
  var me="";
  try{me=String(beamOwnerId());}catch(e){}
  var out=[];
  for(var i=0;i<lastShares.length;i++){
    var s=lastShares[i];
    if(s&&s.source==="guest"&&String(s.owner_id||"")===me)out.push(s);
  }
  return out;
}
/* Relay poll runs ONLY while this browser owns live (unavailable) guest shares. */
function relayActive(){
  if(sharePending>0)return true;
  var mine=myGuestShares();
  for(var i=0;i<mine.length;i++)if(!mine[i].available)return true;
  return false;
}
function updateKeepOpen(){
  var el=$("keepOpenNote");
  if(!el)return;
  el.classList.toggle("hidden",!relayActive());
}
function syncShareUI(){
  var owner=false;
  try{owner=!!isOwner;}catch(e){owner=false;}
  // Owner uploads straight from disk (~/Downloads/Beam-Temp): no picker
  // button any more — the hint below is informational only.
  var hh=$("hostBrowseHint");
  if(hh)hh.classList.toggle("hidden",!owner);
  var tc=$("tempCleanBtn");
  if(tc)tc.classList.toggle("hidden",!owner);
}
/* ---------- guest publish: announce share, then chunked upload ---------- */
/* Announce carries owner identity + V2 piece framing so the /upload_piece
   engine and live relay work. Backend keeps empty owner_id backward-compat
   (available once complete, no live relay). */
function shareAnnounce(name,size,kind){
  var body={name:name,size:size,kind:kind||"file"};
  try{body.owner_id=beamOwnerId();}catch(e){}
  try{body.owner=beamName();}catch(e2){}
  try{
    var plen=((typeof PIECE_LEN!=="undefined")?PIECE_LEN:4*1024*1024);
    if(!(plen>0))plen=4*1024*1024;
    body.piece_len=plen;
    // never noverify: single fast+reliable mode, always checksummed.
  }catch(e3){}
  return postJSON("/api/share/guest",body).then(function(res){
    if(res.status===200&&res.body&&res.body.ok&&res.body.id&&res.body.session)
      return {id:res.body.id,session:res.body.session};
    var err=(res.body&&res.body.error)||T("share_announce_fail");
    throw new Error(err);
  },function(){throw new Error(T("share_announce_fail"));});
}
/* Single fast+reliable mode: always verified 4MB pieces. Old turbo flag
   ignored — kept only so old cached pages don't crash. */
function shareTurbo(){return false;}
function shareDone(rel){
  if(sharePending>0)sharePending--;
  updateKeepOpen();
  try{refresh();}catch(e){}
}
/* Announce in small batches so big folders neither storm the server nor
   hit the share cap silently; progress + per-file reasons always shown. */
var ANN_CHUNK=10;
function sharePublishItems(items,flatLabel){
  var msg=$("msg");
  var valid=[],skipped=0;
  for(var i=0;i<(items||[]).length;i++){
    var o=items[i];
    if(!o||!o.f)continue;
    if(!o.f.size){skipped++;continue;}
    valid.push({f:o.f,name:o.name||o.f.name});
  }
  if(!valid.length){
    if(msg)show(msg,(skipped?("("+skipped+" "+T("ann_skipped")+") "):"")+T("folder_empty"),false);
    return;
  }
  var total=valid.length,ok=[],failed=0,failNotes=[];
  if(msg)show(msg,T("announcing")+" 0 / "+total,true);
  function runChunk(ix){
    if(ix>=valid.length){finishAnnounce();return;}
    var slice=valid.slice(ix,ix+ANN_CHUNK);
    Promise.all(slice.map(function(v){
      return shareAnnounce(v.name,v.f.size,"file").then(function(r){
        return {f:v.f,name:v.name,id:r.id,session:r.session};
      },function(e){
        return {err:((e&&e.message)||T("share_announce_fail")),name:v.name};
      });
    })).then(function(results){
      results.forEach(function(r){
        if(r&&r.err){failed++;if(failNotes.length<3)failNotes.push(r.name+" — "+r.err);}
        else if(r)ok.push(r);
      });
      if(msg)show(msg,T("announcing")+" "+Math.min(ix+ANN_CHUNK,total)+" / "+total,true);
      runChunk(ix+ANN_CHUNK);
    });
  }
  function finishAnnounce(){
    var note=skipped?(" • "+skipped+" "+T("ann_skipped")):"";
    if(!ok.length){
      if(msg)show(msg,T("share_announce_fail")+(failNotes.length?(" — "+failNotes.join(" • ")):"")+note,false);
      return;
    }
    if(failed&&msg)show(msg,T("share_announce_fail")+" ("+failed+" "+T("ann_rejected")+(failNotes.length?(" — "+failNotes.join(" • ")):"")+"))"+note,false);
    else if(note&&msg)show(msg,ok.length+" ✓"+note,true);
    else if(msg)clearMsg(msg);
    var k,bytes=0;
    for(k=0;k<ok.length;k++){bytes+=ok[k].f.size||0;try{shareFiles[ok[k].id]=ok[k].f;}catch(e){}}
    sharePending+=ok.length;
    updateKeepOpen();
    try{refresh();}catch(e2){}
    if(ok.length===1){
      var o1=ok[0];
      enqueueUpload(o1.f,"share:"+o1.id,{id:o1.session},null,{turbo:shareTurbo(),label:o1.name},"share:"+o1.id);
    }else{
      var b=newBatch((flatLabel||(ok.length+" "+T("batch_files"))),ok.length,bytes);
      try{buildBatchRow(b,$("upList"));}catch(e3){}
      for(var j=0;j<ok.length;j++){
        (function(o){
          enqueueUpload(o.f,"share:"+o.id,{id:o.session},b,{turbo:shareTurbo(),label:o.name},"share:"+o.id);
        })(ok[j]);
      }
    }
  }
  runChunk(0);
}
function sharePublishFiles(list){
  var arr=Array.prototype.slice.call(list||[]);
  sharePublishItems(arr.map(function(f){return {f:f,name:f.name};}));
}
function sharePublishFolderItems(items){
  sharePublishItems((items||[]).map(function(o){return {f:o.f,name:o.rel||o.f.name};}));
}
/* ---------- relay (owner serves bytes) + presence ---------- */
function sendPresence(){
  if(typeof serverGone!=="undefined"&&serverGone)return;
  if(document.hidden)return;
  var body={owner_id:beamOwnerId(),name:beamName()};
  try{
    fetch("/api/presence",{method:"POST",headers:shareHeaders({"Content-Type":"application/json"}),body:JSON.stringify(body)}).catch(function(){});
  }catch(e){}
}
function pollRelay(){
  if(typeof serverGone!=="undefined"&&serverGone)return;
  if(document.hidden)return;
  if(!relayActive())return;
  var me=beamOwnerId();
  fetch("/api/relay/need?owner_id="+encodeURIComponent(me),{headers:shareHeaders()}).then(function(r){
    if(!r.ok)throw new Error("http"+r.status);
    return r.json();
  }).then(function(j){
    var jobs=(j&&j.jobs)||[];
    jobs.forEach(function(job){serveRelayJob(job);});
  }).catch(function(){});
}
function serveRelayJob(job){
  try{
    if(!job||!job.token||!job.share)return;
    var f=shareFiles[job.share];
    if(!f)return; /* File gone (e.g. after reload) — skip per contract */
    var off=parseInt(job.offset,10)||0,len=parseInt(job.len,10)||0;
    if(!(len>0)||off<0||off>=f.size)return;
    var end=Math.min(off+len,f.size);
    var blob=null;
    try{blob=f.slice(off,end);}catch(e){return;}
    if(!blob||!blob.size)return;
    fetch("/api/relay/piece?token="+encodeURIComponent(job.token),{method:"POST",headers:shareHeaders(),body:blob}).catch(function(){});
  }catch(e){}
}
/* ---------- temp panel ---------- */
function refreshTemp(){
  var p=$("tempPath"),s=$("tempSize");
  if(!p&&!s)return;
  fetch("/api/temp",{headers:shareHeaders()}).then(function(r){
    if(!r.ok)throw new Error("http"+r.status);
    return r.json();
  }).then(function(j){
    if(p)p.textContent=(j&&j.path)?String(j.path):"—";
    if(s){
      var txt=(j&&typeof j.size==="number")?fmt(j.size):"—";
      if(j&&typeof j.warn_bytes==="number"&&typeof j.size==="number"&&j.warn_bytes>0&&j.size>j.warn_bytes)
        txt+=" — "+T("temp_over");
      s.textContent=txt;
    }
  }).catch(function(){});
}
function tempClean(btn){
  var m=$("netMsg");
  if(btn.dataset.armed!=="1"){
    btn.dataset.armed="1";setBtn(btn,ICONS.trash,T("temp_clean_confirm"));
    setTimeout(function(){try{btn.dataset.armed="0";setBtn(btn,ICONS.trash,T("temp_clean"));}catch(e){}},4000);
    return;
  }
  btn.dataset.armed="0";setBtn(btn,ICONS.trash,T("temp_clean"));
  postJSON("/api/temp/clean",{}).then(function(res){
    if(res.status===200&&res.body&&res.body.ok){
      var freed=(res.body&&typeof res.body.freed==="number")?(" — "+fmt(res.body.freed)):"";
      if(m)show(m,T("temp_cleaned")+freed,true);
      refreshTemp();
      try{refreshSessions();}catch(e){}
    }else if(m)show(m,((res.body&&res.body.error)||T("temp_clean_fail")),false);
  }).catch(function(){if(m)show(m,T("temp_clean_fail"),false);});
}
/* ---------- host browse + share modal (owner only) ---------- */
var browseCur="";
var SHARE_DIR_SVG='<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>';
var SHARE_FILE_SVG='<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>';
function openBrowse(dir){
  try{if(!isOwner)return;}catch(e){return;}
  var z=$("browseZone");
  if(!z)return;
  z.classList.remove("hidden");
  // Empty start dir used to query /api/browse?dir= (400 share_bad_path).
  // Default to the filesystem root so the modal always has a valid start.
  browseLoad(dir||"/");
}
function closeBrowse(){var z=$("browseZone");if(z)z.classList.add("hidden");}
function browseLoad(dir){
  var m=$("browseMsg"),list=$("browseList");
  browseCur=dir||"";
  if(m)clearMsg(m);
  if($("browsePath"))$("browsePath").textContent=browseCur||"/";
  if(list)list.innerHTML="";
  fetch("/api/browse?dir="+encodeURIComponent(browseCur),{headers:shareHeaders()}).then(function(r){
    if(!r.ok)throw new Error("http"+r.status);
    return r.json();
  }).then(function(j){renderBrowse(j||{});}).catch(function(){if(m)show(m,T("browse_fail"),false);});
}
function renderBrowse(j){
  var list=$("browseList");
  if(!list)return;
  list.innerHTML="";
  var cur=(j&&j.dir)?String(j.dir):browseCur;
  browseCur=cur;
  if($("browsePath"))$("browsePath").textContent=cur||"/";
  var up=$("browseUp");
  if(up){
    up.style.display=(j&&j.parent)?"":"none";
    up.onclick=function(){browseLoad(j.parent);};
  }
  var here=$("browseShareHere");
  if(here)here.onclick=function(){browseShare(cur,null);};
  var dirs=(j&&j.dirs)||[],files=(j&&j.files)||[];
  if(!dirs.length&&!files.length){
    var p=document.createElement("p");p.className="small";p.textContent=T("browse_empty");list.appendChild(p);return;
  }
  dirs.forEach(function(x){list.appendChild(browseRow(x.name||"",x.path||"",true,0));});
  files.forEach(function(x){list.appendChild(browseRow(x.name||"",x.path||"",false,x.size||0));});
}
function browseRow(name,path,isDir,size){
  var d=document.createElement("div");d.className="file";
  var ic=document.createElement("span");ic.innerHTML=isDir?SHARE_DIR_SVG:SHARE_FILE_SVG;
  var nm=document.createElement("span");nm.className="name";nm.textContent=name;nm.title=path;
  d.appendChild(ic);d.appendChild(nm);
  if(!isDir){var s=document.createElement("span");s.className="size";s.textContent=fmt(size);d.appendChild(s);}
  if(isDir){
    var ob=document.createElement("button");ob.className="btn-gray mini";setBtn(ob,"",T("browse_open"));
    ob.onclick=function(){browseLoad(path);};
    d.appendChild(ob);
  }
  var sb=document.createElement("button");sb.className="btn-green mini";setBtn(sb,"",T("browse_share"));
  sb.onclick=function(){browseShare(path,sb);};
  d.appendChild(sb);
  return d;
}
/* Host share body is EXACTLY {path}. */
function browseShare(path,btn){
  var m=$("browseMsg");
  if(btn)btn.disabled=true;
  postJSON("/api/share",{path:path}).then(function(res){
    if(btn)btn.disabled=false;
    if(res.status===200&&res.body&&res.body.ok){if(m)show(m,T("share_ok"),true);try{refresh();}catch(e){}}
    else if(m)show(m,((res.body&&res.body.error)||T("share_fail")),false);
  }).catch(function(){if(btn)btn.disabled=false;if(m)show(m,T("share_fail"),false);});
}
/* ---------- unified ticker: presence+temp every 10s, relay every 5s ---------- */
var shareTickN=0;
function tickShares(){
  if(typeof serverGone!=="undefined"&&serverGone)return;
  if(document.hidden)return;
  shareTickN++;
  try{syncShareUI();}catch(e){}
  if(shareTickN%2===1){try{sendPresence();}catch(e2){}try{refreshTemp();}catch(e3){}}
  try{pollRelay();}catch(e4){}
  try{updateKeepOpen();}catch(e5){}
}
(function initShares(){
  try{beamOwnerId();}catch(e){}
  function wire(){
    // hostBrowseBtn removed: owner shares from disk automatically.
    var bc=$("browseClose");
    if(bc&&!bc.dataset.bound){bc.dataset.bound="1";bc.onclick=closeBrowse;}
    var bz=$("browseZone");
    if(bz&&!bz.dataset.bound){bz.dataset.bound="1";bz.addEventListener("click",function(ev){if(ev.target===bz)closeBrowse();});}
    var tc=$("tempCleanBtn");
    if(tc&&!tc.dataset.bound){tc.dataset.bound="1";tc.onclick=function(){tempClean(tc);};}
  }
  try{wire();}catch(e2){}
  try{syncShareUI();}catch(e3){}
  try{sendPresence();}catch(e4){}
  try{refreshTemp();}catch(e5){}
  try{updateKeepOpen();}catch(e6){}
  try{document.addEventListener("visibilitychange",function(){if(!document.hidden){try{sendPresence();}catch(e7){}}});}catch(e8){}
  try{document.addEventListener("keydown",function(ev){if(ev.key==="Escape"||ev.key==="Esc"){var z=$("browseZone");if(z&&!z.classList.contains("hidden"))closeBrowse();}});}catch(e9){}
  try{setInterval(tickShares,5000);}catch(e10){}
})();
