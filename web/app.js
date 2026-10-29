const embedded = window.parent !== window;
const pendingHostRequests = new Map();
let hostSequence = 0;
window.addEventListener('message', (event) => {
  if (event.source !== window.parent || event.origin !== location.origin) return;
  const d = event.data;
  if (d?.type !== 'cpamp:plugin-api:response' || d.version !== 1) return;
  const pending = pendingHostRequests.get(d.id);
  if (pending) { clearTimeout(pending.timer); pendingHostRequests.delete(d.id); pending.resolve(d); }
});
function panelRequest(path, method, body) {
  return new Promise((resolve, reject) => {
    const id = 'quality-' + (++hostSequence);
    const timer = setTimeout(() => { pendingHostRequests.delete(id); reject(new Error('面板连接已失效，请刷新面板后重试。')); }, 35000);
    pendingHostRequests.set(id, { resolve, timer });
    window.parent.postMessage({ type: 'cpamp:plugin-api:request', version: 1, id, path: API + path, method, body }, location.origin);
  });
}
const API = '/v0/management/plugins/model-quality/';
const $ = (id) => document.getElementById(id);
const state = { data: null, key: '', selectedAccounts: new Set(), selectedModels: new Set(), selectedCases: new Set(['probability', 'code', 'instruction']), batch: null, baseline: null, baselineID: '', artifacts: new Map(), cards: new Map(), history: [], historyLimit: 20, tab: 'setup', pollBusy: false, busy: false };
const labels = { queued: '排队中', running: '生成中', cancelling: '正在取消', completed: '已完成', failed: '请求失败', cancelled: '已取消', interrupted: '服务中断' };

function node(tag, attrs = {}, ...children) {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === 'class') n.className = v;
    else if (k.startsWith('on')) n.addEventListener(k.slice(2), v);
    else if (['value', 'checked', 'disabled', 'hidden', 'readOnly', 'open'].includes(k)) n[k] = v;
    else n.setAttribute(k, v);
  }
  for (const child of children.flat()) if (child != null) n.append(child instanceof Node ? child : document.createTextNode(String(child)));
  return n;
}
function badge(text, kind = '') { return node('span', { class: 'badge ' + kind }, text); }
function notice(text, error = false) { $('notice').textContent = text; $('notice').className = error ? 'error' : ''; $('notice').hidden = !text; }
function readableDate(ms) { return new Date(ms).toLocaleString('zh-CN', { hour12: false }); }
function duration(ms) { return ms ? (ms / 1000).toFixed(1) + 's' : '—'; }
function displayAccount(a) { return a.email || a.account || a.label || a.name || a.id; }
function running(b) { return b && ['running', 'cancelling'].includes(b.status); }
function isObjective(c) { return ['json', 'choice', 'solution', 'format-json'].includes(c.kind); }

async function api(path, options = {}) {
  const headers = { ...options.headers };
  if (state.key) headers.Authorization = 'Bearer ' + state.key;
  if (options.body != null) { headers['Content-Type'] = 'application/json'; options.body = JSON.stringify(options.body); }
  let status, body;
  if (embedded) {
    const response = await panelRequest(path, options.method || 'GET', options.body);
    status = response.status; body = response.body;
  } else {
    const r = await fetch(API + path, { ...options, headers, credentials: 'same-origin', cache: 'no-store' });
    status = r.status;
    if (!(r.headers.get('content-type') || '').includes('application/json')) throw new Error(`管理接口返回 HTTP ${status}，请检查插件状态。`);
    body = await r.json();
  }
  if (status === 401 || status === 403) {
    $('login').hidden = embedded;
    throw new Error(embedded ? '面板登录已失效，请重新登录面板。' : '请输入当前服务的管理密钥。');
  }
  if (status < 200 || status >= 300) throw new Error(body.error || `HTTP ${status}`);
  $('login').hidden = true;
  return body;
}
function task(promise) { return Promise.resolve(promise).catch((e) => notice(e.message, true)); }
function changeTab(tab) {
  state.tab = tab;
  for (const id of ['setup', 'results', 'history']) $(id).hidden = id !== tab;
  document.querySelectorAll('[data-tab]').forEach((n) => n.classList.toggle('selected', n.dataset.tab === tab));
  if (tab === 'history') task(loadHistory());
}

async function bootstrap() {
  $('refresh').disabled = true;
  try {
    state.data = await api('bootstrap');
    state.baselineID = state.data.baseline || '';
    const knownAccounts = new Set(state.data.accounts.filter((a) => !a.disabled).map((a) => a.id));
    state.selectedAccounts = new Set([...state.selectedAccounts].filter((id) => knownAccounts.has(id)));
    const models = [...new Set(state.data.accounts.flatMap((a) => a.models || []))].sort();
    state.selectedModels = new Set([...state.selectedModels].filter((m) => models.includes(m)));
    if (!state.selectedModels.size && models.length) state.selectedModels.add(models.find((m) => m === 'gpt-6-astra') || models.find((m) => m.includes('codex')) || models[0]);
    renderCases(); renderModels(); renderAccounts(); renderProviderFilter(); updateEstimate();
    $('build-version').textContent = `v${state.data.version} · ${state.data.grader_version}`;
    $('retention').textContent = `保留 ${state.data.config.retention_days} 天 · 最多 ${state.data.config.max_history} 批 / ${state.data.config.max_disk_mb} MiB。基线单独保留；可在插件配置中调整。`;
    if (state.data.active) { await openBatch(state.data.active); changeTab('results'); }
    notice('');
  } finally { $('refresh').disabled = false; }
}

function renderCases() {
  $('cases').replaceChildren(...state.data.cases.map((c) => node('label', { class: 'case-choice' },
    node('input', { type: 'checkbox', checked: state.selectedCases.has(c.id), onchange: (e) => { e.target.checked ? state.selectedCases.add(c.id) : state.selectedCases.delete(c.id); updateEstimate(); } }),
    node('span', { class: 'case-copy' }, node('strong', {}, c.title), node('small', {}, c.category)), badge(isObjective(c) ? '自动评分' : '人工比较', isObjective(c) ? 'blue' : ''))));
  const previous = $('inspect-case').value;
  $('inspect-case').replaceChildren(...state.data.cases.map((c) => node('option', { value: c.id }, c.title)));
  if (state.data.cases.some((c) => c.id === previous)) $('inspect-case').value = previous;
  showReference();
}
function showReference() {
  const c = state.data?.cases.find((c) => c.id === $('inspect-case').value); if (!c) return;
  $('prompt-preview').textContent = c.prompt;
  $('reference').textContent = c.reference;
  $('expected').textContent = c.expected ? JSON.stringify(c.expected, null, 2) : '';
  $('expected').hidden = !c.expected;
  $('source').replaceChildren('来源：' + c.source + ' · ' + c.version + ' · ' + c.hash.slice(0, 12));
  if (c.source_url) $('source').append(' ', node('a', { href: c.source_url, target: '_blank', rel: 'noreferrer' }, '原始来源 ↗'));
}
function renderModels() {
  const q = $('model-search').value.toLowerCase();
  const models = [...new Set((state.data?.accounts || []).flatMap((a) => a.models || []))].sort();
  $('models').replaceChildren(...models.filter((m) => m.toLowerCase().includes(q)).map((m) => node('button', { type: 'button', class: 'model-chip' + (state.selectedModels.has(m) ? ' selected' : ''), 'aria-pressed': String(state.selectedModels.has(m)), onclick: () => { state.selectedModels.has(m) ? state.selectedModels.delete(m) : state.selectedModels.add(m); renderModels(); updateEstimate(); } }, m)));
  if (!models.length) $('models').append(node('p', { class: 'muted' }, 'CPA 暂未注册可测试模型。'));
}
function filteredAccounts() {
  const q = $('account-search').value.toLowerCase(); const p = $('provider-filter').value;
  return (state.data?.accounts || []).filter((a) => (!p || a.provider === p) && [displayAccount(a), a.note, a.provider].join(' ').toLowerCase().includes(q));
}
function renderProviderFilter() {
  const previous = $('provider-filter').value;
  $('provider-filter').replaceChildren(node('option', { value: '' }, '全部提供商'), ...[...new Set(state.data.accounts.map((a) => a.provider))].sort().map((p) => node('option', { value: p }, p || '其他')));
  $('provider-filter').value = previous;
}
function renderAccounts() {
  const accounts = filteredAccounts();
  $('account-count').textContent = `${state.selectedAccounts.size} / ${state.data.accounts.length}`;
  $('accounts').replaceChildren(...accounts.map((a) => node('label', { class: 'account-choice' + (a.disabled ? ' disabled' : '') },
    node('input', { type: 'checkbox', disabled: a.disabled || !(a.models || []).length, checked: state.selectedAccounts.has(a.id), onchange: (e) => { e.target.checked ? state.selectedAccounts.add(a.id) : state.selectedAccounts.delete(a.id); renderAccounts(); updateEstimate(); } }),
    node('span', { class: 'account-identity' }, node('strong', {}, displayAccount(a)), a.note ? node('small', {}, a.note) : null),
    badge(a.disabled ? '已禁用' : a.unavailable ? '当前不可用' : a.provider || '账号', a.disabled || a.unavailable ? 'bad' : ''))));
  if (!accounts.length) $('accounts').append(node('p', { class: 'empty-note' }, '没有匹配的账号'));
}
function requestSpec() {
  const targets = [];
  for (const a of state.data?.accounts || []) if (state.selectedAccounts.has(a.id)) for (const model of state.selectedModels) if ((a.models || []).includes(model)) targets.push({ auth_id: a.id, model });
  const cases = [...state.selectedCases]; if ($('use-custom').checked) cases.push('custom');
  return { targets, case_ids: cases, custom_prompt: $('use-custom').checked ? $('custom-prompt').value : '', effort: $('effort').value, concurrency: Number($('concurrency').value), repeats: Number($('repeats').value) };
}
function updateEstimate() {
  if (!state.data) return;
  const s = requestSpec(); const count = s.targets.length * s.case_ids.length * s.repeats;
  const excluded = state.selectedAccounts.size * state.selectedModels.size - s.targets.length;
  $('request-count').textContent = `${s.targets.length} 个账号/模型组合 · ${s.case_ids.length} 题 · ${s.repeats || 0} 轮`;
  $('request-detail').textContent = `${count || 0} 次请求 / 上限 ${state.data.max_jobs}` + (excluded ? ` · ${excluded} 个不支持的组合已排除` : '');
  const active = running(state.batch) || !!state.data.active;
  $('start').disabled = state.busy || active || count < 1 || count > state.data.max_jobs || !Number.isInteger(s.concurrency) || s.concurrency < 1 || s.concurrency > 10 || !Number.isInteger(s.repeats) || s.repeats < 1 || s.repeats > 5 || ($('use-custom').checked && !s.custom_prompt.trim());
  $('start').textContent = active ? '测试运行中' : `开始 ${count || 0} 次测试`;
}

async function start() {
  state.busy = true; updateEstimate();
  try { const b = await api('batches', { method: 'POST', body: requestSpec() }); state.data.active = b.id; await openBatch(b.id); changeTab('results'); notice('测试已开始。账号已锁定，可离开页面，稍后在历史中查看结果。'); }
  finally { state.busy = false; updateEstimate(); }
}
async function openBatch(id) {
  state.cards.clear(); $('result-cards').replaceChildren(); state.artifacts.clear();
  state.batch = await api('batch?id=' + encodeURIComponent(id));
  if (state.baselineID && state.baseline?.id !== state.baselineID) state.baseline = await api('batch?id=' + encodeURIComponent(state.baselineID));
  if (!state.baselineID) state.baseline = null;
  $('result-case').replaceChildren(node('option', { value: '' }, '全部题目'), ...state.batch.cases.map((c) => node('option', { value: c.id }, c.title)));
  $('result-status').value = ''; $('only-differences').checked = false;
  renderBatch();
}
async function poll() {
  if (state.pollBusy || (!running(state.batch) && !state.data?.active)) return;
  state.pollBusy = true;
  try {
    const id = running(state.batch) ? state.batch.id : state.data.active;
    const b = await api('batch?id=' + encodeURIComponent(id));
    if (!running(b)) { state.data.active = ''; updateEstimate(); }
    if (state.batch?.id !== id) return;
    state.batch = b;
    if (!running(b)) { state.data.active = ''; updateEstimate(); }
    renderBatch();
  } catch (e) { notice('进度刷新失败：' + e.message + '；后台任务状态会在连接恢复后继续读取。', true); }
  finally { state.pollBusy = false; }
}
function scoreSummary(items) {
  const complete = items.filter((it) => it.status === 'completed');
  const graded = complete.filter((it) => it.grade?.score != null);
  const objective = items.filter((it) => state.batch.cases.some((c) => c.id === it.case_id && isObjective(c) && c.kind !== 'format-json'));
  return { complete, graded, objective, score: graded.length ? graded.reduce((a, it) => a + it.grade.score, 0) / graded.length : null };
}
function renderBatch() {
  const b = state.batch; if (!b) return;
  $('empty-results').hidden = true; $('batch-content').hidden = false;
  $('batch-title').textContent = `${labels[b.status] || b.status} · ${readableDate(b.created_at)}`;
  $('batch-meta').textContent = `${b.items.length} 次请求 · 并发 ${b.concurrency} · 推理 ${b.effort} · ${b.suite_version}` + (b.retry_of ? ' · 手动重试批次' : '');
  const finished = b.items.filter((it) => !['queued', 'running'].includes(it.status)).length;
  $('progress').max = b.items.length || 1; $('progress').value = finished;
  $('run-dot').hidden = !running(b);
  $('cancel').hidden = !running(b); $('cancel').disabled = b.status === 'cancelling';
  $('cancel').textContent = b.status === 'cancelling' ? '等待上游停止…' : '取消本批';
  $('retry-failed').hidden = running(b) || !b.items.some((it) => it.status !== 'completed');
  $('baseline').disabled = b.status !== 'completed'; $('baseline').textContent = b.id === state.baselineID ? '当前比较基线' : '设为比较基线';
  const s = scoreSummary(b.items); const allScored = s.objective.length > 0 && s.graded.length === s.objective.length;
  const box = (title, value, help) => node('div', { class: 'summary-box' }, node('small', {}, title), node('strong', {}, value), node('small', {}, help));
  $('batch-summary').replaceChildren(box('请求进度', `${finished}/${b.items.length}`, `成功 ${s.complete.length} · 失败 ${b.items.filter((it) => it.status === 'failed').length}`),
    box('客观题平均分', allScored ? s.score.toFixed(1) : '—', allScored ? '每题等权；格式另列' : '须完整取得全部客观题分数'),
    box('评分覆盖', `${s.graded.length}/${s.objective.length}`, '失败、截断与无法解析单列'),
    box('输出用量', b.items.reduce((n, it) => n + Number(it.usage?.output_tokens || it.usage?.completion_tokens || 0), 0).toLocaleString(), '上游报告的输出 tokens'));
  $('batch-error').hidden = !b.error; $('batch-error').textContent = b.error || '';
  $('baseline-help').textContent = state.baselineID ? '已设置基线。仅比较同账号、同模型、同推理档位、同题目版本及同轮次的客观题；差值不是模型身份判断。' : '可在历史中设置基线，再以相同题目和参数复测，查看分数变化。';
  renderResultCards();
}
function baselineDelta(it, c) {
  const b = state.baseline; if (!b || b.id === state.batch.id || b.effort !== state.batch.effort || b.grader_version !== state.batch.grader_version || it.grade?.score == null) return null;
  const otherCase = b.cases.find((x) => x.hash === c.hash); if (!otherCase) return null;
  const old = b.items.find((x) => x.auth_id === it.auth_id && x.model === it.model && x.case_id === otherCase.id && x.trial === it.trial && x.status === 'completed' && x.grade?.score != null);
  return old ? it.grade.score - old.grade.score : null;
}
function renderResultCards() {
  const b = state.batch; if (!b) return;
  const caseID = $('result-case').value; const status = $('result-status').value;
  for (const it of b.items) {
    let entry = state.cards.get(it.id); const c = b.cases.find((c) => c.id === it.case_id);
    const signature = JSON.stringify([it, state.baselineID]);
    if (!entry) { entry = { el: node('article', { class: 'result-card', 'data-item': it.id }), signature: '' }; state.cards.set(it.id, entry); $('result-cards').append(entry.el); }
    entry.el.hidden = (!!caseID && caseID !== it.case_id) || (!!status && status !== it.status) || ($('only-differences').checked && it.grade?.score === 100 && it.grade?.format === 'passed');
    if (entry.signature === signature) continue; entry.signature = signature;
    const body = node('div', { class: 'card-body' });
    const tone = it.status === 'completed' ? 'good' : ['failed', 'cancelled', 'interrupted'].includes(it.status) ? 'bad' : 'blue';
    body.append(node('div', { class: 'status-row' }, badge(labels[it.status] || it.status, tone), badge(c.title), badge(`第 ${it.trial} 轮`)));
    if (it.grade) {
      const g = it.grade;
      body.append(node('div', { class: 'score-line' }, c.kind === 'format-json' ? '格式测试' : '客观题', node('strong', {}, c.kind === 'format-json' ? '仅检查格式' : g.score == null ? '答案不可评分' : `${g.score.toFixed(1)} 分 · ${g.passed}/${g.total} 字段`)));
      body.append(node('div', { class: 'status-row' }, badge('格式' + (g.format === 'passed' ? '通过' : '未通过'), g.format === 'passed' ? 'good' : 'bad'), g.reason ? node('small', { class: 'muted' }, g.reason) : null));
      const delta = baselineDelta(it, c); if (delta != null) body.append(badge(`较基线 ${delta >= 0 ? '+' : ''}${delta.toFixed(1)} 分`, delta < 0 ? 'bad' : 'good'));
      if (g.checks?.length) { const details = node('details', {}, node('summary', {}, '逐项评分与参考')); details.append(checkTable(g.checks)); body.append(details); }
    } else if (it.status === 'completed') body.append(node('p', { class: 'muted' }, c.kind === 'visual' ? '绘图仅人工对比，不折算智力分数。' : '此题保留完整回复供人工对比。'));
    if (it.error) body.append(node('p', { class: 'error' }, it.error));
    body.append(node('div', { class: 'metrics' }, `总耗时 ${duration(it.duration_ms || (it.started_at ? Date.now() - it.started_at : 0))} · 首字 ${duration(it.ttft_ms)} · 输出 ${Number(it.usage?.output_tokens || it.usage?.completion_tokens || 0).toLocaleString()} tokens`));
    if (it.response_model) body.append(node('p', { class: 'muted' }, `响应自报模型：${it.response_model}`));
    if (it.preview && !state.artifacts.has(it.id)) body.append(node('pre', { class: 'live-preview' }, it.preview));
    const actions = node('div', { class: 'actions' });
    if (it.status === 'completed' || it.preview) actions.append(node('button', { onclick: () => task(showArtifact(it, c)) }, it.has_html ? '预览 / 原始回复' : '完整回复'));
    if (it.has_html && it.status === 'completed') actions.append(node('button', { onclick: () => task(inlinePreview(it, entry.el)) }, '并排预览'));
    if (!running(b)) actions.append(node('button', { onclick: () => task(retry([it.id])) }, '单项重测'));
    body.append(actions);
    const cached = state.artifacts.get(it.id); if (cached?.inline && cached.html) body.append(previewFrame(cached.html, c.title));
    entry.el.replaceChildren(node('header', {}, node('h3', {}, it.account), it.note ? node('p', {}, it.note) : null, node('p', {}, `${it.provider} · ${it.model} · ${b.effort}`)), body);
  }
}
function checkTable(checks) {
  const table = node('table', { class: 'checks' }, node('thead', {}, node('tr', {}, ['字段', '参考', '实际', '结果'].map((x) => node('th', {}, x)))));
  table.append(node('tbody', {}, checks.map((x) => node('tr', {}, node('td', {}, x.field), node('td', {}, node('pre', {}, JSON.stringify(x.expected))), node('td', {}, node('pre', {}, x.actual === undefined ? '缺失' : JSON.stringify(x.actual))), node('td', {}, badge(x.passed ? '通过' : '不符', x.passed ? 'good' : 'bad'))))));
  return table;
}

const PREVIEW_POLICY = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; connect-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'";
function previewDocument(html) {
  const doc = new DOMParser().parseFromString(html, 'text/html');
  doc.querySelectorAll('base, meta[http-equiv]').forEach((n) => n.remove());
  const meta = doc.createElement('meta'); meta.httpEquiv = 'Content-Security-Policy'; meta.content = PREVIEW_POLICY; doc.head.prepend(meta);
  return '<!doctype html>\n' + doc.documentElement.outerHTML;
}
function previewFrame(html, title) {
  const f = node('iframe', { class: 'preview', title: title + '：隔离预览', sandbox: 'allow-scripts', referrerPolicy: 'no-referrer', allow: "camera 'none'; microphone 'none'; geolocation 'none'; clipboard-read 'none'; clipboard-write 'none'" });
  f.srcdoc = previewDocument(html); return f;
}
async function artifact(it) {
  if (!state.artifacts.has(it.id)) state.artifacts.set(it.id, await api(`artifact?id=${encodeURIComponent(state.batch.id)}&item=${encodeURIComponent(it.id)}`));
  return state.artifacts.get(it.id);
}
async function inlinePreview(it, card) {
  const data = await artifact(it); data.inline = !data.inline;
  const entry = state.cards.get(it.id); if (entry) entry.signature = ''; renderResultCards();
  if (data.inline) card.querySelector('iframe')?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
}
async function showArtifact(it, c) {
  const data = await artifact(it); const item = data.item;
  $('artifact-title').textContent = `${it.account} · ${c.title}`;
  const body = $('artifact-body'); body.replaceChildren();
  body.append(node('p', { class: 'muted' }, `请求模型 ${it.model} · 响应自报 ${item.response_model || '未提供'} · ${state.batch.effort} · ${c.hash.slice(0, 12)}`));
  if (data.html && item.status === 'completed') body.append(previewFrame(data.html, c.title));
  const actions = node('div', { class: 'actions artifact-buttons' }, node('button', { onclick: () => task(navigator.clipboard.writeText(item.reply || '').then(() => notice('已复制原始回复。'))) }, '复制回复'));
  if (data.html) actions.append(node('button', { onclick: () => download(previewDocument(data.html), `model-quality-${it.id}.html`, 'text/html') }, '下载 HTML'));
  body.append(actions, node('pre', { class: 'raw-reply' }, item.reply || '(没有回复)'), node('h3', {}, '参考与来源'), node('p', { class: 'reference' }, c.reference));
  if (item.grade?.checks?.length) body.append(checkTable(item.grade.checks));
  const prompt = node('details', {}, node('summary', {}, '查看本次实际发送的提示词'), node('pre', { class: 'prompt' }, c.prompt)); body.append(prompt);
  if (!$('artifact-dialog').open) $('artifact-dialog').showModal();
}
function download(data, name, type = 'application/json') {
  const url = URL.createObjectURL(new Blob([data], { type }));
  const a = node('a', { href: url, download: name }); document.body.append(a); a.click(); a.remove(); setTimeout(() => URL.revokeObjectURL(url), 1000);
}
async function exportBatch(id) {
  const b = await api('export?id=' + encodeURIComponent(id));
  download(JSON.stringify({ report_version: 1, exported_at: new Date().toISOString(), notes: '原始任务结果；不含管理密钥或账号令牌。绘图和小题集不构成模型身份或降智证明。', batch: b }, null, 2), `model-quality-${id}.json`);
}
async function retry(items = []) {
  const b = await api('retry', { method: 'POST', body: { id: state.batch.id, items } });
  state.data.active = b.id; await openBatch(b.id); changeTab('results'); updateEstimate(); notice('已用原题和原参数新建重试批次，原始记录保留。');
}
async function setBaseline(id) {
  await api('baseline', { method: 'POST', body: { id } }); state.baselineID = id; state.baseline = id ? await api('batch?id=' + encodeURIComponent(id)) : null;
  state.cards.forEach((v) => v.signature = ''); renderBatch(); if (state.tab === 'history') await loadHistory(); notice(id ? '已设置比较基线。' : '已取消比较基线。');
}
async function loadHistory() {
  const h = await api('batches'); state.history = h.batches; state.baselineID = h.baseline; state.data.active = h.active; updateEstimate(); renderHistory();
}
function renderHistory() {
  const list = $('history-list');
  list.replaceChildren(...state.history.slice(0, state.historyLimit).map((b) => {
    const actions = node('div', { class: 'actions' }, node('button', { onclick: () => task(openBatch(b.id).then(() => changeTab('results'))) }, '查看'), node('button', { onclick: () => task(exportBatch(b.id)) }, '导出'));
    if (b.status === 'completed') actions.append(node('button', { onclick: () => task(setBaseline(b.id)), disabled: b.baseline }, b.baseline ? '当前基线' : '设为基线'));
    if (!['running', 'cancelling'].includes(b.status)) actions.append(node('button', { class: 'danger', disabled: b.baseline, onclick: () => { if (confirm('删除此批测试及其回复？此操作无法撤回。')) task(api('batch', { method: 'DELETE', body: { id: b.id } }).then(loadHistory)); } }, '删除'));
    return node('article', { class: 'history-row' }, node('div', {}, node('strong', {}, readableDate(b.created_at)), ' ', badge(labels[b.status] || b.status, b.status === 'completed' ? 'good' : ''), b.baseline ? badge('比较基线', 'blue') : null, node('p', {}, `${b.total} 次请求 · 成功 ${b.completed} · 失败 ${b.failed} · 推理 ${b.effort}` + (b.retry_of ? ' · 重试批次' : ''))), actions);
  }));
  if (!state.history.length) list.append(node('p', { class: 'empty-note' }, '暂无测试记录'));
  $('more-history').hidden = state.historyLimit >= state.history.length;
}

document.querySelectorAll('[data-tab]').forEach((n) => n.addEventListener('click', () => changeTab(n.dataset.tab)));
$('refresh').onclick = () => task(bootstrap());
$('login-form').onsubmit = (e) => { e.preventDefault(); state.key = $('key').value.trim(); $('key').value = ''; task(bootstrap()); };
$('inspect-case').onchange = showReference;
$('model-search').oninput = renderModels;
$('account-search').oninput = renderAccounts;
$('provider-filter').onchange = renderAccounts;
$('select-cases').onclick = () => { const ids = state.data.cases.filter(isObjective).map((c) => c.id); const all = ids.every((id) => state.selectedCases.has(id)); ids.forEach((id) => all ? state.selectedCases.delete(id) : state.selectedCases.add(id)); renderCases(); updateEstimate(); };
$('select-accounts').onclick = () => { const accounts = filteredAccounts().filter((a) => !a.disabled && a.models.length); const all = accounts.every((a) => state.selectedAccounts.has(a.id)); accounts.forEach((a) => all ? state.selectedAccounts.delete(a.id) : state.selectedAccounts.add(a.id)); renderAccounts(); updateEstimate(); };
$('edit-custom').onclick = () => { const c = state.data.cases.find((c) => c.id === $('inspect-case').value); $('custom-prompt').value = c.prompt; $('use-custom').checked = true; $('custom-box').open = true; $('custom-prompt').focus(); updateEstimate(); };
for (const id of ['effort', 'concurrency', 'repeats', 'use-custom', 'custom-prompt']) $(id).addEventListener('input', updateEstimate);
$('start').onclick = () => task(start());
$('go-setup').onclick = () => changeTab('setup');
$('cancel').onclick = () => task(api('cancel', { method: 'POST', body: { id: state.batch.id } }).then(poll));
$('retry-failed').onclick = () => task(retry());
$('export').onclick = () => task(exportBatch(state.batch.id));
$('baseline').onclick = () => task(setBaseline(state.batch.id));
$('clear-baseline').onclick = () => task(setBaseline(''));
$('cleanup').onclick = () => task(api('cleanup', { method: 'POST', body: {} }).then((v) => { notice(`已清理 ${v.deleted} 批过期记录。`); return loadHistory(); }));
for (const id of ['result-case', 'result-status', 'only-differences']) $(id).onchange = renderResultCards;
$('compare-layout').onclick = () => { const wide = $('result-cards').classList.toggle('wide'); $('compare-layout').textContent = wide ? '切换紧凑卡片' : '切换宽卡片'; };
$('more-history').onclick = () => { state.historyLimit += 20; renderHistory(); };
$('close-artifact').onclick = () => $('artifact-dialog').close();
$('artifact-dialog').addEventListener('click', (e) => { if (e.target === $('artifact-dialog')) { const r = e.target.getBoundingClientRect(); if (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom) e.target.close(); } });
setInterval(() => task(poll()), 1000);
task(bootstrap());
