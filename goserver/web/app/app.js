/* Beam web boot — event wiring + polling (loads last). */
/* ---------- ربط الأحداث ---------- */
function autoRefresh(){
  if(!shouldPoll())return;
  // Download progress rows live inside #files, so a re-render would wipe
  // them — skip the files list only while downloading. Uploads live in
  // #upList, so the download list must keep refreshing or new shares never
  // appear (previously activeUploads froze the list for the whole upload).
  if(activeDownloads>0){
    bgIdleCheck();
    return;
  }
  if(document.querySelector('[data-armed="1"]'))return;  // لا تقاطع تأكيد حذف
  refresh();
  refreshSessions();
  bgIdleCheck(); // إطفاء خدمة الخلفية بعد 30s خمول
}
$("copyBtn").onclick=function(){copyText(lastUrl||baseUrl());};
$("copyBtn2").onclick=function(){copyText(lastUrl||baseUrl());};
var hcb=$("heroCopyBtn");if(hcb)hcb.onclick=function(){copyText(lastUrl||baseUrl());};
$("qrWifiBtn").onclick=function(){qrMode="wifi";updateQR();};
$("qrUrlBtn").onclick=function(){qrMode="url";updateQR();};
function setUpMode(m){upMode="reliable";try{$("modeRelUp").className="active";$("modeTurboUp").className="";}catch(e){}try{var s=$("upModeSeg");if(s)s.style.display="none";}catch(e2){}}
function setDlMode(m){dlMode="reliable";try{$("modeRelDl").className="active";$("modeTurboDl").className="";}catch(e){}try{var s2=$("dlModeSeg");if(s2)s2.style.display="none";}catch(e2){}}
$("modeRelUp").onclick=function(){setUpMode("reliable");};
$("modeTurboUp").onclick=function(){setUpMode("turbo");};
$("modeRelDl").onclick=function(){setDlMode("reliable");};
$("modeTurboDl").onclick=function(){setDlMode("turbo");};
$("modeLan").onclick=function(){setAdminMode("lan",true);};
$("modeHotspot").onclick=function(){setAdminMode("hotspot",true);};
$("openCheckBtn").onclick=function(){
  adminOpen=!adminOpen;
  var ocb=$("openCheckBtn").querySelector("span");if(ocb)ocb.textContent=adminOpen?T("open_on"):T("open_off");
  $("openCheckBtn").setAttribute("aria-pressed",adminOpen?"true":"false");
  queueAutoSave();
};
$("showPassBtn").onclick=function(){
  var p=$("admPass"),s=$("showPassBtn");
  var show=p.type==="password";
  p.type=show?"text":"password";
  var sl=s.querySelector("span");if(sl)sl.textContent=show?T("hide_pass"):T("show_pass");
};
$("netStartBtn").onclick=netStart;
$("netStopBtn").onclick=netStop;
$("devRefreshBtn").onclick=fetchClients;
$("logRefreshBtn").onclick=refreshLogs;
$("logClearBtn").onclick=function(){
  var b=$("logClearBtn"),m=$("logMsg");
  if(b.dataset.armed!=="1"){
    b.dataset.armed="1";setBtn(b,ICONS.trash,T("log_clear_confirm"));
    setTimeout(function(){b.dataset.armed="0";setBtn(b,ICONS.trash,T("log_clear"));},4000);
    return;
  }
  b.dataset.armed="0";setBtn(b,ICONS.trash,T("log_clear"));
  postJSON("/api/logs/clear",{}).then(function(res){
    if(res.status===200){show(m,((res.body&&res.body.msg)||T("done_fallback")),true);refreshLogs();}
    else show(m,((res.body&&res.body.error)||T("save_fail")),false);
  }).catch(function(){show(m,T("conn_fail"),false);});
};
/* حفظ تلقائي عند أي تعديل (بمنع ارتداد) — لا زر حفظ */
["admSsid","admPass","cfgPort","cfgTemp","wifiSsid","wifiPass"].forEach(function(id){
  var el=$(id);
  if(el&&!el.dataset.autobound){el.dataset.autobound="1";el.addEventListener("input",queueAutoSave);}
});
(function(){var sel=$("cfgLang");if(sel&&!sel.dataset.autobound){sel.dataset.autobound="1";sel.addEventListener("change",queueAutoSave);}})();
bindStopBtn("srvStopBtn","cfgMsg");
bindStopBtn("srvStopTop","netMsg");
$("picker").addEventListener("change",function(e){
  var files=Array.prototype.slice.call(e.target.files);
  var ad=window._adoptTarget||null;window._adoptTarget=null;
  if(ad&&ad.id&&files.length===1)enqueueUpload(files[0],ad.name,ad,null);
  else uploadFiles(files);
  e.target.value="";
});
var folderPicker=$("folderPicker");
if(folderPicker)folderPicker.addEventListener("change",function(e){
  uploadFolderFiles(e.target.files);
  e.target.value="";
});
var folderBtn=$("folderBtn");
if(folderBtn)folderBtn.onclick=function(){var p=$("folderPicker");if(p)p.click();};
bindSearch();
var drop=$("drop");
function openPicker(){$("picker").click();}
drop.addEventListener("click",openPicker);
drop.addEventListener("keydown",function(e){if(e.key==="Enter"||e.key===" "){e.preventDefault();openPicker();}});
["dragover","dragenter"].forEach(function(ev){drop.addEventListener(ev,function(e){e.preventDefault();drop.classList.add("over");});});
["dragleave","drop"].forEach(function(ev){drop.addEventListener(ev,function(e){e.preventDefault();drop.classList.remove("over");});});
drop.addEventListener("drop",function(e){if(e.dataTransfer)collectDrop(e.dataTransfer);});

/* ---------- بدء التشغيل: دخول مباشر بدون كود + تحديث تلقائي ---------- */
(function initSettingsModal(){
  var sb=$("settingsBtn");
  if(sb)sb.onclick=function(){openSettings();};
  var x=$("settingsClose");
  if(x)x.onclick=function(){closeSettings();};
  var oz=$("ownerZone");
  if(oz)oz.addEventListener("click",function(e){if(e.target===oz)closeSettings();});
  var tabs=document.querySelectorAll(".tabs button");
  for(var i=0;i<tabs.length;i++){
    (function(b){b.onclick=function(){switchSettingsTab(b.getAttribute("data-tab"));};})(tabs[i]);
  }
  document.addEventListener("keydown",function(e){
    if((e.key==="Escape"||e.key==="Esc")&&settingsOpen)closeSettings();
  });
})();
initThemeBtn();
initLang();
idbCleanup();
/* Mobile nav toggle (hamburger → ×, outside/Escape close, desktop cleanup) */
try{
  var nt=$("navToggle"),mn=$("mainNav");
  function setNav(open){
    if(!nt||!mn)return;
    mn.classList.toggle("open",!!open);
    nt.setAttribute("aria-expanded",open?"true":"false");
    try{nt.textContent=open?"×":"☰";}catch(e){}
  }
  if(nt&&mn){
    nt.onclick=function(e){
      if(e&&e.stopPropagation){try{e.stopPropagation();}catch(e2){}}
      setNav(!mn.classList.contains("open"));
    };
    mn.addEventListener("click",function(e){
      if(e.target&&e.target.tagName==="A")setNav(false);
    });
    document.addEventListener("click",function(e){
      if(!mn.classList.contains("open"))return;
      var t=e.target;
      if(t===mn||t===nt)return;
      try{if(mn.contains(t)||(nt.contains&&nt.contains(t)))return;}catch(e2){}
      setNav(false);
    });
    document.addEventListener("keydown",function(e){
      if((e.key==="Escape"||e.key==="Esc")&&mn.classList.contains("open")){setNav(false);try{nt.focus();}catch(e3){}}
    });
    window.addEventListener("resize",function(){
      try{
        if(window.matchMedia&&matchMedia("(min-width:641px)").matches&&mn.classList.contains("open"))setNav(false);
      }catch(e){}
    });
  }
}catch(e){}
/* Scroll-spy: highlight the nav link of the section in view */
try{
  (function(){
    var nav=document.getElementById("mainNav");
    if(!nav||!("IntersectionObserver" in window))return;
    var links=nav.querySelectorAll('a[href^="#"]');
    if(!links.length)return;
    var byId={};
    for(var i=0;i<links.length;i++){byId[links[i].getAttribute("href").slice(1)]=links[i];}
    var obs=new IntersectionObserver(function(entries){
      for(var k=0;k<entries.length;k++){
        var en=entries[k];
        if(!en.isIntersecting)continue;
        var a=byId[en.target.id];
        if(!a)continue;
        for(var j=0;j<links.length;j++)links[j].classList.remove("active");
        a.classList.add("active");
      }
    },{rootMargin:"-40% 0px -55% 0px"});
    for(var id in byId){if(byId.hasOwnProperty(id)){var s=document.getElementById(id);if(s)obs.observe(s);}}
  })();
}catch(e){}
/* Named polling timers so applyLimits() can reset them when /api/status
   reports different intervals (single source: the Go backend). */
var pollTimers=[];
function resetPolling(){
  for(var i=0;i<pollTimers.length;i++){try{clearInterval(pollTimers[i]);}catch(e){}}
  pollTimers=[
    setInterval(function(){if(shouldPoll())fetchStatus();},CONFIG.POLL_STATUS),
    setInterval(function(){if(shouldPollClients())fetchClients();},CONFIG.POLL_CLIENTS),
    setInterval(autoRefresh,CONFIG.POLL_FILES)
  ];
}
resetPolling();
if(shouldPollClients())fetchClients();
// Single fast+reliable mode: hide the old reliable/turbo switches on boot.
try{setUpMode("reliable");}catch(e){}
try{setDlMode("reliable");}catch(e2){}
