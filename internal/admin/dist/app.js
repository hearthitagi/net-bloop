/* net-bloop admin UI — 无框架、无构建步骤。 */
"use strict";

/* ------------------------------------------------------------------ icons */
const ICONS = {
  dash: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
  globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c2.5 2.4 4 5.6 4 9s-1.5 6.6-4 9c-2.5-2.4-4-5.6-4-9s1.5-6.6 4-9Z"/>',
  server: '<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01M7 16.5h.01"/>',
  zap: '<path d="M13 2 4 14h6l-1 8 9-12h-6l1-8Z"/>',
  logs: '<path d="M4 5h16M4 10h16M4 15h10M4 20h7"/>',
  gear: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  refresh: '<path d="M21 12a9 9 0 1 1-3-6.7M21 3v6h-6"/>',
  edit: '<path d="M4 20h4l10-10a2.8 2.8 0 1 0-4-4L4 16v4Z"/><path d="m13.5 6.5 4 4"/>',
  trash: '<path d="M4 7h16M9 7V5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2M6 7l1 13a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1l1-13"/>',
  copy: '<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5a1 1 0 0 1 1-1h9"/>',
  x: '<path d="M6 6l12 12M18 6 6 18"/>',
  play: '<path d="m8 5 11 7-11 7V5Z"/>',
  check: '<path d="m4 12 5 5L20 6"/>',
  alert: '<path d="M12 3 2 21h20L12 3Z"/><path d="M12 10v5M12 18h.01"/>',
  download: '<path d="M12 3v12m0 0 4-4m-4 4-4-4M4 21h16"/>',
  upload: '<path d="M12 21V9m0 0 4 4m-4-4-4 4M4 3h16"/>',
  logout: '<path d="M9 21H5a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h4M16 17l5-5-5-5M21 12H9"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  shield: '<path d="M12 3 5 6v6c0 4 3 7 7 9 4-2 7-5 7-9V6l-7-3Z"/><path d="m9 12 2 2 4-4"/>',
};

const NAV = [
  { id: "dashboard", label: "概览", icon: "dash" },
  { id: "ddns", label: "动态域名", icon: "globe" },
  { id: "proxy", label: "Web 服务", icon: "server" },
  { id: "stun", label: "内网穿透", icon: "zap" },
  { id: "logs", label: "日志", icon: "logs" },
  { id: "settings", label: "设置", icon: "gear" },
];

const PAGE_META = {
  dashboard: { title: "概览", sub: "公网入口、证书与三项核心服务的实时状态" },
  ddns: { title: "动态域名", sub: "把变化的公网 IP 自动同步到 DNS 服务商" },
  proxy: { title: "Web 服务", sub: "同一个端口，按域名把请求分给不同的内网服务" },
  stun: { title: "内网穿透", sub: "确认公网地址是否可达，必要时打洞转发" },
  logs: { title: "运行日志", sub: "三个模块的实时日志流" },
  settings: { title: "设置", sub: "端口、证书、出站代理与 STUN 服务器来源" },
};

const store = {
  session: null,
  setup: false,
  booted: false,
  page: "dashboard",
  state: null,
  providers: [],
  settings: null,
  probe: null,
  logs: [],
  logModule: "",
  logErrorsOnly: false,
  logPaused: false,
  sse: null,
  saving: false,
  lastPaint: 0,
};

/* ---------------------------------------------------------------- helpers */
const app = document.getElementById("app");
const modalRoot = document.getElementById("modal-root");
const toastRoot = document.getElementById("toasts");

function esc(v) {
  return String(v ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function icon(name, cls = "") {
  return `<svg class="${cls}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICONS[name] || ""}</svg>`;
}

function badge(text, tone = "") {
  return `<span class="badge ${tone}">${esc(text)}</span>`;
}

function dot(tone) {
  return `<span class="dot ${tone}"></span>`;
}

function fmtTime(v) {
  if (!v) return "—";
  const d = new Date(v);
  if (isNaN(d.getTime())) return "—";
  const diff = (Date.now() - d.getTime()) / 1000;
  if (diff < 0) return d.toLocaleString("zh-CN", { hour12: false });
  if (diff < 45) return "刚刚";
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`;
  return d.toLocaleString("zh-CN", { hour12: false, month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" });
}

function fmtUptime(sec) {
  sec = Number(sec || 0);
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d) return `${d} 天 ${h} 小时`;
  if (h) return `${h} 小时 ${m} 分`;
  return `${m} 分钟`;
}

function fmtAddr(ip, port) {
  if (!ip) return "";
  return String(ip).includes(":") ? `[${ip}]:${port}` : `${ip}:${port}`;
}

function statusTone(t) {
  return { active: "ok", ok: "ok", punching: "warn", starting: "warn", enabled: "ok", error: "err", stopped: "" }[t] || "";
}

async function api(path, opts = {}) {
  const init = { credentials: "same-origin", ...opts };
  if (init.body && typeof init.body !== "string") {
    init.body = JSON.stringify(init.body);
  }
  init.headers = { ...(init.headers || {}) };
  if (init.body && !init.headers["Content-Type"]) init.headers["Content-Type"] = "application/json";
  const res = await fetch("/api" + path, init);
  const ct = res.headers.get("content-type") || "";
  let data = null;
  if (ct.includes("json")) {
    data = await res.json();
  } else {
    data = await res.text();
  }
  if (res.status === 401) {
    store.session = null;
    store.booted = false;
    pthread();
    throw new Error("登录已过期，请重新登录");
  }
  if (!res.ok) {
    throw new Error((data && data.error) || res.statusText || "请求失败");
  }
  return data;
}

function toast(msg, tone = "ok") {
  const el = document.createElement("div");
  el.className = `toast ${tone}`;
  el.innerHTML = `<span>${esc(msg)}</span><button class="close" aria-label="关闭">×</button>`;
  el.querySelector(".close").addEventListener("click", () => el.remove());
  toastRoot.appendChild(el);
  setTimeout(() => el.remove(), tone === "err" ? 6000 : 3200);
}

async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    toast("已复制");
  } catch {
    const ta = document.createElement("textarea");
    ta.value = text;
    document.body.appendChild(ta);
    ta.select();
    document.execCommand("copy");
    ta.remove();
    toast("已复制");
  }
}

function q(form, name) {
  const el = form.elements[name];
  return el ? el.value : "";
}

function qn(form, name) {
  const v = q(form, name).trim();
  return v === "" ? 0 : Number(v);
}

function checked(form, name) {
  const el = form.elements[name];
  return !!(el && el.checked);
}

/** Turn a Go duration string such as "5m0s" into "5 分钟". */
function durLabel(v) {
  const s = String(v || "").trim();
  if (!s) return "—";
  const m = /^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/.exec(s);
  if (!m) return s;
  const min = Number(m[2] || 0);
  const sec = Number(m[3] || 0);
  const hour = Number(m[1] || 0);
  if (min === 0 && sec === 0) return hour ? `${hour} 小时` : "—";
  if (min === 0 && hour === 0) return `${sec} 秒`;
  if (sec === 0) return hour ? `${hour} 小时 ${min} 分` : `${min} 分钟`;
  return `${hour ? hour + " 小时 " : ""}${min ? min + " 分 " : ""}${sec} 秒`;
}

const DURATION_OPTIONS = ["30s", "1m0s", "2m0s", "5m0s", "10m0s", "30m0s", "1h0m0s"];

function durationSelect(name, value) {
  const vals = DURATION_OPTIONS.slice();
  if (value && !vals.includes(value)) vals.push(value);
  return `<select class="select" id="f-${name}" name="${name}">
    ${vals.map((v) => `<option value="${esc(v)}" ${v === value ? "selected" : ""}>${esc(durLabel(v))}</option>`).join("")}
  </select>`;
}

const PROVIDER_ORDER = ["cloudflare", "aliyun", "dnspod", "godaddy", "porkbun", "callback"];

function orderedProviders() {
  const rank = (name) => {
    const i = PROVIDER_ORDER.indexOf(name);
    return i < 0 ? PROVIDER_ORDER.length : i;
  };
  return store.providers.slice().sort((a, b) => rank(a.name) - rank(b.name));
}

/* -------------------------------------------------------------- navigation */
function currentPage() {
  const h = location.hash.replace(/^#\/?/, "");
  return PAGE_META[h] ? h : "dashboard";
}

function navHtml() {
  return NAV.map((n) => `<a href="#/${n.id}" class="${store.page === n.id ? "active" : ""}">${icon(n.icon)}<span>${n.label}</span></a>`).join("");
}

function pageActions() {
  switch (store.page) {
    case "ddns":
      return `<button class="btn primary" data-action="ddns-add">${icon("plus")}<span>添加域名</span></button>`;
    case "proxy":
      return `<button class="btn primary" data-action="proxy-add">${icon("plus")}<span>添加服务</span></button>`;
    case "stun":
      return `<button class="btn" data-action="probe">${icon("search")}<span>检测 NAT</span></button>
              <button class="btn primary" data-action="stun-add">${icon("plus")}<span>添加穿透</span></button>`;
    case "logs":
      return `<button class="btn" data-action="logs-pause">${store.logPaused ? icon("play") : icon("x")}<span>${store.logPaused ? "继续" : "暂停"}</span></button>
              <button class="btn" data-action="logs-clear">${icon("trash")}<span>清空</span></button>`;
    case "settings":
      return `<button class="btn" data-action="export">${icon("download")}<span>导出配置</span></button>`;
    default:
      return `<button class="btn" data-action="refresh">${icon("refresh")}<span>刷新</span></button>`;
  }
}

/* --------------------------------------------------------------- rendering */
// 最近一次渲染的页面与内容，用于轮询时跳过无变化的重绘。
let viewCache = { page: null, html: "" };

function pthread() {
  if (!store.session) {
    app.className = "";
    app.innerHTML = authHtml();
    bindAuth();
    return;
  }
  if (!store.booted) {
    app.className = "boot";
    app.innerHTML = `<div class="boot-inner"><div class="brand-mark large"></div><p>正在读取运行状态…</p></div>`;
    return;
  }
  app.className = "";
  const meta = PAGE_META[store.page];
  const view = viewHtml();
  app.innerHTML = `
    <div class="layout">
      <aside class="sidebar">
        <div class="brand">
          <div class="brand-mark"></div>
          <div><strong>net-bloop</strong><span>布噜 · 只做三件事</span></div>
        </div>
        <nav class="nav">
          ${navHtml()}
        </nav>
        <div class="side-foot">
          <div class="side-user">
            <div class="avatar">${esc((store.session.user || "A").slice(0, 1).toUpperCase())}</div>
            <div><b>${esc(store.session.user || "管理员")}</b><small>v${esc(store.session.version || "")}</small></div>
            <button class="icon-btn" data-action="logout" title="退出登录" style="margin-left:auto">${icon("logout")}</button>
          </div>
        </div>
      </aside>
      <main class="main">
        <header class="topbar">
          <div>
            <h1>${esc(meta.title)}</h1>
            <div class="sub">${esc(meta.sub)}</div>
          </div>
          <div class="top-actions">${pageActions()}</div>
        </header>
        <section class="view" id="view">${view}</section>
      </main>
    </div>
    <nav class="bottom-nav">${navHtml()}</nav>`;
  viewCache = { page: store.page, html: view };
}

function viewHtml() {
  switch (store.page) {
    case "ddns": return viewDDNS();
    case "proxy": return viewProxy();
    case "stun": return viewStun();
    case "logs": return viewLogs();
    case "settings": return viewSettings();
    default: return viewDashboard();
  }
}

// 只更新面板内容：不重建侧栏/顶栏，保留滚动位置，内容没变就不动 DOM，
// 因此轮询和手动刷新都不会触发 .view 的入场动画（不会闪烁）。
function refreshView() {
  if (!store.session || !store.booted) return;
  const view = document.getElementById("view");
  if (!view) {
    pthread();
    return;
  }
  const html = viewHtml();
  if (viewCache.page === store.page && viewCache.html === html) return;
  const y = window.scrollY;
  const openDetails = [];
  view.querySelectorAll("details").forEach((d, i) => {
    if (d.open) openDetails.push(i);
  });
  view.innerHTML = html;
  if (openDetails.length) {
    const next = view.querySelectorAll("details");
    openDetails.forEach((i) => {
      if (next[i]) next[i].open = true;
    });
  }
  viewCache = { page: store.page, html };
  if (window.scrollY !== y) window.scrollTo(0, y);
}

function safeRepaint() {
  if (document.querySelector(".modal-backdrop")) return;
  const el = document.activeElement;
  if (el && ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName)) return;
  // 日志页由 SSE 自行追加；设置页避免覆盖尚未保存的表单。
  if (store.page === "logs" || store.page === "settings") return;
  refreshView();
}

/* -------------------------------------------------------------- 登录 / 建号 */
function authHtml() {
  const setup = store.setup;
  return `
    <div class="login">
      <div class="login-card">
        <div class="login-brand">
          <div class="brand-mark"></div>
          <div><strong>net-bloop</strong><span>布噜 · 动态域名 / Web 服务 / 内网穿透</span></div>
        </div>
        <h1>${setup ? "先给这条鲸鱼起个门牌" : "回到面板"}</h1>
        <p class="sub">${setup ? "这是第一次启动，设置一个管理账号。之后不会再出现默认密码。" : "用一个账号管理三个模块，忘了就改配置文件里的哈希。"}</p>
        <div id="login-error" class="login-error hidden"></div>
        <form id="auth-form" autocomplete="on">
          <div class="field">
            <label for="login-user">用户名</label>
            <input class="input" id="login-user" name="username" autocomplete="username" placeholder="${setup ? "想叫什么就叫什么" : ""}" ${setup ? 'value="admin"' : ""} required />
          </div>
          <div class="field">
            <label for="login-pass">密码</label>
            <input class="input" id="login-pass" name="password" type="password" autocomplete="${setup ? "new-password" : "current-password"}" placeholder="${setup ? "至少 4 位" : ""}" required />
          </div>
          ${setup ? `<div class="field">
            <label for="login-pass2">再输一次密码</label>
            <input class="input" id="login-pass2" name="password2" type="password" autocomplete="new-password" required />
          </div>` : ""}
          <button class="btn primary" style="width:100%;height:40px;margin-top:4px" type="submit">${setup ? "创建并进入" : "进入面板"}</button>
        </form>
        <div class="login-foot">${setup ? "密码只保存在本机的 TOML 里，哈希由 Argon2id 生成" : "建议只在内网或 VPN 下访问管理端口"}</div>
      </div>
    </div>`;
}

function bindAuth() {
  const form = document.getElementById("auth-form");
  if (!form) return;
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const btn = form.querySelector("button[type=submit]");
    const errBox = document.getElementById("login-error");
    const password = q(form, "password");
    if (store.setup && password !== q(form, "password2")) {
      errBox.textContent = "两次输入的密码不一致";
      errBox.classList.remove("hidden");
      return;
    }
    btn.disabled = true;
    errBox.classList.add("hidden");
    try {
      const path = store.setup ? "/setup" : "/login";
      const data = await api(path, {
        method: "POST",
        body: { username: q(form, "username").trim(), password },
      });
      store.setup = false;
      store.session = { user: data.user, version: "" };
      store.booted = false;
      await bootAfterLogin();
    } catch (err) {
      errBox.textContent = err.message;
      errBox.classList.remove("hidden");
    } finally {
      btn.disabled = false;
    }
  });
}

/* ------------------------------------------------------------------- boot */
async function bootAfterLogin() {
  try {
    const s = await api("/session");
    if (!s.authenticated) {
      store.session = null;
      pthread();
      return;
    }
    store.session = s;
    store.page = currentPage();
    await refreshState();
    await ensureProviders();
    store.booted = true;
    pthread();
    connectSSE();
    if (store.page === "settings") await loadSettings();
    startPolling();
  } catch (err) {
    store.session = null;
    store.booted = false;
    pthread();
    toast(err.message, "err");
  }
}

async function ensureProviders() {
  if (store.providers.length) return;
  try {
    store.providers = await api("/ddns/providers");
  } catch {
    store.providers = [];
  }
}

async function refreshState() {
  store.state = await api("/state");
}

let pollTimer = null;
function startPolling() {
  if (pollTimer) clearInterval(pollTimer);
  pollTimer = setInterval(async () => {
    if (!store.session || !store.booted) return;
    try {
      await refreshState();
      safeRepaint();
    } catch {
      /* transient */
    }
  }, 5000);
}

/* --------------------------------------------------------------- SSE logs */
function connectSSE() {
  if (store.sse) store.sse.close();
  const es = new EventSource("/api/stream");
  store.sse = es;
  es.addEventListener("log", (ev) => {
    try {
      const entry = JSON.parse(ev.data);
      store.logs.push(entry);
      if (store.logs.length > 1500) store.logs.splice(0, store.logs.length - 1500);
      if (store.page === "logs" && !store.logPaused) appendLogLine(entry);
    } catch {
      /* ignore */
    }
  });
  es.onerror = () => {
    /* EventSource retries by itself; nothing to do */
  };
}

/* ---------------------------------------------------------------- dashboard */
function ruleTone(kind, entry) {
  if (kind === "ddns") return entry.status && entry.status.ok ? "ok" : entry.status && entry.status.last_error ? "err" : "warn";
  if (kind === "proxy") return entry.active ? "ok" : entry.rule.enable ? "warn" : "";
  return statusTone(entry.status && entry.status.state);
}

function stunModeLabel(mode) {
  return mode === "punch" ? "打洞转发" : "固定转发";
}

function dashItem(kind, entry) {
  const r = entry.rule;
  let title = r.name || r.domain || "未命名";
  let detail = "";
  let tone = "warn";
  let label = "等待";
  let tag = "";
  if (kind === "ddns") {
    tag = "DDNS";
    detail = `${r.domain} · ${r.type}`;
    if (entry.status && entry.status.ok) { tone = "ok"; label = "已同步"; }
    else if (entry.status && entry.status.last_error) { tone = "err"; label = "失败"; }
    else { label = "等待"; }
  } else if (kind === "proxy") {
    tag = "Web";
    const domains = (r.domains || []).length ? r.domains.join(" / ") : "（默认站点）";
    detail = `${domains} → ${(r.upstreams || [])[0] || "未配置"}`;
    if (!r.enable) { tone = ""; label = "已停用"; }
    else if (entry.active) { tone = "ok"; label = "运行中"; }
    else { tone = "err"; label = "未监听"; }
  } else {
    tag = "穿透";
    const st = entry.status || {};
    detail = `${stunModeLabel(r.mode)} · ${String(r.proto).toUpperCase()}`;
    if (st.access) detail += ` · ${st.access}`;
    else if (r.public_port) detail += ` · 公网 ${r.public_port}`;
    if (!r.enable) { tone = ""; label = "已停用"; }
    else if (st.state === "active") { tone = "ok"; label = st.access ? "已确认" : "就绪"; }
    else if (st.state === "error") { tone = "err"; label = "异常"; }
    else if (st.state === "punching") { tone = "warn"; label = "打洞中"; }
    else { label = "探测中"; }
  }
  return `<div class="list-item">
    ${dot(tone)}
    <div class="li-main">
      <div class="li-title"><span class="name">${esc(title)}</span>${badge(tag)}</div>
      <div class="li-sub">${esc(detail)}</div>
    </div>
    <div class="li-side">${badge(label, tone === "ok" ? "ok" : tone === "err" ? "err" : "")}</div>
  </div>`;
}

// 外网入口：一个端口、一张证书、N 个域名。
function entryStrip(s) {
  const web = s.web || {};
  const cert = s.cert || {};
  const port = web.listen ? String(web.listen).replace(/^.*:/, "") : "—";
  const tone = !web.running ? "err" : !cert.ready ? "warn" : "ok";
  const certText = cert.manual_cert
    ? `手动证书 ${cert.days_left} 天`
    : !cert.enabled
      ? "未启用自动证书"
      : !cert.ready
        ? "证书未就绪"
        : `${cert.days_left} 天后到期`;
  return `<div class="card">
    <div class="card-body" style="padding:16px 20px">
      <div class="row" style="gap:14px">
        ${dot(tone)}
        <div style="flex:1;min-width:200px">
          <div class="li-title"><span class="name" style="font-size:14px">外网入口</span>
            ${badge(web.https ? "HTTPS" : "HTTP", web.running ? "ok" : "err")}
            ${badge(`${web.domains || 0} 个域名`)}
            ${badge(certText, cert.ready ? (cert.days_left < 14 ? "warn" : "ok") : "warn")}
          </div>
          <div class="li-sub">
            <span class="chip">端口 ${esc(port)}</span>
            <span>路由器把公网端口转发到这台设备的 ${esc(port)}</span>
          </div>
        </div>
        <div class="li-side">
          ${web.error ? badge(web.error, "err") : ""}
          <a class="btn small" href="#/settings">调整</a>
        </div>
      </div>
    </div>
  </div>`;
}

function viewDashboard() {
  const s = store.state;
  if (!s) return `<div class="card"><div class="empty">正在读取状态…</div></div>`;
  const ddns = s.ddns || [], proxy = s.proxy || [], stun = s.stun || [];
  const total = (s.counters && s.counters.total) || 0;
  const active = (s.counters && s.counters.active) || 0;
  const ddnsOk = ddns.filter((x) => x.status && x.status.ok).length;
  const proxyOk = proxy.filter((x) => x.active).length;
  const stunOk = stun.filter((x) => x.status && x.status.state === "active").length;
  const items = [];
  ddns.forEach((x) => items.push(dashItem("ddns", x)));
  proxy.forEach((x) => items.push(dashItem("proxy", x)));
  stun.forEach((x) => items.push(dashItem("stun", x)));
  const probe = store.probe;
  const host = s.host || {};
  const cert = s.cert || {};
  const list = s.stunlist || {};
  return `
  <div class="stack">
    ${entryStrip(s)}
    <div class="grid cols-3" style="display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px">
      <div class="card stat info">
        <div class="glyph">${icon("globe")}</div>
        <div class="label">动态域名</div>
        <div class="value">${ddns.length}</div>
        <div class="hint">${ddnsOk} 条已同步到现在这个 IP</div>
      </div>
      <div class="card stat ${proxyOk ? "ok" : "warn"}">
        <div class="glyph">${icon("server")}</div>
        <div class="label">Web 服务</div>
        <div class="value">${proxy.length}</div>
        <div class="hint">${s.web && s.web.running ? `共用一个端口，按域名分发` : "监听未就绪"}</div>
      </div>
      <div class="card stat ${stunOk ? "ok" : "warn"}">
        <div class="glyph">${icon("zap")}</div>
        <div class="label">内网穿透</div>
        <div class="value">${stun.length}</div>
        <div class="hint">${stunOk} 条确认可达</div>
      </div>
    </div>

    <div class="grid cols-2">
      <div class="card">
        <div class="card-head">
          <div><h2>运行概览</h2><p>${active} / ${total} 条规则处于正常状态</p></div>
          <button class="btn small ghost" data-action="refresh">${icon("refresh")}刷新</button>
        </div>
        ${items.length ? `<div class="list">${items.slice(0, 9).join("")}</div>` : `
        <div class="empty">
          <div class="art">${icon("plus")}</div>
          <h3>还没有任何规则</h3>
          <p>先加一条 Web 服务，或者让布噜自己去问一下公网地址。</p>
          <button class="btn primary" data-action="proxy-add">${icon("plus")}添加 Web 服务</button>
        </div>`}
      </div>

      <div class="stack">
        <div class="card">
          <div class="card-head"><div><h2>证书</h2><p>${cert.enabled ? "DNS-01 自动签发与续期" : "当前未启用自动证书"}</p></div>
            <a class="btn small ghost" href="#/settings">设置</a>
          </div>
          <div class="card-body">
            <dl class="kv">
              <dt>状态</dt><dd>${cert.ready ? badge(`${cert.days_left} 天有效期`, cert.days_left < 14 ? "warn" : "ok") : badge(cert.issuing ? "签发中" : "未签发", cert.issuing ? "brand" : "warn")}</dd>
              <dt>覆盖域名</dt><dd>${(cert.domains || []).length ? (cert.domains || []).map(esc).join("、") : "—"}</dd>
              <dt>到期时间</dt><dd>${cert.not_after ? esc(new Date(cert.not_after).toLocaleDateString("zh-CN")) : "—"}</dd>
              <dt>颁发者</dt><dd>${esc(cert.issuer || "—")}</dd>
            </dl>
            ${cert.last_error ? `<div class="alert err" style="margin-top:12px">${icon("alert")}<div>${esc(cert.last_error)}</div></div>` : ""}
          </div>
        </div>

        <div class="card">
          <div class="card-head">
            <div><h2>系统与网络</h2><p>v${esc(s.version || "")} · 运行 ${esc(fmtUptime(s.uptime))}</p></div>
            <button class="btn small" data-action="probe">${icon("search")}测 NAT</button>
          </div>
          <div class="card-body">
            <dl class="kv">
              <dt>平台</dt><dd>${esc(host.os || "")} / ${esc(host.arch || "")}</dd>
              <dt>内网地址</dt><dd>${(host.ips && host.ips.length ? host.ips.map(esc).join("、") : "—")}</dd>
              <dt>管理端口</dt><dd>${esc(host.port || "—")}</dd>
              <dt>STUN 列表</dt><dd>${list.count || 0} 台 · ${esc(list.source || "内置")}</dd>
            </dl>
            ${probe ? `<div class="alert" style="margin-top:12px">${icon("globe")}<div><b>${esc(probe.nat_type)}</b><br>${esc(probe.advice || "")}</div></div>` : ""}
          </div>
        </div>
      </div>
    </div>
  </div>`;
}

/* -------------------------------------------------------------------- DDNS */
function providerLabel(name) {
  const p = store.providers.find((x) => x.name === name);
  return p ? p.label : name || "未选择";
}

function sourceLabel(src) {
  if (!src || src === "auto") return "自动获取";
  if (src.startsWith("stun:")) return "STUN 规则" + (src.slice(5) ? " " + src.slice(5) : "");
  if (src.startsWith("interface:")) return "网卡 " + (src.slice(10) || "自动");
  if (src.startsWith("url:")) return "URL " + src.slice(4);
  return "固定 " + src;
}

function viewDDNS() {
  const list = (store.state && store.state.ddns) || [];
  return `
  <div class="stack">
    <div class="card">
      <div class="card-head">
        <div><h2>域名同步</h2><p>${list.length} 条规则 · 定时检测 IP 变化并更新 DNS</p></div>
      </div>
      ${list.length ? `<div class="list">${list.map((entry) => {
        const r = entry.rule, st = entry.status || {};
        let tone = "warn", label = "等待首次执行";
        if (st.last_error) { tone = "err"; label = "更新失败"; }
        else if (st.ok) { tone = "ok"; label = "已同步"; }
        if (!r.enable) { tone = ""; label = "已停用"; }
        return `<div class="list-item">
          ${dot(tone)}
          <div class="li-main">
            <div class="li-title">
              <span class="name">${esc(r.name || r.domain)}</span>
              ${badge(providerLabel(r.provider), "brand")}
              ${badge(r.type, "")}
              ${r.enable ? "" : badge("已停用", "warn")}
            </div>
            <div class="li-sub">
              <span class="addr">${esc(r.domain)}</span>
              <span>${esc(sourceLabel(r.source))}</span>
              <span>每 ${esc(durLabel(r.interval))}</span>
              ${st.last_ip ? `<span class="chip">${esc(st.last_ip)}</span>` : ""}
              ${st.last_error ? `<span style="color:var(--err)">${esc(st.last_error)}</span>` : ""}
            </div>
          </div>
          <div class="li-side">
            ${badge(label, tone === "ok" ? "ok" : tone === "err" ? "err" : "")}
            <button class="btn small" data-action="ddns-run" data-id="${esc(r.id)}" title="立即执行">${icon("play")}</button>
            <button class="btn small" data-action="ddns-edit" data-id="${esc(r.id)}" title="编辑">${icon("edit")}</button>
            <button class="btn small danger" data-action="ddns-del" data-id="${esc(r.id)}" title="删除">${icon("trash")}</button>
          </div>
        </div>`;
      }).join("")}</div>` : `
      <div class="empty">
        <div class="art">${icon("globe")}</div>
        <h3>还没有动态域名</h3>
        <p>添加一条规则，把公网 IP 自动同步到 Cloudflare、阿里云或 DNSPod。</p>
        <button class="btn primary" data-action="ddns-add">${icon("plus")}添加域名</button>
      </div>`}
    </div>
    <div class="alert">
      ${icon("alert")}
      <div>DDNS 只负责更新 DNS 记录。若没有公网 IP，请先用 STUN 穿透取得公网地址，并在“IP 来源”中选择对应的 STUN 规则。</div>
    </div>
  </div>`;
}

/* ------------------------------------------------------------- DDNS form */
function providerFieldsHtml(providerName, auth) {
  const p = store.providers.find((x) => x.name === providerName);
  if (!p) return `<div class="field full"><div class="help">暂不支持该服务商</div></div>`;
  return p.fields.map((f) => fInput({
    name: "auth_" + f.key,
    label: f.label,
    value: auth[f.key] || "",
    placeholder: f.placeholder || "",
    type: f.secret ? "password" : "text",
    required: !!f.required,
    autocomplete: f.secret ? "new-password" : "off",
  })).join("");
}

function collectAuth(form, providerName) {
  const p = store.providers.find((x) => x.name === providerName);
  const auth = {};
  if (!p) return auth;
  for (const f of p.fields) {
    const v = q(form, "auth_" + f.key).trim();
    if (v) auth[f.key] = v;
  }
  return auth;
}

function parseSource(src) {
  if (!src || src === "auto") return { kind: "auto", detail: "" };
  if (src.startsWith("stun:")) return { kind: "stun", detail: src.slice(5) };
  if (src.startsWith("interface:")) return { kind: "interface", detail: src.slice(10) };
  if (src.startsWith("url:")) return { kind: "url", detail: src.slice(4) };
  return { kind: "static", detail: src };
}

function sourceDetailHtml(kind, detail) {
  if (kind === "auto") {
    return `<div class="field"><label>说明</label><div class="help">优先使用已打通的 STUN 公网地址，否则通过公共接口获取公网 IP。</div></div>`;
  }
  if (kind === "stun") {
    const rules = (store.state && store.state.stun) || [];
    const opts = [{ value: "", label: "任意可用 STUN 规则" }].concat(
      rules.map((x) => ({ value: x.rule.id, label: (x.rule.name || x.rule.target) + " · " + String(x.rule.proto).toUpperCase() + " " + x.rule.local_port }))
    );
    return fSelect({ name: "source_stun", label: "STUN 规则", value: detail, options: opts });
  }
  if (kind === "interface") {
    return fInput({ name: "source_interface", label: "网卡名称", value: detail, placeholder: "留空自动选择，例如 eth0" });
  }
  if (kind === "url") {
    return fInput({ name: "source_url", label: "查询地址", value: detail, placeholder: "https://api.ipify.org", mono: true });
  }
  return fInput({ name: "source_static", label: "固定 IP", value: detail, placeholder: "1.2.3.4", mono: true });
}

function openDdnsForm(rule) {
  if (!store.providers.length) {
    toast("正在读取服务商列表…", "err");
    ensureProviders().then(() => openDdnsForm(rule));
    return;
  }
  const r = rule || {
    name: "", enable: true, provider: "cloudflare",
    domain: "", type: "A", source: "auto", interval: "10m0s", ttl: 600, auth: {},
  };
  const src = parseSource(r.source);
  const body = `
    <div class="form-grid">
      ${fInput({ name: "name", label: "名称", value: r.name, placeholder: "例如：家里 NAS" })}
      ${fSelect({ name: "provider", label: "DNS 服务商", value: r.provider, options: orderedProviders().map((p) => ({ value: p.name, label: p.label })) })}
    </div>
    <div class="form-grid" id="prov-fields">${providerFieldsHtml(r.provider, r.auth || {})}</div>

    <div class="section-title">同步目标</div>
    <div class="form-grid">
      ${fInput({ name: "domain", label: "完整域名", value: r.domain, placeholder: "nas.example.com", required: true, mono: true })}
      ${fSelect({ name: "type", label: "记录类型", value: r.type, options: ["A", "AAAA", "TXT"] })}
      <div class="field"><label for="f-interval">检测间隔</label>${durationSelect("interval", r.interval)}</div>
      <input type="hidden" name="ttl" value="${esc(r.ttl || 600)}" />
    </div>

    <div class="section-title">IP 来源</div>
    <div class="form-grid">
      ${fSelect({ name: "source_kind", label: "获取方式", value: src.kind, options: [
        { value: "auto", label: "自动（STUN / 公网接口）" },
        { value: "stun", label: "STUN 穿透规则" },
        { value: "interface", label: "本机网卡" },
        { value: "url", label: "自定义查询地址" },
        { value: "static", label: "固定 IP" },
      ] })}
      <div id="source-detail">${sourceDetailHtml(src.kind, src.detail)}</div>
    </div>

    <details class="advanced">
      <summary>高级选项</summary>
      <div class="form-grid">
        ${fSwitch({ name: "enable", label: "启用这条规则", checked: r.enable, help: "关闭后保留配置但不再定时同步。" })}
      </div>
    </details>`;

  openModal({
    title: rule ? "编辑动态域名" : "添加动态域名",
    subtitle: "支持 A / AAAA / TXT 记录",
    body,
    onSave: async (form) => {
      const provider = q(form, "provider");
      const kind = q(form, "source_kind");
      let source = "auto";
      if (kind === "stun") source = "stun:" + q(form, "source_stun");
      else if (kind === "interface") source = "interface:" + q(form, "source_interface").trim();
      else if (kind === "url") {
        const u = q(form, "source_url").trim();
        if (!u) throw new Error("请填写查询地址");
        source = "url:" + u;
      } else if (kind === "static") {
        const ip = q(form, "source_static").trim();
        if (!ip) throw new Error("请填写固定 IP");
        source = ip;
      }
      const domain = q(form, "domain").trim();
      if (!domain) throw new Error("请填写域名");
      const payload = {
        id: r.id || "",
        name: q(form, "name").trim() || domain,
        enable: checked(form, "enable"),
        provider,
        domain,
        type: q(form, "type"),
        source,
        interval: q(form, "interval"),
        ttl: qn(form, "ttl") || 600,
        auth: collectAuth(form, provider),
      };
      if (rule) await api("/ddns/" + r.id, { method: "PUT", body: payload });
      else await api("/ddns", { method: "POST", body: payload });
      await refreshState();
      safeRepaint();
      toast(rule ? "规则已更新" : "规则已添加");
    },
  });

  const modal = modalRoot.querySelector("#modal-form");
  modal.querySelector('[name="provider"]').addEventListener("change", (e) => {
    const auth = collectAuth(modal, r.provider);
    r.provider = e.target.value;
    modalRoot.querySelector("#prov-fields").innerHTML = providerFieldsHtml(r.provider, auth);
  });
  const kindSel = modal.querySelector('[name="source_kind"]');
  kindSel.addEventListener("change", () => {
    modalRoot.querySelector("#source-detail").innerHTML = sourceDetailHtml(kindSel.value, "");
  });
}

function openProxyForm(rule) {
  const r = rule || { name: "", enable: true, domains: [], upstreams: [], use_target_host: false, basic_auth: "" };
  const web = (store.state && store.state.web) || {};
  const port = web.listen ? String(web.listen).replace(/^.*:/, "") : "—";
  const body = `
    <div class="alert" style="margin-bottom:16px">${icon("server")}<div>所有服务共用端口 <b>${esc(port)}</b>，靠域名区分。新加一条规则不需要动路由器。</div></div>
    <div class="form-grid">
      ${fInput({ name: "name", label: "名称", value: r.name, placeholder: "例如：qBittorrent" })}
      ${fInput({ name: "basic_auth", label: "访问密码", value: r.basic_auth, placeholder: "user:password", mono: true, help: "留空表示不额外加一层认证。" })}
      ${fTextarea({ name: "domains", label: "域名", value: (r.domains || []).join("\n"), rows: 3, placeholder: "qb.kissshot.site\n*.apps.kissshot.site", help: "每行一个，只写主机名，不要带端口和 https://。支持 *.example.com。留空表示默认站点。", full: true })}
      ${fTextarea({ name: "upstreams", label: "转发到", value: (r.upstreams || []).join("\n"), rows: 3, placeholder: "http://192.168.18.10:8080", help: "每行一个，多个地址轮询。http 或 https 都行，不写协议默认 http。", full: true })}
    </div>
    <details class="advanced">
      <summary>高级选项</summary>
      <div class="form-grid">
        ${fSwitch({ name: "use_target_host", label: "使用上游 Host 头", checked: r.use_target_host, help: "少数面板（如 Portainer）需要。" })}
        ${fSwitch({ name: "enable", label: "启用这条规则", checked: r.enable })}
      </div>
    </details>`;

  openModal({
    title: rule ? "编辑 Web 服务" : "添加 Web 服务",
    subtitle: "把一个域名指到内网的一个服务",
    body,
    onSave: async (form) => {
      const domains = q(form, "domains").split(/[\n,]/).map((s) => s.trim()).filter(Boolean);
      const upstreams = q(form, "upstreams").split(/\n/).map((s) => s.trim()).filter(Boolean);
      if (!upstreams.length) throw new Error("请填写至少一个转发地址");
      const payload = {
        id: r.id || "",
        name: q(form, "name").trim() || domains[0] || "Web 服务",
        enable: checked(form, "enable"),
        domains,
        upstreams,
        basic_auth: q(form, "basic_auth").trim(),
        use_target_host: checked(form, "use_target_host"),
      };
      if (rule) await api("/proxy/" + r.id, { method: "PUT", body: payload });
      else await api("/proxy", { method: "POST", body: payload });
      await refreshState();
      refreshView();
      toast(rule ? "服务已更新" : "服务已添加");
    },
  });
}

/* ------------------------------------------------------------- STUN form */
function splitLines(v) {
  return String(v || "").split(/[\n,]/).map((s) => s.trim()).filter(Boolean);
}

function headersToText(h) {
  return Object.keys(h || {}).map((k) => `${k}: ${h[k]}`).join("\n");
}

function textToHeaders(text) {
  const out = {};
  splitLines(text).forEach((line) => {
    const i = line.indexOf(":");
    if (i > 0) out[line.slice(0, i).trim()] = line.slice(i + 1).trim();
  });
  return out;
}

function openStunForm(rule) {
  const r = rule || {
    name: "", enable: true, proto: "tcp", mode: "punch", public_port: 0, local_port: 26666,
    listen_ip: "0.0.0.0", target: "127.0.0.1:33320", servers: [], heartbeat: "20s", webhook: null,
  };
  const wh = r.webhook || { enable: false, url: "", method: "POST", headers: {}, body: "", only_change: true, timeout: "10s" };
  const legacyForward = r.mode === "forward";
  const body = `
    ${legacyForward ? `<div class="alert warn">${icon("alert")}<div>这条规则原来是「固定转发」。这个版本只保留打洞转发，保存后会改成打洞，公网端口改为由 NAT 分配。</div></div>` : ""}
    <div class="form-grid">
      ${fInput({ name: "name", label: "名称", value: r.name, placeholder: "例如：公网入口" })}
      ${fSelect({ name: "proto", label: "协议", value: r.proto, options: [{ value: "tcp", label: "TCP" }, { value: "udp", label: "UDP" }] })}
    </div>

    <div class="form-grid">
      ${fInput({ name: "local_port", label: "本地监听端口", value: r.local_port || 26666, type: "number", min: 1, step: 1, help: "只填数字，程序自己绑到 0.0.0.0。公网端口由 NAT 分配，这里填的不是公网端口。" })}
      ${fInput({ name: "target", label: "转发到", value: r.target, placeholder: "127.0.0.1:33320", mono: true, help: "收到连接后转给谁，通常是唯一反代。" })}
    </div>

    <details class="advanced">
      <summary>高级选项</summary>
      <div class="form-grid">
        ${fInput({ name: "heartbeat", label: "保活间隔", value: r.heartbeat, placeholder: "20s", help: "多久问一次 STUN 服务器来维持端口映射。20s 足够，别低于 10s——公共服务器不欠我们的。" })}
        ${fInput({ name: "listen_ip", label: "监听地址", value: r.listen_ip, placeholder: "0.0.0.0", mono: true })}
        ${fTextarea({ name: "servers", label: "专用 STUN 服务器", value: (r.servers || []).join("\n"), rows: 2, placeholder: "留空使用全局列表", help: "每行一个 host:port。留空是常态。", full: true })}
        ${fSwitch({ name: "enable", label: "启用这条规则", checked: r.enable })}
      </div>
    </details>

    <details class="advanced" ${wh.enable ? "open" : ""}>
      <summary>Webhook（可选，地址变化时通知）</summary>
      <div class="form-grid">
        ${fSwitch({ name: "wh_enable", label: "地址变化时调用", checked: !!wh.enable, help: "端口固定之后通常不需要。留给以后可能的重定向场景。", full: true })}
      </div>
      <div id="wh-box" class="stack ${wh.enable ? "" : "hidden"}" style="gap:14px;margin-top:14px">
        <div class="form-grid">
          ${fInput({ name: "wh_url", label: "请求地址", value: wh.url, placeholder: "https://...", mono: true, full: true })}
          ${fSelect({ name: "wh_method", label: "方法", value: wh.method || "POST", options: ["POST", "PUT", "PATCH", "GET"] })}
          ${fInput({ name: "wh_timeout", label: "超时", value: wh.timeout || "10s" })}
        </div>
        ${fTextarea({ name: "wh_headers", label: "请求头", value: headersToText(wh.headers), rows: 2, placeholder: "Authorization: Bearer xxx" })}
        ${fTextarea({ name: "wh_body", label: "请求正文", value: wh.body, rows: 4, placeholder: '{"ip":"{{.IP}}","port":{{.Port}}}' })}
        <div class="var-row" id="wh-vars">
          ${["{{.IP}}", "{{.Port}}", "{{.Addr}}", "{{.OldIP}}", "{{.OldPort}}", "{{.OldAddr}}", "{{.Name}}", "{{.Time}}"].map((v) => `<button type="button" class="var" data-insert="${esc(v)}">${esc(v)}</button>`).join("")}
        </div>
        ${fSwitch({ name: "wh_only_change", label: "仅在地址变化时触发", checked: wh.only_change !== false })}
      </div>
    </details>`;

  openModal({
    title: rule ? "编辑穿透规则" : "添加穿透规则",
    subtitle: "在本机开一个端口，通过 STUN 打洞把它暴露到公网",
    body,
    onSave: async (form) => {
      const localPort = qn(form, "local_port");
      const target = q(form, "target").trim();
      if (!localPort) throw new Error("请填写本地监听端口");
      if (!target) throw new Error("请填写转发目标");
      if (!target.includes(":")) throw new Error("目标要带端口，例如 127.0.0.1:33320");
      // Editing the rule that carries this very page would cut the user off.
      const here = window.location.port;
      if (rule && String(rule.local_port) === here && String(localPort) !== here) {
        const ok = window.confirm(
          `你现在正是通过 ${here} 端口打开的这个面板。\n\n保存后该端口会停止监听，面板将无法再从当前地址打开，需要改用内网 IP:16601 或反代域名。\n\n确定继续吗？`
        );
        if (!ok) {
          const e = new Error("canceled");
          e.silent = true;
          throw e;
        }
      }
      const whEnable = checked(form, "wh_enable");
      let webhook = null;
      if (whEnable) {
        const url = q(form, "wh_url").trim();
        if (!url) throw new Error("请填写 Webhook 地址");
        webhook = {
          enable: true,
          url,
          method: q(form, "wh_method"),
          headers: textToHeaders(q(form, "wh_headers")),
          body: q(form, "wh_body"),
          only_change: checked(form, "wh_only_change"),
          timeout: q(form, "wh_timeout").trim() || "10s",
        };
      }
      const payload = {
        id: r.id || "",
        name: q(form, "name").trim() || "公网入口",
        enable: checked(form, "enable"),
        proto: q(form, "proto"),
        mode: "punch",
        public_port: 0,
        local_port: localPort,
        listen_ip: q(form, "listen_ip").trim() || "0.0.0.0",
        target,
        servers: splitLines(q(form, "servers")),
        heartbeat: q(form, "heartbeat").trim() || "20s",
        webhook,
      };
      if (rule) await api("/stun/" + r.id, { method: "PUT", body: payload });
      else await api("/stun", { method: "POST", body: payload });
      // Saving reloads the relay, so the port this page is served through may
      // just have gone away. Treat a failed refresh as a hint, not an error.
      let lost = false;
      try {
        await refreshState();
      } catch (e) {
        lost = true;
      }
      refreshView();
      if (lost) {
        toast("规则已保存。面板入口已断开，请改用内网地址或 16601 端口重新打开", "err");
      } else {
        toast(rule ? "规则已更新" : "规则已添加");
      }
    },
  });

  const form = modalRoot.querySelector("#modal-form");
  const whToggle = form.querySelector('[name="wh_enable"]');
  const whBox = form.querySelector("#wh-box");
  whToggle.addEventListener("change", () => whBox.classList.toggle("hidden", !whToggle.checked));
  const bodyArea = form.querySelector('[name="wh_body"]');
  form.querySelector("#wh-vars").addEventListener("click", (e) => {
    const btn = e.target.closest("[data-insert]");
    if (!btn) return;
    const v = btn.dataset.insert;
    const start = bodyArea.selectionStart || 0;
    bodyArea.value = bodyArea.value.slice(0, start) + v + bodyArea.value.slice(bodyArea.selectionEnd || start);
    bodyArea.focus();
    bodyArea.selectionStart = bodyArea.selectionEnd = start + v.length;
  });
}

function fInput(o) {
  const {
    name, label, value = "", type = "text", placeholder = "", help = "",
    required = false, full = false, mono = false, min, step, autocomplete = "off", readonly = false,
  } = o;
  return `<div class="field ${full ? "full" : ""}">
    <label for="f-${name}">${esc(label)}${required ? ' <span class="req">*</span>' : ""}</label>
    <input class="input ${mono ? "code" : ""}" id="f-${name}" name="${name}" type="${type}" value="${esc(value)}"
      placeholder="${esc(placeholder)}" ${required ? "required" : ""} ${autocomplete ? `autocomplete="${autocomplete}"` : ""}
      ${min !== undefined ? `min="${min}"` : ""} ${step !== undefined ? `step="${step}"` : ""} ${readonly ? "readonly" : ""}
      style="${mono ? "font-family:var(--mono);font-size:12.5px" : ""}" />
    ${help ? `<div class="help">${help}</div>` : ""}
  </div>`;
}

function fSelect(o) {
  const { name, label, value = "", options = [], help = "", full = false } = o;
  return `<div class="field ${full ? "full" : ""}">
    <label for="f-${name}">${esc(label)}</label>
    <select class="select" id="f-${name}" name="${name}">
      ${options.map((op) => {
        const v = typeof op === "string" ? op : op.value;
        const l = typeof op === "string" ? op : op.label;
        return `<option value="${esc(v)}" ${String(value) === String(v) ? "selected" : ""}>${esc(l)}</option>`;
      }).join("")}
    </select>
    ${help ? `<div class="help">${help}</div>` : ""}
  </div>`;
}

function fTextarea(o) {
  const { name, label, value = "", placeholder = "", help = "", full = true, rows = 4, mono = true } = o;
  return `<div class="field ${full ? "full" : ""}">
    <label for="f-${name}">${esc(label)}</label>
    <textarea class="textarea" id="f-${name}" name="${name}" rows="${rows}" placeholder="${esc(placeholder)}"
      style="${mono ? "" : "font-family:var(--font);font-size:13px"}">${esc(value)}</textarea>
    ${help ? `<div class="help">${help}</div>` : ""}
  </div>`;
}

function fSwitch(o) {
  const { name, label, checked = false, help = "", full = true } = o;
  return `<div class="field ${full ? "full" : ""}">
    <label class="switch">
      <input type="checkbox" name="${name}" ${checked ? "checked" : ""} />
      <span class="track"></span><span>${esc(label)}</span>
    </label>
    ${help ? `<div class="help">${help}</div>` : ""}
  </div>`;
}

function openModal(opts) {
  const { title, subtitle = "", body, saveLabel = "保存", note = "", onSave } = opts;
  modalRoot.innerHTML = `
    <div class="modal-backdrop">
      <div class="modal" role="dialog" aria-modal="true">
        <div class="modal-head">
          <div><h2>${esc(title)}</h2>${subtitle ? `<p>${esc(subtitle)}</p>` : ""}</div>
          <button class="btn ghost icon-only" type="button" data-close aria-label="关闭">${icon("x")}</button>
        </div>
        <form id="modal-form" class="modal-body" autocomplete="off">${body}</form>
        <div class="modal-foot">
          <span class="note">${note}</span>
          <button class="btn" type="button" data-close>取消</button>
          <button class="btn primary" type="submit" form="modal-form" id="modal-save">${esc(saveLabel)}</button>
        </div>
      </div>
    </div>`;
  const backdrop = modalRoot.firstElementChild;
  const form = backdrop.querySelector("form");
  const close = () => closeModal();
  backdrop.querySelectorAll("[data-close]").forEach((b) => b.addEventListener("click", close));
  backdrop.addEventListener("mousedown", (e) => {
    if (e.target === backdrop) close();
  });
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    if (store.saving) return;
    const saveBtn = backdrop.querySelector("#modal-save");
    store.saving = true;
    saveBtn.disabled = true;
    const old = saveBtn.textContent;
    saveBtn.textContent = "保存中…";
    try {
      await onSave(form);
      closeModal();
      refreshView();
    } catch (err) {
      if (!err || !err.silent) toast(err.message, "err");
    } finally {
      store.saving = false;
      saveBtn.disabled = false;
      saveBtn.textContent = old;
    }
  });
  setTimeout(() => {
    const first = form.querySelector("input:not([type=hidden]),select,textarea");
    if (first) first.focus();
  }, 40);
}

function closeModal() {
  modalRoot.innerHTML = "";
}

function confirmDialog(o) {
  const { title, message, okLabel = "确认删除" } = o;
  return new Promise((resolve) => {
    modalRoot.innerHTML = `
      <div class="modal-backdrop" style="justify-content:center;align-items:center">
        <div class="modal" style="height:auto;max-width:430px;border-radius:16px" role="alertdialog" aria-modal="true">
          <div class="modal-head"><div><h2>${esc(title)}</h2></div></div>
          <div class="modal-body" style="padding:16px 22px"><p style="color:var(--ink-2);font-size:13px">${esc(message)}</p></div>
          <div class="modal-foot">
            <button class="btn" type="button" data-no>取消</button>
            <button class="btn danger" type="button" data-yes>${esc(okLabel)}</button>
          </div>
        </div>
      </div>`;
    const backdrop = modalRoot.firstElementChild;
    const done = (v) => { closeModal(); resolve(v); };
    backdrop.querySelector("[data-no]").addEventListener("click", () => done(false));
    backdrop.querySelector("[data-yes]").addEventListener("click", () => done(true));
    backdrop.addEventListener("mousedown", (e) => { if (e.target === backdrop) done(false); });
  });
}

function logMatches(e) {
  if (store.logModule && e.module !== store.logModule) return false;
  if (store.logErrorsOnly && e.level !== "error" && e.level !== "warn") return false;
  return true;
}

function logLineHtml(e) {
  const d = new Date(e.time);
  const t = isNaN(d.getTime()) ? "--:--:--" : d.toLocaleTimeString("zh-CN", { hour12: false });
  const lv = e.level || "info";
  return `<div class="log-line">
    <span class="log-time">${esc(t)}</span>
    <span class="log-lv ${esc(lv)}">${esc(lv.toUpperCase())}</span>
    <span class="log-mod">[${esc(e.module || "core")}]</span>
    ${e.rule ? `<span class="log-rule">[${esc(e.rule)}]</span>` : ""}
    <span class="log-msg">${esc(e.msg)}</span>
  </div>`;
}

function filteredLogs() {
  return store.logs.filter(logMatches).slice(-600);
}

function viewLogs() {
  const mods = [
    ["", "全部模块"],
    ["core", "核心"],
    ["ddns", "动态域名"],
    ["proxy", "Web 服务"],
    ["stun", "STUN"],
  ];
  return `
  <div class="stack">
    <div class="card">
      <div class="card-head">
        <div><h2>实时日志</h2><p>最多保留最近 1500 条</p></div>
        <div class="logs-toolbar">
          <select class="select" id="log-module" style="width:auto;height:32px">
            ${mods.map(([v, l]) => `<option value="${v}" ${store.logModule === v ? "selected" : ""}>${l}</option>`).join("")}
          </select>
          <label class="switch" style="font-size:12px">
            <input type="checkbox" id="log-errors" ${store.logErrorsOnly ? "checked" : ""} />
            <span class="track"></span><span>只看警告/错误</span>
          </label>
        </div>
      </div>
      <div class="card-body tight">
        <div class="log-panel" id="log-panel">${logPanelHtml()}</div>
      </div>
    </div>
  </div>`;
}

function logPanelHtml() {
  const rows = filteredLogs();
  if (!rows.length) return `<div class="log-empty">暂无日志，等待模块输出…</div>`;
  return rows.map(logLineHtml).join("");
}

function renderLogPanel() {
  const panel = document.getElementById("log-panel");
  if (!panel) return;
  panel.innerHTML = logPanelHtml();
  panel.scrollTop = panel.scrollHeight;
}

function appendLogLine(e) {
  if (!logMatches(e)) return;
  const panel = document.getElementById("log-panel");
  if (!panel) return;
  if (panel.querySelector(".log-empty")) panel.innerHTML = "";
  const nearBottom = panel.scrollHeight - panel.scrollTop - panel.clientHeight < 60;
  panel.insertAdjacentHTML("beforeend", logLineHtml(e));
  while (panel.querySelectorAll(".log-line").length > 600) {
    const first = panel.querySelector(".log-line");
    if (first) first.remove();
  }
  if (nearBottom) panel.scrollTop = panel.scrollHeight;
}

/* ---------------------------------------------------------------- actions */
function findRule(kind, id) {
  const list = (store.state && store.state[kind]) || [];
  const hit = list.find((x) => x.rule && x.rule.id === id);
  return hit ? hit.rule : null;
}

async function runProbe() {
  toast("正在通过公共 STUN 服务器检测…");
  try {
    const res = await api("/stun/probe", { method: "POST", body: { servers: [] } });
    store.probe = res;
    toast(res.compatible ? "检测完成：可以打洞" : "检测完成：建议使用端口映射", res.compatible ? "ok" : "err");
    refreshView();
  } catch (err) {
    toast("检测失败：" + err.message, "err");
  }
}

async function exportConfig() {
  try {
    const res = await fetch("/api/config/export", { credentials: "same-origin" });
    if (!res.ok) throw new Error("导出失败");
    const blob = await res.blob();
    const a = document.createElement("a");
    const d = new Date();
    const stamp = `${d.getFullYear()}${String(d.getMonth() + 1).padStart(2, "0")}${String(d.getDate()).padStart(2, "0")}`;
    a.href = URL.createObjectURL(blob);
    a.download = `net-bloop-${stamp}.toml`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(a.href), 2000);
    toast("配置已导出");
  } catch (err) {
    toast(err.message, "err");
  }
}

async function importConfig(file) {
  if (!file) return;
  const ok = await confirmDialog({
    title: "导入配置",
    message: `将用 ${file.name} 覆盖当前全部规则。管理端口与密码会保留，确定继续吗？`,
    okLabel: "导入并覆盖",
  });
  if (!ok) return;
  try {
    const text = await file.text();
    await api("/config/import", { method: "POST", body: text, headers: { "Content-Type": "text/plain; charset=utf-8" } });
    await refreshState();
    await loadSettings();
    refreshView();
    toast("配置已导入");
  } catch (err) {
    toast(err.message, "err");
  } finally {
    const input = document.getElementById("import-file");
    if (input) input.value = "";
  }
}

async function doDelete(kind, rule) {
  const ok = await confirmDialog({
    title: "删除规则",
    message: `确定删除「${rule.name || rule.domain || rule.target || rule.id}」吗？此操作不可撤销。`,
  });
  if (!ok) return;
  try {
    await api(`/${kind}/${rule.id}`, { method: "DELETE" });
    await refreshState();
    refreshView();
    toast("已删除");
  } catch (err) {
    toast(err.message, "err");
  }
}

document.addEventListener("click", async (e) => {
  const el = e.target.closest("[data-action]");
  if (!el) return;
  const action = el.dataset.action;
  const id = el.dataset.id;
  try {
    switch (action) {
      case "logout":
        if (store.sse) store.sse.close();
        store.sse = null;
        await api("/logout", { method: "POST", body: {} });
        store.session = null;
        store.booted = false;
        store.state = null;
        store.logs = [];
        if (pollTimer) clearInterval(pollTimer);
        pthread();
        break;
      case "refresh":
        await refreshState();
        refreshView();
        toast("状态已刷新");
        break;
      case "probe":
        await runProbe();
        break;
      case "ddns-add":
        openDdnsForm(null);
        break;
      case "ddns-edit": {
        const r = findRule("ddns", id);
        if (r) openDdnsForm(r);
        break;
      }
      case "ddns-del": {
        const r = findRule("ddns", id);
        if (r) await doDelete("ddns", r);
        break;
      }
      case "ddns-run":
        await api(`/ddns/${id}/run`, { method: "POST", body: {} });
        toast("已触发一次同步");
        setTimeout(async () => { await refreshState(); safeRepaint(); }, 1500);
        break;
      case "proxy-add":
        openProxyForm(null);
        break;
      case "proxy-edit": {
        const r = findRule("proxy", id);
        if (r) openProxyForm(r);
        break;
      }
      case "proxy-del": {
        const r = findRule("proxy", id);
        if (r) await doDelete("proxy", r);
        break;
      }
      case "stun-add":
        openStunForm(null);
        break;
      case "stun-edit": {
        const r = findRule("stun", id);
        if (r) openStunForm(r);
        break;
      }
      case "stun-del": {
        const r = findRule("stun", id);
        if (r) await doDelete("stun", r);
        break;
      }
      case "copy":
        await copyText(el.dataset.copy || "");
        break;
      case "cert-issue": {
        el.disabled = true;
        try {
          await api("/cert/issue", { method: "POST", body: {} });
          toast("已开始签发，稍等片刻刷新看结果");
        } finally {
          el.disabled = false;
        }
        break;
      }
      case "stun-refresh": {
        el.disabled = true;
        const old = el.innerHTML;
        el.textContent = "更新中…";
        try {
          const res = await api("/stun/servers/refresh", { method: "POST", body: {} });
          toast(`已更新 ${res.list.length} 台 STUN 服务器`);
          await refreshState();
          refreshView();
        } finally {
          el.disabled = false;
          el.innerHTML = old;
        }
        break;
      }
      case "logs-pause":
        store.logPaused = !store.logPaused;
        pthread();
        break;
      case "logs-clear":
        store.logs = [];
        renderLogPanel();
        break;
      case "export":
        await exportConfig();
        break;
      case "import": {
        const input = document.getElementById("import-file");
        if (input) input.click();
        break;
      }
      default:
        break;
    }
  } catch (err) {
    toast(err.message, "err");
  }
});

document.addEventListener("change", (e) => {
  if (e.target && e.target.id === "import-file") {
    importConfig(e.target.files && e.target.files[0]);
  }
});

document.addEventListener("submit", async (e) => {
  const form = e.target;
  if (!form || form.dataset.form !== "settings") return;
  e.preventDefault();
  const btn = form.querySelector('button[type="submit"]');
  const password = q(form, "password");
  const password2 = q(form, "password2");
  if (password !== password2) {
    toast("两次输入的密码不一致", "err");
    return;
  }
  btn.disabled = true;
  try {
    const payload = {
      admin_user: q(form, "admin_user").trim(),
      password,
      log_level: q(form, "log_level"),
      server_port: q(form, "server_port").trim(),
      web_port: q(form, "web_port").trim(),
      web_https: checked(form, "web_https"),
      web_redirect_https: checked(form, "web_redirect_https"),
      web_access_log: checked(form, "web_access_log"),
      web_cert_file: q(form, "web_cert_file").trim(),
      web_key_file: q(form, "web_key_file").trim(),
      proxy: q(form, "proxy").trim(),
      cert_enable: checked(form, "cert_enable"),
      cert_email: q(form, "cert_email").trim(),
      cert_domains: splitLines(q(form, "cert_domains")),
      cert_token: q(form, "cert_token").trim(),
      cert_renew_before: q(form, "cert_renew_before").trim(),
      stun_list_enable: checked(form, "stun_list_enable"),
      stun_list_url: q(form, "stun_list_url").trim(),
      stun_list_interval: q(form, "stun_list_interval").trim(),
      stun_list_servers: splitLines(q(form, "stun_list_servers").replace(/,/g, "\n")),
    };
    const res = await api("/settings", { method: "PUT", body: payload });
    await loadSettings();
    await refreshState();
    refreshView();
    toast(res.restart_required ? "已保存，端口改动需要重启容器才生效" : "设置已保存");
  } catch (err) {
    toast(err.message, "err");
  } finally {
    btn.disabled = false;
  }
});

window.addEventListener("keydown", (e) => {
  if (e.key === "Escape") closeModal();
});

window.addEventListener("hashchange", async () => {
  const page = currentPage();
  if (page === store.page) return;
  store.page = page;
  if (page === "settings" && !store.settings) await loadSettings();
  if (store.session && store.booted) {
    pthread();
    if (page === "logs") {
      setTimeout(() => {
        const p = document.getElementById("log-panel");
        if (p) p.scrollTop = p.scrollHeight;
      }, 20);
    }
  }
});

/* ------------------------------------------------------------------- boot */
(async function init() {
  try {
    const s = await api("/session");
    store.setup = !!s.setup_required;
    if (!s.authenticated) {
      pthread();
      return;
    }
    store.session = s;
    store.page = currentPage();
    await refreshState();
    await ensureProviders();
    if (store.page === "settings") await loadSettings();
    store.booted = true;
    pthread();
    connectSSE();
    startPolling();
  } catch (err) {
    pthread();
    toast(err.message, "err");
  }
})();

async function loadSettings() {
  try {
    store.settings = await api("/settings");
  } catch (err) {
    toast(err.message, "err");
  }
}

function viewSettings() {
  const s = store.settings;
  if (!s) return `<div class="card"><div class="empty">正在读取设置…</div></div>`;
  const cert = (store.state && store.state.cert) || {};
  const srv = (store.state && store.state.stunlist) || {};
  return `
  <form class="stack" data-form="settings">
    <div class="masonry">
        <div class="card">
          <div class="card-head"><div><h2>管理员</h2><p>登录这个面板用的账号</p></div></div>
          <div class="card-body">
            <div class="form-grid">
              <div class="field">
                <label>用户名</label>
                <input class="input" name="admin_user" value="${esc(s.admin_user)}" autocomplete="username" />
              </div>
              <div class="field">
                <label>日志级别</label>
                <select class="select" name="log_level">
                  ${["info", "debug", "warn", "error"].map((l) => `<option value="${l}" ${s.log_level === l ? "selected" : ""}>${l}</option>`).join("")}
                </select>
              </div>
              <div class="field">
                <label>新密码</label>
                <input class="input" name="password" type="password" placeholder="留空表示不改" autocomplete="new-password" />
              </div>
              <div class="field">
                <label>再输一次</label>
                <input class="input" name="password2" type="password" placeholder="留空表示不改" autocomplete="new-password" />
              </div>
              <div class="field full">
                <label>管理端口</label>
                <div class="input-prefix"><span>端口</span><input class="input" name="server_port" value="${esc(s.server_port)}" inputmode="numeric" placeholder="16601" /></div>
                <div class="help">只填数字。改完要重启容器才生效。</div>
              </div>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="card-head"><div><h2>Web 服务</h2><p>所有反代规则共用这一个端口</p></div>
            ${(store.state && store.state.web && store.state.web.running) ? badge("监听中", "ok") : badge("未监听", "warn")}
          </div>
          <div class="card-body">
            <div class="form-grid">
              <div class="field">
                <label>监听端口</label>
                <div class="input-prefix"><span>端口</span><input class="input" name="web_port" value="${esc(s.web_port)}" inputmode="numeric" placeholder="33320" /></div>
                <div class="help">路由器把这个数字转到设备上同一个数字。</div>
              </div>
              <div class="field">
                <label>HTTPS</label>
                ${fSwitch({ name: "web_https", label: "启用 HTTPS", checked: s.web_https })}
                ${fSwitch({ name: "web_redirect_https", label: "HTTP 自动跳到 HTTPS", checked: s.web_redirect_https })}
              </div>
              ${fSwitch({ name: "web_access_log", label: "记录访问日志", checked: s.web_access_log, full: true, help: "默认关闭，开启了每一条请求都会写日志，日志多了会盖住其它信息。" })}
            </div>
            <details class="advanced">
              <summary>手填证书（一般不用）</summary>
              <div class="form-grid">
                ${fInput({ name: "web_cert_file", label: "证书文件", value: s.web_cert_file, mono: true, placeholder: "留空则使用下面自动签发的证书" })}
                ${fInput({ name: "web_key_file", label: "私钥文件", value: s.web_key_file, mono: true, placeholder: "相对配置文件目录" })}
              </div>
            </details>
          </div>
        </div>

        <div class="card">
          <div class="card-head">
            <div><h2>证书</h2><p>DNS-01 通配证书，自动续期</p></div>
            ${cert.ready ? badge(`${cert.days_left} 天`, cert.days_left < 14 ? "warn" : "ok") : badge(cert.issuing ? "签发中" : "未签发", "warn")}
          </div>
          <div class="card-body">
            <div class="form-grid">
              ${fSwitch({ name: "cert_enable", label: "启用自动签发", checked: s.cert_enable, full: true, help: "通配符只能用 DNS-01，需要 Cloudflare 的 API Token。" })}
              ${fTextarea({ name: "cert_domains", label: "覆盖域名", value: (s.cert_domains || []).join("\n"), rows: 3, placeholder: "*.kissshot.site", help: "每行一个，通配符写 *.kissshot.site。", full: true })}
              ${fInput({ name: "cert_email", label: "联系邮箱", value: s.cert_email, placeholder: "可选，写错不影响签发" })}
              ${fInput({ name: "cert_renew_before", label: "提前续期", value: s.cert_renew_before, placeholder: "720h" })}
              <div class="field full">
                <label>Cloudflare API Token</label>
                <input class="input" name="cert_token" type="password" autocomplete="off" placeholder="${s.cert_token_set ? "已保存，留空表示不修改" : "需要 Zone.DNS 编辑权限"}" />
                <div class="help">只存在本机 TOML 里，界面上不会回显。</div>
              </div>
            </div>
            ${cert.last_error ? `<div class="alert err" style="margin-top:12px">${icon("alert")}<div>${esc(cert.last_error)}</div></div>` : ""}
            <div class="row" style="margin-top:14px">
              <button class="btn" type="button" data-action="cert-issue">${icon("refresh")}立即签发 / 续期</button>
              <span class="help">${cert.ready ? `有效期至 ${esc(new Date(cert.not_after).toLocaleString("zh-CN"))}` : "签发成功后会自动热加载，不用重启"}</span>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="card-head"><div><h2>STUN 服务器来源</h2><p>每天自动更新一次公共列表</p></div>
            ${badge(`${srv.count || 0} 台`, "ok")}
          </div>
          <div class="card-body">
            <div class="form-grid">
              ${fSwitch({ name: "stun_list_enable", label: "每日自动更新", checked: s.stun_list_enable, full: true })}
              ${fInput({ name: "stun_list_url", label: "更新地址", value: s.stun_list_url, placeholder: "留空用默认的 always-online-stun", full: true, help: "留空会依次尝试默认地址与国内可用的镜像。" })}
              ${fInput({ name: "stun_list_interval", label: "更新间隔", value: s.stun_list_interval, placeholder: "24h" })}
              ${fInput({ name: "stun_list_servers", label: "固定服务器", value: (s.stun_list_servers || []).join(","), placeholder: "留空则只用自动列表", help: "逗号分隔，优先级最高。" })}
            </div>
            <div class="row" style="margin-top:14px">
              <button class="btn" type="button" data-action="stun-refresh">${icon("refresh")}立即更新</button>
              <span class="help">${srv.updated ? `上次 ${esc(fmtTime(srv.updated))} · ${esc(srv.source || "")}` : "还没更新过"}</span>
            </div>
            ${srv.error ? `<div class="alert warn" style="margin-top:12px">${icon("alert")}<div>${esc(srv.error)}</div></div>` : ""}
          </div>
        </div>

      <div class="card">
        <div class="card-head"><div><h2>出站代理</h2><p>列表更新、公网 IP 探测、Webhook 都走它</p></div></div>
        <div class="card-body">
          <div class="field">
            <label>代理地址</label>
            <input class="input" name="proxy" value="${esc(s.proxy)}" placeholder="http://127.0.0.1:7890" />
            <div class="help">host 网络下 127.0.0.1 就是 NAS 本机。留空则直连。</div>
          </div>
        </div>
      </div>

      <div class="card">
        <div class="card-head"><div><h2>配置文件</h2><p>TOML，改完自动热重载</p></div></div>
        <div class="card-body stack">
          <div class="help">${esc(s.config)}</div>
          <div class="row">
            <button class="btn" type="button" data-action="export">${icon("download")}导出</button>
            <button class="btn" type="button" data-action="import">${icon("upload")}导入</button>
            <input type="file" id="import-file" accept=".toml,.txt,text/plain" class="hidden" />
          </div>
          <div class="alert warn">${icon("alert")}<div>导入会覆盖规则，但保留账号与端口。</div></div>
        </div>
      </div>
    </div>

    <div class="row">
      <button class="btn primary" type="submit">保存全部设置</button>
      <span class="help">端口类改动需要重启容器；其余立即生效。</span>
    </div>
  </form>`;
}

function viewProxy() {
  const list = (store.state && store.state.proxy) || [];
  const web = (store.state && store.state.web) || {};
  const port = web.listen ? String(web.listen).replace(/^.*:/, "") : "—";
  return `
  <div class="stack">
    <div class="card">
      <div class="card-head">
        <div><h2>反向代理</h2><p>${list.length} 条规则 · 全部挂在 <span class="code">:${esc(port)}</span> · ${web.running ? badge("监听中", "ok") : badge(web.error || "未监听", "err")}</p></div>
        <button class="btn small" data-action="proxy-add">${icon("plus")}添加</button>
      </div>
      ${list.length ? `<div class="list">${list.map((entry) => {
        const r = entry.rule;
        const tone = !r.enable ? "" : entry.active ? "ok" : "err";
        const label = !r.enable ? "已停用" : entry.active ? "运行中" : "未监听";
        const domains = (r.domains || []).length ? r.domains : ["（默认站点）"];
        return `<div class="list-item">
          ${dot(tone)}
          <div class="li-main">
            <div class="li-title"><span class="name">${esc(r.name || domains[0])}</span></div>
            <div class="li-sub">
              ${domains.map((d) => `<span class="chip">${esc(d)}</span>`).join("")}
              <span>→</span>
              ${(r.upstreams || []).map((u) => `<span class="chip">${esc(u)}</span>`).join("")}
              ${r.basic_auth ? badge("有密码") : ""}
            </div>
          </div>
          <div class="li-side">
            ${badge(label, tone === "ok" ? "ok" : tone === "err" ? "err" : "")}
            <button class="btn small" data-action="proxy-edit" data-id="${esc(r.id)}" title="编辑">${icon("edit")}</button>
            <button class="btn small danger" data-action="proxy-del" data-id="${esc(r.id)}" title="删除">${icon("trash")}</button>
          </div>
        </div>`;
      }).join("")}</div>` : `
      <div class="empty">
        <div class="art">${icon("server")}</div>
        <h3>还没有 Web 服务</h3>
        <p>把域名指向这台设备，再把请求转发到内网的 NAS、路由或容器。</p>
        <button class="btn primary" data-action="proxy-add">${icon("plus")}添加服务</button>
      </div>`}
    </div>
    <div class="alert">${icon("shield")}<div>证书是全局的：一张通配证书覆盖所有域名，不在这里单独配置。域名要自己先在 Cloudflare 里加好 A 记录（灰云，指向公网 IP）。</div></div>
  </div>`;
}

/* ------------------------------------------------------------------- 穿透 */
function stunTone(st) {
  if (!st) return "warn";
  if (st.state === "active") return "ok";
  if (st.state === "error") return "err";
  return "warn";
}

function stunLabel(st) {
  if (!st) return "等待";
  return { active: "已确认", probing: "探测中", punching: "打洞中", starting: "启动中", error: "异常", stopped: "已停止" }[st.state] || "等待";
}

function viewStun() {
  const list = (store.state && store.state.stun) || [];
  const probe = store.probe;
  const srv = (store.state && store.state.stunlist) || {};
  const addr = list.map((e) => (e.status && e.status.access) || "").filter(Boolean)[0] || "";
  return `
  <div class="stack">
    <div class="card">
      <div class="card-body" style="padding:16px 20px">
        <div class="row" style="gap:14px">
          ${dot(addr ? "ok" : "warn")}
          <div style="flex:1;min-width:200px">
            <div class="li-title"><span class="name" style="font-size:14px">公网地址</span>
              ${addr ? badge("已确认", "ok") : badge("还没探测到", "warn")}
            </div>
            <div class="li-sub">${addr ? `<span class="addr">${esc(addr)}</span><span>这是外面看到的地址，端口来自路由器映射表</span>` : `<span>加一条规则，布噜会去问 STUN 服务器“外面看到我是谁”。</span>`}</div>
          </div>
          <div class="li-side"><button class="btn small" data-action="probe">${icon("search")}测 NAT</button></div>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="card-head">
        <div><h2>穿透规则</h2><p>${list.length} 条 · 每条在本机监听一个端口，靠 STUN 打洞映射到公网</p></div>
        <button class="btn small" data-action="stun-add">${icon("plus")}添加</button>
      </div>
      ${list.length ? `<div class="list">${list.map((entry) => {
        const r = entry.rule, st = entry.status || {};
        const tone = !r.enable ? "" : stunTone(st);
        const label = !r.enable ? "已停用" : stunLabel(st);
        return `<div class="list-item">
          ${dot(tone)}
          <div class="li-main">
            <div class="li-title">
              <span class="name">${esc(r.name || "穿透规则")}</span>
              ${badge(String(r.proto || "tcp").toUpperCase(), "brand")}
              ${badge(stunModeLabel(r.mode))}
              ${r.public_port ? badge("公网 " + r.public_port) : ""}
              ${r.webhook && r.webhook.enable ? badge("Webhook", "ok") : ""}
            </div>
            <div class="li-sub">
              ${st.access ? `<span class="addr">${esc(st.access)}</span>` : ""}
              ${st.observed_port ? `<span>STUN 看到 ${esc(st.observed_port)}</span>` : ""}
              ${st.last_heartbeat ? `<span>心跳 ${esc(fmtTime(st.last_heartbeat))}</span>` : ""}
              ${st.last_error ? `<span style="color:var(--err)">${esc(st.last_error)}</span>` : ""}
            </div>
          </div>
          <div class="li-side">
            ${badge(label, tone === "ok" ? "ok" : tone === "err" ? "err" : "")}
            ${st.access ? `<button class="copy" data-action="copy" data-copy="${esc(st.access)}" title="复制">${icon("copy")}</button>` : ""}
            <button class="btn small" data-action="stun-edit" data-id="${esc(r.id)}" title="编辑">${icon("edit")}</button>
            <button class="btn small danger" data-action="stun-del" data-id="${esc(r.id)}" title="删除">${icon("trash")}</button>
          </div>
        </div>`;
      }).join("")}</div>` : `
      <div class="empty">
        <div class="art">${icon("zap")}</div>
        <h3>还没有穿透规则</h3>
        <p>路由器已经做了端口转发的话，加一条「固定转发」规则，只让它确认外面的地址。</p>
        <button class="btn primary" data-action="stun-add">${icon("plus")}添加规则</button>
      </div>`}
    </div>

    <div class="grid cols-2">
      <div class="card">
        <div class="card-head">
          <div><h2>STUN 服务器</h2><p>${srv.count || 0} 台 · 来源 ${esc(srv.source || "内置")}</p></div>
          <button class="btn small" data-action="stun-refresh">${icon("refresh")}${srv.fetching ? "更新中" : "立即更新"}</button>
        </div>
        <div class="card-body">
          <dl class="kv">
            <dt>上次更新</dt><dd>${srv.updated ? esc(fmtTime(srv.updated)) : "尚未更新"}</dd>
            <dt>内置兜底</dt><dd>${srv.builtin || 0} 台</dd>
            <dt>更新地址</dt><dd><a href="#/settings">在设置里修改</a></dd>
          </dl>
          ${srv.error ? `<div class="alert warn" style="margin-top:12px">${icon("alert")}<div>${esc(srv.error)}<br>当前继续使用原有列表。</div></div>` : ""}
        </div>
      </div>

      <div class="card">
        <div class="card-head"><div><h2>两种模式</h2><p>你现在需要的是左边那种</p></div></div>
        <div class="card-body">
          <ul class="hint-list">
            <li><b>固定转发</b>：路由器上把公网端口转发到 Web 服务端口。这里只探测公网 IP，不开监听、不占端口，端口永远不会变。</li>
            <li><b>打洞转发</b>：没有公网 IP 或没法改路由器时才用。会监听本地端口并把流量转发到目标，公网端口由运营商分配，可能变化。</li>
            <li>不管哪种，都建议先用「测 NAT」看一眼映射类型。</li>
          </ul>
        </div>
      </div>
    </div>

    ${probe ? `<div class="card">
      <div class="card-head"><div><h2>最近一次 NAT 检测</h2><p>${esc(probe.local_ip || "")} · ${esc(probe.elapsed_ms || 0)} ms</p></div></div>
      <div class="card-body"><div class="probe">
        <div class="row">${badge(probe.nat_type, probe.compatible ? "ok" : "warn")}${badge("观测端口 " + (probe.mapped_port || "-"))}</div>
        <div class="help">${esc(probe.advice || "")}</div>
      </div></div>
    </div>` : ""}
  </div>`;
}

