// app.js - Shared API client, routing, and utilities

const API = {
  async get(url) { const r = await fetch(url); if (!r.ok) throw new Error(await r.text()); return r.json(); },
  async post(url, data) { const r = await fetch(url, { method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(data) }); if (!r.ok) throw new Error(await r.text()); return r.json(); },
  async put(url, data) { const r = await fetch(url, { method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify(data) }); if (!r.ok) throw new Error(await r.text()); return r.json(); },
  async del(url) { const r = await fetch(url, { method:'DELETE' }); if (!r.ok) throw new Error(await r.text()); return r.json(); }
};

function savePref(k,v) { try { localStorage.setItem('koubo_'+k, JSON.stringify(v)); } catch(e) {} }
function loadPref(k,d) { try { var v = localStorage.getItem('koubo_'+k); return v ? JSON.parse(v) : d; } catch(e) { return d; } }

// ─── 加密 localStorage（用于密钥等敏感字段）───
var SecureStore = (function() {
  var SALT = new Uint8Array([0x6b,0x6f,0x75,0x62,0x6f,0x2d,0x76,0x31]); // "koubo-v1"
  var KEY_PASSWORD = 'koubo-video-tool-local-enc-key';

  function _keyMaterial() {
    return crypto.subtle.importKey(
      'raw', new TextEncoder().encode(KEY_PASSWORD), 'PBKDF2', false, ['deriveKey']
    ).then(function(baseKey) {
      return crypto.subtle.deriveKey(
        { name: 'PBKDF2', salt: SALT, iterations: 100000, hash: 'SHA-256' },
        baseKey, { name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt']
      );
    });
  }

  var _keyPromise = null;
  function _getKey() {
    if (!_keyPromise) _keyPromise = _keyMaterial();
    return _keyPromise;
  }

  async function setItem(key, value) {
    try {
      var k = await _getKey();
      var iv = crypto.getRandomValues(new Uint8Array(12));
      var enc = await crypto.subtle.encrypt(
        { name: 'AES-GCM', iv: iv },
        k,
        new TextEncoder().encode(value)
      );
      var bundle = {
        iv: btoa(String.fromCharCode.apply(null, iv)),
        data: btoa(String.fromCharCode.apply(null, new Uint8Array(enc)))
      };
      localStorage.setItem('secure_' + key, JSON.stringify(bundle));
    } catch(e) {}
  }

  async function getItem(key) {
    try {
      var raw = localStorage.getItem('secure_' + key);
      if (!raw) return '';
      var bundle = JSON.parse(raw);
      var k = await _getKey();
      var iv = new Uint8Array(atob(bundle.iv).split('').map(function(c){return c.charCodeAt(0)}));
      var data = new Uint8Array(atob(bundle.data).split('').map(function(c){return c.charCodeAt(0)}));
      var dec = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: iv }, k, data);
      return new TextDecoder().decode(dec);
    } catch(e) { return ''; }
  }

  async function removeItem(key) {
    localStorage.removeItem('secure_' + key);
  }

  return { setItem: setItem, getItem: getItem, removeItem: removeItem };
})();

function toast(msg) {
  var t = document.getElementById('toast') || (function(){
    var el = document.createElement('div'); el.id='toast'; el.className='toast'; document.body.appendChild(el); return el;
  })();
  t.textContent = msg; t.classList.add('show');
  clearTimeout(t._tid); t._tid = setTimeout(function(){ t.classList.remove('show'); }, 2000);
}

function radioGroup(selector, container) {
  container.querySelectorAll(selector).forEach(function(el){
    el.addEventListener('click', function(){
      container.querySelectorAll(selector).forEach(function(s){ s.classList.remove('on'); });
      el.classList.add('on');
    });
  });
}

// 加载页面 HTML 并执行其中的 <script>（因为 innerHTML 不会执行脚本）
async function loadPage(url) {
  var app = document.getElementById('app');
  var resp = await fetch(url);
  var html = await resp.text();
  // 提取所有 <script> 块，从 HTML 中移除
  var scripts = [];
  html = html.replace(/<script>([\s\S]*?)<\/script>/g, function(_, code) {
    scripts.push(code);
    return '';
  });
  app.innerHTML = html;
  // 创建真正的 <script> 元素并执行
  scripts.forEach(function(code) {
    var el = document.createElement('script');
    el.textContent = code;
    document.body.appendChild(el);
  });
}

// Page routing
async function loadApp() {
  try {
    var cfg = await API.get('/api/settings');
    var hasChanjing = cfg.chanjing && cfg.chanjing.app_id;

    if (!hasChanjing) {
      await loadPage('/setup.html');
      if (typeof initSetup === 'function') initSetup();
    } else {
      await loadPage('/main.html');
      // Load tutorial overlay
      var tut = await fetch('/tutorial.html');
      var tutHtml = await tut.text();
      var tutScripts = [];
      tutHtml = tutHtml.replace(/<script>([\s\S]*?)<\/script>/g, function(_, code) { tutScripts.push(code); return ''; });
      var div = document.createElement('div');
      div.innerHTML = tutHtml;
      document.body.appendChild(div);
      tutScripts.forEach(function(code) { var el = document.createElement('script'); el.textContent = code; document.body.appendChild(el); });
      if (typeof initMain === 'function') initMain();
    }
  } catch(e) {
    document.getElementById('app').innerHTML = '<div class="page"><div class="page-title">连接失败</div><p style="color:#86868b;">无法连接到本地服务，请确认工具正在运行。</p><p style="color:#86868b;font-size:12px;">' + e.message + '</p></div>';
  }
}

document.addEventListener('DOMContentLoaded', loadApp);
