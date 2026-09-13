/* Beam web network — status card, QR, server stop, hotspot, admin, autosave, BG service. */
/* ---------- كارت حالة الشبكة ---------- */
var qrMode="wifi",lastSsid="",lastSecurity="wpa",lastWifiPass="",netOk=false;
function escapeWifi(s){return String(s).replace(/([\\;,:"'])/g,"\\$1");}
function baseUrl(){return location.protocol+"//"+location.host;}
function fetchStatus(){
  var xhr=new XMLHttpRequest();
  try{xhr.open("GET","/api/status",true);}catch(e){renderStatus(null);return;}
  xhr.timeout=5000;
  xhr.onload=function(){
    if(xhr.status===200){try{
      var j=JSON.parse(xhr.responseText);
      try{if(j&&j.limits&&typeof applyLimits==="function"&&typeof resetPolling==="function"){if(applyLimits(j.limits))resetPolling();}}catch(e){}
      renderStatus(j);return;
    }catch(e){}}
    renderStatus(null);
  };
  xhr.onerror=function(){renderStatus(null);};
  xhr.ontimeout=function(){renderStatus(null);};
  try{xhr.send();}catch(e){renderStatus(null);}
}
var serverDefaultLang="ar",isOwner=false,settingsOpen=false;
function renderStatus(j){
  if(serverGone||!$("netSsid"))return;
  var ssid=(j&&j.ssid)?String(j.ssid):"Beam";
  var url=(j&&j.url)?String(j.url):baseUrl();
  var clients=(j&&typeof j.clients!=="undefined")?String(j.clients):"—";var lan=(j&&j.lan_mode)?T("net_lan"):T("net_hs");
  var running=!j||j.running!==false;
  lastSsid=ssid;lastUrl=url;netOk=running;
  lastSecurity=(j&&j.security)?String(j.security):"wpa";
  lastWifiPass=(j&&j.wifi_password)?String(j.wifi_password):"";
  if(j&&j.default_lang)serverDefaultLang=(j.default_lang==="en")?"en":"ar";
  $("netSsid").textContent=ssid;
  $("netAddr").textContent=url.replace(/^.*:\/\//,"");
  var hero=$("heroUrl");if(hero)hero.textContent=url.replace(/^.*:\/\//,"");
  $("netClients").textContent=clients;
  $("netMode").textContent=running?lan:T("stopped_full");
  var sh=$("netShared");if(sh)sh.textContent=(j&&j.shared_dir)?String(j.shared_dir):"—";
  var pa=$("netPretty");if(pa){var md=(j&&j.mdns_url)?String(j.mdns_url).replace(/^.*:\/\//,""):"";var pl=(j&&j.plain_url)?String(j.plain_url).replace(/^.*:\/\//,""):"";if(md&&pl&&pl!==md){var tag=(typeof LANG!=="undefined"&&LANG==="en")?" (Windows)":" (ويندوز)";pa.textContent=md+" • "+pl+tag;}else pa.textContent=md||pl||"—";}
  var idle=$("idleNote");if(idle)idle.textContent=idleText(j&&typeof j.idle_seconds!=="undefined"?j.idle_seconds:-1);
  var hv=$("heroVer");if(hv)hv.textContent="Beam"+((j&&j.version)?(" v"+j.version):"");
  var sm=$("statSsid");if(sm)sm.textContent=ssid;
  var sc=$("statClients");if(sc)sc.textContent=clients;
  var sd=$("statMode");if(sd)sd.textContent=running?lan:T("stopped_full");
  updateQR();
  if(!running)show($("netMsg"),T("srv_down"),false);
  else clearMsg($("netMsg"));
  var owner=!!(j&&j.is_admin);
  isOwner=owner;
  var sb=$("settingsBtn");
  if(sb)sb.classList.toggle("hidden",!owner);
  var st=$("srvStopTop");
  if(st)st.classList.toggle("hidden",!owner);
  if(!owner&&settingsOpen)closeSettings();
  if(owner){
    fetchNetStatus();loadConfig();refreshLogs();
    var explicit=false;
    try{explicit=localStorage.getItem("beam-lang-set")==="1";}catch(e){}
    if(!explicit&&LANG!==serverDefaultLang)setLang(serverDefaultLang,{explicit:false,refetch:false});
  }else{
    var explicitG=false;
    try{explicitG=localStorage.getItem("beam-lang-set")==="1";}catch(e2){}
    if(!explicitG&&LANG!==serverDefaultLang)setLang(serverDefaultLang,{explicit:false,refetch:false});
  }
}
function qrPayload(){
  if(qrMode==="url")return lastUrl||baseUrl();
  var s=escapeWifi(lastSsid||"Beam");
  if(lastSecurity==="open")return "WIFI:T:nopass;S:"+s+";;";
  if(lastWifiPass)return "WIFI:T:WPA;S:"+s+";P:"+escapeWifi(lastWifiPass)+";;";
  return "WIFI:T:WPA;S:"+s+";;";
}
function updateQR(){
  var canvas=$("qrCanvas"),fb=$("qrFallback");
  try{
    var q=drawQR(canvas,qrPayload());
    if(q.size>177)throw new Error("too-long");
    canvas.style.display="";fb.style.display="none";
  }catch(e){
    canvas.style.display="none";fb.style.display="block";
    var link=$("qrLink");link.textContent=lastUrl||baseUrl();link.href=lastUrl||baseUrl();
    $("qrText").textContent=T("qr_wifi_text")+qrPayload();
  }
  $("qrWifiBtn").className=qrMode==="wifi"?"active":"";
  $("qrUrlBtn").className=qrMode==="url"?"active":"";
}

/* ---------- الإيقاف اليدوي الواضح: زر أحمر بارز في الهيدر + زر الإعدادات ---------- */
function idleText(sec){
  if(typeof sec!=="number"||sec<0)return T("idle_off");
  var h=Math.floor(sec/3600),m=Math.floor((sec%3600)/60);
  if(h>0)return T("idle_prefix")+" "+h+" "+T("idle_h")+" "+T("idle_suffix");
  if(m>0)return T("idle_prefix")+" "+m+" "+T("idle_m")+" "+T("idle_suffix");
  return T("idle_soon");
}
function bindStopBtn(id,msgId){
  var b=$(id);if(!b||b.dataset.bound)return;b.dataset.bound="1";
  b.onclick=function(){
    if(b.dataset.done==="1")return;
    b.dataset.done="1";
    b.classList.add("stopped");
    var m=$(msgId);
    postJSON("/api/server/stop",{}).then(function(res){
      if(res.status===200){onServerStopped(m);}
      else{b.dataset.done="";b.classList.remove("stopped");if(m)show(m,((res.body&&res.body.error)||T("stop_fail2")),false);}
    }).catch(function(){b.dataset.done="";b.classList.remove("stopped");if(m)show(m,T("conn_fail"),false);});
  };
}
/* بعد الإيقاف: اقفل التاب، ولو المتصفح رفض اعرض شاشة توقف بدل الصفحة */
var serverGone=false;
function onServerStopped(m){
  serverGone=true;
  if(m)show(m,T("srv_stopped"),true);
  try{if(window.Capacitor&&Capacitor.registerPlugin){var bs=Capacitor.registerPlugin("BeamServer");var q=bs.stop({});if(q&&q.catch)q.catch(function(){});}}catch(e){}
  try{window.close();}catch(e2){}
  setTimeout(function(){
    if(!$("netSsid"))return;
    try{
      document.title="Beam";
      // XSS-safe: DOM + textContent (never innerHTML with T()).
      var wrap=document.createElement("div");wrap.className="wrap";
      var card=document.createElement("div");card.className="card";
      card.setAttribute("style","text-align:center;padding:48px 20px");
      var h=document.createElement("h2");h.textContent=T("srv_stopped");
      card.appendChild(h);wrap.appendChild(card);
      document.body.textContent="";document.body.appendChild(wrap);
    }catch(e2){}
  },400);
}
function loadLocalSettings(){
  var s=null;
  try{s=JSON.parse(localStorage.getItem("beam-settings")||"null");}catch(e){}
  if(!s||typeof s!=="object")return;
  if(s.port)setField("cfgPort",s.port);
  if(typeof s.ssid==="string"&&s.ssid.trim())setField("admSsid",s.ssid.trim());
  var sel=$("cfgLang");
  if(sel&&(s.default_lang==="ar"||s.default_lang==="en"))sel.value=s.default_lang;
}
function saveLocalSettings(body){
  try{localStorage.setItem("beam-settings",JSON.stringify(body));}catch(e){}
}
/* ---------- هوتسبوت الهاتف (داخل التطبيق الأصلي فقط) ---------- */
function nativeBeam(){try{if(window.Capacitor&&Capacitor.getPlatform()==="android"&&Capacitor.registerPlugin)return{beam:Capacitor.registerPlugin("BeamServer"),hs:Capacitor.registerPlugin("Hotspot")};}catch(e){}return null;}
function initPhoneHs(){var nb=nativeBeam();var row=$("phoneHsRow");if(!nb||!row)return;row.style.display="";var b=$("phoneHsBtn");if(b&&!b.dataset.bound){b.dataset.bound="1";b.onclick=phoneHsStart;}}
function phoneHsStart(){
  var m=$("phoneHsMsg"),nb=nativeBeam();if(!nb)return;
  show(m,T("hs_phone_starting"),true);
  var p;
  try{p=nb.hs.startLocalOnly({});}catch(e){show(m,T("hs_phone_manual"),false);return;}
  p.then(function(r){
    var ssid=r&&r.ssid,pass=r&&r.pass,sec=(r&&r.security)||"wpa";
    if(!ssid){show(m,T("hs_phone_manual"),false);return;}
    try{var q=nb.beam.setHotspotCreds({ssid:ssid,pass:pass,security:sec});if(q&&q.catch)q.catch(function(){});}catch(e2){}
    show(m,T("hs_phone_ok")+" "+ssid,true);
    fetchStatus();fetchNetStatus();
  }).catch(function(){show(m,T("hs_phone_manual"),false);});
}
/* ---------- إدارة الشبكة والجهاز (للمدير فقط — تظهر حسب /api/status) ---------- */
var adminMode="lan",adminOpen=false,adminStickyUntil=0;
/* وضع الشبكة المختار + إظهار الفورم المناسب. sticky=true تمنع الاستطلاع
   الدوري من إعادة الكتابة فوق اختيار المستخدم الصريح لمدة 30 ثانية. */
function setAdminMode(m,sticky){
  adminMode=(m==="hotspot")?"hotspot":"lan";
  if(sticky)adminStickyUntil=Date.now()+30000;
  var lan=adminMode==="lan";
  $("modeLan").className=lan?"active":"";
  $("modeHotspot").className=lan?"":"active";
  var hf=$("hotspotForm");if(hf)hf.style.display=lan?"none":"";
  var ln=$("lanNote");if(ln)ln.style.display=lan?"":"none";
}
function setField(id,val){
  var el=$(id);
  if(!el)return;
  try{if(document.activeElement===el)return;}catch(e){}
  el.value=val;
}
function fetchNetStatus(){
  fetch("/api/net/status").then(function(r){
    if(r.status===403){return null;}
    if(!r.ok)throw new Error("http"+r.status);
    return r.json();
  }).then(function(j){
    if(!j)return;
    // اختيار المستخدم الصريح يبقى لزجاً 30s: لا يعيد الاستطلاع كتابته فوقه
    // (هذا كان سبب "رجوع زر الهوتسبوت للواي فاي" — يُطبق فقط المؤكد من السيرفر).
    var serverMode=j.hotspot_running?"hotspot":(j.lan_mode?"lan":"hotspot");
    if(Date.now()<adminStickyUntil&&serverMode!==adminMode){
      /* keep */
    }else if(serverMode!==adminMode){
      var wasHs=adminMode==="hotspot";
      setAdminMode(serverMode,false);
      if(wasHs&&serverMode==="lan")show($("admMsg"),T("hs_back_lan"),false);
    }else{
      setAdminMode(serverMode,false);
    }
    adminOpen=!!j.wifi_open;
    setField("admSsid",j.ssid||"Beam");
    if(j.wifi_password)setField("admPass",j.wifi_password);
    var ob=$("openCheckBtn");
    var obs=ob.querySelector("span");if(obs)obs.textContent=adminOpen?T("open_on"):T("open_off");
    ob.setAttribute("aria-pressed",adminOpen?"true":"false");
    $("admCaps").textContent="AP: "+(j.hotspot_available?T("yes"):T("no"))+" — "+T("caps_perm")+": "+(j.admin_capable?T("ok_perm"):T("need_admin"));
    renderDevices(j,!!j.hotspot_running);
  }).catch(function(){/* الصمت: اللوحة اختيارية */});
}
function renderDevices(j,hotspotOn){
  var box=$("devList");
  if(!box)return;
  var list=(j&&j.clients)||[];
  box.innerHTML="";
  var arr=(list instanceof Array)?list:[];
  var n=(j&&typeof j.clients_count!=="undefined")?j.clients_count:arr.length;
  $("devCount").textContent=String(n);
  if(!arr.length){
    var p=document.createElement("p");p.className="small";
    p.textContent=hotspotOn?T("dev_empty_hotspot"):T("dev_empty_lan");
    box.appendChild(p);
    }else arr.forEach(function(ip){
      var d=document.createElement("div");d.className="kv";
      var k=document.createElement("span");k.className="k";
      var dot=document.createElement("span");dot.className="dot";k.appendChild(dot);
      k.appendChild(document.createTextNode(T("dev_one")));
      var v=document.createElement("span");v.className="v";v.textContent=String(ip);
      d.appendChild(k);d.appendChild(v);box.appendChild(d);
    });
  var ts=$("devTs");
  if(ts){var t=new Date();ts.textContent=T("dev_ts")+t.toLocaleTimeString(LANG==="ar"?"ar-EG":"en-US",{hour:"2-digit",minute:"2-digit",second:"2-digit"});}
}
function fetchClients(){
  if(activeUploads>0||activeDownloads>0)return;
  fetch("/api/net/clients").then(function(r){
    if(!r.ok)throw new Error("http"+r.status);
    return r.json();
  }).then(function(j){
    renderDevices(j,adminMode==="hotspot");
  }).catch(function(){});
}
function netStart(){
  var m=$("admMsg");
  setAdminMode(adminMode,true); // ثبّت الاختيار أثناء التشغيل حتى لا يعيد الاستطلاع كتابته
  var ssid=$("admSsid").value||"Beam";
  var pass=$("admPass").value||"";
  show(m,T("starting_net"),true);
  postJSON("/api/net/start",{mode:adminMode,ssid:ssid,password:pass,open:adminOpen}).then(function(res){
    if(res.status===200&&res.body&&res.body.ok){show(m,(res.body.msg||res.body.msg_ar||T("done_fallback")),true);}
    else show(m,((res.body&&res.body.error)||T("start_fail")),false);
    fetchStatus();fetchNetStatus();
  }).catch(function(){show(m,T("conn_fail"),false);});
}
function netStop(){
  var m=$("admMsg");
  postJSON("/api/net/stop",{}).then(function(res){
    if(res.status===200)show(m,((res.body&&res.body.msg)||(res.body&&res.body.msg_ar)||T("done_fallback")),true);
    else show(m,((res.body&&res.body.error)||T("stop_fail")),false);
    fetchStatus();fetchNetStatus();
  }).catch(function(){show(m,T("conn_fail"),false);});
}
function refreshLogs(){
  var v=$("logView");
  fetch("/api/logs?tail=200").then(function(r){
    if(!r.ok)throw new Error("http"+r.status);
    return r.json();
  }).then(function(j){
    var lines=(j&&j.lines)||[];
    v.textContent=lines.length?lines.join("\n"):T("log_empty");
  }).catch(function(){v.textContent=T("log_fail");});
}
function loadConfig(){
  loadLocalSettings(); // prefill instantly from this browser's copy
  fetch("/api/config").then(function(r){
    if(r.status===403)return null;
    if(!r.ok)throw new Error("http"+r.status);
    return r.json();
  }).then(function(j){
    if(!j)return;
    var c=(j&&j.config)||{};
    setField("cfgPort",c.port||2004);
    var dl=(c.default_lang==="en")?"en":"ar";
    serverDefaultLang=dl;
    var sel=$("cfgLang");if(sel)sel.value=dl;
    var explicit=false;
    try{explicit=localStorage.getItem("beam-lang-set")==="1";}catch(e){}
    if(!explicit&&LANG!==dl)setLang(dl,{explicit:false,refetch:false});
  }).catch(function(){});
}
/* ---------- حفظ تلقائي: أي تعديل صحيح يُحفظ فوراً بلا زر ---------- */
var cfgTimer=null;
function queueAutoSave(){if(cfgTimer)clearTimeout(cfgTimer);cfgTimer=setTimeout(autoSave,800);}
function autoSave(){
  var m=$("cfgMsg");
  var port=parseInt(($("cfgPort")||{}).value,10);
  if(!(port>=1&&port<=65535)){show(m,T("port_range"),false);return;}
  var sel=$("cfgLang");
  var lang=sel?sel.value:"ar";
  var ssid=(($("admSsid")||{}).value||"").trim();
  if(!ssid){show(m,T("ssid_required"),false);return;}
  var pass=(($("admPass")||{}).value)||"";
  if(adminMode==="hotspot"&&!adminOpen&&pass.length<8){show(m,T("pass_short"),false);return;}
  var body={port:port,default_lang:lang,ssid:ssid,password:pass,open:adminOpen};
  postJSON("/api/config",body).then(function(res){
    if(res.status===200&&res.body&&res.body.ok){
      var t=new Date();
      var hh=("0"+t.getHours()).slice(-2),mm=("0"+t.getMinutes()).slice(-2);
      show(m,T("autosaved")+" "+hh+":"+mm,true);
      saveLocalSettings(body); // keep a copy in this browser for next run
      if(sel)serverDefaultLang=(sel.value==="en")?"en":"ar";
    }
    else show(m,((res.body&&res.body.error)||T("save_fail")),false);
  }).catch(function(){show(m,T("conn_fail"),false);});
}
/* ---------- خدمة خلفية الأندرويد (BG): no-op كامل على الويب ---------- */
// تعمل فقط داخل غلاف Capacitor على أندرويد: خدمة أمامية بإشعار دائم تُبقي
// عملية الرفع حية والشاشة مقفولة. على المتصفح/الديسكتوب لا تفعل شيئاً إطلاقاً.
// BG-FALLBACK: لو جمّدت WebView المؤقتات في الخلفية رغم الخدمة على بعض
// الأجهزة، الحل بلجن محلي صغير يستدعي WebView.resumeTimers() (راجع mobile/SETUP.md).
var bgOn=false,bgLastActive=0;
var isLocalServer=(location.hostname==="127.0.0.1"||location.hostname==="localhost");
var bgWakeLock=null; // Screen Wake Lock: يمنع نوم الشاشة أثناء النقل (متصفح + تطبيق)
function bgWake(on){
  try{
    if(on&&"wakeLock" in navigator&&!bgWakeLock){
      navigator.wakeLock.request("screen").then(function(s){bgWakeLock=s;},function(){});
    }else if(!on&&bgWakeLock){
      try{bgWakeLock.release();}catch(e){}
      bgWakeLock=null;
    }
  }catch(e){}
}
document.addEventListener("visibilitychange",function(){if(!document.hidden&&bgOn)bgWake(true);});
function bgPlugin(){try{if(window.Capacitor&&Capacitor.getPlatform()==="android"&&Capacitor.registerPlugin)return Capacitor.registerPlugin("ForegroundService");}catch(e){}return null;}
function bgKick(){bgLastActive=Date.now();bgWake(true);if(bgOn)return;var s=bgPlugin();if(!s)return;try{var p=s.startForegroundService({title:"Beam يرفع…",body:"الرفع مستمر في الخلفية"});if(p&&p.catch)p.catch(function(){});bgOn=true;}catch(e){}}
function bgIdleCheck(){if(!bgOn&&!bgWakeLock)return;if(Date.now()-bgLastActive<30000)return;bgWake(false);if(isLocalServer)return; // السيرفر يعمل هنا — لا توقف الخدمة أبداً
var s=bgPlugin();bgOn=false;if(!s)return;try{var q=s.stopForegroundService();if(q&&q.catch)q.catch(function(){});}catch(e){}}
