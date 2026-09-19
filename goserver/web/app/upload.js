/* Beam web upload — v2 chunked engine, unified queue, registry shares, sessions panel. */
/* ---------- محرك الرفع v2 (قطع متوازية + بصمات + سرعة + استكمال بقوة التورنت) ---------- */
var activeUploads=0,activeDownloads=0;
/* Single fast+reliable mode: 4MB verified pieces x4 lanes. No user choice —
   speed from parallelism, safety from always-on checksums. TURBO_* kept
   for backward compat with old sessions only. */
var PARALLEL=4,PIECE_LEN=4*1024*1024,TURBO_PIECE=8*1024*1024,TURBO_LANES=6;
var upMode="reliable",dlMode="reliable";
function statusOf(uuid){
  return fetch("/upload_status?id="+uuid,{headers:ownerHeaders({"X-Lang":(typeof LANG!=="undefined"?LANG:"ar")})}).then(function(r){
    if(!r.ok)throw new Error("status");
    return r.json();
  }).then(function(j){return j.offset||0;});
}
function uploadFiles(list){
  var arr=Array.prototype.slice.call(list);
  /* Registry model: browser files publish as guest shares (announce, then
     chunked upload with rel "share:"+id). Owner browser files are guest-like. */
  if(typeof sharePublishFiles==="function"){sharePublishFiles(arr);return;}
  var opts={turbo:false};
  if(arr.length>1){
    enqueueBatch(arr.map(function(f){return {f:f,rel:null};}),null,opts);
    return;
  }
  arr.forEach(function(f){enqueueUpload(f,null,null,null,opts);});
}
/* بصمة ملف كامل بقراءة متقطعة (للتحقق النهائي في التيربو — بلا ذاكرة ضخمة) */
function hashFile(f){
  var h=SHA256.create(),CH=4*1024*1024,off=0;
  function step(){
    if(off>=f.size)return Promise.resolve(h.hex());
    var end=Math.min(off+CH,f.size);
    var sl=f.slice(off,end);
    var p=sl.arrayBuffer?sl.arrayBuffer():Promise.reject(new Error("no slice"));
    return p.then(function(ab){h.update(new Uint8Array(ab));off=end;return step();});
  }
  return step();
}
function upKey(f,rel,plen){return "up2:"+(rel||f.name)+"|"+f.size+"|"+(f.lastModified||0)+"|"+(plen||0);}
function upKeyLegacy(f,rel){return "up:"+(rel||f.name)+"|"+f.size+"|"+(f.lastModified||0);}
function storedUp(f,rel){
  try{
    var prefix="up2:"+(rel||f.name)+"|"+f.size+"|"+(f.lastModified||0)+"|";
    try{
      for(var i=0;i<localStorage.length;i++){
        var k=localStorage.key(i);
        if(k&&k.indexOf(prefix)===0){
          try{
            var o=JSON.parse(localStorage.getItem(k));
            if(o&&typeof o.id==="string"&&typeof o.plen==="number")return o;
          }catch(e){}
        }
      }
    }catch(e2){}
    var leg=null;
    try{leg=JSON.parse(localStorage.getItem(upKeyLegacy(f,rel)));}catch(e3){}
    if(leg&&typeof leg.id==="string"&&typeof leg.plen==="number")return leg;
    return null;
  }catch(e){return null;}
}
function storeUp(f,rel,o){try{localStorage.setItem(upKey(f,rel,o&&o.plen),JSON.stringify(o));}catch(e){}}
function clearUp(f,rel){
  try{
    try{localStorage.removeItem(upKeyLegacy(f,rel));}catch(e){}
    var prefix="up2:"+(rel||f.name)+"|"+f.size+"|"+(f.lastModified||0)+"|";
    var rm=[];
    try{
      for(var i=0;i<localStorage.length;i++){var k=localStorage.key(i);if(k&&k.indexOf(prefix)===0)rm.push(k);}
    }catch(e2){}
    for(var j=0;j<rm.length;j++){try{localStorage.removeItem(rm[j]);}catch(e3){}}
  }catch(e){}
}
function pieceCount(size,plen){return Math.max(1,Math.ceil(size/plen));}
function pieceRange(i,plen,size){var s=i*plen;return [s,Math.min(s+plen,size)];}
function pieceLenOf(i,plen,size){var r=pieceRange(i,plen,size);return r[1]-r[0];}
function uploadOne(file,adopt,relPath,onDone,batch,opts,key){
  activeUploads++;
  var rel=relPath||file.name;
  /* Share uploads use rel "share:"+id (server link); show the real name. */
  var disp=((opts&&opts.label)||rel);
  var bkey=(typeof key!=="undefined"&&key!==null)?key:rel;
  var outcome="done";
  var turbo=!!(opts&&opts.turbo),extract=!!(opts&&opts.extract);
  var box=$("upList");
  var wrap=document.createElement("div");wrap.className="up-row";
  var label=document.createElement("div");
  var bar=document.createElement("div");bar.className="prog prog-show";
  var fill=document.createElement("div");bar.appendChild(fill);wrap.appendChild(label);wrap.appendChild(bar);
  var btnRow=document.createElement("div");btnRow.className="xfer-btnrow";
  var pauseBtn=document.createElement("button");pauseBtn.className="btn-gray";setBtn(pauseBtn,ICONS.pause,T("pause"));
  var cancelBtn=document.createElement("button");cancelBtn.className="btn-gray";setBtn(cancelBtn,ICONS.x,T("cancel"));
  btnRow.appendChild(pauseBtn);btnRow.appendChild(cancelBtn);wrap.appendChild(btnRow);
  if(batch&&batch.detailsEl)batch.detailsEl.appendChild(wrap);
  else box.insertBefore(wrap,box.firstChild);
  var st={paused:false,cancelled:false,confirmed:0,t0:Date.now(),speed:0,lastT:Date.now(),lastC:0,
    plen:(turbo?TURBO_PIECE:PIECE_LEN),lanes:(turbo?TURBO_LANES:PARALLEL),turbo:turbo,uuid:null,awaiting:null,aborts:[],gen:0,abortEarly:false};
  function done(){
    if(st.finished)return;
    st.finished=true;
    activeUploads--;
    try{if(typeof rel==="string"&&rel.indexOf("share:")===0&&typeof shareDone==="function")shareDone(rel);}catch(eSH){}
    if(onDone){try{onDone();}catch(e){}}
    var failedNow=(outcome!=="done");
    if(batch){
      try{delete batch.running[bkey];}catch(e2){}
      if(!failedNow){batch.done++;batch.prog[bkey]=file.size;}
      else if(outcome==="cancelled"){batch.cancelledN++;}
      else{batch.failed++;}
      updateBatchRow(batch);
    }
    pauseBtn.style.display="none";
    var rb=recentBox();
    try{if(wrap.parentNode)wrap.parentNode.removeChild(wrap);}catch(e3){}
    try{rb.appendChild(wrap);}catch(e4){}
    if(outcome==="failed"){
      wrap.dataset.pin="1";
      setBtn(cancelBtn,ICONS.x,T("close"));cancelBtn.style.display="";
      cancelBtn.onclick=function(){try{wrap.parentNode.removeChild(wrap);}catch(e5){}};
    }else{
      cancelBtn.style.display="none";
    }
    pruneRecent();
    if(batch)maybeFinishBatch(batch);
    else{refresh();refreshSessions();}
  }
  function fail(t){outcome="failed";label.textContent=t;}
  function render(){
    var pct=file.size?Math.round(st.confirmed/file.size*100):0;
    fill.style.width=pct+"%";
    var rem=file.size-st.confirmed;
    label.textContent=disp+" ("+fmt(file.size)+") — "+pct+"% — "+fmtSpeed(st.speed)+
      (st.speed>0?" — "+T("remaining")+" "+fmtETA(rem/st.speed):"");
    if(batch)batchTick(batch,bkey,st.confirmed);
  }
  function noteSpeed(){
    var now=Date.now(),dt=(now-st.lastT)/1000;
    if(dt>=0.4){var inst=(st.confirmed-st.lastC)/dt;st.speed=st.speed?st.speed*0.6+inst*0.4:inst;st.lastT=now;st.lastC=st.confirmed;}
  }
  if(!file.size){fail(T("up_empty")+disp);done();return;}
  pauseBtn.onclick=function(){
    if(st.awaiting){resumePump();return;}
    st.paused=!st.paused;
    if(st.paused){setBtn(pauseBtn,ICONS.play,T("resume"));}else{setBtn(pauseBtn,ICONS.pause,T("pause"));}
    if(st.paused)label.textContent=T("up_paused")+disp+" ("+Math.round(st.confirmed/file.size*100)+"%)";
    else{st.lastT=Date.now();st.lastC=st.confirmed;render();}
  };
  cancelBtn.onclick=function(){outcome="cancelled";st.cancelled=true;st.abortEarly=true;st.gen++;st.aborts.forEach(function(a){try{a();}catch(e){}});try{clearUp(file,rel);}catch(e2){}label.textContent=T("up_cancelled")+disp;};
  if(batch){batch.running[bkey]=function(){try{cancelBtn.onclick();}catch(e6){}}}
  function regAbort(fn){st.aborts.push(fn);}
  function beginInit(){
    if(st.cancelled||st.abortEarly){done();return Promise.resolve(null);}
    var myGen=st.gen;
    label.textContent=T("up_starting")+disp+" ...";
    var ibody={uuid:st.uuid,name:rel,size:file.size,piece_len:st.plen};
    if(st.turbo)ibody.noverify=true;
    if(extract)ibody.extract=true;
    return postJSON("/upload_init",ibody).then(function(res){
      if(myGen!==st.gen)return null;
      if(st.cancelled||st.abortEarly){done();return null;}
      if(res.status===413){fail(T("up_toobig"));clearUp(file,rel);done();return null;}
      if(res.status!==200||!res.body){fail(T("up_initfail"));done();return null;}
      storeUp(file,rel,{id:st.uuid,plen:st.plen});
      return startUpload(st.uuid,res.body.missing||[],st.plen);
    });
  }
  function isShareRel(){return (typeof rel==="string"&&rel.indexOf("share:")===0);}
  function failShareAdopt(msg){
    // Never orphan a share with a fresh uuid: the share entry stays
    // incomplete (unavailable) and the bytes would land as a legacy file
    // invisible in /api/shares. Fail visibly so counters don't leak.
    fail(msg||(T("up_adoptfail")+disp+". "+T("retry_generic")));
    done();return null;
  }
  function adoptSession(s){
    st.uuid=s.id;
    var myGen=st.gen;
    return fetch("/upload_status?id="+s.id,{headers:ownerHeaders({"X-Lang":(typeof LANG!=="undefined"?LANG:"ar")})}).then(function(r){
      if(!r.ok)throw new Error("nostatus");
      return r.json();
    }).then(function(j){
      if(myGen!==st.gen)return null;
      if(st.cancelled||st.abortEarly){done();return null;}
      /* Share sessions are keyed by the backend: accept its name/piece framing. */
      var isShareAd=isShareRel();
      if(j.size!==file.size||(!isShareAd&&j.name!==rel)){
        if(isShareAd)return failShareAdopt();
        return startFresh();
      }
      // Legacy V1 guest sessions are auto-migrated server-side on status;
      // if the server still returns no piece framing, don't fake-complete
      // with an empty queue — fail visibly instead.
      if(isShareAd&&(typeof j.missing==="undefined"||typeof j.piece_len==="undefined")){
        if(typeof j.offset!=="undefined"&&j.offset>0){
          // Best effort: re-fetch once after migration settles.
          return fetch("/upload_status?id="+s.id,{headers:ownerHeaders({"X-Lang":(typeof LANG!=="undefined"?LANG:"ar")})}).then(function(r2){
            if(!r2.ok)throw new Error("nostatus");
            return r2.json();
          }).then(function(j2){
            if(j2.size!==file.size||typeof j2.missing==="undefined")return failShareAdopt();
            st.plen=j2.piece_len||PIECE_LEN;
            st.turbo=!!j2.noverify;st.lanes=(st.turbo?TURBO_LANES:PARALLEL);
            storeUp(file,rel,{id:s.id,plen:st.plen});
            return startUpload(s.id,j2.missing||[],st.plen);
          }).catch(function(){return failShareAdopt();});
        }
        return failShareAdopt();
      }
      var freshPlen=j.piece_len||PIECE_LEN;
      var freshTurbo=!!j.noverify;
      if(!isShareAd&&(freshPlen!==(s.piece_len||st.plen)||freshTurbo!==st.turbo))return startFresh();
      st.plen=freshPlen;
      if(isShareAd){st.turbo=freshTurbo;st.lanes=(st.turbo?TURBO_LANES:PARALLEL);}
      storeUp(file,rel,{id:s.id,plen:st.plen});
      var miss=(j.missing instanceof Array)?j.missing:[];
      // Guard against empty-queue fake completion: an incomplete share
      // must always have missing pieces.
      if(isShareAd&&!miss.length&&st.confirmed<=0){
        // Server says nothing missing but bytes aren't there — re-derive
        // the full queue from size/plen instead of instantly succeeding.
        var n=Math.max(1,Math.ceil(file.size/st.plen));
        miss=[];for(var mi0=0;mi0<n;mi0++)miss.push(mi0);
      }
      return startUpload(s.id,miss,st.plen);
    }).catch(function(e){
      if(myGen!==st.gen)return null;
      if(isShareRel())return failShareAdopt();
      return startFresh();
    });
  }
  function startUpload(uuid,missing,plen){
    var queue=missing.slice(),inflight=0,failed=null;
    var myGen=st.gen;
    st.aborts=[];
    var missBytes=0,mi;
    for(mi=0;mi<queue.length;mi++)missBytes+=pieceLenOf(queue[mi],plen,file.size);
    st.confirmed=file.size-missBytes;
    if(st.confirmed<0)st.confirmed=0;
    st.lastT=Date.now();st.lastC=st.confirmed;render();
    function launch(){
      if(st.cancelled)return Promise.reject(new Error("cancelled"));
      if(myGen!==st.gen)return Promise.reject(new Error("cancelled"));
      if(failed)return Promise.reject(failed);
      if(!queue.length&&inflight===0)return Promise.resolve(true);
      if(st.paused||!queue.length||inflight>=st.lanes)return sleep(250).then(launch);
      var idx=queue.shift();inflight++;
      return sendPiece(uuid,idx,plen).then(function(){
        inflight--;
        if(myGen!==st.gen)return launch();
        if(st.cancelled||st.paused){queue.push(idx);return launch();}
        st.confirmed+=pieceLenOf(idx,plen,file.size);
        if(st.confirmed>file.size)st.confirmed=file.size;
        noteSpeed();render();
        return launch();
      },function(e){inflight--;failed=failed||e;return Promise.reject(e);});
    }
    var workers=[];
    for(var w=0;w<st.lanes;w++)workers.push(launch());
    return Promise.all(workers).then(function(){
      if(myGen!==st.gen)return null;
      if(st.cancelled||st.abortEarly){done();return null;}
      label.textContent=T("up_verify")+disp+" ...";
      var cbody={id:uuid,name:rel,size:file.size};
      if(extract)cbody.extract=true;
      if(!st.turbo)return postJSON("/upload_complete",cbody);
      if(st.cancelled||st.abortEarly){done();return null;}
      return hashFile(file).then(function(fh){
        if(myGen!==st.gen)return null;
        if(st.cancelled||st.abortEarly){done();return null;}
        cbody.full_hash=fh;
        return postJSON("/upload_complete",cbody);
      });
    }).then(function(res2){
      if(!res2)return;
      if(res2.status===200){
        var secs=Math.max((Date.now()-st.t0)/1000,0.1);
        label.textContent=T("up_done")+disp+" ("+fmtSpeed(file.size/secs)+")";
        fill.style.width="100%";clearUp(file,rel);pauseBtn.style.display="none";cancelBtn.style.display="none";done();
      }
      else if(res2.status===400&&res2.body&&res2.body.missing){
        label.textContent=res2.body.missing.length+" "+T("up_missing_left")+" ...";
        return startUpload(uuid,res2.body.missing,plen);
      }
      else{
        fail(T("up_assemble_fail")+((res2.body&&res2.body.error)||T("retry_generic")));
        // Always release counters: previously 400-without-missing never
        // called done(), leaking activeUploads/sharePending and freezing
        // the download list forever.
        if(!st.finished)done();
      }
    }).catch(function(){
      if(st.cancelled){done();return;}
      st.awaiting={uuid:uuid,plen:plen};
      st.paused=true;setBtn(pauseBtn,ICONS.play,T("resume"));
      fail(T("up_conn")+disp+T("up_conn2"));
    });
  }
  function resumePump(){
    var r=st.awaiting;st.awaiting=null;st.paused=false;
    setBtn(pauseBtn,ICONS.pause,T("pause"));
    st.lastT=Date.now();st.lastC=st.confirmed;
    if(!r){fail(T("up_noresume"));return;}
    var myGen=st.gen;
    fetch("/upload_status?id="+r.uuid,{headers:ownerHeaders({"X-Lang":(typeof LANG!=="undefined"?LANG:"ar")})}).then(function(x){
      if(!x.ok)throw new Error("nostatus");
      return x.json();
    }).then(function(j){
      if(myGen!==st.gen)return null;
      if(st.cancelled||st.abortEarly){done();return null;}
      var isShareRe=isShareRel();
      if(j.size!==file.size||(!isShareRe&&j.name!==rel)){
        if(isShareRe)return failShareAdopt();
        return startFresh();
      }
      if(isShareRe&&(typeof j.missing==="undefined"||typeof j.piece_len==="undefined"))return failShareAdopt();
      var freshPlen=j.piece_len||PIECE_LEN;
      var freshTurbo=!!j.noverify;
      if(!isShareRe&&(freshPlen!==r.plen||freshTurbo!==st.turbo))return startFresh();
      st.plen=freshPlen;
      if(isShareRe){st.turbo=freshTurbo;st.lanes=(st.turbo?TURBO_LANES:PARALLEL);}
      return startUpload(r.uuid,j.missing||[],freshPlen);
    }).catch(function(e){
      if(myGen!==st.gen)return null;
      if(isShareRel())return failShareAdopt();
      return startFresh();
    });
  }
  function sendPiece(uuid,idx,plen){
    var r=pieceRange(idx,plen,file.size),tries=0;
    function attempt(){
      if(st.cancelled||st.abortEarly)return Promise.reject(new Error("cancelled"));
      var hashP=st.turbo?Promise.resolve(""):file.slice(r[0],r[1]).arrayBuffer().then(function(ab){
        return shaU8(new Uint8Array(ab));
      });
      return hashP.then(function(h){
        return xhrBin("/upload_piece?id="+uuid+"&index="+idx+(h?"&hash="+h:""),file.slice(r[0],r[1]),regAbort);
      }).then(function(res){
        if(res.status===-1)return Promise.reject(new Error("cancelled"));
        if(st.cancelled||st.abortEarly)return Promise.reject(new Error("cancelled"));
        if(res.status===200)return true;
        if(res.status===422&&res.body){
          // 422 = piece not accepted — never fake-confirm; fall through to retry.
        }
        else if(res.status>=400&&res.status<500&&res.status!==409&&res.status!==416){
          throw new Error("piece");
        }
        if(++tries<5){
          label.textContent=disp+" — "+T("up_retry")+" "+(idx+1)+" ("+tries+"/5) ...";
          return sleep(500*Math.pow(2,tries-1)+Math.random()*250).then(attempt);
        }
        throw new Error("piece");
      });
    }
    return attempt();
  }
  function startFresh(){
    // Share uploads must never fork a fresh uuid: it orphans the announced
    // share (stays unavailable) and drops bytes as an invisible legacy file.
    if(isShareRel())return failShareAdopt();
    st.uuid=genUuid();
    return beginInit().catch(function(){
      if(st.cancelled){done();return null;}
      fail(T("up_adoptfail")+disp+". "+T("retry_generic"));
      done();return null;
    });
  }
  if(adopt&&adopt.id){
    adoptSession(adopt);
    return;
  }
  var su=storedUp(file,rel);
  if(su&&su.id){
    fetch("/upload_status?id="+su.id,{headers:ownerHeaders({"X-Lang":(typeof LANG!=="undefined"?LANG:"ar")})}).then(function(r){
      if(!r.ok)throw new Error("nostatus");
      return r.json();
    }).then(function(j){
      if(st.cancelled||st.abortEarly){done();return null;}
      if(j.size===file.size&&j.name===rel&&(j.missing||[]).length){
        st.uuid=su.id;st.plen=j.piece_len||PIECE_LEN;
        return startUpload(su.id,j.missing,j.piece_len||PIECE_LEN);
      }
      return startFresh();
    }).catch(startFresh);
    return;
  }
  postJSON("/upload_find",{name:rel,size:file.size}).then(function(res){
    if(res.status===200&&res.body){
      if(res.body.completed){label.textContent=T("up_exists")+res.body.completed;fill.style.width="100%";pauseBtn.style.display="none";cancelBtn.style.display="none";done();return null;}
      var ss=(res.body.sessions||[]).filter(function(s){return s.v===2;});
      if(ss.length)return adoptSession(ss[0]);
    }
    return startFresh();
  }).catch(startFresh);
}

/* ---------- زر الاستكمال الواحد: يظهر فقط عند وجود نقل متوقف ---------- */
function refreshSessions(){
  var box=$("sessions");
  if(!box)return;
  // Sessions panel has no in-place progress UI, so it is safe to refresh
  // during uploads. Only downloads block (bandwidth + CPU on mobile).
  if(activeDownloads>0)return;
  if(document.querySelector('[data-armed="1"]'))return;
  fetch("/upload_sessions",{headers:ownerHeaders({"X-Lang":(typeof LANG!=="undefined"?LANG:"ar")})}).then(function(r){
    if(!r.ok)throw new Error("http"+r.status);
    return r.json();
  }).then(function(j){
    var list=j.sessions||[];
    box.innerHTML="";
    // Hide the whole card when nothing is paused — one less thing to read.
    var card=box.closest?box.closest(".card"):null;
    if(!list.length){
      if(card)card.style.display="none";
      return;
    }
    if(card)card.style.display="";
    list.forEach(function(s){
      var d=document.createElement("div");d.className="file";
      var nm=document.createElement("span");nm.className="name";nm.textContent=s.name;
      try{nm.title=s.name+" ("+fmt(s.size)+") — "+T("sess_pick");}catch(e){}
      var pct=s.size?Math.round(s.received/s.size*100):0;
      var sz=document.createElement("span");sz.className="size";sz.textContent=fmt(s.received)+" / "+fmt(s.size)+" ("+pct+"%)";
      var go=document.createElement("button");go.className="btn-blue";setBtn(go,ICONS.dl,T("sess_resume")+" "+pct+"%");
      go.onclick=function(){
        window._adoptTarget={id:s.id,name:s.name};
        try{document.getElementById("picker").click();}
        catch(e){show($("sessMsg"),T("sess_pick"),false);}
      };
      d.appendChild(nm);d.appendChild(sz);d.appendChild(go);box.appendChild(d);
    });
  }).catch(function(){/* الصمت: اللوحة اختيارية ولا تحجب القائمة */});
}

