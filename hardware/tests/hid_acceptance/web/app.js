// Based on ESP32 iPhone Agent by Henry Yuan, MIT. Observation only; no HID commands.
"use strict";
const $ = id => document.getElementById(id);
const box = $("box"), input = $("input"), hit = $("hit");
let session, seq = 0, ack = 0, count = 0, lost = 0, mode = "action", drag = null;
let queue = [], sending = false, heartbeatBusy = false, fault = "", transportError = "";
const limit = 512;
function rect(el) {
  const r = el.getBoundingClientRect();
  return {x:r.x, y:r.y, width:r.width, height:r.height};
}
function state() {
  return {clicks:count, text:input.value, codepoints:Array.from(input.value, c => c.codePointAt(0)),
    scrollY, mode, viewport:{width:innerWidth,height:innerHeight,dpr:devicePixelRatio},
    targets:{hit:rect(hit), input:rect(input), box:rect(box)}};
}
function health() {
  const error = fault || transportError;
  $("health").className = error ? "error" : "";
  // In static mode successful network traffic never changes the visible page.
  const label = error ? `证据不足：${error}` : mode === "static" ? "静止模式 · 后台继续记录" : `已落盘 ${ack} · 待确认 ${queue.length}`;
  if ($("health").textContent !== label) $("health").textContent = label;
}
async function request(path, data) {
  const controller = new AbortController(), timer = setTimeout(() => controller.abort(), 2500);
  try {
    const response = await fetch(path, {method:data ? "POST" : "GET", cache:"no-store",
      headers:data ? {"Content-Type":"application/json"} : {},
      body:data ? JSON.stringify(data) : undefined, signal:controller.signal});
    const body = await response.json();
    if (!response.ok) throw new Error(`${response.status}: ${body.error}`);
    return body;
  } finally { clearTimeout(timer); }
}
function record(type, event = {}) {
  if (!session) return;
  seq++;
  if (queue.length >= limit) { lost++; fault = "事件队列溢出，请开始新页面/用例"; health(); return; }
  const observed = {};
  for (const key of ["clientX","clientY","buttons","button","pointerId","key","code","deltaX","deltaY","deltaMode","isTrusted"])
    if (event[key] !== undefined) observed[key] = event[key];
  queue.push({...session, seq, type, client_time:Date.now(), monotonic_ms:performance.now(), event:observed, state:state()});
  health(); flush();
}
async function flush() {
  if (sending || !queue.length || fault) return;
  sending = true;
  try {
    while (queue.length && !fault) {
      const first = queue[0], result = await request("/event", first);
      if (result.ack !== first.seq) throw new Error("落盘确认序号不匹配");
      ack = result.ack; queue.shift(); transportError = ""; health();
    }
  } catch (e) { transportError = e.message; health(); }
  finally { sending = false; }
}
async function heartbeat() {
  if (!session || heartbeatBusy) return;
  heartbeatBusy = true;
  try {
    // Immutable counters/state describe the time of observation, not response arrival.
    await request("/heartbeat", {...session, last_seq:seq, pending:queue.length, lost,
      client_time:Date.now(), monotonic_ms:performance.now(), state:state()});
    if (!queue.length) transportError = "";
    health();
  } catch (e) { transportError = e.message; health(); }
  finally { heartbeatBusy = false; }
}
async function init() {
  try {
    session = await request("/session");
    $("identity").textContent = `run ${session.run_id} / page ${session.page_id}`;
    for (const id of ["hit", "input", "mode"]) $(id).disabled = false;
    hit.addEventListener("click", () => { hit.textContent = `CLICK COUNT: ${++count}`; });
    $("mode").addEventListener("click", () => {
      mode = mode === "action" ? "static" : "action";
      document.body.classList.toggle("static", mode === "static");
      input.blur(); $("tick").textContent = "";
      $("mode").textContent = mode === "static" ? "切换到动作模式" : "切换到静止模式";
      record("mode");
    });
    box.addEventListener("pointerdown", e => {
      drag = {x:e.clientX,y:e.clientY,left:box.offsetLeft,top:box.offsetTop}; box.setPointerCapture(e.pointerId);
    });
    box.addEventListener("pointermove", e => {
      if (!drag) return;
      box.style.left = `${drag.left + e.clientX - drag.x}px`;
      box.style.top = `${drag.top + e.clientY - drag.y}px`;
      record("pointermove", e);
    });
    for (const t of ["pointerup","pointercancel"]) box.addEventListener(t, () => { drag = null; });
    for (const t of ["click","pointerdown","pointerup","pointercancel","wheel","scroll","input","keydown","keyup"])
      window.addEventListener(t, e => record(t,e), {passive:true});
    record("loaded");
    setInterval(() => { if (mode === "action") $("tick").textContent = new Date().toLocaleTimeString(); }, 500);
    setInterval(() => { flush(); heartbeat(); }, 1000);
  } catch (e) { fault = `会话建立失败：${e.message}，请刷新`; health(); }
}
init();
