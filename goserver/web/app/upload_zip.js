/* Beam web folder zip — client-side compress (Deflate/STORE) then upload. */
function uploadFolderFiles(list){
  var arr=Array.prototype.slice.call(list),out=[];
  for(var i=0;i<arr.length;i++){
    var f=arr[i],rel=f.webkitRelativePath||"";
    if(!rel)continue;
    rel=String(rel).replace(/\\/g,"/");
    if(rel.charAt(0)==="/"||rel.indexOf("/../")>=0||rel.indexOf("//")>=0)continue;
    var segs=rel.split("/"),bad=false;
    for(var k=0;k<segs.length;k++){
      var s=segs[k].replace(/^\s+|\s+$/g,"");
      if(!s||s==="."||s===".."||s.charAt(0)==="."){bad=true;break;}
    }
    if(bad)continue;
    out.push({f:f,rel:rel});
  }
  if(!out.length){show($("msg"),T("folder_empty"),false);return;}
  if(out.length>MAX_FOLDER_FILES){show($("msg"),T("folder_too_many"),false);return;}
  zipAndUpload(out);
}
/* المجلد يُضغط أولًا دائمًا (أي وضع) ثم يُرفع كملف واحد سريع الاستكمال */
var zipBusy=false;
function zipAndUpload(items){
  if(zipBusy){show($("msg"),T("zipping")+" ...",true);}
  zipBusy=true;
  var m=$("msg"),lastNote=0;
  show(m,T("zipping")+" 0/"+items.length,true);
  buildFolderZip(items,function(d,t){
    var now=Date.now();
    if(d<0||now-lastNote<400)return;
    lastNote=now;
    show(m,T("zipping")+" "+d+"/"+t,true);
  }).then(function(z){
    zipBusy=false;
    var f;
    try{f=new File([z.blob],z.name,{type:"application/zip"});}
    catch(e){show(m,T("zip_build_fail"),false);return;}
    enqueueUpload(f,null,null,null,{extract:true,turbo:(upMode==="turbo")});
    show(m,items.length+" "+T("folder_picked"),true);
  }).catch(function(e){
    zipBusy=false;
    show(m,(e&&e.tooBig)?T("folder_zip_too_big"):T("zip_build_fail"),false);
  });
}
/* سحب مجلدات وإفلاتها: قراءة Directory Entries ثم نفس الطابور */
function walkEntry(entry,path,out,done){
  if(entry.isFile){
    try{entry.file(function(f){out.push({f:f,rel:path+entry.name});done();},function(){done();});}
    catch(e){done();}
  }else if(entry.isDirectory){
    var rd;
    try{rd=entry.createReader();}catch(e2){done();return;}
    var batch=[];
    (function readMore(){
      try{rd.readEntries(function(list){
        if(!list.length){
          var i=0;
          (function next(){
            if(i>=batch.length){done();return;}
            var en=batch[i++];walkEntry(en,path+entry.name+"/",out,next);
          })();
          return;
        }
        batch=batch.concat(list);readMore();
      },function(){done();});}catch(e3){done();}
    })();
  }else{done();}
}
function collectDrop(dt){
  if(!dt)return;
  if(!dt.items||!dt.items.length){
    if(dt.files&&dt.files.length)uploadFiles(dt.files);
    return;
  }
  var entries=[];
  for(var i=0;i<dt.items.length;i++){
    var it=dt.items[i],en=null;
    try{en=it.webkitGetAsEntry?it.webkitGetAsEntry():null;}catch(e){}
    if(en)entries.push(en);
    else if(it.getAsFile){var f=it.getAsFile();if(f)enqueuePlain(f);}
  }
  function enqueuePlain(f){enqueueUpload(f,null,null,null);}
  if(!entries.length)return;
  var out=[],pending=entries.length;
  function fin(){
    if(--pending>0)return;
    if(!out.length)return;
    var hasRel=false;
    for(var k=0;k<out.length;k++)if(out[k].rel.indexOf("/")>=0){hasRel=true;break;}
    if(!hasRel){enqueueBatch(out.map(function(o){return {f:o.f,rel:null};}),null);return;}
    var clean=[];
    for(var j=0;j<out.length;j++){
      var rel=String(out[j].rel).replace(/\\/g,"/");
      if(rel.charAt(0)==="/"||rel.indexOf("/../")>=0||rel.indexOf("//")>=0)continue;
      clean.push({f:out[j].f,rel:rel});
    }
    if(!clean.length)return;
    if(clean.length>MAX_FOLDER_FILES){show($("msg"),T("folder_too_many"),false);return;}
    zipAndUpload(clean);
  }
  entries.forEach(function(e2){walkEntry(e2,"",out,fin);});
}
/* ---------- باني zip محلي: المجلد يُضغط أولًا دائمًا (Deflate إن توفر، وإلا STORE) ---------- */
var ZIP_CRC_T=null;
function zipCrcInit(){
  if(ZIP_CRC_T)return;
  var t=new Array(256);
  for(var n=0;n<256;n++){var c=n;for(var k=0;k<8;k++)c=(c&1)?(0xEDB88320^(c>>>1)):(c>>>1);t[n]=c>>>0;}
  ZIP_CRC_T=t;
}
function zipCrc(u8,crc){
  zipCrcInit();
  crc=(crc===undefined?0xFFFFFFFF:crc)>>>0;
  for(var i=0;i<u8.length;i++)crc=ZIP_CRC_T[(crc^u8[i])&255]^(crc>>>8);
  return crc>>>0;
}
function dosDT(d){
  return [(d.getHours()<<11)|(d.getMinutes()<<5)|(d.getSeconds()>>1),
    ((d.getFullYear()-1980)<<9)|((d.getMonth()+1)<<5)|d.getDate()];
}
function readFileBytes(f){
  if(f.arrayBuffer)return f.arrayBuffer().then(function(ab){return new Uint8Array(ab);});
  return new Promise(function(res,rej){
    var r=new FileReader();
    r.onload=function(){res(new Uint8Array(r.result));};
    r.onerror=function(){rej(r.error||new Error("read"));};
    r.readAsArrayBuffer(f);
  });
}
function deflateRaw(u8){
  return new Promise(function(res,rej){
    var cs;
    try{cs=new CompressionStream("deflate-raw");}
    catch(e){rej(e);return;}
    var reader=cs.readable.getReader(),writer=cs.writable.getWriter(),chunks=[];
    (function pump(){
      reader.read().then(function(r){
        if(r.done){res(chunks);return;}
        chunks.push(r.value);pump();
      },rej);
    })();
    var CH=1<<20,off=0;
    (function feed(){
      if(off>=u8.length){writer.close();return;}
      var end=Math.min(off+CH,u8.length);
      writer.write(u8.subarray(off,end)).then(function(){off=end;feed();},rej);
    })();
  });
}
function concatU8(list){
  var total=0,i;
  for(i=0;i<list.length;i++)total+=list[i].length;
  var out=new Uint8Array(total),off=0;
  for(i=0;i<list.length;i++){out.set(list[i],off);off+=list[i].length;}
  return out;
}
function buildFolderZip(items,onProg){
  var i,total=0;
  for(i=0;i<items.length;i++)total+=items[i].f.size||0;
  if(total>MAX_ZIP_INPUT){var e0=new Error("zip too big");e0.tooBig=true;return Promise.reject(e0);}
  var deflateOK=false;
  try{deflateOK=(typeof CompressionStream!=="undefined")&&(typeof TextEncoder!=="undefined");}catch(e){}
  if(typeof TextEncoder==="undefined")return Promise.reject(new Error("no encoder"));
  var te=new TextEncoder();
  var parts=[],clen=0,central=[],ccount=0,dirs={},dt=dosDT(new Date());
  function pushU8(u8){parts.push(u8);clen+=u8.length;}
  function w16(a,v){a.push(v&255,(v>>8)&255);}
  function w32(a,v){v=v>>>0;a.push(v&255,(v>>8)&255,(v>>16)&255,(v>>24)&255);}
  function emitLocal(nb,method,crc,comp,uncomp){
    var h=[];w32(h,0x04034b50);w16(h,20);w16(h,0x800);w16(h,method);
    w16(h,dt[0]);w16(h,dt[1]);w32(h,crc);w32(h,comp);w32(h,uncomp);
    w16(h,nb.length);w16(h,0);
    var off=clen;
    pushU8(Uint8Array.from(h));pushU8(nb);
    return off;
  }
  function emitCentral(nb,method,crc,comp,uncomp,off,isDir){
    var h=[];w32(h,0x02014b50);w16(h,20);w16(h,20);w16(h,0x800);w16(h,method);
    w16(h,dt[0]);w16(h,dt[1]);w32(h,crc);w32(h,comp);w32(h,uncomp);
    w16(h,nb.length);w16(h,0);w16(h,0);w16(h,0);w16(h,0);
    w32(h,isDir?0x10<<16:0);w32(h,off);
    central.push(Uint8Array.from(h));central.push(nb);ccount++;
  }
  items.forEach(function(o){
    var p=o.rel,idx;
    while((idx=p.lastIndexOf("/"))>0){p=p.slice(0,idx);dirs[p]=1;}
  });
  var dirList=Object.keys(dirs).sort();
  var chain=Promise.resolve();
  dirList.forEach(function(dp){
    chain=chain.then(function(){
      var nb=te.encode(dp+"/");
      var off=emitLocal(nb,0,0,0,0);
      emitCentral(nb,0,0,0,0,off,true);
      if(onProg)onProg(-1,items.length);
    });
  });
  items.forEach(function(o,idx){
    chain=chain.then(function(){return readFileBytes(o.f);}).then(function(u8){
      var nb=te.encode(o.rel);
      var crc=(~zipCrc(u8))>>>0;
      function store(method,data){
        var off=emitLocal(nb,method,crc,data.length,u8.length);
        pushU8(data);
        emitCentral(nb,method,crc,data.length,u8.length,off,false);
        if(onProg)onProg(idx+1,items.length);
      }
      if(deflateOK&&u8.length>0){
        return deflateRaw(u8).then(function(ch){
          var flat=concatU8(ch);
          if(flat.length<u8.length)store(8,flat);
          else store(0,u8);
        },function(){store(0,u8);});
      }
      store(0,u8);
    });
  });
  return chain.then(function(){
    var cdStart=clen,i;
    for(i=0;i<central.length;i++)pushU8(central[i]);
    var cdSize=clen-cdStart,e=[];
    w32(e,0x06054b50);w16(e,0);w16(e,0);w16(e,ccount);w16(e,ccount);
    w32(e,cdSize);w32(e,cdStart);w16(e,0);
    pushU8(Uint8Array.from(e));
    var tops={},tn=0;
    items.forEach(function(o){var g=o.rel.split("/")[0];if(!tops[g]){tops[g]=1;tn++;}});
    var name=tn===1?(items[0].rel.split("/")[0]+".zip"):"folders.zip";
    return {blob:new Blob(parts,{type:"application/zip"}),name:name,count:items.length};
  });
}
