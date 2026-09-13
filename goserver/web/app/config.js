/* Beam web config — central tunables (values may be refreshed from /api/status limits). */
"use strict";
/* Beam CONFIG — حدود مركزية (بدون bundler) */
var CONFIG={SEARCH_MAX:200, ZIP_MAX_DESKTOP:1*1024*1024*1024, ZIP_MAX_MOBILE:200*1024*1024, DL_FAST_WARN:200*1024*1024, POLL_STATUS:10000, POLL_CLIENTS:5000, POLL_FILES:5000};
function isMobileUI(){try{if(window.matchMedia&&matchMedia("(max-width:640px)").matches)return true;var ua=navigator.userAgent||"";return /Android|iPhone|iPad|Mobile/i.test(ua);}catch(e){return false;}}
/* applyLimits refreshes CONFIG from /api/status limits (single source:
   the Go backend). Upload/download tunables follow when present.
   Returns true when a polling interval changed (caller resets timers). */
function applyLimits(l){
  if(!l||typeof l!=="object")return false;
  var changed=false;
  function setNum(k,v){if(typeof v==="number"&&isFinite(v)&&v>0&&CONFIG[k]!==v){CONFIG[k]=v;changed=true;}}
  setNum("POLL_STATUS",l.poll_status_ms);
  setNum("POLL_CLIENTS",l.poll_clients_ms);
  setNum("POLL_FILES",l.poll_files_ms);
  setNum("SEARCH_MAX",l.search_max);
  if(typeof l.piece_default==="number"&&l.piece_default>0)PIECE_LEN=l.piece_default;
  if(typeof l.piece_max==="number"&&l.piece_max>0)TURBO_PIECE=Math.min(l.piece_max,8*1024*1024);
  return changed;
}
