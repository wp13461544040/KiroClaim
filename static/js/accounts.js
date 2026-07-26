// 账号管理模块

// 筛选状态（账号池仅显示未分配）
let accountStatusFilter = '';
let accountSubscriptionFilter = '';
let accountKeyword = '';

// 批量选择
let selectedAccountIds = new Set();

async function loadAccounts(page = 1) {
  accountKeyword = (document.getElementById('accountKeyword')?.value || '').trim();
  const createdFrom = document.getElementById('accountCreatedFrom')?.value || '';
  const createdTo = document.getElementById('accountCreatedTo')?.value || '';
  // 账号池强制只看未分配
  let url = `/admin/accounts?page=${page}&size=15&used=false`;
  if (accountStatusFilter) url += `&status=${accountStatusFilter}`;
  if (accountSubscriptionFilter) url += `&subscription=${encodeURIComponent(accountSubscriptionFilter)}`;
  if (accountKeyword) url += `&keyword=${encodeURIComponent(accountKeyword)}`;
  if (createdFrom) url += `&created_from=${createdFrom}`;
  if (createdTo) url += `&created_to=${createdTo}`;

  const r = await api('GET', url);
  const tbody = document.getElementById('accountsBody');
  if (r.code !== 0 || !r.data.list.length) {
    tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;color:#999;padding:40px">暂无未分配账号</td></tr>';
    updateAccountBatchBtn();
    return;
  }
  tbody.innerHTML = r.data.list.map(a => {
    const creditLimit = Math.max(0, Number(a.CreditLimit) || 0);
    const creditUsed = Math.max(0, Number(a.CreditUsed) || 0);
    const creditPct = creditLimit > 0 ? Math.min(100, Math.round(creditUsed / creditLimit * 100)) : 0;
    const creditCls = creditPct >= 90 ? 'danger' : creditPct >= 70 ? 'warn' : '';
    const creditText = creditLimit > 0
      ? `${creditUsed.toFixed(1)} / ${creditLimit.toFixed(0)}`
      : '-';
    const checked = selectedAccountIds.has(a.ID) ? 'checked' : '';
    return `<tr>
      <td data-label="选择"><input type="checkbox" class="k-checkbox" ${checked} onchange="toggleAccountSelect(${a.ID}, this.checked)"></td>
      <td data-label="ID" style="color:#999">${a.ID}</td>
      <td data-label="邮箱" class="account-email-cell">${a.Email || '-'}</td>
      <td data-label="健康状态">${healthBadge(a.Status)}</td>
      <td data-label="订阅" class="account-subscription-cell">${subscriptionBadge(a.Subscription)}</td>
      <td data-label="额度用量" class="account-usage-cell">
        <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;font-size:12px;color:var(--text-muted);max-width:160px">
          <span>${creditText}</span>
          <span style="font-variant-numeric:tabular-nums;color:${creditPct>=90?'#dc2626':creditPct>=70?'#f59e0b':'var(--text-muted)'}">${creditLimit>0?creditPct+'%':''}</span>
        </div>
        <div class="k-progress-bg"><div class="k-progress-fill ${creditCls}" style="width:${creditPct}%"></div></div>
      </td>
      <td data-label="最后检查" style="color:#999;font-size:12px">${a.LastCheckedAt ? new Date(a.LastCheckedAt).toLocaleString('zh-CN', {hour12:false, month:'2-digit', day:'2-digit', hour:'2-digit', minute:'2-digit'}) : '-'}</td>
      <td data-label="操作" class="account-action-cell">
        <div class="account-actions">
          <button class="ui-btn ui-btn-secondary ui-btn-sm" onclick="showAccountDetail(${a.ID})">详细</button>
          <button class="ui-btn ui-btn-secondary ui-btn-sm" onclick="refreshAccount(${a.ID}, 'accounts', this)">刷新</button>
          <button class="ui-btn ui-btn-danger ui-btn-sm" onclick="deleteAccount(${a.ID})">删除</button>
        </div>
      </td>
    </tr>`;
  }).join('');
  renderPagination('accountsPagination', r.data.total, 15, page, loadAccounts);
  updateAccountBatchBtn();
  // 更新全选框状态
  const selectAll = document.getElementById('selectAllAccounts');
  if (selectAll) {
    const checkboxes = tbody.querySelectorAll('input[type="checkbox"]');
    selectAll.checked = checkboxes.length > 0 && [...checkboxes].every(cb => cb.checked);
  }
}

// 删除账号（source 可选：'assigned' 表示从已分配面板触发，之后重载对应表）
async function deleteAccount(id, source) {
  if (!confirm('确认删除该账号？')) return;

  const btn = event.target;
  const originalText = btn.textContent;
  btn.disabled = true;
  btn.textContent = '删除中...';

  const r = await api('DELETE', '/admin/accounts/' + id);
  if (r.code === 0) {
    btn.textContent = '已删除';
    showToast('账号删除成功', 'success');
    setTimeout(() => {
      if (source === 'assigned') loadAssignedAccounts(1);
      else loadAccounts(1);
      loadAccountSubscriptionFilter();
      loadStats && loadStats();
    }, 500);
  } else {
    btn.textContent = '失败';
    showToast('删除失败：' + (r.msg || '未知错误'), 'error');
    setTimeout(() => {
      btn.textContent = originalText;
      btn.disabled = false;
    }, 2000);
  }
}

async function refreshAccount(id, source, btn) {
  if (!btn) return;
  const originalText = btn.textContent;
  btn.disabled = true;
  btn.textContent = '刷新中...';
  try {
    const r = await api('POST', `/admin/accounts/${id}/refresh`);
    const msg = r.message || r.msg || '刷新完成';
    if (r.code === 0) {
      const status = r.data?.status;
      if (status === 'suspended') {
        showToast('账号已被判定封禁', 'error');
      } else {
        showToast(msg, 'success');
      }
    } else {
      showToast(msg, 'error');
    }
    if (source === 'assigned') {
      loadAssignedAccounts(1);
    } else {
      loadAccounts(1);
    }
    loadAccountSubscriptionFilter();
    loadStats && loadStats();
  } catch (e) {
    showToast('刷新失败：' + e.message, 'error');
  } finally {
    btn.disabled = false;
    btn.textContent = originalText;
  }
}

function escapeHtml(value) {
  return String(value == null ? '' : value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function renderImportBadDetails(d) {
  const details = Array.isArray(d.badDetails) ? d.badDetails : [];
  if (!details.length) return '';

  const rows = details.map(item => {
    const row = Number(item.row) || '-';
    const reason = escapeHtml(item.reason || '上游未返回具体错误');
    return `<div style="display:grid;grid-template-columns:74px minmax(0,1fr);gap:10px;padding:8px 0;border-top:1px solid #fee2e2">
      <span style="color:#991b1b;font-weight:600;white-space:nowrap">第 ${row} 行</span>
      <code style="white-space:pre-wrap;word-break:break-word;color:#7f1d1d;font-family:monospace;font-size:12px;line-height:1.6">${reason}</code>
    </div>`;
  }).join('');

  const more = Number(d.badDetailMore) || 0;
  const moreText = more > 0
    ? `<div style="padding-top:8px;color:#991b1b">还有 ${more} 条异常未展示，请分批导入查看完整上游响应。</div>`
    : '';

  return `<div style="margin-top:12px;padding:12px 14px;background:#fff7f7;border:1px solid #fecaca;border-radius:6px;font-size:13px;line-height:1.6">
    <div style="font-weight:700;color:#991b1b;margin-bottom:4px">上游报错</div>
    ${rows}
    ${moreText}
  </div>`;
}

function openAccountInfoModal(title) {
  const old = document.getElementById('accountInfoModal');
  if (old) old.remove();

  const overlay = document.createElement('div');
  overlay.id = 'accountInfoModal';
  overlay.className = 'modal-overlay active account-info-modal';
  overlay.innerHTML = `
    <div class="modal-content account-info-content">
      <div class="modal-header">
        <span class="modal-title">${escapeHtml(title)}</span>
        <button type="button" class="modal-close" aria-label="关闭" title="关闭" onclick="closeAccountInfoModal()">&times;</button>
      </div>
      <div class="modal-body" id="accountInfoBody">
        <div class="account-modal-loading">
          <span class="modal-spinner" aria-hidden="true"></span>
          <span>正在加载...</span>
        </div>
      </div>
    </div>`;
  document.body.appendChild(overlay);
  return document.getElementById('accountInfoBody');
}

function closeAccountInfoModal() {
  const modal = document.getElementById('accountInfoModal');
  if (modal) modal.remove();
}

async function showAccountDetail(id) {
  const body = openAccountInfoModal('账号详细 #' + id);
  try {
    const r = await api('GET', `/admin/accounts/${id}/detail`);
    if (r.code !== 0) {
      body.innerHTML = renderAccountModalError(r.message || r.msg || '账号详情加载失败');
      return;
    }
    body.innerHTML = renderAccountDetail(r.data || {}) + renderAccountModelsSection(`
      <div class="account-model-loading">
        <span class="modal-spinner" aria-hidden="true"></span>
        <span>正在加载可用模型...</span>
      </div>`);
    await loadAccountModelsIntoDetail(id);
  } catch (e) {
    body.innerHTML = renderAccountModalError('账号详情加载失败：' + e.message);
  }
}

async function loadAccountModelsIntoDetail(id) {
  const panel = document.getElementById('accountModelsPanel');
  if (!panel) return;
  try {
    const r = await api('GET', `/admin/accounts/${id}/models`);
    if (r.code !== 0) {
      panel.innerHTML = renderAccountModalError(r.message || r.msg || '模型列表加载失败');
      return;
    }
    panel.innerHTML = renderAccountModels(r.data || {});
  } catch (e) {
    panel.innerHTML = renderAccountModalError('模型列表加载失败：' + e.message);
  }
}

function renderAccountModalError(message) {
  return `<div class="account-modal-error">${escapeHtml(message)}</div>`;
}

function renderAccountDetail(data) {
  const sub = data.subscription || {};
  const rows = [
    ['ID', data.id || '-'],
    ['邮箱', data.email || '-'],
    ['用户 ID', data.userId || '-'],
    ['Subscription', sub.title || '-'],
    ['订阅类型', sub.type || '-'],
    ['重置时间', formatUpstreamTime(data.nextDateReset)]
  ];

  return `
    <div class="account-detail-grid">
      ${rows.map(([label, value]) => `
        <div class="account-detail-row">
          <span>${escapeHtml(label)}</span>
          <strong>${escapeHtml(value)}</strong>
        </div>`).join('')}
    </div>`;
}

function renderAccountModelsSection(content) {
  return `
    <div class="account-detail-section">
      <div class="account-detail-section-title">可用模型</div>
      <div id="accountModelsPanel">${content}</div>
    </div>`;
}

function renderAccountModels(data) {
  const models = Array.isArray(data.models) ? data.models : [];
  if (!models.length) {
    return '<div class="account-empty-state">暂无可用模型</div>';
  }
  return `
    <div class="model-list">
      ${models.map(model => {
        const name = model.modelName || model.modelId || 'Model';
        const inputTypes = Array.isArray(model.supportedInputTypes) ? model.supportedInputTypes : [];
        const tokenLimits = model.tokenLimits || {};
        const params = [
          inputTypes.length ? `输入类型：${inputTypes.join(' / ')}` : '',
          model.rateUnit ? `倍率：${formatUsageNumber(model.rateMultiplier)} ${model.rateUnit}` : '',
          tokenLimits.maxInputTokens ? `输入上限：${formatTokenCount(tokenLimits.maxInputTokens)}` : '',
          tokenLimits.maxOutputTokens ? `输出上限：${formatTokenCount(tokenLimits.maxOutputTokens)}` : ''
        ].filter(Boolean);
        return `
          <div class="model-item">
            <img class="model-icon" src="${modelIconUrl(model)}" alt="" loading="lazy">
            <div class="model-content">
              <div class="model-title">
                <span>${escapeHtml(name)}</span>
              </div>
              <div class="model-id">${escapeHtml(model.modelId || '-')}</div>
              <div class="model-desc">${escapeHtml(model.description || '')}</div>
              <div class="model-params">
                ${params.map(item => `<div class="model-param">${escapeHtml(item)}</div>`).join('')}
              </div>
            </div>
          </div>`;
      }).join('')}
    </div>`;
}

function modelIconUrl(model) {
  const value = `${model.modelId || ''} ${model.modelName || ''}`.toLowerCase();
  const lobeCdn = 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/';
  if (value.includes('deepseek')) return `${lobeCdn}deepseek-color.svg`;
  if (value.includes('qwen')) return `${lobeCdn}qwen-color.svg`;
  if (value.includes('glm')) return `${lobeCdn}zhipu-color.svg`;
  if (value.includes('minimax')) return `${lobeCdn}minimax-color.svg`;
  return `${lobeCdn}modelscope-color.svg`;
}

function formatUsageNumber(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return '-';
  if (Math.abs(n) >= 1000) return n.toLocaleString('zh-CN', { maximumFractionDigits: 1 });
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 2 });
}

function formatTokenCount(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return '-';
  if (n >= 1000) return Math.round(n / 1000) + 'K';
  return String(n);
}

function formatUpstreamTime(value) {
  const n = Number(value);
  if (!Number.isFinite(n) || n <= 0) return '-';
  const ms = n > 10000000000 ? n : n * 1000;
  return new Date(ms).toLocaleString('zh-CN', {
    hour12: false,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  });
}

// 模态框控制
function showImportModal() {
  document.getElementById('importModal').classList.add('active');

  const taskId = localStorage.getItem('importTaskId');
  const total = localStorage.getItem('importTaskTotal');

  if (taskId && total) {
    const resultEl = document.getElementById('importResult');
    const btn = document.querySelector('#importModal button[onclick="doImport(this)"]');
    btn.disabled = true;
    btn.textContent = '导入中...';
    pollImportStatus(taskId, parseInt(total), resultEl, btn);
  }
}

function closeImportModal() {
  const taskId = localStorage.getItem('importTaskId');
  if (taskId) {
    if (!confirm('导入任务正在进行中，确定要关闭吗？')) {
      return;
    }
  }

  document.getElementById('importModal').classList.remove('active');
  document.getElementById('importResult').innerHTML = '';
}

// 导入账号
async function doImport(btn) {
  const raw = document.getElementById('importJson').value.trim();
  const resultEl = document.getElementById('importResult');

  let data;
  try {
    data = JSON.parse(raw);
  } catch (e) {
    resultEl.innerHTML = '<span style="color:red">JSON 格式错误：' + e.message + '</span>';
    showToast('JSON 格式错误', 'error');
    return;
  }

  const total = Array.isArray(data) ? data.length : 1;

  btn.disabled = true;
  btn.textContent = '提交中...';
  resultEl.innerHTML = `<div style="display:flex;align-items:center;gap:10px;padding:12px;background:#fafafa;border:1px solid #eaeaea;border-radius:6px;font-size:13px;color:#666">
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="animation:spin 1s linear infinite;flex-shrink:0">
      <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83"/>
    </svg>
    正在提交导入任务...
  </div>`;

  try {
    const r = await api('POST', '/admin/accounts/import', Array.isArray(data) ? data : [data]);
    if (r.code === 0) {
      const taskId = r.data.taskId;
      btn.textContent = '导入中...';

      localStorage.setItem('importTaskId', taskId);
      localStorage.setItem('importTaskTotal', total);

      pollImportStatus(taskId, total, resultEl, btn);
    } else {
      resultEl.innerHTML = `<div style="padding:12px;background:#fef2f2;border:1px solid #fecaca;border-radius:6px;font-size:13px;color:#991b1b">提交失败：${r.message || r.msg || '未知错误'}</div>`;
      showToast('提交失败：' + (r.message || r.msg || '未知错误'), 'error');
      btn.disabled = false;
      btn.textContent = '执行导入';
    }
  } catch (e) {
    resultEl.innerHTML = `<div style="padding:12px;background:#fef2f2;border:1px solid #fecaca;border-radius:6px;font-size:13px;color:#991b1b">提交出错：${e.message}</div>`;
    showToast('提交出错', 'error');
    btn.disabled = false;
    btn.textContent = '执行导入';
  }
}

// 轮询导入任务状态
async function pollImportStatus(taskId, total, resultEl, btn) {
  const checkStatus = async () => {
    try {
      const r = await api('GET', `/admin/accounts/import/status/${taskId}`);
      if (r.code === 0) {
        const d = r.data;

        resultEl.innerHTML = `<div style="display:flex;align-items:center;gap:10px;padding:12px;background:#fafafa;border:1px solid #eaeaea;border-radius:6px;font-size:13px;color:#666">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="animation:spin 1s linear infinite;flex-shrink:0">
            <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83"/>
          </svg>
          正在处理：<strong style="color:#171717;margin:0 4px">${d.processed}</strong> / ${d.total}
          (成功: ${d.imported}, 重复: ${d.skippedDup}, 失败: ${d.skippedBad})
        </div>`;

        if (d.status === 'completed') {
          resultEl.innerHTML = `<div style="padding:14px 16px;background:#f0fdf4;border:1px solid #bbf7d0;border-radius:6px;font-size:13px;line-height:2">
            <div style="font-weight:600;font-size:14px;margin-bottom:6px">导入完成</div>
            <div>成功写入：<strong>${d.imported}</strong> 条</div>
            <div>重复跳过：<strong>${d.skippedDup}</strong> 条</div>
            <div>检查不通过（封禁/异常）：<strong>${d.skippedBad}</strong> 条</div>
          </div>${renderImportBadDetails(d)}`;

          localStorage.removeItem('importTaskId');
          localStorage.removeItem('importTaskTotal');

          if (d.imported > 0) {
            document.getElementById('importJson').value = '';
            loadAccounts(1);
            loadAccountSubscriptionFilter();
            showToast(`成功导入 ${d.imported} 个账号`, 'success');
          } else {
            showToast('没有新账号被导入', 'info');
          }

          btn.disabled = false;
          btn.textContent = '执行导入';
        } else if (d.status === 'failed') {
          resultEl.innerHTML = `<div style="padding:12px;background:#fef2f2;border:1px solid #fecaca;border-radius:6px;font-size:13px;color:#991b1b">导入失败</div>`;
          showToast('导入失败', 'error');

          localStorage.removeItem('importTaskId');
          localStorage.removeItem('importTaskTotal');

          btn.disabled = false;
          btn.textContent = '执行导入';
        } else {
          setTimeout(checkStatus, 1000);
        }
      } else {
        resultEl.innerHTML = `<div style="padding:12px;background:#fef2f2;border:1px solid #fecaca;border-radius:6px;font-size:13px;color:#991b1b">查询状态失败</div>`;
        btn.disabled = false;
        btn.textContent = '执行导入';
      }
    } catch (e) {
      resultEl.innerHTML = `<div style="padding:12px;background:#fef2f2;border:1px solid #fecaca;border-radius:6px;font-size:13px;color:#991b1b">查询状态出错</div>`;
      btn.disabled = false;
      btn.textContent = '执行导入';
    }
  };

  checkStatus();
}


// 账号状态筛选
function selectAccountStatus(value, text) {
  accountStatusFilter = value;
  document.getElementById('accountStatusText').textContent = text;

  document.querySelectorAll('#accountStatusDropdown .k-dropdown-item').forEach(item => {
    item.classList.remove('selected');
  });
  event.target.classList.add('selected');

  toggleDropdown('accountStatusDropdown');
  loadAccounts(1);
}

// 账号订阅筛选
function selectAccountSubscription(value, text) {
  accountSubscriptionFilter = value;
  document.getElementById('accountSubscriptionText').textContent = text;

  document.querySelectorAll('#accountSubscriptionDropdown .k-dropdown-item').forEach(item => {
    item.classList.remove('selected');
  });
  event.target.classList.add('selected');

  toggleDropdown('accountSubscriptionDropdown');
  loadAccounts(1);
}

// 重置账号筛选
function resetAccountFilters() {
  accountStatusFilter = '';
  accountSubscriptionFilter = '';
  accountKeyword = '';

  const keywordInput = document.getElementById('accountKeyword');
  if (keywordInput) keywordInput.value = '';
  const createdFromInput = document.getElementById('accountCreatedFrom');
  if (createdFromInput) createdFromInput.value = '';
  const createdToInput = document.getElementById('accountCreatedTo');
  if (createdToInput) createdToInput.value = '';

  document.getElementById('accountStatusText').textContent = '全部状态';
  document.getElementById('accountSubscriptionText').textContent = '全部订阅';

  document.querySelectorAll('#accountStatusDropdown .k-dropdown-item, #accountSubscriptionDropdown .k-dropdown-item').forEach(item => {
    item.classList.remove('selected');
  });
  document.querySelector('#accountStatusDropdown .k-dropdown-item:first-child')?.classList.add('selected');
  document.querySelector('#accountSubscriptionDropdown .k-dropdown-item:first-child')?.classList.add('selected');

  loadAccounts(1);
}

// 切换单个账号选择
function toggleAccountSelect(id, checked) {
  if (checked) {
    selectedAccountIds.add(id);
  } else {
    selectedAccountIds.delete(id);
  }
  updateAccountBatchBtn();
  // 同步全选框状态
  const checkboxes = document.querySelectorAll('#accountsBody input[type="checkbox"]');
  const selectAll = document.getElementById('selectAllAccounts');
  if (selectAll) {
    selectAll.checked = checkboxes.length > 0 && [...checkboxes].every(cb => cb.checked);
  }
}

// 全选/取消全选
function toggleSelectAllAccounts(checked) {
  const checkboxes = document.querySelectorAll('#accountsBody input[type="checkbox"]');
  checkboxes.forEach(cb => {
    cb.checked = checked;
    const id = parseInt(cb.closest('tr').querySelector('td:nth-child(2)').textContent);
    if (checked) {
      selectedAccountIds.add(id);
    } else {
      selectedAccountIds.delete(id);
    }
  });
  updateAccountBatchBtn();
}

// 更新批量删除按钮显示
function updateAccountBatchBtn() {
  const btn = document.getElementById('batchDeleteAccountsBtn');
  const count = document.getElementById('selectedAccountCount');
  if (btn && count) {
    count.textContent = selectedAccountIds.size;
    btn.style.display = selectedAccountIds.size > 0 ? '' : 'none';
  }
}

// 批量删除账号
async function batchDeleteAccounts() {
  if (selectedAccountIds.size === 0) return;
  if (!confirm(`确认删除选中的 ${selectedAccountIds.size} 个账号？`)) return;

  const r = await api('POST', '/admin/accounts/batch-delete', { ids: [...selectedAccountIds] });
  if (r.code === 0) {
    showToast(`成功删除 ${r.data?.deleted || selectedAccountIds.size} 个账号`, 'success');
    selectedAccountIds.clear();
    updateAccountBatchBtn();
    const selectAll = document.getElementById('selectAllAccounts');
    if (selectAll) selectAll.checked = false;
    loadAccounts(1);
    loadAccountSubscriptionFilter();
    loadStats();
  } else {
    showToast('批量删除失败：' + (r.message || r.msg || '未知错误'), 'error');
  }
}

// 一键清理所有已封禁账号
async function deleteBannedAccounts() {
  if (!confirm('确认删除全部「已封禁」状态的账号？此操作不可恢复')) return;
  const r = await api('POST', '/admin/accounts/delete-by-status', { status: 'suspended' });
  if (r.code === 0) {
    showToast(`已清理 ${r.data?.deleted || 0} 个封禁账号`, 'success');
    loadAccounts(1);
    loadAccountSubscriptionFilter();
    loadStats();
  } else {
    showToast('清理失败：' + (r.message || r.msg || '未知错误'), 'error');
  }
}

// 一键清空号池（高危操作，双重确认）
async function doClearAllAccounts() {
  if (!confirm('此操作将删除号池中的【所有账号】，且不可恢复！\n\n确定要继续吗？')) return;
  var input = prompt('请输入「清空」两个字以确认操作：');
  if (input !== '清空') {
    showToast('操作已取消', 'info');
    return;
  }

  var r = await api('POST', '/admin/accounts/clear-all', { confirm: true });
  if (r.code === 0) {
    showToast('号池已清空，共删除 ' + (r.data?.deleted || 0) + ' 个账号', 'success');
    selectedAccountIds.clear();
    updateAccountBatchBtn();
    loadAccounts(1);
    loadAccountSubscriptionFilter();
    loadStats && loadStats();
  } else {
    showToast('清空失败：' + (r.message || r.msg || '未知错误'), 'error');
  }
}

// 按数据库中实际存在的订阅动态填充账号订阅下拉
async function loadAccountSubscriptionFilter() {
  const r = await api('GET', '/admin/accounts/subscription-stats');
  if (r.code !== 0 || !Array.isArray(r.data)) return;

  const menu = document.querySelector('#accountSubscriptionDropdown .k-dropdown-menu');
  if (!menu) return;

  const items = ['<div class="k-dropdown-item ' + (accountSubscriptionFilter ? '' : 'selected') +
    '" onclick="selectAccountSubscription(\'\', \'全部订阅\')">全部订阅</div>'];
  r.data.forEach(function(it) {
    const value = it.subscription || '';
    if (!value) return;
    const selected = accountSubscriptionFilter === value ? 'selected' : '';
    const label = value;
    items.push(
      '<div class="k-dropdown-item ' + selected + '" ' +
      'onclick=\'selectAccountSubscription(' + JSON.stringify(value) + ', ' + JSON.stringify(label) + ')\'>' +
      escapeHtml(label) + ' <span style="color:#999;font-size:12px">(' + it.unusedCount + ')</span></div>'
    );
  });
  menu.innerHTML = items.join('');
}


let assignedStatusFilter = '';
let assignedKeyword = '';

async function loadAssignedAccounts(page = 1) {
  assignedKeyword = (document.getElementById('assignedKeyword')?.value || '').trim();
  let url = `/admin/accounts?page=${page}&size=15&used=true`;
  if (assignedStatusFilter) url += `&status=${assignedStatusFilter}`;
  if (assignedKeyword) url += `&keyword=${encodeURIComponent(assignedKeyword)}`;

  const r = await api('GET', url);
  const tbody = document.getElementById('assignedBody');
  if (r.code !== 0 || !r.data.list.length) {
    tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;color:#999;padding:40px">暂无已分配账号</td></tr>';
    return;
  }
  tbody.innerHTML = r.data.list.map(a => {
    const creditLimit = Math.max(0, Number(a.CreditLimit) || 0);
    const creditUsed = Math.max(0, Number(a.CreditUsed) || 0);
    const creditPct = creditLimit > 0 ? Math.min(100, Math.round(creditUsed / creditLimit * 100)) : 0;
    const creditCls = creditPct >= 90 ? 'danger' : creditPct >= 70 ? 'warn' : '';
    const creditText = creditLimit > 0 ? `${creditUsed.toFixed(1)} / ${creditLimit.toFixed(0)}` : '-';

    const fmtTime = v => v ? new Date(v).toLocaleString('zh-CN', {hour12:false, month:'2-digit', day:'2-digit', hour:'2-digit', minute:'2-digit'}) : '-';

    return `<tr>
      <td data-label="ID" style="color:#999">${a.ID}</td>
      <td data-label="邮箱" class="account-email-cell">${a.Email || '-'}</td>
      <td data-label="健康状态">${healthBadge(a.Status)}</td>
      <td data-label="订阅" class="account-subscription-cell">${subscriptionBadge(a.Subscription)}</td>
      <td data-label="额度用量" class="account-usage-cell">
        <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;font-size:12px;color:var(--text-muted);max-width:160px">
          <span>${creditText}</span>
          <span style="font-variant-numeric:tabular-nums;color:${creditPct>=90?'#dc2626':creditPct>=70?'#f59e0b':'var(--text-muted)'}">${creditLimit>0?creditPct+'%':''}</span>
        </div>
        <div class="k-progress-bg"><div class="k-progress-fill ${creditCls}" style="width:${creditPct}%"></div></div>
      </td>
      <td data-label="分配时间" style="color:#999;font-size:12px">${fmtTime(a.UsedAt)}</td>
      <td data-label="最后检查" style="color:#999;font-size:12px">${fmtTime(a.LastCheckedAt)}</td>
      <td data-label="操作" class="account-action-cell">
        <div class="account-actions">
          <button class="ui-btn ui-btn-secondary ui-btn-sm" onclick="showAccountDetail(${a.ID})">详细</button>
          <button class="ui-btn ui-btn-secondary ui-btn-sm" onclick="refreshAccount(${a.ID}, 'assigned', this)">刷新</button>
          <button class="ui-btn ui-btn-danger ui-btn-sm" onclick="deleteAccount(${a.ID}, 'assigned')">删除</button>
        </div>
      </td>
    </tr>`;
  }).join('');
  renderPagination('assignedPagination', r.data.total, 15, page, loadAssignedAccounts);
}

function selectAssignedStatus(value, text) {
  assignedStatusFilter = value;
  document.getElementById('assignedStatusText').textContent = text;
  document.querySelectorAll('#assignedStatusDropdown .k-dropdown-item').forEach(item => item.classList.remove('selected'));
  event.target.classList.add('selected');
  toggleDropdown('assignedStatusDropdown');
  loadAssignedAccounts(1);
}

function resetAssignedFilters() {
  assignedStatusFilter = '';
  assignedKeyword = '';
  const k = document.getElementById('assignedKeyword');
  if (k) k.value = '';
  document.getElementById('assignedStatusText').textContent = '全部状态';
  document.querySelectorAll('#assignedStatusDropdown .k-dropdown-item').forEach(item => item.classList.remove('selected'));
  document.querySelector('#assignedStatusDropdown .k-dropdown-item:first-child')?.classList.add('selected');
  loadAssignedAccounts(1);
}

// 一键清空已分配账号（高危操作，双重确认）
async function doClearAssignedAccounts() {
  if (!confirm('此操作将删除所有【已分配】的账号并解除卡密绑定，不可恢复！\n\n确定要继续吗？')) return;
  var input = prompt('请输入「清空」两个字以确认操作：');
  if (input !== '清空') {
    showToast('操作已取消', 'info');
    return;
  }

  var r = await api('POST', '/admin/accounts/clear-assigned', { confirm: true });
  if (r.code === 0) {
    showToast('已分配账号已清空，共删除 ' + (r.data?.deleted || 0) + ' 个', 'success');
    loadAssignedAccounts(1);
    loadStats && loadStats();
  } else {
    showToast('清空失败：' + (r.message || r.msg || '未知错误'), 'error');
  }
}
