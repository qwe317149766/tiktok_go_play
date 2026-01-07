<script setup>
import { reactive, onMounted, onUnmounted, watch, ref, computed, nextTick } from 'vue'
import { CheckLogin, SelectFile, SelectDirectory, RunRegistration, GetLanguagePack, SetLanguage, TestAPIPushURL, GetConfig, UpdateConfig, GetArchives, OpenFolder, OpenFile, DeleteArchive, StopRegistration, GetTotalStats, ResetTotalStats } from '../wailsjs/go/main/App'

// ... (existing code)

function resetStats() {
  if (confirm(state.i18n.confirm_clear_stats)) {
    ResetTotalStats()
    // Optimistic update
    state.stats.total_success = 0
    state.stats.success = 0
    state.stats.failed = 0
  }
}
import { EventsOn } from '../wailsjs/runtime'

const state = reactive({
  isLoggedIn: false,
  activeTab: 'execution', // 'execution' | 'parameters' | 'archives'
  cardCode: "",
  expiryDate: "",
  mid: "FETCHING...",
  archives: [],
  lang: "zh-CN",
  i18n: {}, // Language pack
  config: {
    reg_mode: "sms",
    sms_file: "",
    email_file: "",
    success_dir: "./success",
    failure_dir: "./failure",
    cookie_dir: "./cookies",
    two_factor_dir: "./success_2fa",
    concurrency: 10,
    auto_2fa: true,
    font_size: 14,
    theme_color: 'indigo'
  },
  stats: {
    success: 0,
    failed: 0,
    total_success: 0
  },
  isRunning: false,
  isStopping: false,
  logs: [] // Display logs
})

const logBuffer = []
let logFlushTimer = null

// Debounce helper
const debounce = (fn, delay) => {
  let timeoutId
  return (...args) => {
    clearTimeout(timeoutId)
    timeoutId = setTimeout(() => fn(...args), delay)
  }
}

// --- Virtual Scroll: Logs ---
const logViewRef = ref(null)
const scrollTop = ref(0)
const containerHeight = ref(0)
const itemHeight = 24 // Increased to 24px for better visibility
const followLogs = ref(true)

const visibleLogs = computed(() => {
  const total = state.logs.length
  const totalHeight = total * itemHeight
  
  if (containerHeight.value === 0) return { items: [], startIndex: 0, totalHeight: 0, offsetY: 0 }

  const start = Math.floor(scrollTop.value / itemHeight)
  const visibleCount = Math.ceil(containerHeight.value / itemHeight)
  const buffer = 5
  
  const startIndex = Math.max(0, start - buffer)
  const endIndex = Math.min(total, start + visibleCount + buffer)
  
  const offsetY = startIndex * itemHeight
  
  return {
    items: state.logs.slice(startIndex, endIndex),
    startIndex,
    totalHeight,
    offsetY
  }
})

const onLogScroll = (e) => {
  const el = e.target
  scrollTop.value = el.scrollTop
  
  // Check if user is near top (Newest logs)
  followLogs.value = el.scrollTop < 50
}

const updateContainerHeight = () => {
  if (logViewRef.value) {
    containerHeight.value = logViewRef.value.clientHeight
  }
  if (archiveViewRef.value) {
    archiveContainerHeight.value = archiveViewRef.value.clientHeight
  }
}

// --- Virtual Scroll: Archives ---
const archiveViewRef = ref(null)
const archiveScrollTop = ref(0)
const archiveContainerHeight = ref(0)
const archiveItemHeight = 73 // Approximate height of a row
const archiveVisible = computed(() => {
  const total = state.archives.length
  
  if (archiveContainerHeight.value === 0) return { items: [], paddingTop: 0, paddingBottom: 0 }

  const start = Math.floor(archiveScrollTop.value / archiveItemHeight)
  const visibleCount = Math.ceil(archiveContainerHeight.value / archiveItemHeight)
  const buffer = 5
  
  const startIndex = Math.max(0, start - buffer)
  const endIndex = Math.min(total, start + visibleCount + buffer)
  
  const paddingTop = startIndex * archiveItemHeight
  const paddingBottom = (total - endIndex) * archiveItemHeight
  
  return {
    items: state.archives.slice(startIndex, endIndex),
    paddingTop,
    paddingBottom,
    startIndex
  }
})

const onArchiveScroll = (e) => {
    archiveScrollTop.value = e.target.scrollTop
}

const themes = {
  indigo: { name: 'Indigo', primary: '#6366f1', ring: 'rgba(99, 102, 241, 0.3)' },
  emerald: { name: 'Emerald', primary: '#10b981', ring: 'rgba(16, 185, 129, 0.3)' },
  rose: { name: 'Rose', primary: '#f43f5e', ring: 'rgba(244, 63, 94, 0.3)' },
  amber: { name: 'Amber', primary: '#f59e0b', ring: 'rgba(245, 158, 11, 0.3)' },
  cyan: { name: 'Cyan', primary: '#06b6d4', ring: 'rgba(6, 182, 212, 0.3)' },
  violet: { name: 'Violet', primary: '#8b5cf6', ring: 'rgba(139, 92, 246, 0.3)' },
}

onMounted(async () => {
  console.log("App Mounted Start")
  // Load default language pack
  try {
    state.i18n = await GetLanguagePack(state.lang)
  } catch (e) {
    console.error("Failed to load language pack", e)
    // Fallback minimal pack
    state.i18n = {
      login_title: "Login",
      login_btn: "Login",
      run: "Run",
      stop: "Stop",
      settings: "Settings",
      menu_execution: "Execution",
      menu_params: "Parameters",
      menu_app_settings: "App Settings",
      menu_archives: "Archives",
      stats_success: "Success",
      stats_failed: "Failed",
      sms_mode: "SMS Mode",
      email_mode: "Email Mode",
      source_data: "Source Data",
      output_config: "Output Config",
      select_file: "Select File",
      save_path: "Save Path",
      logs: "Logs",
      logs_clear: "Clear",
      logs_empty: "No logs yet...",
      success_path_title: "Success Path",
      failure_path_title: "Failure Path",
      cookie_path_title: "Cookie Path",
      storage_paths: "Storage Paths",
      storage_paths_desc: "Where to save files",
      engine_perf: "Engine",
      engine_perf_desc: "Performance settings",
      parallel_workers: "Workers",
      auto_2fa_sec: "Auto 2FA",
      task_safety: "Safety",
      task_safety_desc: "Limits and safety",
      max_reg_limit: "Max Reg Limit",
      max_phone_usage: "Max Phone Usage",
      external_int: "Integration",
      external_int_desc: "External APIs",
      api_push_url: "API Push URL",
      font_size: "Font Size",
      theme_color: "Theme Color",
      expiry_date: "Expiry Date",
      lang_select: "Language",
      save_config: "Save Config",
      param_desc: "Configuration parameters",
      card_code: "Card Code",
      login_desc: "Please login to continue",
      hardware_id: "Hardware ID",
      alert_task_completed: "Task Completed: Max limit of %d reached.",
      alert_login_failed: "Login Failed: ",
      alert_select_file: "Please select input file",
      alert_settings_saved: "Settings saved successfully",
      alert_settings_save_failed: "Failed to save settings",
      alert_api_success: "API Connection Successful!",
      alert_api_failed: "API Connection Failed. Please check the logs.",
      confirm_clear_stats: "Are you sure you want to clear all statistics?",
      confirm_delete_file: "Are you sure you want to delete this file?"
    }
  }
  
  // Fetch actual config from backend
  try {
    const backendConfig = await GetConfig()
    if (backendConfig) {
        Object.assign(state.config, backendConfig)
    }
  } catch (e) {
    console.error("Failed to load config", e)
  }

  // Fetch cumulative stats
  try {
    const total = await GetTotalStats()
    state.stats.total_success = total
  } catch (e) {
    console.error("Failed to fetch total stats", e)
  }

  // Initial archives fetch
  try {
    fetchArchives()
  } catch(e) { console.error(e) }

  // Start log flusher
  logFlushTimer = setInterval(() => {
    // SECURITY: Ensure container height is updated if it was missed initially
    if (containerHeight.value === 0) updateContainerHeight()

    if (logBuffer.length > 0) {
      // Always take from buffer to prevent growth
      const chunk = logBuffer.splice(0, 1000).reverse()
      
      // Update state
      state.logs.unshift(...chunk)
      
      // Cap at 1000 to keep UI fast
      if (state.logs.length > 1000) {
        state.logs = state.logs.slice(0, 1000)
      }
      
      // Only perform scrolling/DOM logic if tab is active
      if (state.activeTab === 'execution') {
        if (followLogs.value && logViewRef.value) {
            nextTick(() => {
               if (logViewRef.value) logViewRef.value.scrollTop = 0
            })
        }
      }
    }
  }, 300)

  // Better way to handle height: on resize, not on a timer
  window.addEventListener('resize', debounce(updateContainerHeight, 200))
  // Initial delay to ensure DOM is ready
  setTimeout(updateContainerHeight, 500)


  // Listen for backend log batches (optimized)
  EventsOn("log_batch", (batch) => {
    logBuffer.push(...batch)
  })

  // Listen for status updates (e.g. engine stopped automatically)
  EventsOn("engine_status", (status) => {
    if (status === "stopped") {
      state.isRunning = false
      state.isStopping = false
    }
  })

  EventsOn("show_alert", (msg) => {
    alert(msg)
  })

  // Listen for stats
  EventsOn("stats", (data) => {
    state.stats.success = data.success
    state.stats.failed = data.failed
    state.stats.total_success = data.total_success
  })
})

onUnmounted(() => {
  if (logFlushTimer) clearInterval(logFlushTimer)
})

// const { GetConfig, UpdateConfig, GetArchives, OpenFolder, TestAPIPushURL: TestAPI } = window.go.main.App;

function fetchArchives() {
  GetArchives().then(list => {
    state.archives = list
  })
}

function deleteFile(path) {
  if (confirm(state.i18n.confirm_delete_file)) {
    DeleteArchive(path).then(success => {
      if (success) fetchArchives()
    })
  }
}

// Watch tab change to refresh archives
watch(() => state.activeTab, (newTab) => {
  if (newTab === 'archives') fetchArchives()
})

function chooseSuccessDir() {
  SelectDirectory(state.i18n.success_folder).then(path => {
    if (path) state.config.success_dir = path
  })
}

function saveConfig() {
  UpdateConfig(state.config).then(success => {
    if (success) {
      alert(state.i18n.alert_settings_saved)
    } else {
      alert(state.i18n.alert_settings_save_failed)
    }
  })
}

// Watch language change
watch(() => state.lang, async (newLang) => {
  state.i18n = await GetLanguagePack(newLang)
  await SetLanguage(newLang)
})

// Watch theme color change
watch(() => state.config.theme_color, (newColor) => {
  const theme = themes[newColor] || themes.indigo
  document.documentElement.style.setProperty('--primary-color', theme.primary)
  document.documentElement.style.setProperty('--primary-ring', theme.ring)
}, { immediate: true })

// Watch font size change
watch(() => state.config.font_size, (newSize) => {
  document.documentElement.style.setProperty('--app-font-size', `${newSize}px`)
}, { immediate: true })

function testPushURL() {
  if (!state.config.push_url) return
  TestAPIPushURL(state.config.push_url).then(success => {
    if (success) {
      alert(state.i18n.alert_api_success)
    } else {
      alert(state.i18n.alert_api_failed)
    }
  })
}

// Auto-save config when it changes
const autoSaveConfig = debounce(() => {
  UpdateConfig(state.config).then(success => {
    if (success) {
      console.log("Configuration auto-saved")
    } else {
      console.error("Failed to auto-save configuration")
    }
  })
}, 1000)

watch(() => state.config, () => {
  autoSaveConfig()
}, { deep: true })


const loginTransition = ref(false)
const dashboardEnter = ref(false)

function performLogin() {
    if (!state.cardCode) return
    
    // Start exit animation
    loginTransition.value = true
    
    CheckLogin(state.cardCode).then(result => {
        if (result.success) {
            // Wait for exit animation
            setTimeout(() => {
                state.isLoggedIn = true
                state.expiryDate = result.expiry
                state.mid = result.mid
                
                // Start dashboard entrance
                setTimeout(() => {
                    dashboardEnter.value = true
                }, 100)
            }, 800)
        } else {
            loginTransition.value = false
            alert(state.i18n.alert_login_failed + result.error)
        }
    })
}

function chooseSmsFile() {
  SelectFile(state.i18n.select_file).then(path => {
    if (path) state.config.sms_file = path
  })
}

function chooseEmailFile() {
  SelectFile(state.i18n.select_file).then(path => {
    if (path) state.config.email_file = path
  })
}



const toggleEngine = debounce(async () => {
  if (state.isStopping) return

  if (state.isRunning) {
    // Stop Engine
    console.log("Requesting Stop...")
    state.isStopping = true
    await StopRegistration()
    // State update will be handled by event or manually if needed
    // But usually we wait for "stopped" event. For responsiveness:
    // state.isRunning = false // Let event handle it confirms backend stopped
  } else {
    // Start Engine
    if (state.config.reg_mode === 'sms' && !state.config.sms_file) {
      alert(state.i18n.alert_select_file)
      return
    }
    if (state.config.reg_mode === 'email' && !state.config.email_file) {
      alert(state.i18n.alert_select_file)
      return
    }

    state.isRunning = true
    state.logs = [] // Clear logs on start
    await RunRegistration({
      mode: state.config.reg_mode,
      file: state.config.reg_mode === 'sms' ? state.config.sms_file : state.config.email_file,
      saveDir: state.config.success_dir
    })
  }
}, 300) // 300ms debounce
</script>

<template>
  <div class="app-container min-h-screen flex flex-col font-sans text-slate-200">
    <!-- Login Screen -->
    <!-- Login Screen -->
    <div v-if="!state.isLoggedIn" class="flex-grow flex items-center justify-center p-4 overflow-hidden relative">
      <!-- Animated Background Elements -->
      <div class="absolute top-0 left-0 w-full h-full overflow-hidden pointer-events-none">
          <div class="absolute top-[-20%] left-[-10%] w-[50%] h-[50%] rounded-full bg-indigo-500/20 blur-[120px] animate-blob"></div>
          <div class="absolute bottom-[-20%] right-[-10%] w-[50%] h-[50%] rounded-full bg-violet-500/20 blur-[120px] animate-blob animation-delay-2000"></div>
          <div class="absolute top-[40%] left-[40%] w-[30%] h-[30%] rounded-full bg-emerald-500/10 blur-[100px] animate-blob animation-delay-4000"></div>
      </div>

      <div 
        :class="loginTransition ? 'scale-150 opacity-0 blur-xl' : 'scale-100 opacity-100 blur-0'"
        class="glass p-8 rounded-3xl w-full max-w-md shadow-2xl border border-white/10 transform transition-all duration-700 ease-in-out z-10 animate-slide-up"
      >
        <div class="text-center mb-10">
          <div class="inline-block p-4 rounded-2xl bg-indigo-500/20 mb-4 shadow-[0_0_20px_rgba(99,102,241,0.5)] animate-pulse-slow">
            <i class="fas fa-key text-4xl text-indigo-400"></i>
          </div>
          <h1 class="text-3xl font-bold tracking-tight text-white drop-shadow-lg">{{ state.i18n.login_title }}</h1>
          <p class="text-slate-400 mt-2">{{ state.i18n.login_desc }}</p>
        </div>
        
        <div class="space-y-6">
          <div class="space-y-2 group">
            <label class="text-xs font-bold text-slate-500 uppercase tracking-widest px-1 group-focus-within:text-indigo-400 transition-colors">{{ state.i18n.card_code }}</label>
            <input 
              v-model="state.cardCode" 
              type="text" 
              class="w-full bg-slate-950/50 border border-slate-700/50 focus:border-indigo-500 rounded-xl py-3 px-4 outline-none transition-all shadow-inner focus:shadow-[0_0_15px_rgba(99,102,241,0.3)]"
              placeholder="••••••••••••••••"
              @keyup.enter="performLogin"
            >
          </div>
          
          <button @click="performLogin" class="w-full bg-gradient-to-r from-indigo-600 to-violet-600 hover:from-indigo-500 hover:to-violet-500 py-3 rounded-xl font-bold shadow-lg shadow-indigo-500/20 transform active:scale-95 transition-all relative overflow-hidden group">
            <span class="relative z-10">{{ state.i18n.login_btn }}</span>
            <div class="absolute inset-0 bg-white/20 translate-y-full group-hover:translate-y-0 transition-transform duration-300"></div>
          </button>
          
          <div class="text-[10px] text-slate-600 text-center uppercase tracking-[0.2em] mt-8 bg-black/20 py-2 rounded-lg backdrop-blur-sm">
            {{ state.i18n.hardware_id }} {{ state.mid }}
          </div>
        </div>
      </div>
    </div>

    <!-- Main Dashboard -->
    <div 
        v-else 
        :class="dashboardEnter ? 'opacity-100 scale-100 translate-y-0' : 'opacity-0 scale-95 translate-y-10'"
        class="flex-grow flex flex-row overflow-hidden transition-all duration-700 ease-out"
    >
      <!-- Sidebar -->
      <aside class="w-48 shrink-0 glass border-r border-white/5 flex flex-col p-6 space-y-8">
        <div class="space-y-1">
          <h2 class="text-xs font-bold text-indigo-400 uppercase tracking-widest">{{ state.i18n.settings }}</h2>
          <p class="text-[10px] text-slate-500 font-mono">{{ state.i18n.expiry_date }}: {{ state.expiryDate }}</p>
          <button @click="state.isLoggedIn = false; state.cardCode = ''; loginTransition = false; dashboardEnter = false" class="text-[10px] text-rose-400 hover:text-rose-300 font-bold uppercase tracking-widest mt-2 flex items-center space-x-1 cursor-pointer transition-colors">
            <i class="fas fa-sign-out-alt"></i>
            <span>{{ state.i18n.logout }}</span>
          </button>
        </div>

        <nav class="space-y-2 flex-grow">
          <button 
            @click="state.activeTab = 'execution'"
            :class="state.activeTab === 'execution' ? 'active-tab' : 'inactive-tab'"
            class="w-full flex items-center space-x-3 p-3 rounded-xl border border-transparent transition-all"
          >
            <i class="fas fa-rocket text-sm"></i>
            <span class="font-semibold text-sm">{{ state.i18n.menu_execution }}</span>
          </button>
          <button 
            @click="state.activeTab = 'parameters'"
            :class="state.activeTab === 'parameters' ? 'active-tab' : 'inactive-tab'"
            class="w-full flex items-center space-x-3 p-3 rounded-xl border border-transparent transition-all"
          >
            <i class="fas fa-sliders-h text-sm"></i>
            <span class="font-semibold text-sm">{{ state.i18n.menu_params }}</span>
          </button>
          <button 
            @click="state.activeTab = 'app_settings'"
            :class="state.activeTab === 'app_settings' ? 'active-tab' : 'inactive-tab'"
            class="w-full flex items-center space-x-3 p-3 rounded-xl border border-transparent transition-all"
          >
            <i class="fas fa-cog text-sm"></i>
            <span class="font-semibold text-sm">{{ state.i18n.menu_app_settings }}</span>
          </button>
          <button 
            @click="state.activeTab = 'archives'"
            :class="state.activeTab === 'archives' ? 'active-tab' : 'inactive-tab'"
            class="w-full flex items-center space-x-3 p-3 rounded-xl border border-transparent transition-all"
          >
            <i class="fas fa-folder-open text-sm"></i>
            <span class="font-semibold text-sm">{{ state.i18n.menu_archives }}</span>
          </button>
        </nav>

        <div class="pt-6 border-t border-white/5 space-y-4">
          <div class="flex items-center justify-between text-[11px] text-slate-500">
            <span>{{ state.i18n.lang_select }}</span>
            <select v-model="state.lang" class="bg-transparent text-slate-300 outline-none cursor-pointer">
              <option value="zh-CN">中文</option>
              <option value="en-US">English</option>
              <option value="ru-RU">Русский</option>
            </select>
          </div>
        </div>
      </aside>

      <!-- Main Content -->
      <main class="flex-grow flex flex-col p-8 space-y-8 overflow-hidden">
        <!-- EXECUTION TAB -->
        <template v-if="state.activeTab === 'execution'">
          <header class="flex justify-between items-start">
            <div class="flex space-x-4">
              <div class="glass p-5 rounded-2xl min-w-[120px] border border-white/5">
                <span class="text-[10px] font-bold text-slate-500 uppercase tracking-wider block mb-1">{{ state.i18n.stats_success }}</span>
                <span class="text-3xl font-black text-emerald-400 tabular-nums">{{ state.stats.success }}</span>
              </div>
              <div class="glass p-5 rounded-2xl min-w-[120px] border border-white/5">
                <span class="text-[10px] font-bold text-slate-500 uppercase tracking-wider block mb-1">{{ state.i18n.stats_failed }}</span>
                <span class="text-3xl font-black text-rose-400 tabular-nums">{{ state.stats.failed }}</span>
              </div>
              <div class="glass p-5 rounded-2xl min-w-[120px] border border-white/5 relative group">
                <span class="text-[10px] font-bold text-slate-500 uppercase tracking-wider block mb-1">{{ state.i18n.stats_total }}</span>
                <span class="text-3xl font-black text-indigo-400 tabular-nums">{{ state.stats.total_success }}</span>
                <button @click="resetStats" class="absolute top-2 right-2 p-1.5 text-slate-600 hover:text-rose-400 hover:bg-rose-400/10 rounded-lg opacity-0 group-hover:opacity-100 transition-all cursor-pointer" :title="state.i18n.logs_clear">
                  <i class="fas fa-trash-alt text-xs"></i>
                </button>
              </div>
            </div>

             <div class="flex space-x-3">
               <div class="bg-black/30 rounded-2xl p-1 flex border border-white/5">
                 <button 
                  @click="state.config.reg_mode = 'sms'"
                  :class="state.config.reg_mode === 'sms' ? 'bg-primary text-white shadow-lg' : 'text-slate-500 hover:text-slate-300'"
                  class="px-4 py-2 rounded-xl text-xs font-bold transition-all"
                 >{{ state.i18n.sms_mode }}</button>
                  <button 
                   @click="alert(state.i18n.under_dev)"
                   :class="state.config.reg_mode === 'email' ? 'bg-primary text-white shadow-lg' : 'text-slate-500 hover:text-slate-300'"
                   class="px-4 py-2 rounded-xl text-xs font-bold transition-all"
                  >{{ state.i18n.email_mode }}</button>
               </div>
               <button 
                @click="toggleEngine" 
                :class="state.isRunning ? 'bg-rose-500 hover:bg-rose-600 shadow-rose-900/20' : 'bg-primary hover:bg-primary-hover shadow-indigo-600/20'"
                :disabled="state.isStopping"
                class="px-8 py-3 rounded-2xl font-bold shadow-xl active:scale-95 transition-all w-32 flex justify-center items-center"
               >
                 <span v-if="state.isStopping"><i class="fas fa-spinner fa-spin"></i></span>
                 <span v-else>{{ state.isRunning ? 'STOP' : state.i18n.run }}</span>
               </button>
            </div>
          </header>

          <!-- Config Cards -->
          <section class="grid grid-cols-2 gap-6">
            <div class="glass p-6 rounded-3xl border border-white/5 space-y-4">
              <div class="flex items-center justify-between">
                <h3 class="text-xs font-bold text-slate-400 uppercase tracking-wider">{{ state.i18n.source_data }}</h3>
                <i class="fas fa-file-import text-indigo-400/50"></i>
              </div>
              <div class="space-y-2">
                <div v-if="state.config.reg_mode === 'sms'" class="flex space-x-2">
                  <input v-model="state.config.sms_file" readonly type="text" class="flex-grow bg-black/20 border border-white/5 rounded-xl px-4 py-2 text-[11px] outline-none" :placeholder="state.i18n.select_file + '...'">
                  <button @click="chooseSmsFile" class="bg-white/5 hover:bg-white/10 px-4 rounded-xl text-[11px] font-bold border border-white/5 transition-all">{{ state.i18n.select_file }}</button>
                </div>
                <div v-else class="flex space-x-2">
                  <input v-model="state.config.email_file" readonly type="text" class="flex-grow bg-black/20 border border-white/5 rounded-xl px-4 py-2 text-[11px] outline-none" :placeholder="state.i18n.select_file + '...'">
                  <button @click="chooseEmailFile" class="bg-white/5 hover:bg-white/10 px-4 rounded-xl text-[11px] font-bold border border-white/5 transition-all">{{ state.i18n.select_file }}</button>
                </div>
              </div>
            </div>

            <div class="glass p-6 rounded-3xl border border-white/5 space-y-4">
              <div class="flex items-center justify-between">
                <h3 class="text-xs font-bold text-slate-400 uppercase tracking-wider">{{ state.i18n.output_config }}</h3>
                <i class="fas fa-folder-open text-indigo-400/50"></i>
              </div>
              <div class="space-y-3">
                <div class="flex items-center space-x-2">
                  <div class="w-20 text-[9px] text-slate-500 uppercase">{{ state.i18n.cookie_folder }}</div>
                  <input v-model="state.config.cookie_path" readonly type="text" class="flex-grow bg-black/20 border border-white/5 rounded-xl px-4 py-2 text-[11px] outline-none" :placeholder="state.i18n.save_path + '...'">
                  <button @click="SelectDirectory(state.i18n.cookie_folder).then(p => state.config.cookie_path = p || state.config.cookie_path)" class="bg-white/5 hover:bg-white/10 px-3 py-2 rounded-xl text-[10px] border border-white/5 transition-all"><i class="fas fa-search"></i></button>
                </div>
              </div>
            </div>
          </section>

          <!-- Dynamic Log Console -->
          <section class="flex-grow flex flex-col bg-black/80 rounded-xl border border-white/10 overflow-hidden relative shadow-inner font-mono text-xs">
            <div class="bg-black/40 px-4 py-2 flex justify-between items-center border-b border-white/5">
              <div class="flex items-center space-x-2">
                <div class="w-1.5 h-1.5 rounded-full bg-emerald-500"></div>
                <span class="text-[10px] font-bold text-slate-500 uppercase tracking-widest">TERMINAL</span>
              </div>
              <button @click="state.logs = []" class="text-[10px] hover:text-white text-slate-600 transition-colors">CLEAR</button>
            </div>
            
            <div 
              ref="logViewRef"
              @scroll="onLogScroll"
              class="flex-grow overflow-y-auto select-text relative scrollbar-hide"
            >
              <div v-if="state.logs.length === 0" class="text-slate-700 italic p-4 text-[10px]">> Waiting for logs...</div>
              
              <!-- Virtual Spacer -->
              <div :style="{ height: visibleLogs.totalHeight + 'px' }" class="relative w-full">
                 <!-- Visible Logs Container -->
                 <div :style="{ transform: `translateY(${visibleLogs.offsetY}px)` }" class="absolute top-0 left-0 w-full">
                    <div v-for="(log, idx) in visibleLogs.items" :key="visibleLogs.startIndex + idx" class="px-3 h-[24px] flex items-center whitespace-nowrap overflow-hidden">
                      <span class="text-slate-600 mr-2 shrink-0">[{{ log.time }}]</span>
                      <span :class="{'text-rose-400': log.msg.includes('[Error]'), 'text-emerald-400': log.msg.includes('Success'), 'text-slate-300': true}" class="truncate">{{ log.msg }}</span>
                    </div>
                 </div>
              </div>
            </div>
          </section>
        </template>

        <!-- PARAMETERS TAB -->
        <template v-else-if="state.activeTab === 'parameters'">
          <div class="flex-grow flex flex-col adaptive-layout overflow-hidden px-4">
            <!-- Compact Header -->
            <div class="flex items-center justify-between py-3 border-b border-white/5 h-16 shrink-0">
              <div class="flex items-center space-x-3">
                <div class="p-1.5 bg-indigo-500/10 rounded-lg shrink-0">
                  <i class="fas fa-sliders-h text-indigo-400 text-[10px]"></i>
                </div>
                <div class="min-w-0">
                  <h2 class="text-xs font-black text-white tracking-widest uppercase truncate">{{ state.i18n.menu_params }}</h2>
                  <p class="text-[9px] text-slate-500 truncate opacity-70">{{ state.i18n.param_desc }}</p>
                </div>
              </div>
              <button @click="saveConfig" class="bg-indigo-600/90 hover:bg-indigo-600 text-white px-4 py-1.5 rounded-lg font-bold shadow-lg shadow-indigo-900/20 transition-all flex items-center space-x-2 active:scale-95 text-[9px] uppercase tracking-widest shrink-0">
                <i class="fas fa-save"></i>
                <span>{{ state.i18n.save_config }}</span>
              </button>
            </div>

            <!-- Settings Grid -->
            <div class="flex-grow overflow-y-auto pr-1 custom-scroll py-4">
              <div class="grid grid-cols-2 gap-4">
                
                <!-- Card 1: Engine -->
                <div class="glass p-4 rounded-xl border border-white/5 space-y-4 relative">
                  <div class="flex items-center justify-between border-b border-white/5 pb-2">
                    <h3 class="text-indigo-400 font-bold text-[10px] uppercase tracking-wider flex items-center">
                      <i class="fas fa-microchip mr-2 opacity-70"></i> {{ state.i18n.engine_perf }}
                    </h3>
                    <div class="has-tooltip">
                       <i class="fas fa-info-circle text-slate-700 cursor-help hover:text-indigo-400 text-[10px] transition-colors"></i>
                       <div class="tooltip-box">{{ state.i18n.engine_perf_desc }}</div>
                    </div>
                  </div>
                  
                  <div class="space-y-4">
                    <!-- Concurrency -->
                    <div class="space-y-2">
                      <div class="flex justify-between items-center px-1">
                        <span class="text-[11px] font-medium text-slate-400">{{ state.i18n.parallel_workers }}</span>
                        <span class="text-[10px] font-mono text-indigo-400 font-bold bg-indigo-500/10 px-2 py-0.5 rounded border border-indigo-500/10">{{ state.config.concurrency }}</span>
                      </div>
                      <input v-model.number="state.config.concurrency" type="range" min="1" max="500" class="w-full h-1 bg-slate-800 rounded-full appearance-none cursor-pointer accent-indigo-500">
                    </div>

                    <!-- Auto 2FA (Enhanced Visibility) -->
                    <div class="flex items-center justify-between bg-gradient-to-r from-indigo-500/10 to-transparent p-4 rounded-xl border border-indigo-500/20 shadow-sm">
                      <div class="flex flex-col">
                        <span class="text-[12px] font-bold text-indigo-300">{{ state.i18n.auto_2fa_sec }}</span>
                        <span class="text-[9px] text-slate-500 mt-0.5">Automatically enable two-factor auth</span>
                      </div>
                      <label class="relative inline-flex items-center cursor-pointer scale-110 mr-2">
                        <input type="checkbox" v-model="state.config.auto_2fa" class="sr-only peer">
                        <div class="w-10 h-6 bg-slate-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-indigo-500 shadow-inner"></div>
                      </label>
                    </div>
                  </div>
                </div>

                <!-- Card 2: Safety -->
                <div class="glass p-4 rounded-xl border border-white/5 space-y-4 relative">
                  <div class="flex items-center justify-between border-b border-white/5 pb-2">
                    <h3 class="text-emerald-400 font-bold text-[10px] uppercase tracking-wider flex items-center">
                      <i class="fas fa-shield-alt mr-2 opacity-70"></i> {{ state.i18n.task_safety }}
                    </h3>
                    <div class="has-tooltip">
                       <i class="fas fa-info-circle text-slate-700 cursor-help hover:text-emerald-400 text-[10px] transition-colors"></i>
                       <div class="tooltip-box">{{ state.i18n.task_safety_desc }}</div>
                    </div>
                  </div>

                  <div class="grid grid-cols-2 gap-4">
                    <div class="space-y-2">
                       <label class="text-xs font-medium text-slate-500 pl-1">{{ state.i18n.max_reg_limit }}</label>
                       <input v-model.number="state.config.max_reg_count" type="number" class="w-full bg-black/20 border border-white/5 rounded-xl py-2.5 px-4 text-sm outline-none focus:border-emerald-500/20 transition-all font-bold text-emerald-400/90" placeholder="0">
                    </div>
                    <div class="space-y-2">
                       <label class="text-xs font-medium text-slate-500 pl-1">{{ state.i18n.max_phone_usage }}</label>
                       <input v-model.number="state.config.max_phone_usage" type="number" class="w-full bg-black/20 border border-white/5 rounded-xl py-2.5 px-4 text-sm outline-none focus:border-emerald-500/20 transition-all font-bold text-emerald-400/90" placeholder="1">
                    </div>
                  </div>
                </div>

                <!-- Card 3: Integration -->
                <div class="glass p-4 rounded-xl border border-white/5 space-y-4 relative">
                  <div class="flex items-center justify-between border-b border-white/5 pb-2">
                    <h3 class="text-amber-400 font-bold text-[10px] uppercase tracking-wider flex items-center">
                      <i class="fas fa-plug mr-2 opacity-70"></i> {{ state.i18n.external_int }}
                    </h3>
                    <div class="has-tooltip">
                       <i class="fas fa-info-circle text-slate-700 cursor-help hover:text-amber-400 text-[10px] transition-colors"></i>
                       <div class="tooltip-box">{{ state.i18n.external_int_desc }}</div>
                    </div>
                  </div>

                  <div class="space-y-2">
                    <label class="text-xs font-medium text-slate-500 pl-1">{{ state.i18n.api_push_url }}</label>
                    <div class="flex space-x-2">
                      <input v-model="state.config.push_url" type="text" class="flex-grow bg-black/20 border border-white/5 rounded-xl py-2.5 px-4 text-sm font-mono outline-none focus:border-amber-500/20 transition-all" placeholder="https://...">
                      <button @click="testPushURL" class="bg-amber-500/5 hover:bg-amber-500/10 text-amber-500 px-3 rounded-xl border border-amber-500/10 text-xs"><i class="fas fa-link"></i></button>
                    </div>
                  </div>
                </div>

                <div class="glass p-4 rounded-xl border border-white/5 space-y-4 relative">
                  <div class="flex items-center justify-between border-b border-white/5 pb-2">
                    <h3 class="text-slate-400 font-bold text-[10px] uppercase tracking-wider flex items-center">
                      <i class="fas fa-hdd mr-2 opacity-70"></i> {{ state.i18n.storage_paths }}
                    </h3>
                    <div class="has-tooltip">
                       <i class="fas fa-info-circle text-slate-700 cursor-help hover:text-white text-[10px] transition-colors"></i>
                       <div class="tooltip-box">{{ state.i18n.storage_paths_desc }}</div>
                    </div>
                  </div>

                  <div class="space-y-3">
                    <div class="flex items-center bg-black/10 rounded-xl overflow-hidden border border-white/5 px-2 py-1">
                      <div class="w-24 text-[10px] font-bold text-slate-600 uppercase text-center border-r border-white/5 py-1 tracking-tighter">{{ state.i18n.cookie_path_title }}</div>
                      <input v-model="state.config.cookie_path" readonly class="flex-grow bg-transparent px-3 py-2 text-xs outline-none text-slate-400">
                      <i @click="SelectDirectory('Select Cookies Storage Path').then(p => state.config.cookie_path = p || state.config.cookie_path)" class="fas fa-folder text-xs text-slate-600 hover:text-white cursor-pointer px-2 transition-all"></i>
                    </div>
                    <div class="flex items-center bg-black/10 rounded-xl overflow-hidden border border-white/5 px-2 py-1">
                      <div class="w-24 text-[10px] font-bold text-slate-600 uppercase text-center border-r border-white/5 py-1 tracking-tighter">{{ state.i18n.success_path_title }}</div>
                      <input v-model="state.config.success_path" readonly class="flex-grow bg-transparent px-3 py-2 text-xs outline-none text-slate-400">
                      <i @click="SelectDirectory('Select 2FA Success Path').then(p => state.config.success_path = p || state.config.success_path)" class="fas fa-folder text-xs text-slate-600 hover:text-white cursor-pointer px-2 transition-all"></i>
                    </div>
                    <div class="flex items-center bg-black/10 rounded-xl overflow-hidden border border-white/5 px-2 py-1">
                      <div class="w-24 text-[10px] font-bold text-slate-600 uppercase text-center border-r border-white/5 py-1 tracking-tighter">{{ state.i18n.failure_path_title }}</div>
                      <input v-model="state.config.failure_path" readonly class="flex-grow bg-transparent px-3 py-2 text-xs outline-none text-slate-400">
                      <i @click="SelectDirectory('Select 2FA Failure Path').then(p => state.config.failure_path = p || state.config.failure_path)" class="fas fa-folder text-xs text-slate-600 hover:text-white cursor-pointer px-2 transition-all"></i>
                    </div>
                    <div class="flex items-center bg-black/10 rounded-xl overflow-hidden border border-white/5 px-2 py-1">
                      <div class="w-24 text-[10px] font-bold text-slate-600 uppercase text-center border-r border-white/5 py-1 tracking-tighter">{{ state.i18n.proxy_source }}</div>
                      <input v-model="state.config.proxy_file" class="flex-grow bg-transparent px-3 py-2 text-xs outline-none text-slate-400" placeholder="Path/to/file or http://api.link">
                      <i @click="SelectFile('Select Proxy File').then(p => state.config.proxy_file = p || state.config.proxy_file)" class="fas fa-file-alt text-xs text-slate-600 hover:text-white cursor-pointer px-2 transition-all"></i>
                    </div>
                  </div>
                </div>

              </div>
            </div>
          </div>
        </template>

        <!-- APP SETTINGS TAB -->
        <template v-else-if="state.activeTab === 'app_settings'">
          <div class="flex-grow flex flex-col adaptive-layout overflow-hidden px-4">
             <div class="flex items-center justify-between py-3 border-b border-white/5 h-16 shrink-0">
              <div class="flex items-center space-x-3">
                <div class="p-1.5 bg-primary-10 rounded-lg shrink-0">
                  <i class="fas fa-cog text-primary-text text-[10px]"></i>
                </div>
                <div class="min-w-0">
                  <h2 class="text-xs font-black text-white tracking-widest uppercase truncate">{{ state.i18n.menu_app_settings }}</h2>
                </div>
              </div>
              <button @click="saveConfig" class="bg-primary hover:bg-primary-hover text-white px-4 py-1.5 rounded-lg font-bold shadow-lg shadow-indigo-900/20 transition-all flex items-center space-x-2 active:scale-95 text-[9px] uppercase tracking-widest shrink-0">
                <i class="fas fa-save"></i>
                <span>{{ state.i18n.save_config }}</span>
              </button>
            </div>

            <div class="flex-grow overflow-y-auto pr-1 custom-scroll py-4">
              <div class="grid grid-cols-2 gap-4">
                  <!-- Global Font Size -->
                  <div class="glass p-6 rounded-3xl border border-white/5 space-y-6">
                    <h3 class="text-xs font-bold text-slate-400 uppercase tracking-wider">{{ state.i18n.font_size }}</h3>
                    <div class="space-y-4">
                      <div class="flex justify-between items-center px-1">
                        <span class="text-[11px] font-medium text-slate-400">Scale</span>
                        <span class="text-[10px] font-mono text-primary-text font-bold bg-white/5 px-2 py-0.5 rounded border border-white/5">{{ state.config.font_size }}px</span>
                      </div>
                      <input v-model.number="state.config.font_size" type="range" min="10" max="32" class="w-full h-1 bg-slate-800 rounded-full appearance-none cursor-pointer accent-primary-bg">
                    </div>
                  </div>

                   <!-- Theme Color -->
                  <div class="glass p-6 rounded-3xl border border-white/5 space-y-6">
                    <h3 class="text-xs font-bold text-slate-400 uppercase tracking-wider">{{ state.i18n.theme_color }}</h3>
                    <div class="grid grid-cols-3 gap-3">
                       <button 
                         v-for="(t, key) in themes" 
                         :key="key" 
                         @click="state.config.theme_color = key"
                         :class="state.config.theme_color === key ? 'ring-2 ring-white scale-110' : 'opacity-80 hover:opacity-100 hover:scale-105'"
                         class="h-10 rounded-xl transition-all shadow-lg flex items-center justify-center"
                         :style="{backgroundColor: t.primary}"
                       >
                         <i v-if="state.config.theme_color === key" class="fas fa-check text-white text-xs drop-shadow-md"></i>
                       </button>
                    </div>
                  </div>
              </div>
            </div>
          </div>
        </template>

        <!-- ARCHIVES TAB -->
        <template v-else-if="state.activeTab === 'archives'">
          <div class="flex-grow glass rounded-[2.5rem] border border-white/5 p-8 flex flex-col space-y-6 overflow-hidden relative z-10">
            <div class="flex items-center justify-between">
               <div>
                 <h2 class="text-2xl font-bold text-white">{{ state.i18n.menu_archives }}</h2>
                 <p class="text-xs text-slate-500">History of successful account registrations</p>
               </div>
               <div class="flex space-x-2">
                 <button @click="fetchArchives" class="p-2 hover:bg-white/5 rounded-lg text-slate-400 cursor-pointer transition-all active:scale-95"><i class="fas fa-sync-alt"></i></button>
                 <button @click="OpenFolder(state.config.success_dir)" class="bg-indigo-600/20 text-indigo-400 px-4 py-2 rounded-xl text-xs font-bold border border-indigo-600/30 hover:bg-indigo-600/30 transition-all cursor-pointer active:scale-95">
                   OPEN SUCCESS FOLDER
                 </button>
               </div>
            </div>

            <div ref="archiveViewRef" @scroll="onArchiveScroll" class="flex-grow overflow-y-auto rounded-2xl border border-white/5 bg-black/20 custom-scroll-light">
              <table class="w-full text-left text-xs border-collapse relative">
                <thead class="sticky top-0 bg-slate-900/90 backdrop-blur-md shadow-xl z-20">
                  <tr class="text-slate-500 uppercase tracking-widest font-bold">
                    <th class="p-4 border-b border-white/5 w-1/3">File Name</th>
                    <th class="p-4 border-b border-white/5 w-1/4">Date Modified</th>
                    <th class="p-4 border-b border-white/5 w-1/6">Size</th>
                    <th class="p-4 border-b border-white/5 text-right">Action</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-white/5">
                  <tr v-if="state.archives.length === 0">
                    <td colspan="4" class="p-10 text-center text-slate-600 italic">No records found yet.</td>
                  </tr>
                  
                  <!-- Virtual Spacer Top -->
                   <tr v-if="archiveVisible.paddingTop > 0">
                     <td colspan="4" :style="{ height: archiveVisible.paddingTop + 'px' }"></td>
                   </tr>

                  <tr v-for="file in archiveVisible.items" :key="file.path" class="hover:bg-white/5 transition-colors group h-[73px]">
                    <td class="p-4 font-mono text-slate-300">
                      <div class="flex items-center">
                          <i class="far fa-file-alt mr-3 text-indigo-400 opacity-50 group-hover:text-indigo-400 group-hover:opacity-100 group-hover:scale-110 transition-all"></i>
                          <span class="truncate">{{ file.name }}</span>
                      </div>
                    </td>
                    <td class="p-4 text-slate-500">{{ file.time }}</td>
                    <td class="p-4 text-slate-500">{{ (file.size / 1024).toFixed(1) }} KB</td>
                     <td class="p-4 text-right">
                      <div class="flex justify-end space-x-2">
                        <button @click="OpenFile(file.path)" title="Open File" class="bg-white/5 hover:bg-indigo-600/20 px-3 py-1.5 rounded-lg border border-white/10 transition-all cursor-pointer active:scale-95 text-slate-400 hover:text-indigo-400">
                          <i class="fas fa-external-link-alt text-[10px]"></i>
                        </button>
                        <button @click="deleteFile(file.path)" title="Delete File" class="bg-white/5 hover:bg-red-600/20 px-3 py-1.5 rounded-lg border border-white/10 transition-all cursor-pointer active:scale-95 text-slate-400 hover:text-red-400">
                          <i class="fas fa-trash-alt text-[10px]"></i>
                        </button>
                      </div>
                    </td>
                  </tr>

                  <!-- Virtual Spacer Bottom -->
                   <tr v-if="archiveVisible.paddingBottom > 0">
                     <td colspan="4" :style="{ height: archiveVisible.paddingBottom + 'px' }"></td>
                   </tr>

                </tbody>
              </table>
            </div>

            <!-- Stats Footer -->
            <div class="grid grid-cols-2 gap-4 pt-4">
              <div class="bg-white/5 p-4 rounded-2xl border border-white/5 text-center">
                <div class="text-[10px] text-slate-500 uppercase">Total Files</div>
                <div class="text-xl font-bold text-white">{{ state.archives.length }}</div>
              </div>
               <div class="bg-white/5 p-4 rounded-2xl border border-white/5 text-center cursor-pointer hover:bg-white/10 transition-all active:scale-95" @click="OpenFolder(state.config.cookie_path)">
                <div class="text-[10px] text-slate-500 uppercase">Main Records</div>
                <div class="text-xl font-bold text-indigo-400 hover:underline">Explore Cookies</div>
              </div>
               <div class="bg-white/5 p-4 rounded-2xl border border-white/5 text-center cursor-pointer hover:bg-white/10 transition-all active:scale-95" @click="OpenFolder(state.config.success_path)">
                <div class="text-[10px] text-slate-500 uppercase">2FA Records</div>
                <div class="text-xl font-bold text-emerald-400 hover:underline">Explore 2FA</div>
              </div>
            </div>
          </div>
        </template>
      </main>
    </div>
  </div>
</template>

<style>
@keyframes blob {
  0% { transform: translate(0px, 0px) scale(1); }
  33% { transform: translate(30px, -50px) scale(1.1); }
  66% { transform: translate(-20px, 20px) scale(0.9); }
  100% { transform: translate(0px, 0px) scale(1); }
}
.animate-blob {
  animation: blob 7s infinite;
}
.animation-delay-2000 {
  animation-delay: 2s;
}
.animation-delay-4000 {
  animation-delay: 4s;
}
@keyframes slide-up {
    from { opacity: 0; transform: translateY(20px); }
    to { opacity: 1; transform: translateY(0); }
}
.animate-slide-up {
    animation: slide-up 1s cubic-bezier(0.16, 1, 0.3, 1) forwards;
}
.animate-pulse-slow {
    animation: pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}
/* Scrollbar improvements */
.custom-scroll-light::-webkit-scrollbar {
  width: 6px;
}
.custom-scroll-light::-webkit-scrollbar-track {
  background: rgba(255, 255, 255, 0.02);
}
.custom-scroll-light::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.1);
  border-radius: 10px;
}
.custom-scroll-light::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.2);
}
</style>

<style>
/* Reset and Globals */
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700;900&family=Fira+Code:wght@400;500&display=swap');

:root {
  color-scheme: dark;
  font-size: var(--app-font-size, 14px);
}

body {
  margin: 0;
  padding: 0;
  background-color: #020617;
  overflow: hidden;
  user-select: none;
  font-family: 'Inter', system-ui, -apple-system, sans-serif;
}

.adaptive-layout {
  height: 100%;
}

.app-container {
  background: radial-gradient(circle at 0% 0%, #1e1b4b 0%, rgba(2, 6, 23, 0) 50%),
              radial-gradient(circle at 100% 100%, #1e1b4b 0%, rgba(2, 6, 23, 0) 50%),
              #020617;
}

.glass {
  background: rgba(15, 23, 42, 0.4);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
}

/* Tooltip Styles */
.has-tooltip {
  position: relative;
}
.tooltip-box {
  position: absolute;
  bottom: 120%;
  right: 0;
  width: 200px;
  background: #1e1b4b;
  border: 1px solid rgba(255,255,255,0.1);
  padding: 10px;
  border-radius: 12px;
  font-size: 10px;
  color: #94a3b8;
  line-height: 1.4;
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.5);
  opacity: 0;
  pointer-events: none;
  transform: translateY(10px);
  transition: all 0.2s ease;
  z-index: 50;
}
.has-tooltip:hover .tooltip-box {
  opacity: 1;
  transform: translateY(0);
}

/* Custom Slider */
input[type="range"] {
  -webkit-appearance: none;
  appearance: none;
  background: transparent;
}

input[type="range"]::-webkit-scrollbar {
  display: none;
}

input[type="range"]::-webkit-slider-runnable-track {
  width: 100%;
  height: 6px;
  cursor: pointer;
  background: rgba(30, 41, 59, 0.5);
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.05);
}

input[type="range"]::-webkit-slider-thumb {
  height: 16px;
  width: 16px;
  border-radius: 50%;
  background: var(--primary-color);
  cursor: pointer;
  -webkit-appearance: none;
  margin-top: -6px;
  box-shadow: 0 0 15px var(--primary-ring);
  border: 2px solid white;
  transition: all 0.2s ease;
}

input[type="range"]:hover::-webkit-slider-thumb {
  transform: scale(1.2);
  filter: brightness(1.2);
}

.custom-scroll::-webkit-scrollbar {
  width: 4px;
}
.custom-scroll::-webkit-scrollbar-track {
  background: transparent;
}
.custom-scroll::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.05);
  border-radius: 10px;
}
.custom-scroll::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.1);
}

/* Animations */
@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.animate-pulse {
  animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}

:root {
  --primary-color: #6366f1;
  --primary-ring: rgba(99, 102, 241, 0.3);
}

.text-primary-text { color: var(--primary-color); }
.bg-primary { background-color: var(--primary-color); }
.bg-primary-hover:hover { filter: brightness(1.1); }
.bg-primary-10 { background-color: var(--primary-ring); }
.accent-primary-bg { accent-color: var(--primary-color); }

.active-tab {
  background-color: var(--primary-ring);
  color: var(--primary-color);
  border-color: var(--primary-ring);
}
.inactive-tab {
  color: #94a3b8;
}
.inactive-tab:hover {
  background-color: rgba(255,255,255,0.05);
  color: #e2e8f0;
}
</style>
