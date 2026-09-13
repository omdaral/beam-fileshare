/* Beam web upload queue — unified lanes, grouped batches, recent list. */
/* ---------- طابور رفع موحد + دفعات مجمعة: سقف الشرائط ---------- */
var MAX_FOLDER_FILES=2000,UP_PAR=3,RECENT_MAX=5;
var upQueue=[],upActive=0,batchSeq=0;
function enqueueUpload(f,rel,adopt,batch,opts){upQueue.push({f:f,rel:rel,adopt:adopt,batch:batch,opts:opts||null});pumpQueue();}
function pumpQueue(){
  bgKick(); // خدمة خلفية الأندرويد (no-op على الويب)
  while(upActive<UP_PAR&&upQueue.length){
    var it=upQueue.shift();
    if(it.batch&&it.batch.cancelled){dropBatchItem(it.batch,it);continue;}
    upActive++;
    uploadOne(it.f,it.adopt,it.rel,function(){upActive--;pumpQueue();},it.batch,it.opts);
  }
}
function groupKey(rel){var i=rel.indexOf("/");return i>=0?rel.slice(0,i):"";}
function newBatch(label,total,bytes){
  batchSeq++;
  return {id:batchSeq,label:label,total:total,done:0,failed:0,cancelledN:0,
    bytesTotal:bytes||0,prog:{},running:{},cancelled:false,finished:false,
    collapsed:total>5,groupEl:null,fillEl:null,countEl:null,detailsEl:null,
    chevEl:null,cancelEl:null};
}
function enqueueBatch(items,label,opts){
  var box=$("upList");
  var groups={},order=[];
  items.forEach(function(o){
    var g=(o.rel&&o.rel.indexOf("/")>=0)?groupKey(o.rel):"";
    if(!groups[g]){groups[g]={items:[]};order.push(g);}
    groups[g].items.push(o);
  });
  order.forEach(function(g){
    var list=groups[g].items;
    if(g===""&&list.length===1&&!label){
      enqueueUpload(list[0].f,list[0].rel||null,null,null,opts||null);
      return;
    }
    var bytes=0,i;
    for(i=0;i<list.length;i++)bytes+=list[i].f.size||0;
    var b=newBatch(label||(g===""?(list.length+" "+T("batch_files")):g),list.length,bytes);
    buildBatchRow(b,box);
    for(i=0;i<list.length;i++)enqueueUpload(list[i].f,list[i].rel||null,null,b,opts||null);
  });
}
function buildBatchRow(b,box){
  var g=document.createElement("div");g.className="batch";
  var head=document.createElement("div");head.className="batch-head";
  var chev=document.createElement("span");chev.className="chev";
  var ic=document.createElement("span");
  ic.innerHTML='<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>';
  var nm=document.createElement("span");nm.className="name";nm.textContent=b.label;
  var c=document.createElement("span");c.className="count";
  var x=document.createElement("button");x.className="btn-gray mini";setBtn(x,ICONS.x,T("batch_cancel"));
  x.onclick=function(ev){if(ev)ev.stopPropagation();cancelBatch(b);};
  head.appendChild(chev);head.appendChild(ic);head.appendChild(nm);head.appendChild(c);head.appendChild(x);
  head.onclick=function(){b.collapsed=!b.collapsed;syncBatchRow(b);};
  var bar=document.createElement("div");bar.className="prog";bar.style.display="block";
  var fill=document.createElement("div");bar.appendChild(fill);
  var det=document.createElement("div");det.className="batch-details";
  g.appendChild(head);g.appendChild(bar);g.appendChild(det);
  box.insertBefore(g,box.firstChild);
  b.groupEl=g;b.fillEl=fill;b.countEl=c;b.detailsEl=det;b.chevEl=chev;b.cancelEl=x;
  syncBatchRow(b);
}
function syncBatchRow(b){
  if(!b.groupEl)return;
  b.detailsEl.style.display=b.collapsed?"none":"";
  b.chevEl.textContent=b.collapsed?"▸":"▾";
}
function batchBytes(b){
  var t=0;
  for(var k in b.prog)if(b.prog.hasOwnProperty(k))t+=b.prog[k];
  return t;
}
function updateBatchRow(b){
  if(!b.groupEl)return;
  var t=batchBytes(b);
  if(t>b.bytesTotal)t=b.bytesTotal;
  var pct=b.bytesTotal?Math.round(t/b.bytesTotal*100):0;
  b.fillEl.style.width=pct+"%";
  var waiting=b.total-b.done-b.failed-b.cancelledN;
  if(waiting<0)waiting=0;
  var txt=(b.done+b.failed+b.cancelledN)+" / "+b.total+" • "+pct+"%";
  if(b.failed>0)txt+=" • "+b.failed+" "+T("batch_failed");
  if(waiting>0&&!b.finished)txt+=" • +"+waiting+" "+T("batch_queued");
  if(b.finished)txt+=" • "+T("batch_done");
  b.countEl.textContent=txt;
}
function batchTick(b,rel,confirmed){
  if(!b||b.cancelled||b.finished)return;
  b.prog[rel]=confirmed;
  updateBatchRow(b);
}
function dropBatchItem(b,it){
  b.total--;
  b.bytesTotal-=it.f.size||0;
  if(b.bytesTotal<0)b.bytesTotal=0;
  updateBatchRow(b);
  maybeFinishBatch(b);
}
function cancelBatch(b){
  if(!b||b.cancelled||b.finished)return;
  b.cancelled=true;
  upQueue=upQueue.filter(function(it){
    if(it.batch===b){b.total--;b.bytesTotal-=it.f.size||0;return false;}
    return true;
  });
  if(b.bytesTotal<0)b.bytesTotal=0;
  for(var k in b.running)if(b.running.hasOwnProperty(k)){try{b.running[k]();}catch(e){}}
  updateBatchRow(b);
  maybeFinishBatch(b);
}
function maybeFinishBatch(b){
  if(!b||b.finished)return;
  for(var k in b.running)if(b.running.hasOwnProperty(k))return;
  for(var i=0;i<upQueue.length;i++)if(upQueue[i].batch===b)return;
  b.finished=true;
  if(b.cancelEl)b.cancelEl.style.display="none";
  updateBatchRow(b);
  try{setTimeout(function(){b.collapsed=true;syncBatchRow(b);},4000);}catch(e){}
}
function recentBox(){
  var box=$("upList");
  var r=document.getElementById("recentStrip");
  if(r)return r;
  r=document.createElement("div");r.id="recentStrip";
  var h=document.createElement("div");h.className="recent-head";
  var s=document.createElement("span");s.setAttribute("data-i18n","recent_ops");s.textContent=T("recent_ops");
  h.appendChild(s);r.appendChild(h);
  box.insertBefore(r,box.firstChild);
  return r;
}
function pruneRecent(){
  var rb=document.getElementById("recentStrip");
  if(!rb||!rb.querySelectorAll)return;
  var rows=rb.querySelectorAll(".up-row"),alive=[];
  for(var i=0;i<rows.length;i++)if(!rows[i].dataset.pin)alive.push(rows[i]);
  while(alive.length>RECENT_MAX){var old=alive.shift();try{rb.removeChild(old);}catch(e){}}
}
