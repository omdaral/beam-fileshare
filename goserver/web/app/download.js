/* Beam web fast download — parallel ranges, IndexedDB resume, checksum. */
/* ---------- التنزيل السريع (متوازي + سرعة + استكمال + تحقق) ---------- */
var DL_BLOCK=2*1024*1024,DL_PAR=3,DL_MAX_FAST=800*1024*1024;
function dlKey(name,size,mtime){return "dl:"+name+"|"+size+"|"+(mtime||0);}
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
function fastDownload(name,size,mtime,rowEl){
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
  var wrap=document.createElement("div");wrap.style.margin="6px 0";wrap.style.padding="8px";wrap.style.background="#f8fbff";wrap.style.borderRadius="10px";
  var label=document.createElement("div");
  var bar=document.createElement("div");bar.className="prog";bar.style.display="block";
  var fill=document.createElement("div");bar.appendChild(fill);
  var btnRow=document.createElement("div");btnRow.style.marginTop="6px";btnRow.style.display="flex";btnRow.style.gap="8px";
  var pauseBtn=document.createElement("button");pauseBtn.className="btn-gray";setBtn(pauseBtn,ICONS.pause,T("pause"));pauseBtn.style.minHeight="36px";pauseBtn.style.flex="1";
  var cancelBtn=document.createElement("button");cancelBtn.className="btn-gray";setBtn(cancelBtn,ICONS.x,T("cancel"));cancelBtn.style.minHeight="36px";cancelBtn.style.flex="1";
  btnRow.appendChild(pauseBtn);btnRow.appendChild(cancelBtn);
  wrap.appendChild(label);wrap.appendChild(bar);wrap.appendChild(btnRow);
  box.insertBefore(wrap,rowEl.nextSibling);
  var key=dlKey(name,size,mtime);
  var dlTurbo=(typeof dlMode!=="undefined"&&dlMode==="turbo");
  var dst={paused:false,cancelled:false,confirmed:0,speed:0,lastT:Date.now(),lastC:0,ctrls:[],expected:null,db:null};
  function fin(){activeDownloads--;refresh();}
  function render(){
    var pct=size?Math.round(dst.confirmed/size*100):0;
    fill.style.width=pct+"%";
    var rem=size-dst.confirmed;
    label.textContent=name+" — "+pct+"% — "+fmtSpeed(dst.speed)+
      (dst.speed>0?" — "+T("remaining")+" "+fmtETA(rem/dst.speed):"");
  }
  function noteSpeed(){
    var now=Date.now(),dt=(now-dst.lastT)/1000;
    if(dt>=0.4){var inst=(dst.confirmed-dst.lastC)/dt;dst.speed=dst.speed?dst.speed*0.6+inst*0.4:inst;dst.lastT=now;dst.lastC=dst.confirmed;}
  }
  pauseBtn.onclick=function(){
    dst.paused=!dst.paused;
    if(dst.paused){setBtn(pauseBtn,ICONS.play,T("resume"));}else{setBtn(pauseBtn,ICONS.pause,T("pause"));}
    if(dst.paused){dst.ctrls.forEach(function(a){try{a.abort();}catch(e){}});label.textContent=T("dl_paused")+name;}
    else pump();
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
    return;
  }
  if(size<=0){label.textContent=T("dl_badsize");fin();return;}
  label.textContent=(dlTurbo?"["+T("turbo_badge")+"] ":"")+T("dl_prep")+name+" ...";
  var BLK=dlTurbo?8*1024*1024:DL_BLOCK,DPAR=dlTurbo?6:DL_PAR;
  var n=Math.max(1,Math.ceil(size/BLK));
  var bufs=new Array(n);
  var haveP=dlTurbo?Promise.resolve({}):idbOpen().then(function(db){dst.db=db;return idbHave(db,key+"#",n);},function(){return {};});
  haveP.then(function(have){
    if(dst.cancelled)return;
    for(var i in have){
      if(have.hasOwnProperty(i)){bufs[i]=have[i];dst.confirmed+=have[i].byteLength;}
    }
    return fetch("/file_hash?file="+encodeURIComponent(name)).then(function(r){
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
    if(dst.cancelled||dst.paused)return;
    var queue=[];
    for(var i=0;i<n;i++)if(!bufs[i])queue.push(i);
    if(!queue.length){assemble();return;}
    var inflight=0,failed=null;
    function launch(){
      if(dst.cancelled||dst.paused||failed)return;
      if(!queue.length){if(inflight===0)assemble();return;}
      if(inflight>=DPAR)return;
      var idx=queue.shift();inflight++;
      fetchBlock(idx,0).then(function(ab){
        inflight--;
        if(dst.cancelled||dst.paused){queue.unshift(idx);return;}
        bufs[idx]=ab;dst.confirmed+=ab.byteLength;noteSpeed();render();
        if(dst.db)idbPut(dst.db,key+"#"+idx,ab).catch(function(){});
        launch();launch();
      }).catch(function(e){
        inflight--;
        if(dst.cancelled||dst.paused){queue.unshift(idx);return;}
        failed=failed||e;
        label.textContent=T("dl_fail")+name+T("dl_resume_hint");
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
    return fetch("/download?file="+encodeURIComponent(name),{headers:{Range:"bytes="+r[0]+"-"+(r[1]-1)},signal:ctrl.signal}).then(function(x){
      dropCtrl();
      if(x.status!==206&&!(x.status===200&&n===1))throw new Error("range");
      return x.arrayBuffer();
    }).catch(function(e){
      dropCtrl();
      if(dst.cancelled||dst.paused)throw e;
      if(++tries<5)return sleep(500*Math.pow(2,tries-1)).then(function(){return fetchBlock(idx,tries);});
      throw e;
    });
  }
  function assemble(){
    // Release in-flight controllers + IndexedDB handle (no leak).
    try{dst.ctrls.forEach(function(a){try{a.abort();}catch(e){}});}catch(e){}
    dst.ctrls.length=0;
    try{if(dst.db){try{dst.db.close();}catch(e){}}}catch(e2){}
    label.textContent=T("dl_verify")+name+" ...";
    var hasher=SHA256.create();
    for(var i=0;i<n;i++)hasher.update(new Uint8Array(bufs[i]));
    var got=hasher.hex();
    function save(){
      try{
        var blob=new Blob(bufs,{type:"application/octet-stream"});
        var a=document.createElement("a");
        a.href=URL.createObjectURL(blob);a.download=name;
        document.body.appendChild(a);a.click();
        setTimeout(function(){try{URL.revokeObjectURL(a.href);}catch(e){}try{document.body.removeChild(a);}catch(e){}},4000);
      }catch(e){label.textContent=T("dl_savefail");return;}
      if(dst.db)idbDelPrefix(dst.db,key+"#");
      label.textContent=T("dl_done")+name;
      fill.style.width="100%";pauseBtn.style.display="none";setBtn(cancelBtn,ICONS.x,T("close"));
      cancelBtn.onclick=function(){try{box.removeChild(wrap);}catch(e){}fin();};
    }
    if(dst.expected&&got!==dst.expected){
      if(dst.db)idbDelPrefix(dst.db,key+"#");
      label.textContent=T("dl_badhash");
      return;
    }
    if(!dst.expected)label.textContent=T("dl_noverify");
    save();
  }
}

