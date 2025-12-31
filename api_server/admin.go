package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func md5HexLower(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (s *Server) routesAdmin(mux *http.ServeMux) {
	mux.HandleFunc("/admin", s.handleAdminPage)
	mux.HandleFunc("/admin/api_keys/add", s.handleAdminAddAPIKey)
	mux.HandleFunc("/admin/devices/import", s.handleAdminImportDevices)
	mux.HandleFunc("/admin/cookies/import", s.handleAdminImportCookies)
	mux.HandleFunc("/admin/cookies/clear", s.handleAdminClearCookies)
	mux.HandleFunc("/admin/cookies/stats", s.handleAdminCookiesStats)
	mux.HandleFunc("/admin/pools/stats", s.handleAdminPoolsStats)
}

func (s *Server) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeHTML(w, http.StatusOK, adminHTML)
}

func (s *Server) handleAdminAddAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeHTML(w, http.StatusBadRequest, "invalid form")
		return
	}

	if s.cfg.AdminPasswordMD5 == "" {
		writeHTML(w, http.StatusInternalServerError, "ADMIN_PASSWORD_MD5 not set")
		return
	}

	pass := r.FormValue("password")
	if md5HexLower(pass) != s.cfg.AdminPasswordMD5 {
		writeHTML(w, http.StatusUnauthorized, "invalid password")
		return
	}

	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	merchant := strings.TrimSpace(r.FormValue("merchant_name"))
	creditDeltaStr := strings.TrimSpace(r.FormValue("credit_delta"))
	if apiKey == "" {
		writeHTML(w, http.StatusBadRequest, "api_key is required")
		return
	}
	delta, err := strconv.ParseInt(creditDeltaStr, 10, 64)
	if err != nil || delta <= 0 {
		writeHTML(w, http.StatusBadRequest, "credit_delta must be > 0")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.repo.UpsertAPIKeyAddCredit(ctx, apiKey, merchant, delta); err != nil {
		writeHTML(w, http.StatusInternalServerError, "db error: "+err.Error())
		return
	}
	// 写入 redis 永久缓存（新增/更新后立即回填）
	_ = s.refreshAPIKeyCache(r.Context(), apiKey)

	writeHTML(w, http.StatusOK, "ok")
}

const adminHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1"/>
  <title>Admin</title>
  <style>
    body { font-family: -apple-system,BlinkMacSystemFont,Segoe UI,Roboto,Helvetica,Arial; background:#0b1220; color:#e6edf3; margin:0; }
    .wrap { max-width: 820px; margin: 40px auto; padding: 0 16px; }
    .card { background:#111a2e; border:1px solid #22304f; border-radius:12px; padding:18px; }
    .grid { display:grid; grid-template-columns: 1fr; gap:14px; }
    h1 { margin: 0 0 10px; font-size: 20px; }
    h2 { margin: 0 0 10px; font-size: 16px; }
    p { margin: 0 0 14px; color:#9fb0d0; }
    label { display:block; font-size:12px; color:#9fb0d0; margin:12px 0 6px; }
    input { width:100%; padding:10px 12px; border-radius:10px; border:1px solid #2a3a61; background:#0b1326; color:#e6edf3; }
    textarea { width:100%; min-height:120px; padding:10px 12px; border-radius:10px; border:1px solid #2a3a61; background:#0b1326; color:#e6edf3; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
    select { width:100%; padding:10px 12px; border-radius:10px; border:1px solid #2a3a61; background:#0b1326; color:#e6edf3; }
    button { margin-top:16px; padding:10px 14px; border-radius:10px; border:1px solid #2a3a61; background:#1b4bff; color:white; cursor:pointer; }
    code { background:#0b1326; padding:2px 6px; border-radius:6px; border:1px solid #2a3a61; }
    .row { display:flex; gap:10px; align-items:center; flex-wrap:wrap; }
    .small { font-size:12px; color:#9fb0d0; }
    .out { white-space: pre-wrap; background:#0b1326; border:1px solid #2a3a61; border-radius:10px; padding:10px 12px; color:#e6edf3; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; min-height:64px; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="grid">
      <div class="card">
        <h1>Admin</h1>
        <p class="small">所有写操作都需要提供 <code>password</code>，后端用 <code>MD5(password)</code> 与 <code>ADMIN_PASSWORD_MD5</code> 比对。</p>
      </div>

      <div class="card">
        <h2>池子统计（分库）</h2>
        <p class="small">自动按 <code>REDIS_DEVICE_POOL_SHARDS</code> / <code>REDIS_COOKIE_POOL_SHARDS</code> 展示每个池子的数量。</p>
        <div class="row">
          <button id="poolRefresh" type="button">刷新</button>
          <span class="small" id="poolHint"></span>
        </div>
        <div id="poolOut" class="out"></div>
      </div>

      <div class="card">
        <h2>新增/追加 API Key 额度</h2>
        <form method="post" action="/admin/api_keys/add">
          <label>管理员密码</label>
          <input name="password" type="password" autocomplete="current-password" required />

          <label>API Key</label>
          <input name="api_key" type="text" placeholder="例如: YOUR_KEY" required />

          <label>商家名称（可选）</label>
          <input name="merchant_name" type="text" placeholder="例如: merchant_a" />

          <label>增加额度（credit_delta，必须 > 0）</label>
          <input name="credit_delta" type="number" min="1" step="1" value="1000" required />

          <button type="submit">提交</button>
        </form>
      </div>

      <div class="card">
        <h2>批量导入设备到 Redis</h2>
        <p class="small">支持文件上传或粘贴 JSONL（每行一个设备 JSON）。会按 <code>REDIS_DEVICE_POOL_SHARDS</code> 自动分配到各个设备池。模式：<code>overwrite</code>=全覆盖；<code>evict</code>=按淘汰策略腾位置。若容量不足，会在结果里返回“剩余设备”。</p>
        <form id="devForm">
          <label>管理员密码</label>
          <input name="password" type="password" autocomplete="current-password" required />

          <label>导入模式</label>
          <select name="mode">
            <option value="evict">evict（按淘汰策略）</option>
            <option value="overwrite">overwrite（全覆盖）</option>
          </select>

          <label>设备文件（可选，优先于下方文本）</label>
          <input name="devices_file" type="file" accept=".txt,.json,.jsonl,text/plain,application/json" />

          <label>设备 JSONL（每行一个 JSON）</label>
          <textarea name="devices" placeholder='{"cdid":"...","create_time":"2025-12-31 01:11:00", ...}' required></textarea>

          <button type="submit">导入到 Redis</button>
        </form>
        <div class="row" style="margin-top:10px;">
          <button id="devCopyRemain" type="button">复制剩余设备</button>
          <span class="small" id="devRemainHint"></span>
        </div>
        <div id="devOut" class="out"></div>
      </div>

      <div class="card">
        <h2>批量导入 Cookies 到 Redis（startUp cookie 池）</h2>
        <p class="small">支持文件上传或粘贴（每行一条）。会按 <code>REDIS_COOKIE_POOL_SHARDS</code> 自动分配到各个 cookies 池。格式：<code>k=v; k2=v2</code> 或 JSON <code>{"k":"v"}</code>。</p>
        <form id="ckForm">
          <label>管理员密码</label>
          <input name="password" type="password" autocomplete="current-password" required />

          <label>导入模式</label>
          <select name="mode">
            <option value="append">append（追加）</option>
            <option value="evict">evict（按使用次数最大淘汰）</option>
            <option value="overwrite">overwrite（全覆盖）</option>
          </select>

          <label>Cookies 文件（可选，优先于下方文本）</label>
          <input name="cookies_file" type="file" accept=".txt,.json,.jsonl,text/plain,application/json" />

          <label>Cookies（每行一条）</label>
          <textarea name="cookies" placeholder='sessionid=...; sid_tt=...&#10;{"sessionid":"...","sid_tt":"..."}' required></textarea>

          <button type="submit">导入到 Redis</button>
        </form>
        <div class="row" style="margin-top:10px;">
          <button id="ckClear" type="button">清空 Redis Cookies</button>
          <span class="small" id="ckStat"></span>
        </div>
        <div id="ckOut" class="out"></div>
      </div>
    </div>
  </div>

<script>
async function postForm(url, formEl) {
  const fd = new FormData(formEl);
  const resp = await fetch(url, { method: "POST", body: fd });
  const text = await resp.text();
  let data = null;
  try { data = JSON.parse(text); } catch (e) {}
  return { ok: resp.ok, status: resp.status, text, data };
}

// pool stats
const poolOut = document.getElementById("poolOut");
const poolHint = document.getElementById("poolHint");
async function refreshPools() {
  poolOut.textContent = "loading...";
  const resp = await fetch("/admin/pools/stats");
  const t = await resp.text();
  try {
    const d = JSON.parse(t);
    poolOut.textContent = JSON.stringify(d, null, 2);
    poolHint.textContent = "ok";
  } catch (e) {
    poolOut.textContent = t;
    poolHint.textContent = "error";
  }
}
document.getElementById("poolRefresh").addEventListener("click", refreshPools);
setInterval(refreshPools, 5000);
refreshPools();

// devices
const devForm = document.getElementById("devForm");
const devOut = document.getElementById("devOut");
const devCopyRemain = document.getElementById("devCopyRemain");
const devRemainHint = document.getElementById("devRemainHint");
let devRemaining = "";
devForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  devOut.textContent = "loading...";
  devRemainHint.textContent = "";
  devRemaining = "";
  const r = await postForm("/admin/devices/import", devForm);
  if (r.data) {
    devOut.textContent = JSON.stringify(r.data, null, 2);
    if (Array.isArray(r.data.remaining_devices) && r.data.remaining_devices.length > 0) {
      devRemaining = r.data.remaining_devices.join("\n");
      devRemainHint.textContent = "剩余设备: " + r.data.remaining_devices.length + " 条";
    } else {
      devRemainHint.textContent = "剩余设备: 0 条";
    }
  } else {
    devOut.textContent = r.text;
  }
});
devCopyRemain.addEventListener("click", async () => {
  if (!devRemaining) { alert("没有剩余设备"); return; }
  await navigator.clipboard.writeText(devRemaining);
  alert("已复制");
});

// cookies
async function refreshCookieStat() {
  const resp = await fetch("/admin/cookies/stats");
  const t = await resp.text();
  try {
    const d = JSON.parse(t);
    document.getElementById("ckStat").textContent = "当前内存 cookies: " + d.count;
  } catch(e) {}
}
const ckForm = document.getElementById("ckForm");
const ckOut = document.getElementById("ckOut");
ckForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  ckOut.textContent = "loading...";
  const r = await postForm("/admin/cookies/import", ckForm);
  ckOut.textContent = r.data ? JSON.stringify(r.data, null, 2) : r.text;
  refreshCookieStat();
});
document.getElementById("ckClear").addEventListener("click", async () => {
  const pass = prompt("输入管理员密码以清空：");
  if (!pass) return;
  const fd = new FormData();
  fd.append("password", pass);
  const resp = await fetch("/admin/cookies/clear", { method: "POST", body: fd });
  const t = await resp.text();
  ckOut.textContent = t;
  refreshCookieStat();
});
refreshCookieStat();
</script>
</body>
</html>`

func writeHTML(w http.ResponseWriter, code int, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(html))
}


