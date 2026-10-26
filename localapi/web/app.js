const $ = s => document.querySelector(s)
const I = {
  home: '<svg viewBox="0 0 24 24"><path d="M4 11 12 4l8 7v8a1 1 0 0 1-1 1h-5v-6h-4v6H5a1 1 0 0 1-1-1z"/></svg>',
  back: '<svg viewBox="0 0 24 24"><path d="m14 6-6 6 6 6"/></svg>',
  film: '<svg viewBox="0 0 24 24"><rect x="4" y="4" width="16" height="16" rx="2.5"/><path d="M9 4v16M15 4v16M4 9h5M15 9h5M4 15h5M15 15h5"/></svg>',
  refresh: '<svg viewBox="0 0 24 24"><path d="M20 11a8 8 0 1 0-2.3 6.2"/><path d="M20 5v6h-6"/></svg>',
  shield: '<svg viewBox="0 0 24 24"><path d="M12 3 20 7v5c0 4.8-3.4 7.8-8 9-4.6-1.2-8-4.2-8-9V7z"/><path d="m8.5 12 2.3 2.3 4.7-4.8"/></svg>',
  play: '<svg viewBox="0 0 24 24"><path d="M7 4.8v14.4L20 12z"/></svg>',
  cam: '<svg viewBox="0 0 24 24"><rect x="3" y="7" width="13" height="10" rx="3"/><path d="m16 11 5-3v8l-5-3z"/></svg>',
  pip: '<svg viewBox="0 0 24 24"><rect x="3" y="5" width="18" height="14" rx="3"/><rect x="12" y="11" width="7" height="6" rx="1.5"/></svg>',
  pop: '<svg viewBox="0 0 24 24"><path d="M14 4h6v6M20 4l-8 8"/><path d="M19 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h5"/></svg>',
  online: '<svg viewBox="0 0 24 24"><path d="M4 12a8 8 0 0 1 16 0M7 12a5 5 0 0 1 10 0M10 12a2 2 0 0 1 4 0"/><circle cx="12" cy="16" r=".8"/></svg>',
  offline: '<svg viewBox="0 0 24 24"><path d="M4 12a8 8 0 0 1 13.3-6M20 12a8 8 0 0 1-13.3 6"/><path d="m4 4 16 16"/></svg>'
}
const S = { state: null, view: 'home', cam: null, clip: null, nonce: 0, wakeGen: 0, err: '', busy: false }
const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
const ico = (n, cls = '') => `<span class="ico ${cls}" aria-hidden="true">${I[n] || ''}</span>`

async function api(path, opt = {}) {
  const r = await fetch(path, { cache: 'no-store', ...opt, headers: { ...(opt.body ? { 'Content-Type': 'application/json' } : {}), ...(opt.headers || {}) } })
  if (!r.ok) {
    let msg = r.statusText || `HTTP ${r.status}`
    try { const j = await r.json(); msg = j.detail || j.title || msg } catch {}
    throw new Error(msg)
  }
  if (r.status === 204) return null
  const ct = r.headers.get('content-type') || ''
  return ct.includes('json') ? r.json() : r.text()
}

let toastTimer
function toast(msg) {
  const t = $('#toast')
  t.textContent = msg
  t.classList.add('on')
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => t.classList.remove('on'), 2600)
}

function ago(s) {
  if (!s) return ''
  const t = new Date(s)
  if (isNaN(t)) return ''
  const d = (Date.now() - t.getTime()) / 1000
  if (d < 45) return 'just now'
  if (d < 3600) return `${Math.max(1, Math.round(d / 60))}m ago`
  if (d < 172800) return `${Math.round(d / 3600)}h ago`
  return `${Math.round(d / 86400)}d ago`
}
function day(s) {
  const d = new Date(s)
  if (isNaN(d)) return ''
  return d.toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' })
}
function time(s) {
  const d = new Date(s)
  return isNaN(d) ? '' : d.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
}
function cameras() { return (S.state && S.state.cameras) || [] }
function findCam(id) { return cameras().find(c => c.id === id) }
function online() { return cameras().filter(c => c.online).length }
function watchOf(st, id) {
  const w = ((st && st.watches) || []).find(x => x.id === id)
  if (w) return w
  return st && st.watchId === id ? { id, live: !!st.watchLive, note: st.watchNote } : null
}
function liveIDs() {
  const st = S.state || {}, set = new Set()
  for (const w of st.watches || []) if (w.live) set.add(w.id)
  if (st.watchLive && st.watchId) set.add(st.watchId)
  return set
}
function clipByID(id) { return ((S.state && S.state.clips) || []).find(c => c.id === id) }

function header() {
  const st = S.state || {}
  const cam = S.view === 'camera' ? findCam(S.cam) : null
  const clip = S.view === 'clip' ? clipByID(S.clip) : null
  $('#back').innerHTML = S.view === 'home' ? I.home : I.back
  $('#back').title = S.view === 'home' ? 'Home' : 'Back'
  $('#clips').innerHTML = I.film
  $('#clips').classList.toggle('on', S.view === 'clips' || S.view === 'clip')
  $('#refresh').innerHTML = I.refresh
  const a = $('#arm')
  a.innerHTML = I.shield
  a.classList.toggle('on', !!(st.armedKnown && st.armed))
  a.disabled = !st.loggedIn || !st.armedKnown
  a.style.opacity = a.disabled ? '.45' : '1'
  $('#home').textContent = cam ? cam.name : clip ? (clip.camera || 'Clip') : (st.homeName || 'Blink cameras')
  const n = cameras().length
  const off = Math.max(0, n - online())
  let sub
  if (!st.loggedIn) sub = 'Not signed in'
  else if (S.view === 'camera' && cam) sub = `${cam.online ? 'Online' : 'Offline'}${cam.battery ? ' · ' + cam.battery : ''}`
  else if (S.view === 'clip') sub = clip ? `${day(clip.at)} · ${time(clip.at)}` : 'Clip'
  else if (S.view === 'clips') sub = `${(st.clips || []).length} recent clips`
  else sub = `${online()} of ${n} cameras online${off ? ` · ${off} offline` : ''}`
  $('#sub').innerHTML = st.loggedIn && S.view !== 'clips' && S.view !== 'clip' ? `<i></i>${esc(sub)}` : esc(sub)
}

function camCard(c) {
  const live = liveIDs().has(c.id)
  return `<article class="cam ${c.online || live ? '' : 'dead'}" data-cam="${esc(c.id)}" tabindex="0" role="button" aria-label="${esc(c.name)}">
    <img class="thumb" src="/api/cameras/${encodeURIComponent(c.id)}/thumb?v=${c.thumbRev || 0}" alt="">
    <div class="shade"></div>
    <div class="name">${esc(c.name)}</div>
    <div class="pill ${live ? 'live' : ''} ${c.online ? '' : 'off'}"><u></u>${live ? 'LIVE' : c.online ? 'Online' : 'Offline'}</div>
    ${c.online || live ? `<div class="play">${I.play}</div>` : `<div class="offmsg"><div><b>${esc(c.kind === 'doorbell' ? 'Doorbell' : 'Camera')} is offline</b><span>Open for details</span></div></div>`}
    <div class="foot"><span class="ago">${esc(ago(c.seen))}</span>${c.battery ? `<span class="bat">${esc(c.battery)}</span>` : ''}</div>
  </article>`
}

function homeView() {
  const st = S.state || {}
  if (!st.loggedIn) return `<div class="empty"><div class="big">${I.cam}</div><b>Sign in from the tray app or add a camera</b><span>Open BlinkView on this PC and sign in with your Blink account. Live video and controls will appear here.</span></div>`
  const list = cameras()
  const sync = st.sync ? `<div class="sync"><span class="dot ${st.sync.warn ? 'warn' : ''}"></span><b>${esc(st.sync.name)}</b><span class="${st.sync.warn ? 'warn' : ''}">${esc(st.sync.status)}</span></div>` : ''
  if (!list.length) return `${sync}<div class="empty"><div class="big">${I.cam}</div><b>No cameras yet</b><span>Cameras on this Blink account will show up here.</span></div>`
  return `${sync}<div class="phead"><h1>Cameras</h1><span>${list.length} total</span></div><div class="grid">${list.map(camCard).join('')}</div>`
}

function cameraView() {
  const c = findCam(S.cam)
  if (!c) return homeView()
  const poster = c.thumbRev ? `<img class="poster" src="/api/cameras/${encodeURIComponent(c.id)}/thumb?v=${c.thumbRev}" alt="">` : ''
  return `<div class="detail" data-detail="${esc(c.id)}">
    <div class="player">
      ${poster}
      ${c.online ? `<video id="live" autoplay playsinline controls></video>` : ''}
      ${c.online ? `<div class="vbtns"><button class="vbtn" data-act="pip" title="Picture in picture">${I.pip}</button><button class="vbtn" data-act="pop" title="Open in a separate window">${I.pop}</button></div>` : ''}
      <div class="loading ${c.online ? '' : 'hide'}" id="load"><div><div class="spin"></div><span>${c.online ? 'Waking camera…' : ''}</span></div></div>
    </div>
    <div class="dmeta"><div><h1>${esc(c.name)}</h1><p>${esc(c.online ? 'Online' : 'Offline')}${c.seen ? ` · last clip ${ago(c.seen)}` : ''}</p></div>${c.battery ? `<div class="dbat">${esc(c.battery)}</div>` : ''}</div>
    <div class="actions">
      <button class="btn" data-act="snap" ${c.online ? '' : 'disabled'}>Snapshot</button>
      <button class="btn" data-act="pip" ${c.online ? '' : 'disabled'}>PiP</button>
      <button class="btn" data-act="pop" ${c.online ? '' : 'disabled'}>Pop out</button>
      <button class="btn pri" data-act="retry">${c.online ? 'Reconnect' : 'Try live view'}</button>
    </div>
    <p class="note ${c.online ? '' : 'bad'}" id="lnote">${c.online ? 'Video may take a few seconds to start. Use the video controls for sound.' : 'This camera is offline. Live view will start when it comes back.'}</p>
  </div>`
}

function clipsView() {
  const st = S.state || {}
  if (!st.loggedIn) return homeView()
  const clips = st.clips || []
  if (!clips.length) {
    const hint = st.clipStatus ? `<span class="warn">${esc(st.clipStatus)}</span>` : '<span>Cloud clips and Sync Module recordings will appear here.</span>'
    return `<div class="empty"><div class="big">${I.film}</div><b>No clips yet</b>${hint}</div>`
  }
  let out = `<div class="phead"><h1>Clips</h1><span>${clips.length} recent</span></div><div class="cliplist">`
  let last = ''
  for (const c of clips) {
    const d = day(c.at)
    if (d !== last) { out += `<div class="day">${esc(d)}<span>${clips.filter(x => day(x.at) === d).length} events</span></div>`; last = d }
    out += `<button class="clip" data-clip="${esc(c.id)}"><div class="ic">${I.play}</div><div class="cx"><b>${esc(c.camera || c.home)}</b><span>${esc(c.home)}</span><span class="kind">${esc(c.kind)}</span></div><div class="tm">${esc(time(c.at))}</div></button>`
  }
  return out + '</div>'
}

function clipView() {
  const c = clipByID(S.clip) || { id: S.clip, camera: 'Clip', home: '', kind: '', at: null }
  const meta = [c.camera, day(c.at), time(c.at)].filter(Boolean).join(' · ')
  return `<div class="detail">
    <div class="player">
      <video id="clipvideo" autoplay playsinline controls src="/api/clips/${encodeURIComponent(c.id)}/video"></video>
      <div class="loading" id="cload"><div><div class="spin"></div><span>Preparing clip…</span></div></div>
    </div>
    <div class="dmeta"><div><h1>${esc(c.camera || 'Clip')}</h1><p>${esc(meta)}</p></div>${c.size ? `<div class="dbat">${(c.size / 1048576).toFixed(1)} MB</div>` : ''}</div>
    <div class="actions">
      <button class="btn" data-act="pipclip">Picture in picture</button>
      <button class="btn pri" data-act="clipsback">Back to clips</button>
    </div>
    <p class="note" id="clnote">${c.source === 'local' ? 'Preparing the recording from the Sync Module. This can take a few seconds.' : 'Loading clip.'}</p>
  </div>`
}

function bindClip() {
  const v = $('#clipvideo'), load = $('#cload'), note = $('#clnote')
  if (!v) return
  const hide = () => {
    load && load.classList.add('hide')
    if (note) {
      note.classList.remove('bad')
      const c = clipByID(S.clip)
      note.textContent = c && c.source === 'local' ? 'Sync Module recording. Use the video controls for sound.' : 'Use the video controls for sound.'
    }
  }
  v.addEventListener('playing', hide)
  v.addEventListener('loadeddata', hide)
  v.addEventListener('waiting', () => load && load.classList.remove('hide'))
  v.addEventListener('error', () => {
    if (!note) return
    load && load.classList.add('hide')
    note.classList.add('bad')
    note.textContent = 'This clip could not be loaded. It may have expired from local storage.'
  })
  setTimeout(() => { if (v.readyState >= 2) hide() }, 1200)
}

function bodySig() {
  const st = S.state || {}
  return JSON.stringify([S.view, S.cam, S.clip, S.nonce, st.loggedIn, st.armed, st.armedKnown, st.homeName, (st.cameras || []).map(c => [c.id, c.online, c.thumbRev, c.seen, c.battery]), st.sync, st.watchId, st.watchLive, st.watches, st.clipStatus, (st.clips || []).map(c => [c.id, c.kind, c.source])])
}

function render(force = false) {
  header()
  const el = $('#body')
  if (S.view === 'camera') {
    const key = `${S.cam}|${S.nonce}`
    if (force || el.dataset.key !== key) {
      stopVideo()
      el.dataset.key = key
      el.dataset.sig = ''
      el.innerHTML = cameraView()
      bindCamera()
    }
    return
  }
  if (S.view === 'clip') {
    const key = `clip|${S.clip}`
    if (force || el.dataset.key !== key) {
      stopVideo()
      stopClip()
      el.dataset.key = key
      el.dataset.sig = ''
      el.innerHTML = clipView()
      bindClip()
    }
    return
  }
  el.dataset.key = ''
  const sig = bodySig()
  if (!force && el.dataset.sig === sig) return
  el.dataset.sig = sig
  stopClip()
  el.innerHTML = S.view === 'clips' ? clipsView() : homeView()
  el.style.animation = 'none'
  void el.offsetWidth
  el.style.animation = ''
}

function stopVideo() {
  S.wakeGen++
  const v = $('#live')
  if (!v) return
  try { v.pause(); v.removeAttribute('src'); v.load() } catch {}
}

function stopClip() {
  const v = $('#clipvideo')
  if (!v) return
  try { v.pause(); v.removeAttribute('src'); v.load() } catch {}
}

function bindCamera() {
  const c = findCam(S.cam)
  if (!c || !c.online) return
  const v = $('#live'), load = $('#load'), note = $('#lnote')
  if (!v) return
  const gen = ++S.wakeGen
  const alive = () => gen === S.wakeGen && S.view === 'camera' && S.cam === c.id && v.isConnected
  const hide = () => load && load.classList.add('hide')
  const show = () => { if (load) load.classList.remove('hide') }
  const fail = msg => {
    if (!alive()) return
    hide()
    note.classList.add('bad')
    note.textContent = msg
  }
  note.classList.remove('bad')
  note.textContent = 'Waking camera.'
  const attach = () => {
    if (!alive()) return
    v.addEventListener('playing', hide)
    v.addEventListener('loadeddata', hide)
    v.addEventListener('waiting', show)
    v.addEventListener('error', () => fail('Live view could not start. The camera may be offline, busy, or still waking up.'))
    v.src = `/api/cameras/${encodeURIComponent(c.id)}/live.mp4?t=${Date.now()}`
    note.textContent = 'Use the video controls for sound.'
    v.play().catch(() => {
      if (!alive()) return
      note.classList.remove('bad')
      note.textContent = 'Press play on the video for sound.'
    })
    setTimeout(() => { if (alive() && v.readyState >= 2) hide() }, 1200)
  }
  ;(async () => {
    try {
      if (!(watchOf(S.state, c.id) || {}).live) {
        await api(`/api/cameras/${encodeURIComponent(c.id)}/wake`, { method: 'POST' })
      }
      let seen = false
      for (let i = 0; i < 120; i++) {
        if (!alive()) return
        const st = await api('/api/state')
        S.state = st
        const w = watchOf(st, c.id)
        if (w && w.live) seen = true
        if (seen && (!w || !w.live)) {
          fail(String((w && w.note) || '').replace(/^Error:\s*/, '') || 'Live view stopped before it started. Try Reconnect.')
          return
        }
        if (w && w.live && !w.note) { attach(); return }
        if (w && String(w.note || '').startsWith('Error:')) {
          fail(String(w.note).replace(/^Error:\s*/, ''))
          return
        }
        if (i === 40) note.textContent = 'Still waking camera.'
        await new Promise(r => setTimeout(r, 500))
      }
      fail('Live view is taking too long. Try Reconnect.')
    } catch (e) {
      fail(e.message)
    }
  })()
}

async function poll(silent = false) {
  try {
    const st = await api('/api/state')
    S.state = st
    S.err = ''
    render()
  } catch (e) {
    S.err = e.message
    if (!silent) toast(e.message)
    header()
  }
}

async function arm() {
  const st = S.state
  if (!st || !st.armedKnown || S.busy) return
  S.busy = true
  const next = !st.armed
  st.armed = next
  render(true)
  try {
    await api('/api/arm', { method: 'POST', body: JSON.stringify({ armed: next }) })
    toast(next ? 'System armed' : 'System disarmed')
    setTimeout(() => poll(true), 1200)
  } catch (e) {
    st.armed = !next
    render(true)
    toast(e.message)
  } finally { S.busy = false }
}

function openCamera(id) {
  const c = findCam(id)
  if (!c) return
  stopClip()
  S.view = 'camera'
  S.cam = id
  S.clip = null
  S.nonce++
  render(true)
}

function openClip(id) {
  stopVideo()
  S.view = 'clip'
  S.clip = id
  S.cam = null
  S.nonce++
  render(true)
}

function openClips() {
  stopVideo()
  stopClip()
  S.view = 'clips'
  S.cam = null
  S.clip = null
  S.nonce++
  render(true)
}

document.addEventListener('click', e => {
  const clip = e.target.closest('[data-clip]')
  if (clip) { openClip(clip.dataset.clip); return }
  const cam = e.target.closest('[data-cam]')
  if (cam) { openCamera(cam.dataset.cam); return }
  const act = e.target.closest('[data-act]')
  if (act) {
    if (act.dataset.act === 'pip' || act.dataset.act === 'pipclip') {
      const v = act.dataset.act === 'pip' ? $('#live') : $('#clipvideo')
      if (!v) return
      if (document.pictureInPictureElement) { document.exitPictureInPicture().catch(() => {}) ; return }
      if (!document.pictureInPictureEnabled) { toast('Picture-in-picture is not supported by this browser'); return }
      v.requestPictureInPicture().catch(x => toast(x.message || 'Picture-in-picture could not start'))
    }
    if (act.dataset.act === 'pop') {
      const c = findCam(S.cam)
      if (!c) return
      const w = window.open(`/pip?cam=${encodeURIComponent(c.id)}`, `blink-${c.id}`, 'popup,width=640,height=420,resizable=yes')
      if (!w) toast('Allow pop-ups for this page to use the pop-out player')
    }
    if (act.dataset.act === 'clipsback') { openClips(); return }
    if (act.dataset.act === 'snap') {
      const id = S.cam
      api(`/api/cameras/${encodeURIComponent(id)}/snapshot`, { method: 'POST' }).then(() => {
        toast('Snapshot requested')
        setTimeout(() => poll(true), 5000)
      }).catch(x => toast(x.message))
    }
    if (act.dataset.act === 'retry') {
      const c = findCam(S.cam)
      if (!c) return
      if (!c.online) { poll(); toast('Camera is offline'); return }
      S.nonce++
      render(true)
    }
    return
  }
})
$('#back').addEventListener('click', () => {
  if (S.view === 'clip') { openClips(); return }
  if (S.view === 'camera') stopVideo()
  if (S.view !== 'home') { S.view = 'home'; S.cam = null; S.clip = null; S.nonce++; render(true) }
  else poll()
})
$('#clips').addEventListener('click', () => {
  if (S.view === 'clip') { openClips(); return }
  if (S.view === 'clips') { S.view = 'home'; S.cam = null; S.clip = null; S.nonce++; render(true); return }
  openClips()
})
$('#refresh').addEventListener('click', async () => {
  try {
    await api('/api/refresh', { method: 'POST' })
    toast('Refreshing cameras')
    setTimeout(() => poll(true), 1600)
  } catch (e) { toast(e.message) }
  if (S.view === 'camera') { S.nonce++; render(true) }
})
$('#arm').addEventListener('click', arm)

document.addEventListener('keydown', e => {
  if (e.key !== 'Escape') return
  if (S.view === 'clip') { openClips(); return }
  if (S.view !== 'home') {
    stopVideo(); stopClip(); S.view = 'home'; S.cam = null; S.clip = null; S.nonce++; render(true)
  }
})

async function boot() {
  const q = new URLSearchParams(location.search)
  await poll(true)
  if (q.get('clip')) { openClip(q.get('clip')); return }
  const cam = q.get('cam')
  if (cam) {
    for (let i = 0; i < 20 && !findCam(cam); i++) await new Promise(r => setTimeout(r, 500))
    openCamera(cam)
  }
}
boot()
setInterval(() => { if (!document.hidden) poll(true) }, 10000)
