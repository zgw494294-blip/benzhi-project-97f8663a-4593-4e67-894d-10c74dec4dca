const state = {assays: [], view: null, readiness: null, tab: 'overview'};
const labels = {draft: '草稿', observing: '观察中', correction: '整改中', review: '待复核', archived: '已归档'};
const countFields = ['normal_count', 'abnormal_count', 'hard_seed_count', 'rotten_count', 'ungerminated_count'];
const $ = selector => document.querySelector(selector);
const escapeHTML = value => String(value ?? '').replace(/[&<>'"]/g, char => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;'}[char]));

async function request(path, options = {}) {
  const response = await fetch(path, {headers: {'Content-Type': 'application/json', ...(options.headers || {})}, ...options});
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = new Error(body.message || `请求失败 (${response.status})`);
    error.field = body.field;
    error.errors = body.errors || [];
    error.currentRevision = body.current_revision;
    throw error;
  }
  return body;
}

function notify(message, failed = false) {
  const node = $('#notice');
  node.textContent = message;
  node.className = `notice${failed ? ' error' : ''}`;
  setTimeout(() => node.classList.add('hidden'), 5000);
}

async function loadList() {
  state.assays = await request('/api/assays');
  $('#assay-list').innerHTML = state.assays.length ? state.assays.map(item =>
    `<button class="assay-item ${state.view?.assay.id === item.id ? 'active' : ''}" data-id="${item.id}"><strong>${escapeHTML(item.sample_accession)}</strong><small>${escapeHTML(item.laboratory_batch_no)} · ${labels[item.state] || item.state} · r${item.revision}</small></button>`
  ).join('') : '<p class="empty-row">尚无检验批次</p>';
  document.querySelectorAll('.assay-item').forEach(node => node.onclick = () => openAssay(node.dataset.id));
}

async function openAssay(id) {
  try {
    state.view = await request(`/api/assays/${id}`);
    state.readiness = state.view.assay.state === 'draft' ? await request(`/api/assays/${id}/readiness`) : null;
    $('#welcome').classList.add('hidden');
    $('#detail').classList.remove('hidden');
    renderAll();
    await loadList();
  } catch (error) { notify(error.message, true); }
}

function renderAll() {
  const a = state.view.assay;
  $('#state-chip').textContent = labels[a.state] || a.state;
  $('#assay-title').textContent = a.sample_accession;
  $('#assay-subtitle').textContent = `${a.laboratory_batch_no} · 检验员 ${a.operator_name} · 复核员 ${a.reviewer_name}`;
  $('#revision').textContent = a.revision;
  renderOverview(); renderObservation(); renderDeviations(); renderReview(); renderTimeline(); selectTab(state.tab);
}

function metricHTML() {
  const m = state.view.metrics;
  return `<div class="metrics"><div class="metric-card"><span>累计发芽率</span><strong>${(m.cumulative_rate * 100).toFixed(2)}%</strong></div><div class="metric-card"><span>发芽势</span><strong>${(m.germination_vigor * 100).toFixed(2)}%</strong></div><div class="metric-card ${m.max_dispersion > state.view.assay.protocol.dispersion_limit ? 'warning' : ''}"><span>最大组间离散度</span><strong>${m.max_dispersion.toFixed(4)}</strong></div><div class="metric-card"><span>观察完整性</span><strong>${m.complete_observation ? '完整' : '待录入'}</strong></div></div>`;
}

function renderOverview() {
  const a = state.view.assay, p = a.protocol, ready = state.readiness;
  const preview = ready ? `<div class="readiness ${ready.ready ? 'ready' : 'blocked'}"><strong>冻结预览：${ready.preview.total_seeds} 粒，${ready.preview.observation_units} 个观察单元</strong>${ready.issues.length ? `<ul>${ready.issues.map(issue => `<li data-field="${escapeHTML(issue.field)}">${escapeHTML(issue.field)}：${escapeHTML(issue.message)}</li>`).join('')}</ul>` : '<p>领域边界与样本唯一性检查全部通过。</p>'}</div>` : '';
  const actions = a.state === 'draft' ? `<div class="actions"><button id="edit-draft">修订草稿</button><button class="primary" id="freeze" ${ready?.ready ? '' : 'disabled'}>冻结方案并进入观察</button></div>` : '<span class="chip">关键参数已锁定</span>';
  $('#tab-overview').innerHTML = `${metricHTML()}<div class="panel"><div class="panel-head"><h3>方案基线</h3>${actions}</div>${preview}<div class="grid"><div class="kv"><span>培养温度</span>${p.temperature_celsius} °C</div><div class="kv"><span>培养基质</span>${escapeHTML(p.substrate)}</div><div class="kv"><span>光照周期</span>${p.light_cycle_hours} 小时/日</div><div class="kv"><span>观察窗口</span>${p.observation_days} 日</div><div class="kv"><span>重复组</span>${p.replicate_count} 组 × ${p.seeds_per_replicate} 粒</div><div class="kv"><span>离散度阈值</span>${p.dispersion_limit}</div><div class="kv"><span>规则版本</span>${escapeHTML(a.protocol_version)}</div><div class="kv"><span>冻结时间</span>${p.frozen_at ? new Date(p.frozen_at).toLocaleString() : '尚未冻结'}</div></div><p><strong>正常幼苗规则：</strong>${escapeHTML(p.normal_seedling_rule)}</p></div>`;
  $('#edit-draft')?.addEventListener('click', showDraftDialog);
  $('#freeze')?.addEventListener('click', () => mutate(`/api/assays/${a.id}/freeze`, {expected_revision: a.revision, principal: {name: a.operator_name, role: 'operator'}}, '方案已冻结，批次进入观察阶段'));
}

function readingRow(day, rep, current, p, stateName) {
  const item = current.get(`${day}:${rep}`) || {};
  const inputs = countFields.map(name => `<td><input name="${name}" type="number" min="0" value="${item[name] ?? (name === 'ungerminated_count' ? p.seeds_per_replicate : 0)}"></td>`).join('');
  const correction = stateName === 'correction' ? `<button class="save-reading">定向更正</button>` : '';
  return `<tr data-day="${day}" data-rep="${rep}"><td>第 ${day} 日</td><td>R${rep}</td>${inputs}<td>${item.recorded_at ? new Date(item.recorded_at).toLocaleString() : '—'}</td><td>${correction}</td></tr>`;
}

function renderObservation() {
  const a = state.view.assay, p = a.protocol;
  const current = new Map(state.view.current_readings.map(item => [`${item.day_no}:${item.replicate_no}`, item]));
  let rows = '';
  for (let day = 1; day <= p.observation_days; day++) {
    for (let rep = 1; rep <= p.replicate_count; rep++) rows += readingRow(day, rep, current, p, a.state);
    rows += `<tr class="day-action"><td colspan="9"><button class="save-day primary" data-day="${day}" ${a.state !== 'observing' ? 'disabled' : ''}>保存第 ${day} 日全部重复组</button></td></tr>`;
  }
  $('#tab-observation').innerHTML = `${metricHTML()}<div class="panel"><div class="panel-head"><div><h3>整日批量观察</h3><p>每次提交覆盖该日全部重复组；任一组错误时整日不保存。</p></div><button id="seal" class="primary" ${a.state !== 'observing' ? 'disabled' : ''}>结束观察并生成复核材料</button></div><div class="table-scroll"><table><thead><tr><th>观察日</th><th>重复组</th><th>正常</th><th>异常幼苗</th><th>硬实粒</th><th>腐烂粒</th><th>未发芽</th><th>记录时间</th><th>操作</th></tr></thead><tbody>${rows}</tbody></table></div></div>`;
  document.querySelectorAll('.save-day').forEach(button => button.onclick = () => saveDay(+button.dataset.day));
  document.querySelectorAll('.save-reading').forEach(button => button.onclick = () => saveReading(button.closest('tr')));
  $('#seal').onclick = () => mutate(`/api/assays/${a.id}/seal`, {expected_revision: a.revision, principal: {name: a.operator_name, role: 'operator'}}, '封存条件已复验');
}

async function saveDay(day) {
  const a = state.view.assay;
  const observations = [...document.querySelectorAll(`tr[data-day="${day}"]`)].map(row => {
    const reading = {replicate_no: +row.dataset.rep};
    row.querySelectorAll('input').forEach(input => reading[input.name] = +input.value);
    return reading;
  });
  await mutate(`/api/assays/${a.id}/observations/day`, {expected_revision: a.revision, day_no: day, recorded_by: a.operator_name, observations}, `第 ${day} 日全部重复组已原子保存`);
}

async function saveReading(row) {
  const a = state.view.assay;
  const body = {expected_revision: a.revision, day_no: +row.dataset.day, replicate_no: +row.dataset.rep, recorded_by: a.operator_name};
  row.querySelectorAll('input').forEach(input => body[input.name] = +input.value);
  await mutate(`/api/assays/${a.id}/observations`, body, '整改范围内读数已生成新版本');
}

function targetText(deviation) {
  return `第 ${deviation.target_days.join('、')} 日；重复组 ${deviation.target_replicates.map(rep => `R${rep}`).join('、')}`;
}

function renderDeviations() {
  const a = state.view.assay;
  const rows = a.deviations.length ? a.deviations.map(item => `<tr><td>${escapeHTML(item.rule_code)} #${item.occurrence}</td><td>${escapeHTML(targetText(item))}</td><td class="status-${item.status}">${item.status === 'open' ? '待关闭' : '已关闭'}</td><td>${escapeHTML(item.trigger_metric)}</td><td>${escapeHTML(item.current_verification)}</td><td>${item.status === 'open' ? `<button class="resolve" data-id="${item.id}">提交整改证据</button>` : `${new Date(item.closed_at).toLocaleString()}<br><small>${item.retest_observation_ids.length} 个证据版本</small>`}</td></tr>`).join('') : '<tr><td colspan="6" class="empty-row">当前无异常项；封存时还会执行完整性复验。</td></tr>';
  $('#tab-deviation').innerHTML = `<div class="panel"><div class="panel-head"><h3>异常发生与定向补测证据链</h3><span>${a.deviations.filter(item => item.status === 'open').length} 项待关闭</span></div><table><thead><tr><th>规则/发生</th><th>目标范围</th><th>状态</th><th>首次触发</th><th>当前复验</th><th>闭环</th></tr></thead><tbody>${rows}</tbody></table></div>`;
  document.querySelectorAll('.resolve').forEach(button => button.onclick = () => showDeviationDialog(button.dataset.id));
}

function checklistFromPage() {
  return [...document.querySelectorAll('.checklist-item')].map(row => ({code: row.dataset.code, label: row.dataset.label, status: row.querySelector('select').value, opinion: row.querySelector('input').value.trim()}));
}

function differenceHTML(difference = {}) {
  const observations = difference.new_observation_ids || [], deviations = difference.deviation_changes || [], metrics = difference.metric_changes || [];
  if (!observations.length && !deviations.length && !metrics.length) return '无差异摘要';
  return `${observations.length} 个新增观察版本；${deviations.map(escapeHTML).join('、') || '异常状态无变化'}；${metrics.map(item => `${escapeHTML(item.name)} ${item.before.toFixed(4)}→${item.after.toFixed(4)}`).join('、') || '指标无变化'}`;
}

function renderReview() {
  const a = state.view.assay, material = state.view.review_material;
  const checklist = (a.review_checklist || []).map(item => `<div class="checklist-item" data-code="${item.code}" data-label="${escapeHTML(item.label)}"><strong>${escapeHTML(item.label)}</strong><select ${a.state !== 'review' ? 'disabled' : ''}><option value="pending" ${item.status === 'pending' ? 'selected' : ''}>待审阅</option><option value="passed" ${item.status === 'passed' ? 'selected' : ''}>通过</option><option value="returned" ${item.status === 'returned' ? 'selected' : ''}>退回</option></select><input placeholder="逐项意见" value="${escapeHTML(item.opinion || '')}" ${a.state !== 'review' ? 'disabled' : ''}></div>`).join('');
  const history = material.review_history.length ? material.review_history.map(record => `<tr><td>v${record.version}</td><td>${escapeHTML(record.decision)}</td><td>r${record.material_revision}</td><td>${escapeHTML(record.opinion)}</td><td>${record.correction_scope?.length || 0} 项</td><td>${escapeHTML(differenceHTML(record.difference))}</td></tr>`).join('') : '<tr><td colspan="6" class="empty-row">尚无复核历史</td></tr>';
  let actions = '';
  if (a.state === 'review') actions = '<button id="return-review" class="danger">按范围退回</button><button id="approve" class="primary">批准并归档</button>';
  if (a.state === 'correction') actions = '<button id="resubmit" class="primary">生成差异并重提</button>';
  const report = a.report ? `<div class="panel"><h3>不可变归档报告</h3><p class="consistency ${state.view.report_consistent ? '' : 'bad'}">摘要一致性校验：${state.view.report_consistent ? '通过' : '不一致'}</p><p><strong>SHA-256：</strong>${escapeHTML(a.report.evidence_digest)}</p></div>` : '';
  $('#tab-review').innerHTML = `<div class="panel"><div class="panel-head"><h3>技术复核清单</h3><div class="actions">${actions}</div></div><p>材料 revision：r${a.review_material_revision || '—'}</p><div class="checklist">${checklist || '<p>进入待复核后生成清单。</p>'}</div><div class="grid">${material.derivations.map(item => `<div class="kv"><span>${item.label}</span><strong>${item.result}</strong><div class="formula">${item.formula}</div></div>`).join('')}</div><h3>历轮清单、范围与差异</h3><table><thead><tr><th>版本</th><th>决定</th><th>材料</th><th>意见</th><th>整改范围</th><th>重提差异</th></tr></thead><tbody>${history}</tbody></table></div>${report}`;
  $('#return-review')?.addEventListener('click', () => showReviewDialog('return'));
  $('#approve')?.addEventListener('click', () => showReviewDialog('approve'));
  $('#resubmit')?.addEventListener('click', () => showReviewDialog('resubmit'));
}

function renderTimeline() {
  $('#tab-timeline').innerHTML = `<div class="panel"><h3>状态与操作轨迹</h3><div class="timeline">${state.view.assay.audit_trail.map(event => `<div class="event"><strong>r${event.revision} · ${escapeHTML(event.action)}</strong><div>${escapeHTML(event.actor)}</div><small>${new Date(event.created_at).toLocaleString()}</small></div>`).join('')}</div></div>`;
}

function selectTab(name) {
  state.tab = name;
  document.querySelectorAll('.tabs button').forEach(button => button.classList.toggle('active', button.dataset.tab === name));
  document.querySelectorAll('.tab-panel').forEach(panel => panel.classList.add('hidden'));
  $(`#tab-${name}`).classList.remove('hidden');
}

async function mutate(path, body, message, form) {
  try {
    state.view = await request(path, {method: 'POST', body: JSON.stringify(body)});
    state.readiness = state.view.assay.state === 'draft' ? await request(`/api/assays/${state.view.assay.id}/readiness`) : null;
    renderAll(); await loadList(); notify(message); return true;
  } catch (error) {
    notify(`${error.message}${error.currentRevision ? `（当前 revision：${error.currentRevision}）` : ''}`, true);
    if (form) markFieldErrors(form, error.errors.length ? error.errors : error.field ? [{field: error.field, message: error.message}] : []);
    return false;
  }
}

function markFieldErrors(form, errors) {
  form.querySelectorAll('.field-error').forEach(node => node.remove());
  errors.forEach(issue => {
    const name = issue.field.replace(/^protocol\./, '').split('.').pop();
    const input = form.elements[name];
    if (input) input.insertAdjacentHTML('afterend', `<small class="field-error">${escapeHTML(issue.message)}</small>`);
  });
}

function protocolFromForm(form) {
  const values = Object.fromEntries(new FormData(form));
  const protocol = {substrate: values.substrate, normal_seedling_rule: values.normal_seedling_rule};
  ['temperature_celsius', 'light_cycle_hours', 'observation_days', 'replicate_count', 'seeds_per_replicate', 'dispersion_limit'].forEach(key => protocol[key] = +values[key]);
  return {values, protocol};
}

function fillDraftForm(form, assay) {
  ['sample_accession', 'laboratory_batch_no', 'operator_name', 'reviewer_name'].forEach(name => form.elements[name].value = assay[name]);
  Object.keys(assay.protocol).forEach(name => { if (form.elements[name] && name !== 'frozen_at') form.elements[name].value = assay.protocol[name]; });
}

function showDraftDialog() {
  const form = $('#create-form');
  form.dataset.mode = 'draft';
  fillDraftForm(form, state.view.assay);
  $('#create-dialog h2').textContent = '修订检验批次草稿';
  $('#create-dialog button[type="submit"]').textContent = '保存修订';
  $('#create-dialog').showModal();
}

function showDeviationDialog(id) {
  const deviation = state.view.assay.deviations.find(item => item.id === id);
  const candidates = state.view.assay.observations.filter(item => new Date(item.recorded_at) > new Date(deviation.opened_at) && deviation.target_days.includes(item.day_no) && deviation.target_replicates.includes(item.replicate_no));
  $('#action-title').textContent = `整改 ${deviation.rule_code} #${deviation.occurrence}`;
  $('#action-fields').innerHTML = `<div class="action-stack"><p>目标：${escapeHTML(targetText(deviation))}</p><label>异常原因<textarea name="reason" required></textarea></label><label>定向补测动作<textarea name="corrective_action" required></textarea></label><fieldset><legend>选择目标范围内、打开后产生的证据版本</legend>${candidates.length ? candidates.map(item => `<label><input type="checkbox" name="evidence_ids" value="${item.id}">第 ${item.day_no} 日 R${item.replicate_no} · ${item.supersedes_id ? '新版本' : '首次补录'} · ${new Date(item.recorded_at).toLocaleString()}</label>`).join('') : '<p>暂无可用证据，请先补测。</p>'}</fieldset></div>`;
  const dialog = $('#action-dialog'); dialog.dataset.mode = 'deviation'; dialog.dataset.id = id; dialog.showModal();
}

function scopeChoices() {
  const readings = state.view.current_readings.map(item => `<label><input type="checkbox" name="scope" value="observation:${item.day_no}:${item.replicate_no}">第 ${item.day_no} 日 R${item.replicate_no}</label>`).join('');
  const deviations = state.view.assay.deviations.map(item => `<label><input type="checkbox" name="scope" value="deviation:${item.id}">${escapeHTML(item.rule_code)} #${item.occurrence}</label>`).join('');
  const sections = ['protocol:方案基线', 'readings:原始读数', 'metrics:指标推导', 'deviations:异常闭环', 'audit:操作轨迹'].map(value => { const [code, label] = value.split(':'); return `<label><input type="checkbox" name="scope" value="section:${code}">${label}</label>`; }).join('');
  return `<fieldset><legend>结构化整改范围</legend><div class="scope-grid">${readings}${deviations}${sections}</div></fieldset>`;
}

function showReviewDialog(mode) {
  const titles = {return: '退回复核材料', approve: '批准并归档', resubmit: '整改后重提'};
  $('#action-title').textContent = titles[mode];
  let fields = '<label>意见<textarea name="opinion" required></textarea></label>';
  if (mode === 'return') fields += scopeChoices();
  $('#action-fields').innerHTML = `<div class="action-stack">${fields}</div>`;
  const dialog = $('#action-dialog'); dialog.dataset.mode = mode; dialog.dataset.id = ''; dialog.showModal();
}

function parseScope(values) {
  return values.map(value => {
    const [type, first, second] = value.split(':');
    if (type === 'observation') return {type, day_no: +first, replicate_no: +second};
    if (type === 'deviation') return {type, deviation_id: first};
    return {type, section: first};
  });
}

$('#action-form').onsubmit = async event => {
  event.preventDefault();
  const dialog = $('#action-dialog'), a = state.view.assay, formData = new FormData(event.currentTarget);
  const mode = dialog.dataset.mode;
  let path, body, message;
  if (mode === 'deviation') {
    path = `/api/assays/${a.id}/deviations/${dialog.dataset.id}/resolve`;
    body = {expected_revision: a.revision, actor: a.operator_name, reason: formData.get('reason'), corrective_action: formData.get('corrective_action'), evidence_ids: formData.getAll('evidence_ids')};
    message = '异常证据有效且规则复验通过，发生项已关闭';
  } else if (mode === 'return') {
    path = `/api/assays/${a.id}/review/return`;
    body = {expected_revision: a.revision, reviewer: a.reviewer_name, opinion: formData.get('opinion'), checklist: checklistFromPage(), correction_scope: parseScope(formData.getAll('scope'))};
    message = '复核材料已按结构化范围退回';
  } else if (mode === 'resubmit') {
    path = `/api/assays/${a.id}/review/resubmit`;
    body = {expected_revision: a.revision, operator: a.operator_name, opinion: formData.get('opinion')};
    message = '实际整改差异已固化并重提';
  } else {
    path = `/api/assays/${a.id}/review/approve`;
    body = {expected_revision: a.revision, reviewer: a.reviewer_name, opinion: formData.get('opinion'), checklist: checklistFromPage()};
    message = '清单门禁通过，报告已归档';
  }
  if (await mutate(path, body, message, event.currentTarget)) dialog.close();
};

$('#create-form').onsubmit = async event => {
  event.preventDefault(); const {values, protocol} = protocolFromForm(event.currentTarget);
  if (event.currentTarget.dataset.mode === 'draft') {
    const a = state.view.assay;
    const saved = await mutate(`/api/assays/${a.id}/draft`, {expected_revision: a.revision, actor: a.operator_name, sample_accession: values.sample_accession, laboratory_batch_no: values.laboratory_batch_no, operator_name: values.operator_name, reviewer_name: values.reviewer_name, protocol}, '草稿已修订，revision 已递增', event.currentTarget);
    if (saved) $('#create-dialog').close();
    return;
  }
  try {
    const created = await request('/api/assays', {method: 'POST', body: JSON.stringify({sample_accession: values.sample_accession, laboratory_batch_no: values.laboratory_batch_no, operator_name: values.operator_name, reviewer_name: values.reviewer_name, protocol})});
    $('#create-dialog').close(); event.currentTarget.reset(); await openAssay(created.assay.id); notify('检验批次草稿已创建');
  } catch (error) { notify(error.message, true); markFieldErrors(event.currentTarget, error.errors || []); }
};

document.querySelectorAll('[data-close]').forEach(button => button.onclick = () => button.closest('dialog').close());
document.querySelectorAll('.tabs button').forEach(button => button.onclick = () => selectTab(button.dataset.tab));
$('#new-assay').onclick = () => {
  const form = $('#create-form');
  form.dataset.mode = 'create'; form.reset();
  $('#create-dialog h2').textContent = '新建检验批次';
  $('#create-dialog button[type="submit"]').textContent = '保存草稿';
  $('#create-dialog').showModal();
};
$('#refresh-list').onclick = loadList;
loadList().catch(error => notify(error.message, true));
