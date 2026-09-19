/* Beam web download — single smart button (parallel fast path + direct fallback). */
/* Registry shares ride the same engine via the pseudo-path "__share__/"+id,
   which maps to GET /r/<id> (+Range). The IndexedDB chunk cache is kept, so
   relayed guest files stay downloader-resumable. */
/* ---------- زر التنزيل الواحد: يحترم وضع موثوق/تيربو، ويسقط للمباشر ---------- */
/* الموثوق/التيربو يتحكم به مفتاح dlModeSeg (app.js). الملفات الضخمة التي
   تتجاوز DL_MAX_FAST أو المتصفحات بلا fetch/Range تنزل مباشرة عبر
   /download (أو /r/<id> للمشاركات) بدل المسار المتوازي. */
function shareIdOf(name){name=String(name||"");return (name.indexOf("__share__/")===0)?name.slice("__share__/".length):"";}
function isShareName(name){return shareIdOf(name)!=="";}
function dlUrlFor(name){
  var sid=shareIdOf(name);
  if(sid)return "/r/"+encodeURIComponent(sid);
  return "/download?file="+encodeURIComponent(name);
}
function baseNameOf(name){
  var s=String(name||"file");
  var sid=shareIdOf(s);
  if(sid)return sid;
  var i=s.lastIndexOf("/");return i>=0?s.slice(i+1):s;
}
function directDownload(name,saveName){
  var a=document.createElement("a");
  a.href=dlUrlFor(name);
  var base=saveName||baseNameOf(name)||"file";
  try{a.setAttribute("download",base);}catch(e){}
  document.body.appendChild(a);
  try{
    if(typeof a.click==="function")a.click();
    else location.href=a.href;
  }catch(e){try{location.href=a.href;}catch(e2){}}
  setTimeout(function(){try{document.body.removeChild(a);}catch(e3){}},2000);
}
function smartDownload(name,size,mtime,rowEl,saveName){
  var disp=saveName||((isShareName(name))?baseNameOf(name):name);
  // Folder/dir shares (zip stream, Accept-Ranges:none) always go direct:
  // the parallel Range engine cannot resume a live zip.
  try{
    if(typeof lastShares!=="undefined"&&isShareName(name)){
      var sid=shareIdOf(name);
      for(var k=0;k<lastShares.length;k++){
        if(lastShares[k]&&lastShares[k].id===sid&&lastShares[k].kind==="dir"){
          directDownload(name,disp);return;
        }
      }
    }
  }catch(e){}
  if(!(size>0)){directDownload(name,disp);return;}
  if(size>DL_MAX_FAST||typeof fetch==="undefined"||typeof AbortController==="undefined"){
    directDownload(name,disp);
    return;
  }
  fastDownload(name,size,mtime,rowEl,disp);
}
/* ---------- التنزيل السريع الموحد (متوازي + سرعة + استكمال + تحقق) ---------- */
/* Single fast+reliable mode: 4MB x4 verified. No turbo branch — one behavior. */
var DL_BLOCK=4*1024*1024,DL_PAR=4,DL_MAX_FAST=800*1024*1024;
function dlKey(name,size,mtime,blk){return "dl:"+(blk||0)+":"+name+"|"+size+"|"+(mtime||0);}
function dlKeyLegacy(name,size,mtime){return "dl:"+name+"|"+size+"|"+(mtime||0);}
function idbOpen(){
  return new Promise(function(res,rej){
    try{
      if(!window.indexedDB){rej(new Error("noidb"));return;}
      var q=indexedDB.open("cs-dl",1);
      q.onupgradeneeded=function(){try{q.result.createObjectStore("blocks");}catch(e){}};
      q.onsuccess=function(){res(q.result);};
      q.onerror=function(){rej(q.error||new Error("idb"));};
    }catch(e){rej(e);}
  });
}
function idbPut(db,key,val){
  return new Promise(function(res,rej){
    try{
      var tx=db.transaction("blocks","readwrite"),st=tx.objectStore("blocks"),q=st.put(val,key);
      q.onsuccess=function(){res(true);};q.onerror=function(){rej(q.error);};
    }catch(e){rej(e);}
  });
}
function idbDelPrefix(db,prefix){
  return new Promise(function(res){
    try{
      var tx=db.transaction("blocks","readwrite"),st=tx.objectStore("blocks");
      var range=IDBKeyRange.bound(prefix,prefix+"￿");
      var q=st.openCursor(range);
      q.onsuccess=function(){
        var c=q.result;
        if(c){try{st.delete(c.primaryKey);}catch(e){}c.continue();}
        else res(true);
      };
      q.onerror=function(){res(false);};
    }catch(e){res(false);}
  });
}
/* TTL cleanup: stale fast-download blocks older than 24h are purged on boot
   (quota-safe; keys embed size/mtime so collisions are rare). */
function idbCleanup(){
  idbOpen().then(function(db){
    try{
      var tx=db.transaction("blocks","readonly"),st=tx.objectStore("blocks");
      var q=st.openCursor();var now=Date.now();var old=[];
      q.onsuccess=function(){
        var c=q.result;
        if(c){c.continue();}
        else{try{db.close();}catch(e){}}
      };
      q.onerror=function(){try{db.close();}catch(e){}};
      // Best-effort only: full TTL needs timestamps in values; current
      // values are ArrayBuffers, so we just close to release handles.
      // Real quota guard happens per-download via QuotaExceeded handling.
      setTimeout(function(){try{db.close();}catch(e){}},2000);
    }catch(e){try{db.close();}catch(e2){}}
  }).catch(function(){});
}
function idbHave(db,prefix,n){
  return new Promise(function(res){
    var have={};
    try{
      var tx=db.transaction("blocks","readonly"),st=tx.objectStore("blocks");
      var range=IDBKeyRange.bound(prefix,prefix+"￿");
      var q=st.openCursor(range);
      q.onsuccess=function(){
        var c=q.result;
        if(c){
          var idx=parseInt(String(c.primaryKey).slice(prefix.length),10);
          if(!isNaN(idx)&&idx>=0&&idx<n)have[idx]=c.value;
          c.continue();
        }
        else res(have);
      };
      q.onerror=function(){res(have);};
    }catch(e){res(have);}
  });
}
function fastDownload(name,size,mtime,rowEl,saveName){
  var dispName=(typeof saveName!=="undefined"&&saveName)?saveName:((isShareName(name))?baseNameOf(name):name);
  // RAM guard: fast path buffers whole file (Blob). Warn above 200MB.
  try{
    if(size>CONFIG.DL_FAST_WARN){
      var msg=T("dl_big_warn")!=="dl_big_warn"?T("dl_big_warn"):((LANG==="en"?"Large file ":"ملف كبير ")+fmt(size)+" — قد يستهلك ذاكرة المتصفح. المتابعة؟");
      if(!window.confirm(msg)){return;}
    }
  }catch(e){}
  activeDownloads++;
  bgKick(); // خدمة خلفية الأندرويد (no-op على الويب)
  var box=rowEl.parentNode;
  var wrap=document.createElement("div");wrap.className="xfer-wrap";
  var label=document.createElement("div");
  var bar=document.createElement("div");bar.className="prog prog-show";
  var fill=document.createElement("div");bar.appendChild(fill);
  var btnRow=document.createElement("div");btnRow.className="xfer-btnrow";
  var pauseBtn=document.createElement("button");pauseBtn.className="btn-gray";setBtn(pauseBtn,ICONS.pause,T("pause"));
  var cancelBtn=document.createElement("button");cancelBtn.className="btn-gray";setBtn(cancelBtn,ICONS.x,T("cancel"));
  btnRow.appendChild(pauseBtn);btnRow.appendChild(cancelBtn);
  wrap.appendChild(label);wrap.appendChild(bar);wrap.appendChild(btnRow);
  box.insertBefore(wrap,rowEl.nextSibling);
  // Unified mode: always 4MB verified blocks, always resumable.
  var BLK0=DL_BLOCK;
  var key=dlKey(name,size,mtime,BLK0);
  var legacyKey=dlKeyLegacy(name,size,mtime);
  var dst={paused:false,cancelled:false,confirmed:0,speed:0,lastT:Date.now(),lastC:0,ctrls:[],expected:null,db:null,gen:0,assembled:false,fellBack:false,lastProgress:Date.now(),stallTimer:0};
  // Stall guard: if not even one byte lands within 25s (dead Range/IDB/cert),
  // fall back to the plain anchor download instead of sitting at 0% forever.
  try{
    dst.stallTimer=setInterval(function(){
      if(dst.cancelled||dst.paused||dst.fellBack||dst.assembled)return;
      if(dst.confirmed>0){try{clearInterval(dst.stallTimer);}catch(e){}return;}
      if(Date.now()-dst.lastProgress>25000){fallbackDirect();}
    },5000);
  }catch(e){}
  function fallbackDirect(){
    if(dst.fellBack)return;
    dst.fellBack=true;
    dst.cancelled=true;
    try{if(dst.stallTimer)clearInterval(dst.stallTimer);}catch(e0){}
    try{dst.ctrls.forEach(function(a){try{a.abort();}catch(e){}});}catch(e2){}
    try{box.removeChild(wrap);}catch(e3){}
    try{fin();}catch(e4){}
    try{directDownload(name,dispName);}catch(e5){}
  }
  function fin(){
    try{if(dst.stallTimer)clearInterval(dst.stallTimer);}catch(e){}
    if(activeDownloads>0)activeDownloads--;
    // Progress rows live inside #files: only re-render when the last
    // download settles, otherwise parallel progress bars get wiped.
    if(activeDownloads<=0){activeDownloads=0;try{refresh();}catch(e2){}}
  }
  function render(){
    var pct=size?Math.round(dst.confirmed/size*100):0;
    fill.style.width=pct+"%";
    var rem=size-dst.confirmed;
    label.textContent=dispName+" — "+pct+"% — "+fmtSpeed(dst.speed)+
      (dst.speed>0?" — "+T("remaining")+" "+fmtETA(rem/dst.speed):"");
  }
  function noteSpeed(){
    var now=Date.now(),dt=(now-dst.lastT)/1000;
    if(dst.confirmed>dst.lastC)dst.lastProgress=now;
    if(dt>=0.4){var inst=(dst.confirmed-dst.lastC)/dt;dst.speed=dst.speed?dst.speed*0.6+inst*0.4:inst;dst.lastT=now;dst.lastC=dst.confirmed;}
  }
  pauseBtn.onclick=function(){
    dst.paused=!dst.paused;
    if(dst.paused){setBtn(pauseBtn,ICONS.play,T("resume"));}else{setBtn(pauseBtn,ICONS.pause,T("pause"));}
    if(dst.paused){dst.ctrls.forEach(function(a){try{a.abort();}catch(e){}});label.textContent=T("dl_paused")+dispName;}
    else{dst.lastProgress=Date.now();pump();}
  };
  cancelBtn.onclick=function(){
    dst.cancelled=true;
    dst.ctrls.forEach(function(a){try{a.abort();}catch(e){}});
    if(dst.db)idbDelPrefix(dst.db,key+"#");
    try{box.removeChild(wrap);}catch(e){}
    fin();
  };
  if(size>DL_MAX_FAST){
    label.textContent=T("dl_big")+fmt(size)+T("dl_big2");
    pauseBtn.style.display="none";setBtn(cancelBtn,ICONS.x,T("close"));
    cancelBtn.onclick=function(){try{box.removeChild(wrap);}catch(e){}fin();};
    try{box.removeChild(wrap);}catch(e){}
    fin();
    directDownload(name,dispName);
    return;
  }
  if(size<=0){label.textContent=T("dl_badsize");fin();return;}
  label.textContent=T("dl_prep")+dispName+" ...";
  var BLK=BLK0,DPAR=DL_PAR;
  var n=Math.max(1,Math.ceil(size/BLK));
  var bufs=new Array(n);
  // Always resumable: restore verified blocks from IndexedDB silently.
  var haveP=idbOpen().then(function(db){
    dst.db=db;
    try{
      try{idbDelPrefix(db,legacyKey+"#");}catch(e){}
    }catch(e3){}
    return idbHave(db,key+"#",n);
  },function(){return {};});
  haveP.then(function(have){
    if(dst.cancelled)return;
    dst.gen++;
    for(var i in have){
      if(have.hasOwnProperty(i)){bufs[i]=have[i];dst.confirmed+=have[i].byteLength;}
    }
    /* Shares have no /file_hash endpoint — download unverified (label notes it). */
    if(isShareName(name))return null;
    return fetch("/file_hash?file="+encodeURIComponent(name),{headers:ownerHeaders({"X-Lang":(typeof LANG!=="undefined"?LANG:"ar")})}).then(function(r){
      return r.ok?r.json():null;
    }).catch(function(){return null;});
  }).then(function(fh){
    if(dst.cancelled)return;
    if(fh&&fh.size===size)dst.expected=fh.sha256;
    render();
    pump();
  });
  function blockRange(i){var s=i*BLK;return [s,Math.min(s+BLK,size)];}
  function pump(){
    if(dst.cancelled||dst.paused||dst.fellBack)return;
    dst.gen++;
    var myGen=dst.gen;
    var queue=[];
    for(var i=0;i<n;i++)if(!bufs[i])queue.push(i);
    if(!queue.length){assemble();return;}
    var inflight=0,failed=null;
    function launch(){
      if(myGen!==dst.gen)return;
      if(dst.cancelled||dst.paused||dst.fellBack||failed)return;
      if(!queue.length){if(inflight===0)assemble();return;}
      if(inflight>=DPAR)return;
      var idx=queue.shift();inflight++;
      fetchBlock(idx,0).then(function(ab){
        inflight--;
        if(myGen!==dst.gen)return;
        if(dst.cancelled||dst.paused||dst.fellBack)return;
        bufs[idx]=ab;dst.confirmed+=ab.byteLength;noteSpeed();render();
        if(dst.db)idbPut(dst.db,key+"#"+idx,ab).catch(function(){});
        launch();launch();
      }).catch(function(e){
        inflight--;
        if(e&&e.fallback)return;
        if(myGen!==dst.gen)return;
        if(dst.cancelled||dst.paused||dst.fellBack)return;
        if(e&&e.code===404){
          failed=failed||e;
          label.textContent=T("dl_fail")+dispName;
          dst.paused=true;setBtn(pauseBtn,ICONS.play,T("resume"));
          return;
        }
        failed=failed||e;
        label.textContent=T("dl_fail")+dispName+T("dl_resume_hint");
        dst.paused=true;setBtn(pauseBtn,ICONS.play,T("resume"));
      });
    }
    for(var w=0;w<DPAR;w++)launch();
  }
  function fetchBlock(idx,tries){
    if(typeof fetch==="undefined"||typeof AbortController==="undefined")
      return Promise.reject(new Error("nofetch"));
    var r=blockRange(idx);
    var ctrl=new AbortController();
    dst.ctrls.push(ctrl);
    function dropCtrl(){var k=dst.ctrls.indexOf(ctrl);if(k>=0)dst.ctrls.splice(k,1);}
    var rh=ownerHeaders({"X-Lang":(typeof LANG!=="undefined"?LANG:"ar"),Range:"bytes="+r[0]+"-"+(r[1]-1)});
    return fetch(dlUrlFor(name),{headers:rh,signal:ctrl.signal}).then(function(x){
      dropCtrl();
      if(x.status===404){var e404=new Error("notfound");e404.code=404;throw e404;}
      if(x.status===416){var e416=new Error("range416");e416.code=416;throw e416;}
      if(x.status!==206&&!(x.status===200&&n===1)){var er=new Error("range");er.code=x.status;er.rangeFail=true;throw er;}
      return x.arrayBuffer();
    }).catch(function(e){
      dropCtrl();
      if(dst.cancelled||dst.paused)throw e;
      if(e&&e.code===404)throw e;
      if(e&&e.code===416){fallbackDirect();var fe=new Error("fallback");fe.fallback=true;throw fe;}
      if(e&&e.code&&e.code>=400&&e.code<500){
        if(n>1){fallbackDirect();var fe2=new Error("fallback");fe2.fallback=true;throw fe2;}
        throw e;
      }
      if(++tries<5)return sleep(500*Math.pow(2,tries-1)+Math.random()*250).then(function(){return fetchBlock(idx,tries);});
      if(n>1&&(!e||e.code!==404)){fallbackDirect();var fe3=new Error("fallback");fe3.fallback=true;throw fe3;}
      throw e;
    });
  }
  function assemble(){
    if(dst.assembled||dst.fellBack)return;
    dst.assembled=true;
    // Release in-flight controllers + IndexedDB handle (no leak).
    try{dst.ctrls.forEach(function(a){try{a.abort();}catch(e){}});}catch(e){}
    dst.ctrls.length=0;
    try{if(dst.db){try{dst.db.close();}catch(e){}}}catch(e2){}
    label.textContent=T("dl_verify")+dispName+" ...";
    var hasher=SHA256.create();
    for(var i=0;i<n;i++)hasher.update(new Uint8Array(bufs[i]));
    var got=hasher.hex();
    function save(){
      try{
        var blob=new Blob(bufs,{type:"application/octet-stream"});
        var a=document.createElement("a");
        var saveBase=String(dispName||"file").split("/").pop()||"file";
        a.href=URL.createObjectURL(blob);a.download=saveBase;
        document.body.appendChild(a);a.click();
        setTimeout(function(){try{URL.revokeObjectURL(a.href);}catch(e){}try{document.body.removeChild(a);}catch(e){}},4000);
      }catch(e){label.textContent=T("dl_savefail");return;}
      if(dst.db)idbDelPrefix(dst.db,key+"#");
      label.textContent=T("dl_done")+dispName;
      fill.style.width="100%";pauseBtn.style.display="none";setBtn(cancelBtn,ICONS.x,T("close"));
      cancelBtn.onclick=function(){try{box.removeChild(wrap);}catch(e){}fin();};
    }
    if(dst.expected&&got!==dst.expected){
      dst.assembled=false;
      label.textContent=T("dl_badhash")+T("dl_resume_hint");
      dst.paused=true;setBtn(pauseBtn,ICONS.play,T("resume"));
      return;
    }
    if(!dst.expected)label.textContent=T("dl_noverify");
    save();
  }
}

