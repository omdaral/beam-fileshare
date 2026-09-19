/* Beam web API layer — transport (fetch/XHR), retry sleep, UUID, SHA-256. */
function sleep(ms){return new Promise(function(r){setTimeout(r,ms);});}
function genUuid(){
  try{
    var b=new Uint8Array(16);(window.crypto||window.msCrypto).getRandomValues(b);
    var s="";for(var i=0;i<b.length;i++)s+=("0"+b[i].toString(16)).slice(-2);
    return s;
  }catch(e){
    // احتياطي hex فقط (السيرفر يرفض أي حرف خارج [0-9a-zA-Z_-])
    var h="";for(var k=0;k<32;k++)h+="0123456789abcdef"[Math.floor(Math.random()*16)];
    return h;
  }
}
function postJSON(url,obj){
  return fetch(url,{method:"POST",headers:ownerHeaders({"Content-Type":"application/json","X-Lang":(typeof LANG!=="undefined"?LANG:"ar")}),body:JSON.stringify(obj)}).then(function(r){
    return r.json().then(function(j){return {status:r.status,body:j};}).catch(function(){return {status:r.status,body:null};});
  });
}
/* Owner pairing — session token proved once (loopback/native), then stored
   in THIS browser and presented on every admin call, so any URL on any
   paired browser is recognized as owner. Server stays source of truth
   (is_admin comes back in every response). Token lives in headers only,
   never in URLs/history. */
function ownerTokenGet(){try{return localStorage.getItem("beam-owner")||"";}catch(e){return "";}}
function ownerTokenSet(t){try{if(t&&/^[0-9a-f]{64}$/.test(t))localStorage.setItem("beam-owner",t);}catch(e){}}
function ownerHeaders(h){
  h=h||{};
  try{var t=ownerTokenGet();if(t)h["X-Beam-Owner"]=t;}catch(e){}
  return h;
}
function ownerTokenLearn(j){
  // Accept the token only from a response that already proves ownership.
  try{if(j&&j.is_admin&&j.owner_token)ownerTokenSet(String(j.owner_token));}catch(e){}
}
function xhrBin(url,blob,onAbort){
  return new Promise(function(resolve){
    var xhr=new XMLHttpRequest();xhr.open("POST",url);
    xhr.timeout=25000;
    if(onAbort)onAbort(function(){try{xhr.abort();}catch(e){}});
    xhr.onload=function(){
      var j=null;try{j=JSON.parse(xhr.responseText);}catch(e){}
      resolve({status:xhr.status,body:j});
    };
    xhr.onerror=function(){resolve({status:0,body:null});};
    xhr.ontimeout=function(){resolve({status:0,body:null,timeout:true});};
    xhr.onabort=function(){resolve({status:-1,body:null});};
    try{xhr.send(blob);}catch(e){resolve({status:0,body:null});}
  });
}
/* SHA-256 مضمن وسريع (~40MB/s — يعمل على أي متصفح حتى http غير المشفر).
   واجهة: SHA256.hex(bytes) و SHA256.create() مع update(bytes)/hex()،
   و shaU8(u8)‎ التي تستخدم crypto.subtle الأصلي حيث يتوفر وتسقط للpure-JS.
   bytes: Uint8Array أو Array أرقام. متحقق منه آلياً ضد متجهات معروفة. */
var SHA256=(function(){
  var K=[0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,
  0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,
  0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,
  0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,
  0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,
  0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,
  0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,
  0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2];
  function rotr(x,n){return (x>>>n)|(x<<(32-n));}
  function create(){
    var h=[0x6a09e667,0xbb67ae85,0x3c6ef372,0xa54ff53a,0x510e527f,0x9b05688c,0x1f83d9ab,0x5be0cd19];
    var buf=new Uint8Array(64),bufLen=0,len=0;
    function compressArr(p,o){
      var w=new Array(64),a,b,c,d,e,f,g,hh,t1,t2,i;
      for(i=0;i<16;i++)w[i]=(p[o+i*4]<<24)|(p[o+i*4+1]<<16)|(p[o+i*4+2]<<8)|p[o+i*4+3];
      for(i=16;i<64;i++){
        var s0=rotr(w[i-15],7)^rotr(w[i-15],18)^(w[i-15]>>>3);
        var s1=rotr(w[i-2],17)^rotr(w[i-2],19)^(w[i-2]>>>10);
        w[i]=(w[i-16]+s0+w[i-7]+s1)|0;
      }
      a=h[0];b=h[1];c=h[2];d=h[3];e=h[4];f=h[5];g=h[6];hh=h[7];
      for(i=0;i<64;i++){
        var S1=rotr(e,6)^rotr(e,11)^rotr(e,25);
        var ch=(e&f)^(~e&g);
        t1=(hh+S1+ch+K[i]+w[i])|0;
        var S0=rotr(a,2)^rotr(a,13)^rotr(a,22);
        var maj=(a&b)^(a&c)^(b&c);
        t2=(S0+maj)|0;
        hh=g;g=f;f=e;e=(d+t1)|0;d=c;c=b;b=a;a=(t1+t2)|0;
      }
      h[0]=(h[0]+a)|0;h[1]=(h[1]+b)|0;h[2]=(h[2]+c)|0;h[3]=(h[3]+d)|0;
      h[4]=(h[4]+e)|0;h[5]=(h[5]+f)|0;h[6]=(h[6]+g)|0;h[7]=(h[7]+hh)|0;
    }
    return {
      update:function(bytes){
        var n=bytes.length,pos=0,i,take;
        len+=n;
        if(bufLen>0){
          take=Math.min(64-bufLen,n);
          for(i=0;i<take;i++)buf[bufLen+i]=bytes[i]&255;
          bufLen+=take;pos+=take;
          if(bufLen===64){compressArr(buf,0);bufLen=0;}
        }
        while(pos+64<=n){compressArr(bytes,pos);pos+=64;}
        while(pos<n){buf[bufLen++]=bytes[pos++]&255;}
        return this;
      },
      hex:function(){
        var bitLen=len*8;
        var lo=bitLen>>>0,hi=Math.floor(bitLen/4294967296);
        var tail=new Uint8Array(128),tp=0,i;
        for(i=0;i<bufLen;i++)tail[tp++]=buf[i];
        tail[tp++]=0x80;
        if(tp>56){while(tp<64)tail[tp++]=0;compressArr(tail,0);tp=0;}
        while(tp<56)tail[tp++]=0;
        tail[tp++]=(hi>>>24)&255;tail[tp++]=(hi>>>16)&255;tail[tp++]=(hi>>>8)&255;tail[tp++]=hi&255;
        tail[tp++]=(lo>>>24)&255;tail[tp++]=(lo>>>16)&255;tail[tp++]=(lo>>>8)&255;tail[tp++]=lo&255;
        compressArr(tail,0);
        var s="";
        for(i=0;i<8;i++)s+=("00000000"+(h[i]>>>0).toString(16)).slice(-8);
        return s;
      }
    };
  }
  return {create:create,hex:function(b){return create().update(b).hex();}};
})();
function shaU8(u8){
  try{
    if(window.crypto&&crypto.subtle&&window.isSecureContext){
      return crypto.subtle.digest("SHA-256",u8).then(function(d){
        var v=new Uint8Array(d),s="";
        for(var i=0;i<v.length;i++)s+=("0"+v[i].toString(16)).slice(-2);
        return s;
      }).catch(function(){return SHA256.hex(u8);});
    }
  }catch(e){}
  return Promise.resolve(SHA256.hex(u8));
}

