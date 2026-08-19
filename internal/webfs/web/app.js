'use strict';

// Native-frontend controller for the SPC engine. Talks to the JSON API the Go
// server exposes; no framework, no build step. Covers the full read/write
// loop: create chart -> add measurements -> view control chart + zones ->
// see violations -> see capability.

const api = {
  get:  (p)        => fetch(p).then(r => r.ok ? r.json() : Promise.reject(r.statusText)),
  post: (p, body)  => fetch(p, {method:'POST',  headers:{'Content-Type':'application/json'}, body: JSON.stringify(body)}).then(r => r.ok ? r.json() : Promise.reject(r.statusText)),
  del:  (p)        => fetch(p, {method:'DELETE'}).then(r => r.ok ? r.json() : Promise.reject(r.statusText)),
  put:  (p, body)  => fetch(p, {method:'PUT',    headers:{'Content-Type':'application/json'}, body: JSON.stringify(body)}).then(r => r.ok ? r.json() : Promise.reject(r.statusText)),
};

let current = null;

function el(id) { return document.getElementById(id); }

function loadCharts() {
  api.get('/charts?archived=0').then(d => {
    const box = el('chart-list'); box.innerHTML = '';
    (d.charts||[]).forEach(c => {
      const card = document.createElement('div'); card.className = 'card';
      card.innerHTML = `<div class="name">${escapeHtml(c.name)} <span class="id">${c.chart_id}</span></div>
        <div class="id">${c.chart_type} · n=${c.subgroup_size} · ${c.characteristic||''}</div>`;
      card.onclick = () => openChart(c.chart_id);
      box.appendChild(card);
    });
  }).catch(err => alert('加载失败: ' + err));
}

function openChart(id) {
  current = id;
  el('detail').hidden = false;
  el('chart-title').textContent = '控制图 ' + id;
  buildMeasForm();
  refresh();
}

function buildMeasForm() {
  api.get('/charts/' + current).then(c => {
    const box = el('meas-form'); box.innerHTML = '';
    if (c.chart_type === 'individuals') {
      box.innerHTML = `<div class="row"><label>单值</label><input id="v1" type="number" step="any"></div>`;
    } else if (c.chart_type === 'xbar_r') {
      let html = `<div class="row"><label>子组 (n=${c.subgroup_size})</label></div>`;
      for (let i = 0; i < c.subgroup_size; i++) html += `<input class="xv" type="number" step="any">`;
      box.innerHTML = html;
    } else {
      box.innerHTML = `<div class="row"><label>不合格数 d</label><input id="def" type="number" step="1"></div>
        <div class="row"><label>样本量 n</label><input id="nobs" type="number" step="1"></div>`;
    }
  });
}

function submitMeasurement() {
  api.get('/charts/' + current).then(c => {
    let body = {};
    if (c.chart_type === 'individuals') {
      body.values = [parseFloat(el('v1').value)];
    } else if (c.chart_type === 'xbar_r') {
      body.values = [...document.querySelectorAll('.xv')].map(e => parseFloat(e.value));
    } else {
      body.defectives = parseInt(el('def').value, 10);
      body.n_observed = parseInt(el('nobs').value, 10);
    }
    api.post('/charts/' + current + '/measurements', body).then(refresh).catch(e => alert('提交失败: ' + e));
  });
}

function refresh() {
  if (!current) return;
  Promise.all([
    api.get('/charts/' + current + '/limits'),
    api.get('/charts/' + current + '/violations'),
    api.get('/charts/' + current + '/capability'),
    api.get('/charts/' + current + '/zones'),
    api.get('/charts/' + current + '/measurements'),
  ]).then(([lim, vio, cap, zone, ms]) => {
    el('limits').textContent = JSON.stringify(lim, null, 2);
    el('violations').textContent = (vio.violations||[]).length ? JSON.stringify(vio.violations, null, 2) : '(无失控)';
    el('capability').textContent = JSON.stringify(cap, null, 2);
    renderMeasurements(ms.measurements||[]);
    renderChart(zone, ms.measurements||[]);
  }).catch(e => alert('刷新失败: ' + e));
}

function renderMeasurements(ms) {
  const box = el('measurements'); box.innerHTML = '';
  ms.forEach(m => {
    const card = document.createElement('div'); card.className = 'card';
    const tag = m.excluded ? ' (已排除)' : '';
    card.innerHTML = `<div class="name">#${m.subgroup_seq} = ${fmt(m.value_avg)}${tag}</div>
      <div class="id">${m.measurement_id}${m.range_value ? ' · MR/R=' + fmt(m.range_value) : ''}</div>`;
    box.appendChild(card);
  });
}

// A simple SVG line chart of the controlled quantity with CL/UCL/LCL lines.
function renderChart(zone, ms) {
  const box = el('chart-canvas'); box.innerHTML = '';
  if (!ms.length) { box.textContent = '(无数据)'; return; }
  const w = 700, h = 280, pad = 36;
  const xs = ms.map(m => m.value_avg);
  const allVals = xs.concat([zone.ucl||0, zone.lcl||0, zone.cl||0]);
  let min = Math.min(...allVals), max = Math.max(...allVals);
  if (min === max) { min -= 1; max += 1; }
  const span = max - min;
  const x = i => pad + (ms.length === 1 ? w/2 : (i/(ms.length-1))*(w-2*pad));
  const y = v => pad + (1-(v-min)/span)*(h-2*pad);
  let svg = `<svg width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">`;
  // guide lines
  const lines = [['UCL', zone.ucl, '#e11'], ['CL', zone.cl, '#888'], ['LCL', zone.lcl, '#e11'], ['+2s', zone.plus2s, '#69b'], ['-2s', zone.minus2s, '#69b']];
  lines.forEach(l => { if (l[1] !== undefined && l[1] !== null && !isNaN(l[1])) svg += line(x(0), y(l[1]), x(ms.length-1), y(l[1]), l[2]); });
  // points
  let path = '';
  xs.forEach((v, i) => path += (i ? ' L' : 'M') + ' ' + x(i).toFixed(1) + ' ' + y(v).toFixed(1));
  svg += `<path d="${path}" fill="none" stroke="#1f3a5f" stroke-width="2"/>`;
  xs.forEach((v, i) => svg += circle(x(i), y(v), 3, '#1f3a5f'));
  svg += `</svg>`;
  box.innerHTML = svg;
}

// --- SVG primitive helpers (template strings to avoid DOM verbosity) ---
function line(x1, y1, x2, y2, color) { return `<line x1="${x1}" y1="${y1}" x2="${x2}" y2="${y2}" stroke="${color}" stroke-width="1" stroke-dasharray="4 3"/>`; }
function circle(cx, cy, r, fill) { return `<circle cx="${cx}" cy="${cy}" r="${r}" fill="${fill}"/>`; }

function fmt(v) { return v === undefined || v === null ? '' : (Math.round(v*1e4)/1e4); }
function escapeHtml(s) { return (s||'').replace(/[&<>"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c])); }

// --- new-chart modal (inline prompt flow) ---
function newChart() {
  const name = prompt('控制图名称'); if (!name) return;
  const characteristic = prompt('质量特性名'); if (!characteristic) return;
  const ct = prompt('图类型 (individuals / xbar_r / p_chart)', 'individuals');
  const n = ct === 'xbar_r' ? parseInt(prompt('子组大小 n (2-10)', '5'), 10) : 1;
  const usl = prompt('USL (留空=无)');
  const lsl = prompt('LSL (留空=无)');
  const body = { name, characteristic, chart_type: ct, unit: prompt('单位', 'mm'), subgroup_size: n||1 };
  if (usl) body.usl = parseFloat(usl);
  if (lsl) body.lsl = parseFloat(lsl);
  api.post('/charts', body).then(() => loadCharts()).catch(e => alert('创建失败: ' + e));
}

window.onload = () => {
  el('refresh').onclick = loadCharts;
  el('new-btn').onclick = newChart;
  el('add-meas').onclick = submitMeasurement;
  loadCharts();
};
