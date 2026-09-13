/* Beam web upload — v2 chunked engine + resume helpers + sessions panel. */
/* Beam web upload — v2 chunked engine, unified queue, folder zip, sessions panel. */
/* ---------- محرك الرفع v2 (قطع متوازية + بصمات + سرعة + استكمال بقوة التورنت) ---------- */
var activeUploads=0,activeDownloads=0;
var PARALLEL=3,PIECE_LEN=2*1024*1024,TURBO_PIECE=8*1024*1024,TURBO_LANES=6;
var upMode="reliable",dlMode="reliable",MAX_ZIP_INPUT=1*1024*1024*1024;
try{if(isMobileUI())MAX_ZIP_INPUT=CONFIG.ZIP_MAX_MOBILE;}catch(e){}
function statusOf(uuid){
  return fetch("/upload_status?id="+uuid).then(function(r){
    if(!r.ok)throw new Error("status");
    return r.json();
  }).then(function(j){return j.offset||0;});
}
function uploadFiles(list){
  var arr=Array.prototype.slice.call(list);
  var opts={turbo:(upMode==="turbo")};
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
function upKey(f,rel){return "up:"+(rel||f.name)+"|"+f.size+"|"+(f.lastModified||0);}
function storedUp(f,rel){try{return JSON.parse(localStorage.getItem(upKey(f,rel)));}catch(e){return null;}}
function storeUp(f,rel,o){try{localStorage.setItem(upKey(f,rel),JSON.stringify(o));}catch(e){}}
function clearUp(f,rel){try{localStorage.removeItem(upKey(f,rel));}catch(e){}}
function pieceCount(size,plen){return Math.max(1,Math.ceil(size/plen));}
function pieceRange(i,plen,size){var s=i*plen;return [s,Math.min(s+plen,size)];}
function pieceLenOf(i,plen,size){var r=pieceRange(i,plen,size);return r[1]-r[0];}
function uploadOne(file,adopt,relPath,onDone,batch,opts){
  activeUploads++;
  var rel=relPath||file.name;
  var outcome="done";
  var turbo=!!(opts&&opts.turbo),extract=!!(opts&&opts.extract);
  var box=$("upList");
  var wrap=document.createElement("div");wrap.className="up-row";wrap.style.margin="10px 0";
  var label=document.createElement("div");
  var bar=document.createElement("div");bar.className="prog";bar.style.display="block";
  var fill=document.createElement("div");bar.appendChild(fill);wrap.appendChild(label);wrap.appendChild(bar);
  var btnRow=document.createElement("div");btnRow.style.marginTop="6px";btnRow.style.display="flex";btnRow.style.gap="8px";
  var pauseBtn=document.createElement("button");pauseBtn.className="btn-gray";setBtn(pauseBtn,ICONS.pause,T("pause"));pauseBtn.style.minHeight="36px";pauseBtn.style.flex="1";
  var cancelBtn=document.createElement("button");cancelBtn.className="btn-gray";setBtn(cancelBtn,ICONS.x,T("cancel"));cancelBtn.style.minHeight="36px";cancelBtn.style.flex="1";
  btnRow.appendChild(pauseBtn);btnRow.appendChild(cancelBtn);wrap.appendChild(btnRow);
  if(batch&&batch.detailsEl)batch.detailsEl.appendChild(wrap);
  else box.insertBefore(wrap,box.firstChild);
  var st={paused:false,cancelled:false,confirmed:0,t0:Date.now(),speed:0,lastT:Date.now(),lastC:0,
    plen:(turbo?TURBO_PIECE:PIECE_LEN),lanes:(turbo?TURBO_LANES:PARALLEL),turbo:turbo,uuid:null,awaiting:null,aborts:[]};
  function done(){
    if(st.finished)return;
    st.finished=true;
    activeUploads--;
    if(onDone){try{onDone();}catch(e){}}
    var failedNow=(outcome!=="done");
    if(batch){
      try{delete batch.running[rel];}catch(e2){}
      if(!failedNow){batch.done++;batch.prog[rel]=file.size;}
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
    refresh();refreshSessions();
  }
  function fail(t){outcome="failed";label.textContent=t;}
  function render(){
    var pct=file.size?Math.round(st.confirmed/file.size*100):0;
    fill.style.width=pct+"%";
    var rem=file.size-st.confirmed;
    label.textContent=(st.turbo?"["+T("turbo_badge")+"] ":"")+rel+" ("+fmt(file.size)+") — "+pct+"% — "+fmtSpeed(st.speed)+
      (st.speed>0?" — "+T("remaining")+" "+fmtETA(rem/st.speed):"");
    if(batch)batchTick(batch,rel,st.confirmed);
  }
  function noteSpeed(){
    var now=Date.now(),dt=(now-st.lastT)/1000;
    if(dt>=0.4){var inst=(st.confirmed-st.lastC)/dt;st.speed=st.speed?st.speed*0.6+inst*0.4:inst;st.lastT=now;st.lastC=st.confirmed;}
  }
  if(!file.size){fail(T("up_empty")+rel);done();return;}
  pauseBtn.onclick=function(){
    if(st.awaiting){resumePump();return;}
    st.paused=!st.paused;
    if(st.paused){setBtn(pauseBtn,ICONS.play,T("resume"));}else{setBtn(pauseBtn,ICONS.pause,T("pause"));}
    if(st.paused)label.textContent=T("up_paused")+rel+" ("+Math.round(st.confirmed/file.size*100)+"%)";
    else{st.lastT=Date.now();st.lastC=st.confirmed;render();}
  };
  cancelBtn.onclick=function(){outcome="cancelled";st.cancelled=true;st.aborts.forEach(function(a){try{a();}catch(e){}});label.textContent=T("up_cancelled")+rel;};
  if(batch){batch.running[rel]=function(){try{cancelBtn.onclick();}catch(e6){}}}
  function regAbort(fn){st.aborts.push(fn);}
  function beginInit(){
    label.textContent=T("up_starting")+rel+" ...";
    var ibody={uuid:st.uuid,name:rel,size:file.size,piece_len:st.plen};
    if(st.turbo)ibody.noverify=true;
    if(extract)ibody.extract=true;
    return postJSON("/upload_init",ibody).then(function(res){
      if(res.status===413){fail(T("up_toobig"));clearUp(file,rel);done();return null;}
      if(res.status!==200||!res.body){fail(T("up_initfail"));done();return null;}
      storeUp(file,rel,{id:st.uuid,plen:st.plen});
      return startUpload(st.uuid,res.body.missing||[],st.plen);
    });
  }
  function adoptSession(s){
    st.uuid=s.id;st.plen=s.piece_len||PIECE_LEN;
    storeUp(file,rel,{id:s.id,plen:st.plen});
    return fetch("/upload_status?id="+s.id).then(function(r){
      if(!r.ok)throw new Error("nostatus");
      return r.json();
    }).then(function(j){
      if(j.size!==file.size||j.name!==rel)return startFresh();
      if(j.noverify&&!st.turbo){st.turbo=true;st.lanes=TURBO_LANES;}
      return startUpload(s.id,j.missing||[],st.plen);
    }).catch(startFresh);
  }
  function startUpload(uuid,missing,plen){
    var queue=missing.slice(),inflight=0,failed=null;
    st.aborts=[];
    st.confirmed=file.size-queue.length*plen;
    if(st.confirmed<0)st.confirmed=0;
    st.lastT=Date.now();st.lastC=st.confirmed;render();
    function launch(){
      if(st.cancelled)return Promise.reject(new Error("cancelled"));
      if(failed)return Promise.reject(failed);
      if(!queue.length&&inflight===0)return Promise.resolve(true);
      if(st.paused||!queue.length||inflight>=st.lanes)return sleep(250).then(launch);
      var idx=queue.shift();inflight++;
      return sendPiece(uuid,idx,plen).then(function(){
        inflight--;
        st.confirmed+=pieceLenOf(idx,plen,file.size);
        if(st.confirmed>file.size)st.confirmed=file.size;
        noteSpeed();render();
        return launch();
      },function(e){inflight--;failed=failed||e;return Promise.reject(e);});
    }
    var workers=[];
    for(var w=0;w<st.lanes;w++)workers.push(launch());
    return Promise.all(workers).then(function(){
      if(st.cancelled){done();return null;}
      label.textContent=T("up_verify")+rel+" ...";
      var cbody={id:uuid,name:rel,size:file.size};
      if(extract)cbody.extract=true;
      if(!st.turbo)return postJSON("/upload_complete",cbody);
      return hashFile(file).then(function(fh){
        cbody.full_hash=fh;
        return postJSON("/upload_complete",cbody);
      });
    }).then(function(res2){
      if(!res2)return;
      if(res2.status===200){
        var secs=Math.max((Date.now()-st.t0)/1000,0.1);
        label.textContent=(st.turbo?"["+T("turbo_badge")+"] ":"")+T("up_done")+rel+" ("+fmtSpeed(file.size/secs)+")";
        fill.style.width="100%";clearUp(file,rel);pauseBtn.style.display="none";cancelBtn.style.display="none";done();
      }
      else if(res2.status===400&&res2.body&&res2.body.missing){
        label.textContent=res2.body.missing.length+" "+T("up_missing_left")+" ...";
        return startUpload(uuid,res2.body.missing,plen);
      }
      else fail(T("up_assemble_fail")+((res2.body&&res2.body.error)||T("retry_generic")));
      if(res2.status!==400&&!st.finished)done();
    }).catch(function(){
      if(st.cancelled){done();return;}
      st.awaiting={uuid:uuid,plen:plen};
      st.paused=true;setBtn(pauseBtn,ICONS.play,T("resume"));
      fail(T("up_conn")+rel+T("up_conn2"));
    });
  }
  function resumePump(){
    var r=st.awaiting;st.awaiting=null;st.paused=false;
    setBtn(pauseBtn,ICONS.pause,T("pause"));
    st.lastT=Date.now();st.lastC=st.confirmed;
    if(!r){fail(T("up_noresume"));return;}
    fetch("/upload_status?id="+r.uuid).then(function(x){
      if(!x.ok)throw new Error("nostatus");
      return x.json();
    }).then(function(j){
      if(j.size!==file.size||j.name!==rel)return startFresh();
      if(j.noverify&&!st.turbo){st.turbo=true;st.lanes=TURBO_LANES;}
      return startUpload(r.uuid,j.missing||[],r.plen);
    }).catch(startFresh);
  }
  function sendPiece(uuid,idx,plen){
    var r=pieceRange(idx,plen,file.size),tries=0;
    function attempt(){
      if(st.cancelled)return Promise.reject(new Error("cancelled"));
      var hashP=st.turbo?Promise.resolve(""):file.slice(r[0],r[1]).arrayBuffer().then(function(ab){
        return shaU8(new Uint8Array(ab));
      });
      return hashP.then(function(h){
        return xhrBin("/upload_piece?id="+uuid+"&index="+idx+(h?"&hash="+h:""),file.slice(r[0],r[1]),regAbort);
      }).then(function(res){
        if(res.status===-1)return Promise.reject(new Error("cancelled"));
        if(res.status===200)return true;
        if(res.status===422&&res.body){
          var miss=res.body.missing||[];
          if(miss.indexOf(idx)===-1)return true;
        }
        if(++tries<5){
          label.textContent=rel+" — "+T("up_retry")+" "+(idx+1)+" ("+tries+"/5) ...";
          return sleep(500*Math.pow(2,tries-1)).then(attempt);
        }
        throw new Error("piece");
      });
    }
    return attempt();
  }
  function startFresh(){
    st.uuid=genUuid();
    return beginInit().catch(function(){
      if(st.cancelled){done();return null;}
      fail(T("up_adoptfail")+rel+". "+T("retry_generic"));
      done();return null;
    });
  }
  if(adopt&&adopt.id){
    adoptSession(adopt);
    return;
  }
  var su=storedUp(file,rel);
  if(su&&su.id){
    fetch("/upload_status?id="+su.id).then(function(r){
      if(!r.ok)throw new Error("nostatus");
      return r.json();
    }).then(function(j){
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

/* ---------- لوحة الجلسات القابلة للاستكمال ---------- */
function refreshSessions(){
  var box=$("sessions");
  if(!box)return;
  if(activeUploads>0||activeDownloads>0)return;
  if(document.querySelector('[data-armed="1"]'))return;
  fetch("/upload_sessions").then(function(r){
    if(!r.ok)throw new Error("http"+r.status);
    return r.json();
  }).then(function(j){
    var list=j.sessions||[];
    box.innerHTML="";
    if(!list.length){var p=document.createElement("p");p.textContent=T("sess_empty");;box.appendChild(p);return;}
    list.forEach(function(s){
      var d=document.createElement("div");d.className="file";
      var nm=document.createElement("span");nm.className="name";nm.textContent=s.name;
      var pct=s.size?Math.round(s.received/s.size*100):0;
      var sz=document.createElement("span");sz.className="size";sz.textContent=fmt(s.received)+" / "+fmt(s.size)+" ("+pct+"%)";
      var go=document.createElement("button");go.className="btn-blue";go.style.minHeight="48px";setBtn(go,ICONS.dl,T("sess_resume"));
      go.onclick=function(){
        window._adoptTarget={id:s.id,name:s.name};
        try{document.getElementById("picker").click();}
        catch(e){show($("sessMsg"),T("sess_pick"),false);}
      };
      d.appendChild(nm);d.appendChild(sz);d.appendChild(go);box.appendChild(d);
    });
  }).catch(function(){/* الصمت: اللوحة اختيارية ولا تحجب القائمة */});
}

