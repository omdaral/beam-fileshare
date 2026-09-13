/* Beam web boot — event wiring + polling (loads last). */
/* ---------- ربط الأحداث ---------- */
function autoRefresh(){
  if(!shouldPoll())return;
  if(activeUploads>0||activeDownloads>0)return;  // لا تحديث أثناء نقل شغال
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
function setUpMode(m){upMode=(m==="turbo")?"turbo":"reliable";$("modeRelUp").className=upMode==="reliable"?"active":"";$("modeTurboUp").className=upMode==="turbo"?"active":"";}
function setDlMode(m){dlMode=(m==="turbo")?"turbo":"reliable";$("modeRelDl").className=dlMode==="reliable"?"active":"";$("modeTurboDl").className=dlMode==="turbo"?"active":"";}
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
["admSsid","admPass","cfgPort"].forEach(function(id){
  var el=$(id);
  if(el&&!el.dataset.autobound){el.dataset.autobound="1";el.addEventListener("input",queueAutoSave);}
});
(function(){var sel=$("cfgLang");if(sel&&!sel.dataset.autobound){sel.dataset.autobound="1";sel.addEventListener("change",queueAutoSave);}})();
bindStopBtn("srvStopBtn","cfgMsg");
bindStopBtn("srvStopTop","netMsg");
$("picker").addEventListener("change",function(e){
  var files=Array.prototype.slice.call(e.target.files);
  var ad=window._adoptTarget||null;window._adoptTarget=null;
  if(ad&&ad.id&&files.length===1)enqueueUpload(files[0],ad,ad.name,null);
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
/* Mobile nav toggle (hamburger) */
try{
  var nt=$("navToggle"),mn=$("mainNav");
  if(nt&&mn){
    nt.onclick=function(){
      var open=mn.classList.toggle("open");
      nt.setAttribute("aria-expanded",open?"true":"false");
    };
    mn.addEventListener("click",function(e){
      if(e.target&&e.target.tagName==="A"){mn.classList.remove("open");nt.setAttribute("aria-expanded","false");}
    });
  }
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
