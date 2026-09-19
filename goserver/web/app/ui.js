/* Beam web UI helpers — DOM, messages, format, icons, theme, modal, polling guards. */
function $(id){return document.getElementById(id);}
function show(el,txt,ok){el.textContent=txt;el.className="msg "+(ok?"ok":"err");}
function clearMsg(el){el.textContent="";el.className="msg";}
function fmt(n){var ar=LANG!=="en";if(n<1024)return n+" "+(ar?"بايت":"B");if(n<1048576)return (n/1024).toFixed(1)+" "+(ar?"ك.ب":"KB");if(n<1073741824)return (n/1048576).toFixed(1)+" "+(ar?"م.ب":"MB");return (n/1073741824).toFixed(2)+" "+(ar?"ج.ب":"GB");}

function fmtSpeed(v){if(!(v>0))return "…";var ar=LANG!=="en",ps=ar?"/ث":"/s";if(v<1024)return Math.round(v)+" "+(ar?"بايت":"B")+ps;if(v<1048576)return (v/1024).toFixed(1)+" "+(ar?"ك.ب":"KB")+ps;if(v<1073741824)return (v/1048576).toFixed(1)+" "+(ar?"م.ب":"MB")+ps;return (v/1073741824).toFixed(2)+" "+(ar?"ج.ب":"GB")+ps;}
function fmtETA(sec){if(!isFinite(sec)||sec<0)return "…";var ar=LANG!=="en";sec=Math.round(sec);if(sec<60)return sec+" "+(ar?"ث":"s");var m=Math.floor(sec/60);if(m<60)return m+(ar?" د ":"m ")+(sec%60)+" "+(ar?"ث":"s");return Math.floor(m/60)+(ar?" س ":"h ")+(m%60)+(ar?" د":"m");}
/* ---------- نسخ الرابط ---------- */
var lastUrl="";
function copyText(t){
  function done(ok){
    var m=$("netMsg");
    if(ok)show(m,T("copied_ok"),true);
    else show(m,T("copy_fail"),false);
  }
  if(navigator.clipboard&&navigator.clipboard.writeText){
    navigator.clipboard.writeText(t).then(function(){done(true);},function(){fallbackCopy(t,done);});
  }else{fallbackCopy(t,done);}
}
function fallbackCopy(t,cb){
  try{
    var ta=document.createElement("textarea");ta.value=t;ta.style.position="fixed";ta.style.opacity="0";
    document.body.appendChild(ta);ta.select();
    var ok=document.execCommand("copy");document.body.removeChild(ta);cb(ok);
  }catch(e){cb(false);}
}

/* ============================================================
   مولد QR مضمن (pure-JS) — وضع Byte، مستوى L، إصدارات 1..5
   ============================================================ */
/* QR حقيقي: مكتبة qrcode-generator (Kazuhiko Arase, MIT) مضمنة محلياً
   في /vendor/qrcode.js — اختيار تلقائي للإصدار والقناع (1-40). */
function drawQR(canvas,text){
  var qr=qrcode(0,"L");
  qr.addData(text);
  qr.make();
  var n=qr.getModuleCount(),quiet=4,scale=8;
  canvas.width=(n+quiet*2)*scale;canvas.height=(n+quiet*2)*scale;
  var ctx=canvas.getContext("2d");
  ctx.fillStyle="#ffffff";ctx.fillRect(0,0,canvas.width,canvas.height);
  ctx.fillStyle="#000000";
  for(var y=0;y<n;y++)for(var x=0;x<n;x++)if(qr.isDark(y,x))ctx.fillRect((x+quiet)*scale,(y+quiet)*scale,scale,scale);
  return {size:n};
}
/* ---------- نافذة إعدادات المالك (Modal — تظهر لصاحب الجهاز فقط) ---------- */
function openSettings(){
  if(!isOwner)return;
  var oz=$("ownerZone");
  if(!oz)return;
  settingsOpen=true;
  oz.classList.remove("hidden");
  try{document.body.classList.add("modal-open");}catch(e){}
  fetchNetStatus();loadConfig();refreshLogs();fetchClients();
  initPhoneHs();
  var c=$("settingsClose");
  if(c){try{c.focus();}catch(e2){}}
  // Focus trap (30 lines): keep Tab inside modal, return focus on close.
  try{
    if(!oz.dataset.trapped){
      oz.dataset.trapped="1";
      oz.addEventListener("keydown",function(e){
        if(e.key!=="Tab"||!settingsOpen)return;
        var f=oz.querySelectorAll('button,[href],input,select,textarea,[tabindex]:not([tabindex="-1"])');
        f=Array.prototype.filter.call(f,function(el){return !el.disabled&&el.offsetParent!==null;});
        if(!f.length)return;
        var first=f[0],last=f[f.length-1];
        if(e.shiftKey&&document.activeElement===first){e.preventDefault();last.focus();}
        else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first.focus();}
      });
    }
  }catch(e){}
}
function closeSettings(){
  settingsOpen=false;
  var oz=$("ownerZone");
  if(oz)oz.classList.add("hidden");
  try{document.body.classList.remove("modal-open");}catch(e){}
  var sb=$("settingsBtn");
  if(sb){try{sb.focus();}catch(e2){}}
}
function switchSettingsTab(name){
  var btns=document.querySelectorAll(".tabs button");
  for(var i=0;i<btns.length;i++)btns[i].classList.toggle("active",btns[i].getAttribute("data-tab")===name);
  var panels=document.querySelectorAll(".tab-panel");
  for(var k=0;k<panels.length;k++)panels[k].classList.toggle("active",panels[k].id===name);
}
var ICONS={dl:'<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>',bolt:'<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>',trash:'<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>',pause:'<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>',play:'<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polygon points="5 3 19 12 5 21 5 3"/></svg>',x:'<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>',moon:'<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>',sun:'<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>',power:'<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18.36 6.64a9 9 0 1 1-12.73 0"/><line x1="12" y1="2" x2="12" y2="12"/></svg>'};
ICONS.qr='<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><path d="M14 14h3v3h-3zM21 14v.01M14 21v.01M18 18h3v3h-3z"/></svg>';
function setBtn(el,icon,label){el.innerHTML=icon;var s=document.createElement("span");s.textContent=label;el.appendChild(s);}
/* ---------- الثيم ---------- */
function applyTheme(t){if(t!=="dark")t="light";document.documentElement.setAttribute("data-theme",t);try{localStorage.setItem("beam-theme",t);}catch(e){}var b=$("themeBtn");if(b)setBtn(b,t==="dark"?ICONS.sun:ICONS.moon,t==="dark"?T("theme_light"):T("theme_dark"));}
function toggleTheme(){var cur=document.documentElement.getAttribute("data-theme");applyTheme(cur==="light"?"dark":"light");}
function initThemeBtn(){var b=$("themeBtn");if(!b)return;b.onclick=toggleTheme;var cur="light";try{cur=localStorage.getItem("beam-theme")||"light";}catch(e){}if(cur!=="dark")cur="light";applyTheme(cur);var lb=$("langBtn");if(lb)lb.onclick=function(){setLang(LANG==="ar"?"en":"ar");};}

/* Unified polling guards: never poll when tab hidden; clients/logs only
   when owner modal open (saves radio + server forks on cafés). */
function shouldPoll(){if(document.hidden||serverGone)return false;return true;}
function shouldPollClients(){if(!shouldPoll())return false;if(activeUploads>0||activeDownloads>0)return false;return settingsOpen&&isOwner;}
