const $ = (id) => document.getElementById(id);
let setupMode = false;
let statsTimer;

async function api(path, opts = {}) {
  const res = await fetch(path, { headers: { "Content-Type": "application/json" }, ...opts });
  const body = await res.json().catch(() => ({}));
  if (res.status === 401 && !path.startsWith("/api/auth")) { showAuth(); throw new Error("unauthorized"); }
  if (!res.ok) throw new Error(body.error || res.statusText);
  return body;
}

function fmtBytes(n) {
  const u = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return n.toFixed(i ? 1 : 0) + " " + u[i];
}

function fmtUptime(s) {
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
  return (d ? d + " ngày " : "") + h + " giờ " + m + " phút";
}

function setBar(id, pct) { $(id).style.width = Math.min(100, pct).toFixed(0) + "%"; }

async function init() {
  const st = await api("/api/auth/status");
  if (st.logged_in) return showDash();
  setupMode = !st.setup;
  showAuth();
}

function showAuth() {
  clearInterval(statsTimer);
  $("dash").classList.add("hidden");
  $("auth").classList.remove("hidden");
  $("auth-hint").textContent = setupMode ? "Lần đầu chạy: tạo tài khoản quản trị (mật khẩu ≥ 8 ký tự)." : "";
  $("auth-submit").textContent = setupMode ? "Tạo tài khoản" : "Đăng nhập";
}

$("auth-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const f = new FormData(e.target);
  $("auth-error").textContent = "";
  try {
    await api(setupMode ? "/api/auth/setup" : "/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ username: f.get("username"), password: f.get("password") }),
    });
    setupMode = false;
    showDash();
  } catch (err) { $("auth-error").textContent = err.message; }
});

$("logout").addEventListener("click", async () => {
  await api("/api/auth/logout", { method: "POST" });
  showAuth();
});

function showDash() {
  $("auth").classList.add("hidden");
  $("dash").classList.remove("hidden");
  loadStats();
  loadApps();
  loadMusic();
  loadTracks();
  loadBluetooth();
  clearInterval(statsTimer);
  statsTimer = setInterval(() => { loadStats(); loadMusic(); }, 5000);
}

async function loadStats() {
  const s = await api("/api/system");
  $("host").textContent = s.hostname;
  $("cpu").textContent = s.cpu_percent.toFixed(0) + "% · " + s.cpu_cores + " nhân";
  setBar("cpu-bar", s.cpu_percent);
  $("mem").textContent = fmtBytes(s.mem_used) + " / " + fmtBytes(s.mem_total);
  setBar("mem-bar", (100 * s.mem_used) / s.mem_total);
  $("disk").textContent = fmtBytes(s.disk_used) + " / " + fmtBytes(s.disk_total);
  setBar("disk-bar", (100 * s.disk_used) / s.disk_total);
  $("uptime").textContent = fmtUptime(s.uptime_sec);
  $("load").textContent = "load " + s.load1.toFixed(2);
}

const stateLabel = { running: "Đang chạy", stopped: "Đã dừng", not_installed: "Chưa cài", unknown: "Không rõ" };

function el(tag, props = {}, children = []) {
  const e = Object.assign(document.createElement(tag), props);
  e.append(...children);
  return e;
}

async function loadApps() {
  const apps = await api("/api/apps");
  $("apps").replaceChildren(...apps.map(appCard));
}

function appCard(a) {
  const btn = (label, action, cls = "") =>
    el("button", { textContent: label, className: cls, onclick: (e) => runAction(e.target, a, action) });
  const actions = [];
  if (!a.installed) actions.push(btn("Cài đặt", "install"));
  else {
    if (a.state === "running") {
      const url = location.protocol + "//" + location.hostname + ":" + a.port + (a.path || "");
      actions.push(el("button", { textContent: "Mở", onclick: () => window.open(url, "_blank") }));
      actions.push(btn("Dừng", "stop", "ghost"));
    } else actions.push(btn("Chạy", "start"));
    actions.push(btn("Gỡ", "remove", "danger"));
  }
  return el("div", { className: "card app" }, [
    el("div", { className: "top" }, [
      el("span", { className: "icon", textContent: a.icon }),
      el("div", {}, [el("h3", { textContent: a.name }), el("span", { className: "muted", textContent: a.category + " · cổng " + a.port })]),
    ]),
    el("p", { textContent: a.description }),
    el("div", {}, [el("span", { className: "badge " + a.state, textContent: stateLabel[a.state] || a.state })]),
    el("div", { className: "actions" }, actions),
  ]);
}

async function runAction(button, app, action) {
  if (action === "remove" && !confirm("Gỡ " + app.name + "? Dữ liệu của app vẫn được giữ lại.")) return;
  const card = button.closest(".app");
  card.querySelectorAll("button").forEach((b) => (b.disabled = true));
  button.textContent = action === "install" ? "Đang cài (tải image)…" : "Đang xử lý…";
  try { await api("/api/apps/" + app.id + "/" + action, { method: "POST" }); }
  catch (err) { alert(app.name + ": " + err.message); }
  loadApps();
}


// ---- Music ----
let allTracks = [];

async function loadMusic() {
  const m = await api("/api/music");
  $("music-off").classList.toggle("hidden", m.available);
  $("music-dir").textContent = m.dir;
  $("music-now").textContent = m.playing ? (m.paused ? "⏸ " : "▶ ") + (m.title || "") : "Đang dừng";
  if (m.playing && document.activeElement !== $("m-vol")) $("m-vol").value = Math.round(m.volume);
}

async function loadTracks() {
  try { allTracks = await api("/api/music/tracks"); } catch { allTracks = []; }
  $("m-count").textContent = "Danh sách bài (" + allTracks.length + ")";
  renderTracks();
}

function renderTracks() {
  const q = $("m-filter").value.toLowerCase();
  const items = allTracks.filter((t) => t.toLowerCase().includes(q)).slice(0, 300);
  $("m-tracks").replaceChildren(...items.map((t) =>
    el("li", { textContent: t, title: "Phát từ bài này", onclick: () => music("play", { track: t }) })));
}

async function music(action, body = {}) {
  try { await api("/api/music/" + action, { method: "POST", body: JSON.stringify(body) }); }
  catch (err) { alert(err.message); }
  loadMusic();
}

$("m-filter").addEventListener("input", renderTracks);
$("m-play").onclick = () => music("play");
$("m-pause").onclick = () => music("pause");
$("m-next").onclick = () => music("next");
$("m-prev").onclick = () => music("prev");
$("m-stop").onclick = () => music("stop");
$("m-vol").onchange = (e) => music("volume", { volume: Number(e.target.value) });

// ---- Bluetooth ----
const btIcon = (icon) => (icon || "").startsWith("audio") ? "🔊" : icon === "phone" ? "📱" : icon === "computer" ? "💻" : "🔹";

async function loadBluetooth() {
  let st;
  try { st = await api("/api/bluetooth"); }
  catch (err) { st = { available: true, powered: false, devices: [], error: err.message }; }
  const off = $("bt-off");
  $("bt-scan").classList.toggle("hidden", !st.available);
  if (!st.available) {
    off.textContent = "Chưa có bluetoothctl. Cài bằng: sudo pacman -S bluez bluez-utils && sudo systemctl enable --now bluetooth";
  } else if (!st.powered) {
    off.replaceChildren("Bluetooth đang tắt. ", el("button", { textContent: "Bật", onclick: () => bt("power") }));
  }
  off.classList.toggle("hidden", st.available && st.powered);
  if (st.error) { off.textContent = st.error; off.classList.remove("hidden"); }
  $("bt-devices").replaceChildren(...(st.devices || []).map(btRow));
}

function btRow(d) {
  const b = (label, action, cls = "") => el("button", { textContent: label, className: cls, onclick: (e) => bt(action, d.mac, e.target) });
  const state = d.connected ? "Đã kết nối" : d.paired ? "Đã ghép" : "Chưa ghép";
  return el("li", {}, [
    el("span", { textContent: btIcon(d.icon) }),
    el("span", { className: "name" }, [d.name, el("small", { textContent: state + " · " + d.mac })]),
    d.connected ? b("Ngắt", "disconnect", "ghost") : b("Kết nối", "connect"),
    ...(d.paired ? [b("Quên", "forget", "danger")] : []),
  ]);
}

async function bt(action, mac, button) {
  if (action === "forget" && !confirm("Quên thiết bị này?")) return;
  const scanBtn = $("bt-scan");
  if (button) { button.disabled = true; button.textContent = "…"; }
  if (action === "scan") { scanBtn.disabled = true; scanBtn.textContent = "Đang tìm (10 giây)…"; }
  try { await api("/api/bluetooth/" + action, { method: "POST", body: JSON.stringify({ mac }) }); }
  catch (err) { alert(err.message); }
  scanBtn.disabled = false; scanBtn.textContent = "Tìm thiết bị";
  loadBluetooth();
}

$("bt-scan").onclick = () => bt("scan");

init();
