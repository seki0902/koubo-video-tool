// app.js - Shared API client, routing, and utilities

const API = {
  async get(url) { const r = await fetch(url); if (!r.ok) throw new Error(await r.text()); return r.json(); },
  async post(url, data) { const r = await fetch(url, { method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(data) }); if (!r.ok) throw new Error(await r.text()); return r.json(); },
  async put(url, data) { const r = await fetch(url, { method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify(data) }); if (!r.ok) throw new Error(await r.text()); return r.json(); }
};

function savePref(k,v) { try { localStorage.setItem('koubo_'+k, JSON.stringify(v)); } catch(e) {} }
function loadPref(k,d) { try { var v = localStorage.getItem('koubo_'+k); return v ? JSON.parse(v) : d; } catch(e) { return d; } }

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
      var div = document.createElement('div');
      div.innerHTML = await tut.text();
      document.body.appendChild(div);
      if (typeof initMain === 'function') initMain();
    }
  } catch(e) {
    document.getElementById('app').innerHTML = '<div class="page"><div class="page-title">连接失败</div><p style="color:#86868b;">无法连接到本地服务，请确认工具正在运行。</p><p style="color:#86868b;font-size:12px;">' + e.message + '</p></div>';
  }
}

document.addEventListener('DOMContentLoaded', loadApp);
