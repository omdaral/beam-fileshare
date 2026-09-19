/* Beam web folder upload — direct transfer without any compression. */
function cleanFolderRel(rel){
  rel=String(rel||"").replace(/\\/g,"/");
  if(!rel||rel.charAt(0)==="/"||rel.indexOf("/../")>=0||rel.indexOf("//")>=0)return "";
  var segs=rel.split("/"),out=[];
  for(var k=0;k<segs.length;k++){
    var s=segs[k].replace(/^\s+|\s+$/g,"");
    if(!s||s==="."||s===".."||s.charAt(0)===".")return "";
    out.push(s);
  }
  return out.join("/");
}
function collectFolderItems(list){
  var arr=Array.prototype.slice.call(list||[]),out=[];
  for(var i=0;i<arr.length;i++){
    var f=arr[i],rel=f.webkitRelativePath||"";
    if(!rel)continue;
    rel=cleanFolderRel(rel);
    if(!rel)continue;
    out.push({f:f,rel:rel});
  }
  return out;
}
/* المجلد يُنشر ملف-ملف كمشاركات ضيف بنفس أسماء مساراته — بدون أي ضغط في المتصفح */
function uploadFolderFiles(list){
  var out=collectFolderItems(list);
  if(!out.length){show($("msg"),T("folder_empty"),false);return;}
  if(typeof sharePublishFolderItems==="function"){
    sharePublishFolderItems(out);
    show($("msg"),out.length+" "+T("folder_picked"),true);
    return;
  }
  var opts={turbo:false};
  enqueueBatch(out,null,opts);
  show($("msg"),out.length+" "+T("folder_picked"),true);
}
/* سحب مجلدات وإفلاتها: قراءة Directory Entries ثم نفس الطابور المباشر */
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
  function enqueuePlain(f){
    if(typeof sharePublishFiles==="function"){sharePublishFiles([f]);return;}
    enqueueUpload(f,null,null,null);
  }
  if(!entries.length)return;
  var out=[],pending=entries.length;
  function fin(){
    if(--pending>0)return;
    if(!out.length){show($("msg"),T("folder_empty"),false);return;}
    var hasRel=false;
    for(var k=0;k<out.length;k++)if(out[k].rel.indexOf("/")>=0){hasRel=true;break;}
    if(!hasRel){
      if(typeof sharePublishFolderItems==="function"){sharePublishFolderItems(out);return;}
      enqueueBatch(out.map(function(o){return {f:o.f,rel:null};}),null);return;
    }
    var clean=[];
    for(var j=0;j<out.length;j++){
      var rel=cleanFolderRel(out[j].rel);
      if(!rel)continue;
      clean.push({f:out[j].f,rel:rel});
    }
    if(!clean.length){show($("msg"),T("folder_empty"),false);return;}
    if(typeof sharePublishFolderItems==="function"){
      sharePublishFolderItems(clean);
      show($("msg"),clean.length+" "+T("folder_picked"),true);
      return;
    }
    var opts={turbo:false};
    enqueueBatch(clean,null,opts);
    show($("msg"),clean.length+" "+T("folder_picked"),true);
  }
  entries.forEach(function(e2){walkEntry(e2,"",out,fin);});
}
