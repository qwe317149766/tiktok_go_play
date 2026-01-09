<script setup>
import { ref, onMounted, reactive, computed } from 'vue'

// Localization Data
const i18n = {
  en: {
    loginTitle: "Authentication",
    loginBtn: "Unlock System",
    files: "Configuration Assets",
    acc: "Account File",
    target: "Target Matrix",
    proxy: "Proxy Pipeline",
    settings: "Runtime Parameters",
    title: "Thread Title",
    msg: "Message Body",
    grpSize: "Group Size",
    conc: "Concurrent Threads",
    retry: "Fault Tolerance",
    maxDM: "Global Limit",
    interval: "Pulse Delay",
    start: "Start Mission",
    stop: "Abort Mission",
    expiry: "Service Expiry: ",
    proxyModeFile: "ASSET FILE",
    proxyModeDirect: "MANUAL INJECT",
    accStats: "Health Monitor",
    tabTask: "Command Center",
    tabSettings: "Parameters",
    tabLogs: "Detailed Logs",
    tabData: "Data Management",
    maxDMPerAcc: "Unit Limit"
  },
  zh: {
    loginTitle: "系统身份验证",
    loginBtn: "点击进入系统",
    files: "基础资源配置",
    acc: "账号资产文件",
    target: "目标地址矩阵",
    proxy: "代理通讯链路",
    settings: "运行策略设置",
    title: "私信显示标题",
    msg: "私信内容模板",
    grpSize: "群组载荷人数",
    conc: "工作执行线程",
    retry: "异常重试机制",
    maxDM: "任务分发上限",
    interval: "下发频率(ms)",
    start: "开启自动化执行",
    stop: "停止正在运行",
    expiry: "授权有效期: ",
    proxyModeFile: "文件读取",
    proxyModeDirect: "手动输入",
    accStats: "健康状态实时监控",
    tabTask: "控制面板",
    tabSettings: "参数配置",
    tabLogs: "时序日志",
    tabData: "数据管理",
    maxDMPerAcc: "单号载荷上限"
  }
}

// State
const isLoggedIn = ref(false)
const currentLang = ref('zh') 
const loginMsg = ref('')
const expiryInfo = ref('')
const logs = ref([])
const cardCode = ref('')
const proxyMode = ref('file')
const activeTab = ref('task')
const isRunning = ref(false)
const isStopping = ref(false)

const config = reactive({
  account_file: '',
  target_file: '',
  proxy_file: '',
  proxy_content: '',
  thread_title: '',
  msg_content: '',
  group_min: 3,
  group_max: 5,
  concurrency: 10,
  retry_count: 3,
  max_dm_count: 0,
  max_dm_per_account: 0,
  interval: 1000
})

const stats = reactive({
  success: 0,
  failure: 0,
  loggedOut: 0,
  banned: 0,
  risk: 0,
  corrupted: 0,
  remaining: 0
})

const t = computed(() => i18n[currentLang.value] || i18n.en)

// Data Management State
const currentDir = ref('')
const filesInDir = ref([])

// Methods
function changeLang(lang) { currentLang.value = lang }

async function doLogin() {
  if (!cardCode.value) return
  loginMsg.value = "鉴权中..."
  try {
    const res = await window.go.main.App.CheckLogin(cardCode.value)
    if (res.success) {
      isLoggedIn.value = true
      expiryInfo.value = res.expiry
    } else {
      loginMsg.value = res.error || "Login Failed"
    }
  } catch (e) { loginMsg.value = "Err: " + e }
}

async function selectFile(field) {
  try {
    const path = await window.go.main.App.SelectFile("Select File")
    if (path) {
      if (field === 'acc') config.account_file = path
      if (field === 'target') config.target_file = path
      if (field === 'proxy') config.proxy_file = path
      saveConfig()
    }
  } catch(e) {}
}

async function startTask() {
  if (isRunning.value) return
  await saveConfig()
  Object.keys(stats).forEach(k => stats[k] = 0)
  isRunning.value = true
  isStopping.value = false
  const params = {
    ...config,
    proxy_file: proxyMode.value === 'file' ? config.proxy_file : '',
    proxy_content: proxyMode.value === 'direct' ? config.proxy_content : '',
    group_min: parseInt(config.group_min),
    group_max: parseInt(config.group_max),
    concurrency: parseInt(config.concurrency),
    retry_count: parseInt(config.retry_count),
    max_dm_count: parseInt(config.max_dm_count),
    max_dm_per_account: parseInt(config.max_dm_per_account),
    interval: parseInt(config.interval)
  }
  try {
    const res = await window.go.main.App.StartTask(params)
    addLog(`[SYSTEM] Started: ${res}`)
  } catch(e) {
    addLog(`[SYSTEM] Start Error: ${e}`)
    isRunning.value = false
  }
}

async function stopTask() {
  if (isStopping.value || !isRunning.value) return
  isStopping.value = true
  addLog(`[SYSTEM] Shutting down...`)
  try { await window.go.main.App.StopTask() } catch(e) {}
}

async function loadDir(dir) {
  currentDir.value = dir
  try {
    const res = await window.go.main.App.ListFiles(dir)
    filesInDir.value = res ? res.filter(f => f.name.toLowerCase().endsWith('.txt')) : []
  } catch(e) {}
}

async function deleteFile(path) {
  if (!confirm("Delete file?")) return
  try {
    const ok = await window.go.main.App.DeleteFile(path)
    if (ok) loadDir(currentDir.value)
  } catch(e) {}
}

async function openFile(path) {
  try { await window.go.main.App.OpenFile(path) } catch(e) {}
}

async function saveConfig() {
  try {
    await window.go.main.App.SaveConfig({
      ...config,
      language: currentLang.value,
      card_code: cardCode.value
    })
  } catch(e) {}
}

function addLog(msg) {
  logs.value.push(msg)
  if (logs.value.length > 500) logs.value.shift()
  setTimeout(() => {
    const el = document.getElementById('log-box')
    if (el) el.scrollTop = el.scrollHeight
  }, 20)
}

onMounted(async () => {
  if (window.runtime) {
    window.runtime.EventsOn("log", d => addLog(`[${d.time}] ${d.msg}`))
    window.runtime.EventsOn("stats", d => {
      Object.keys(d).forEach(k => { if (stats[k] !== undefined) stats[k] = d[k] })
    })
    window.runtime.EventsOn("stopped", () => {
      isRunning.value = false
      isStopping.value = false
      addLog(`[SYSTEM] Task Halted`)
    })
  }
  try {
    const conf = await window.go.main.App.GetConfig()
    if (conf) {
      if (conf.language) currentLang.value = conf.language.substring(0,2)
      Object.keys(config).forEach(k => { if (conf[k] !== undefined) config[k] = conf[k] })
      cardCode.value = conf.card_code || ""
      if (cardCode.value) doLogin()
    }
  } catch(e) {}
})
</script>

<template>
  <div class="app-shell">
    <div class="top-nav" v-if="isLoggedIn">
      <div class="nav-branding">INSTAGRAM MONITOR <span>ELITE</span></div>
      <div class="nav-extra">
         <span class="expiry-tag" v-if="expiryInfo">{{ t.expiry }} {{ expiryInfo }}</span>
         <select v-model="currentLang" class="lang-sel">
            <option value="zh">中文</option>
            <option value="en">English</option>
         </select>
      </div>
    </div>

    <!-- Auth -->
    <div v-if="!isLoggedIn" class="auth-box">
        <div class="auth-inner">
            <div class="auth-logo">🔒</div>
            <h2>{{ t.loginTitle }}</h2>
            <input type="password" v-model="cardCode" placeholder="Access Token..." @keyup.enter="doLogin">
            <button class="login-btn" @click="doLogin">{{ t.loginBtn }}</button>
            <transition name="fade"><p v-if="loginMsg" class="err-msg">{{ loginMsg }}</p></transition>
        </div>
    </div>

    <!-- Main Content -->
    <div v-else class="main-body">
      <div class="main-side">
          <div v-for="tab in ['task','settings','logs','data']" 
               :key="tab" 
               class="side-nav-item" 
               :class="{active: activeTab===tab}"
               @click="activeTab=tab; currentDir=''">
               <span class="side-icon">{{ {task:'📊',settings:'⚙️',logs:'📡',data:'💾'}[tab] }}</span>
               <span class="side-text">{{ t['tab' + tab.charAt(0).toUpperCase() + tab.slice(1)] }}</span>
          </div>
      </div>

      <div class="main-view">
          <!-- TASK CONSOLE -->
          <div v-if="activeTab==='task'" class="view-panel scroll-y">
              <div class="stats-hero-grid">
                  <div class="stat-big success">
                      <div class="label">SUCCESS PACKETS</div>
                      <div class="val">{{ stats.success }}</div>
                  </div>
                  <div class="stat-big failed">
                      <div class="label">FAILURE FLAGS</div>
                      <div class="val">{{ stats.failure }}</div>
                  </div>
              </div>

              <div class="health-grid-wrap">
                  <div class="title">{{ t.accStats }} Dashboard</div>
                  <div class="grid">
                      <div class="item"><div class="l">REMAINING</div><div class="v c-blue">{{ stats.remaining }}</div></div>
                      <div class="item"><div class="l">LOGOUT</div><div class="v c-warn">{{ stats.loggedOut }}</div></div>
                      <div class="item"><div class="l">BANNED</div><div class="v c-red">{{ stats.banned }}</div></div>
                      <div class="item" :class="{danger: stats.risk > 0}"><div class="l">RISK</div><div class="v c-pink">{{ stats.risk }}</div></div>
                      <div class="item"><div class="l">FORMAT ERR</div><div class="v">{{ stats.corrupted }}</div></div>
                  </div>
              </div>

              <div class="btn-group-action">
                  <button class="btn btn-go" :disabled="isRunning" @click="startTask">{{ isRunning ? 'ENGINE ACTIVE...' : t.start }}</button>
                  <button class="btn btn-no" :disabled="!isRunning || isStopping" @click="stopTask">{{ isStopping ? 'HALTING...' : t.stop }}</button>
              </div>

              <div class="console-box">
                  <div class="head">System Terminal Output</div>
                  <div class="body" id="log-box">
                      <div v-for="(l, i) in logs" :key="i" class="line"><span class="arr">></span> {{ l }}</div>
                  </div>
              </div>
          </div>

          <!-- SETTINGS -->
          <div v-if="activeTab==='settings'" class="view-panel scroll-y">
              <div class="settings-two-col">
                  <!-- Card 1 -->
                  <div class="card">
                      <div class="card-head">{{ t.files }}</div>
                      <div class="form-item">
                          <label>{{ t.acc }}</label>
                          <div class="input-action"><input readonly v-model="config.account_file"><button @click="selectFile('acc')">SELECT</button></div>
                      </div>
                      <div class="form-item">
                          <label>{{ t.target }}</label>
                          <div class="input-action"><input readonly v-model="config.target_file"><button @click="selectFile('target')">SELECT</button></div>
                      </div>
                      <div class="form-item">
                          <label>{{ t.proxy }}</label>
                          <div class="toggle-bar">
                              <span :class="{active: proxyMode==='file'}" @click="proxyMode='file'">FILE</span>
                              <span :class="{active: proxyMode==='direct'}" @click="proxyMode='direct'">DIRECT</span>
                          </div>
                          <div v-if="proxyMode==='file'" class="input-action mt-2"><input readonly v-model="config.proxy_file"><button @click="selectFile('proxy')">SELECT</button></div>
                          <textarea v-else v-model="config.proxy_content" class="modern-area mt-2" placeholder="http://ip:port"></textarea>
                      </div>
                  </div>

                  <!-- Card 2 -->
                  <div class="card">
                      <div class="card-head">{{ t.settings }}</div>
                      <div class="form-item">
                          <label>{{ t.title }}</label>
                          <input class="modern-input" v-model="config.thread_title">
                      </div>
                      <div class="form-item">
                          <label>{{ t.msg }}</label>
                          <textarea class="modern-area" v-model="config.msg_content" rows="3"></textarea>
                      </div>
                      <div class="form-row">
                          <div class="form-item"><label>{{ t.grpSize }}</label><div class="input-split"><input type="number" v-model="config.group_min">~<input type="number" v-model="config.group_max"></div></div>
                          <div class="form-item"><label>{{ t.conc }}</label><input type="number" class="modern-input" v-model="config.concurrency"></div>
                      </div>
                      <div class="form-row">
                          <div class="form-item"><label>{{ t.maxDM }}</label><input type="number" class="modern-input" v-model="config.max_dm_count"></div>
                          <div class="form-item"><label>{{ t.interval }}</label><input type="number" class="modern-input" v-model="config.interval"></div>
                      </div>
                  </div>
              </div>
          </div>

          <!-- LOGS -->
          <div v-if="activeTab==='logs'" class="view-panel">
              <div class="full-log">
                  <div class="full-log-head">STDOUT TERMINAL <button @click="logs=[]">EMPTY</button></div>
                  <div class="full-log-body" id="log-box">
                      <div v-for="(l, i) in logs" :key="i" class="full-line"><span>{{ i+1 }}</span>{{ l }}</div>
                  </div>
              </div>
          </div>

          <!-- DATA -->
          <div v-if="activeTab==='data'" class="view-panel scroll-y">
              <div v-if="!currentDir" class="data-grid">
                  <div v-for="d in ['success_uids','failed_uids','fail_risk','login_rerequire','login_banned','login_risk','login_corrupted']" 
                       :key="d" class="data-card" @click="loadDir(d)">
                      <div class="icon">📂</div>
                      <div class="info">
                          <div class="name">{{ d.replace(/_/g,' ').toUpperCase() }}</div>
                          <div class="sub">DATABASE RESOURCE</div>
                      </div>
                  </div>
              </div>
              <div v-else class="explorer">
                  <div class="explorer-nav"><button @click="currentDir=''">⬅ GO BACK</button> <span>LOCATION: /{{ currentDir }}</span></div>
                  <div class="explorer-list">
                      <div v-for="f in filesInDir" :key="f.path" class="file-item">
                          <div class="f-info">
                              <div class="fn">{{ f.name }}</div>
                              <div class="fs">LINES: {{ f.line_count }} | SIZE: {{ (f.size / 1024).toFixed(1) }} KB</div>
                          </div>
                          <div class="f-btns"><button class="v" @click="openFile(f.path)">OPEN</button><button class="d" @click="deleteFile(f.path)">ERASE</button></div>
                      </div>
                      <div v-if="filesInDir.length===0" class="empty">EMPTY RESOURCE DIRECTORY</div>
                  </div>
              </div>
          </div>
      </div>
    </div>
  </div>
</template>

<style>
:root {
  --bg-black: #080808;
  --bg-dark: #111111;
  --bg-card: #1a1a1a;
  --accent: #bc9aff;
  --text: #eeeeee;
  --dim: #777777;
  --border: #222222;
  --green: #00ff99;
  --red: #ff3333;
  --blue: #00ccff;
  --warn: #ffcc00;
  --pink: #ff00ff;
}

body, html { margin:0; padding:0; height:100vh; background:var(--bg-black); color:var(--text); font-family: -apple-system, sans-serif; overflow:hidden; }

.app-shell { height:100vh; display:flex; flex-direction:column; }
.top-nav { height:60px; background:#000; border-bottom:1px solid var(--border); display:flex; align-items:center; justify-content:space-between; padding:0 30px; }
.nav-branding { font-size:18px; font-weight:900; letter-spacing:1px; color:#fff; }
.nav-branding span { color:var(--accent); }
.expiry-tag { font-size:12px; font-weight:bold; color:var(--accent); background:rgba(188,154,255,0.1); padding:5px 15px; border-radius:20px; margin-right:20px; border:1px solid rgba(188,154,255,0.2); }
.lang-sel { background:#000; color:#eee; border:1px solid var(--border); padding:5px 10px; border-radius:4px; font-size:12px; }

/* Auth */
.auth-box { flex:1; display:flex; align-items:center; justify-content:center; background:radial-gradient(circle, #1a1a1a 0%, #000 100%); }
.auth-inner { background:var(--bg-dark); border:1px solid var(--border); padding:60px; border-radius:20px; width:380px; text-align:center; box-shadow:0 10px 40px rgba(0,0,0,0.5); }
.auth-logo { font-size:50px; margin-bottom:20px; }
.auth-inner h2 { color:var(--accent); margin-bottom:40px; font-size:24px; font-weight:900; }
.auth-inner input { width:100%; height:50px; background:#000; border:1px solid var(--border); color:#fff; padding:0 15px; border-radius:10px; margin-bottom:20px; text-align:center; font-size:16px; box-sizing:border-box; }
.login-btn { width:100%; height:50px; background:var(--accent); border:none; color:#000; font-weight:900; border-radius:10px; cursor:pointer; }
.err-msg { color:var(--red); font-weight:bold; margin-top:20px; }

/* Layout */
.main-body { flex:1; display:flex; overflow:hidden; }
.main-side { width:220px; background:#000; border-right:1px solid var(--border); display:flex; flex-direction:column; padding-top:20px; }
.side-nav-item { padding:20px 30px; cursor:pointer; color:var(--dim); font-weight:bold; display:flex; align-items:center; gap:15px; transition:0.2s; position:relative; }
.side-nav-item:hover { color:#fff; background:rgba(255,255,255,0.02); }
.side-nav-item.active { color:#fff; background:rgba(188,154,255,0.05); }
.side-nav-item.active::after { content:''; position:absolute; right:0; top:10px; bottom:10px; width:4px; background:var(--accent); border-radius:4px 0 0 4px; box-shadow:0 0 10px var(--accent); }
.side-icon { font-size:18px; }

.main-view { flex:1; background:var(--bg-black); position:relative; overflow:hidden; display:flex; flex-direction:column; }
.view-panel { flex:1; padding:35px; box-sizing:border-box; }
.scroll-y { overflow-y:auto; }

/* Stats Hero */
.stats-hero-grid { display:grid; grid-template-columns:1fr 1fr; gap:25px; margin-bottom:30px; }
.stat-big { background:var(--bg-dark); border:1px solid var(--border); padding:35px; border-radius:15px; text-align:center; position:relative; }
.stat-big .label { font-size:12px; font-weight:900; color:var(--dim); margin-bottom:10px; letter-spacing:1px; }
.stat-big .val { font-size:64px; font-weight:900; }
.success .val { color:var(--green); text-shadow:0 0 15px rgba(0,255,153,0.2); }
.failed .val { color:var(--red); text-shadow:0 0 15px rgba(255,51,51,0.2); }
.success { border-bottom:4px solid var(--green); }
.failed { border-bottom:4px solid var(--red); }

/* Monitor */
.health-grid-wrap { background:var(--bg-dark); border:1px solid var(--border); padding:25px; border-radius:15px; margin-bottom:30px; }
.health-grid-wrap .title { font-size:12px; font-weight:900; color:var(--accent); margin-bottom:20px; text-transform:uppercase; letter-spacing:1px; }
.health-grid-wrap .grid { display:grid; grid-template-columns:repeat(5, 1fr); gap:15px; }
.item { background:#000; border:1px solid var(--border); padding:15px; border-radius:10px; text-align:center; }
.item .l { font-size:10px; font-weight:bold; color:var(--dim); margin-bottom:10px; }
.item .v { font-size:26px; font-weight:900; }
.c-blue { color:var(--blue); } .c-warn { color:var(--warn); } .c-red { color:var(--red); } .c-pink { color:var(--pink); }
.item.danger { border-color:var(--pink); background:rgba(255,0,255,0.03); }

/* Mission Controls */
.btn-group-action { display:flex; gap:20px; margin-bottom:30px; }
.btn { flex:1; padding:22px; font-weight:900; border:none; border-radius:12px; cursor:pointer; font-size:16px; color:#000; transition:0.2s; }
.btn-go { background:var(--green); }
.btn-go:hover { transform:translateY(-2px); box-shadow:0 5px 15px rgba(0,255,153,0.3); }
.btn-no { background:var(--red); color:#fff; }
.btn-no:hover { transform:translateY(-2px); box-shadow:0 5px 15px rgba(255,51,51,0.3); }
.btn:disabled { opacity:0.1; transform:none !important; cursor:not-allowed; }

.console-box { flex:1; background:#000; border:1px solid var(--border); border-radius:12px; display:flex; flex-direction:column; min-height:220px; }
.console-box .head { background:#0a0a0a; padding:10px 20px; font-size:11px; color:var(--dim); border-bottom:1px solid var(--border); font-weight:bold; }
.console-box .body { padding:20px; flex:1; overflow-y:auto; font-family:monospace; }
.line { color:var(--green); font-size:12px; margin-bottom:6px; opacity:0.8; }
.arr { color:var(--accent); margin-right:10px; font-weight:bold; }

/* Settings Coherent Grid */
.settings-two-col { display:grid; grid-template-columns:repeat(auto-fit, minmax(400px, 1fr)); gap:30px; }
.card { background:var(--bg-dark); border:1px solid var(--border); padding:30px; border-radius:15px; }
.card-head { font-size:16px; font-weight:900; color:var(--accent); margin-bottom:30px; border-bottom:1px solid var(--border); padding-bottom:15px; }
.form-item { margin-bottom:25px; }
.form-item label { font-size:13px; font-weight:900; color:var(--blue); display:block; margin-bottom:10px; text-transform:uppercase; }
.input-action { display:flex; background:#000; border-radius:8px; border:1px solid var(--border); overflow:hidden; }
.input-action input { flex:1; background:transparent; border:none; color:#fff; padding:12px 15px; outline:none; font-size:14px; }
.input-action button { background:var(--border); border:none; color:#fff; padding:0 20px; font-weight:bold; cursor:pointer; transition:0.2s; font-size:12px; }
.input-action button:hover { background:var(--accent); color:#000; }

.modern-input, .modern-area { width:100%; background:#000; border:1px solid var(--border); color:#fff; padding:12px 15px; border-radius:8px; box-sizing:border-box; outline:none; transition:0.2s; font-size:14px; }
.modern-input:focus, .modern-area:focus { border-color:var(--accent); }
.toggle-bar { display:flex; background:#000; border:1px solid var(--border); border-radius:8px; padding:4px; }
.toggle-bar span { flex:1; text-align:center; padding:10px; cursor:pointer; font-size:12px; font-weight:bold; color:var(--dim); border-radius:6px; }
.toggle-bar span.active { background:var(--accent); color:#000; }

.form-row { display:grid; grid-template-columns:1fr 1fr; gap:20px; }
.input-split { display:flex; align-items:center; background:#000; border:1px solid var(--border); border-radius:8px; }
.input-split input { flex:1; background:transparent; border:none; color:#fff; text-align:center; padding:12px 0; outline:none; }

/* Full Log */
.full-log { flex:1; height:100%; background:#000; border:1px solid var(--border); border-radius:15px; overflow:hidden; display:flex; flex-direction:column; }
.full-log-head { background:#0a0a0a; padding:15px 30px; display:flex; justify-content:space-between; align-items:center; border-bottom:1px solid var(--border); font-size:12px; font-weight:900; color:var(--dim); letter-spacing:1px; }
.full-log-head button { background:var(--red); border:none; color:#fff; padding:6px 15px; border-radius:4px; font-size:11px; cursor:pointer; }
.full-log-body { flex:1; overflow-y:auto; padding:30px; font-family:monospace; }
.full-line { display:flex; gap:25px; margin-bottom:10px; color:#aaa; font-size:14px; }
.full-line span { color:#333; width:40px; text-align:right; flex-shrink:0; }

/* Data Manager */
.data-grid { display:grid; grid-template-columns:repeat(auto-fill, minmax(280px, 1fr)); gap:25px; }
.data-card { background:var(--bg-dark); border:1px solid var(--border); padding:35px; border-radius:15px; display:flex; gap:20px; align-items:center; cursor:pointer; transition:0.2s; }
.data-card:hover { border-color:var(--accent); transform:translateY(-5px); background:#111; }
.data-card .icon { font-size:35px; }
.data-card .name { font-weight:900; font-size:16px; color:#fff; margin-bottom:5px; }
.data-card .sub { font-size:10px; color:var(--dim); font-weight:bold; }

.explorer-nav { display:flex; justify-content:space-between; align-items:center; margin-bottom:35px; background:var(--bg-dark); padding:15px 25px; border-radius:10px; border:1px solid var(--border); }
.explorer-nav button { background:var(--border); border:none; color:#fff; padding:8px 20px; font-weight:900; cursor:pointer; font-size:12px; border-radius:6px; }
.explorer-nav button:hover { background:var(--accent); color:#000; }
.explorer-nav span { font-family:monospace; color:var(--accent); font-weight:bold; }

.explorer-list { display:grid; gap:15px; }
.file-item { background:var(--bg-dark); border:1px solid var(--border); padding:20px 30px; border-radius:12px; display:flex; justify-content:space-between; align-items:center; transition:0.2s; }
.file-item:hover { border-color:var(--blue); }
.f-info .fn { font-size:16px; font-weight:bold; margin-bottom:5px; }
.f-info .fs { font-size:11px; color:var(--dim); font-weight:bold; }
.f-btns { display:flex; gap:12px; }
.f-btns button { border:none; border-radius:6px; padding:10px 20px; color:#fff; font-weight:bold; font-size:12px; cursor:pointer; transition:0.2s; }
.f-btns .v { background:rgba(0,210,255,0.1); color:var(--blue); border:1px solid var(--blue); }
.f-btns .v:hover { background:var(--blue); color:#000; }
.f-btns .d { background:rgba(255,51,51,0.1); color:var(--red); border:1px solid var(--red); }
.f-btns .d:hover { background:var(--red); color:#fff; }

.empty { padding:100px; text-align:center; color:var(--dim); font-weight:bold; font-size:16px; border:2px dashed var(--border); border-radius:20px; }

/* Scroll Fix */
div::-webkit-scrollbar { width: 6px; }
div::-webkit-scrollbar-track { background: transparent; }
div::-webkit-scrollbar-thumb { background: #222; border-radius: 10px; }
div::-webkit-scrollbar-thumb:hover { background: var(--accent); }
</style>
