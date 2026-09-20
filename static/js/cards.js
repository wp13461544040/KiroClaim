// 卡密管理模块

let cardStatusFilter = '';
let cardKeyword = '';
let cardSubscriptionFilter = '';
let cardShopStatusFilter = '';
let cardShopGroupFilter = '';
let cardShopPriceGroups = [];
let genSubscription = '';
let genShopImageData = '';
let selectedCardIds = new Set();

// 格式化复制单个卡密
async function copyCardFormatted(code) {
  try {
    const r = await api('GET', '/admin/settings');
    const template = r.code === 0 && r.data?.cardCopyTemplate ? r.data.cardCopyTemplate : '';
    const text = template ? `卡密: ${code}\n\n${template}` : code;
    copyToClipboard(text);
  } catch (err) {
    copyToClipboard(code);
  }
}

// 批量格式化复制
async function copyAllFormatted() {
  const codesText = document.getElementById('generatedCodes').value;
  const codes = codesText.split('\n').filter(c => c.trim());
  try {
    const r = await api('GET', '/admin/settings');
    const template = r.code === 0 && r.data?.cardCopyTemplate ? r.data.cardCopyTemplate : '';
    const formatted = codes.map(code => template ? `卡密: ${code}\n\n${template}` : code).join('\n\n---\n\n');
    copyToClipboard(formatted);
  } catch (err) {
    copyToClipboard(codesText);
  }
}

function escapeHtml(value) {
  return String(value == null ? '' : value).replace(/[&<>"']/g, function(c) {
    return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];
  });
}

function escapeAttr(value) {
  return escapeHtml(value).replace(/`/g, '&#96;');
}

function cardSubscriptionLabel(subscription) {
  return subscription ? subscription : '-';
}

function cardStatusBadge(status) {
  var map = {
    unused:   '<span class="k-badge k-badge-success">未使用</span>',
    active:   '<span class="k-badge k-badge-neutral">使用中</span>'
  };
  return map[status] || map.unused;
}

function formatCardPrice(amount) {
  return '¥' + (Number(amount || 0) / 100).toFixed(2);
}

function shopRelationLabel(status) {
  return ({ available: '可售', reserved: '已预留', sold: '已售' })[status] || status || '-';
}

function renderCardShopCell(card) {
  if (!card.ShopProductID) return '<span class="k-badge k-badge-neutral">未上架</span>';
  const active = !!card.ShopProductActive;
  const badge = active ? '<span class="k-badge k-badge-success">已上架</span>' : '<span class="k-badge k-badge-neutral">已下架</span>';
  return '<div style="display:flex;flex-direction:column;align-items:flex-start;gap:4px">' + badge +
    '<span style="font-size:12px;color:var(--text-muted);white-space:nowrap">' +
    formatCardPrice(card.ShopPrice) + ' · #' + card.ShopProductID + ' · ' + escapeHtml(shopRelationLabel(card.ShopRelationStatus)) +
    '</span></div>';
}

async function loadCards(page = 1) {
  cardKeyword = (document.getElementById('cardKeyword')?.value || '').trim();
  const createdFrom = document.getElementById('cardCreatedFrom')?.value || '';
  const createdTo = document.getElementById('cardCreatedTo')?.value || '';
  const size = getPageSize('cards', 15);
  let url = `/admin/cards?page=${page}&size=${size}`;
  if (cardStatusFilter) url += `&status=${cardStatusFilter}`;
  if (cardKeyword) url += `&keyword=${encodeURIComponent(cardKeyword)}`;
  if (createdFrom) url += `&created_from=${createdFrom}`;
  if (createdTo) url += `&created_to=${createdTo}`;
  if (cardSubscriptionFilter) url += `&subscription=${encodeURIComponent(cardSubscriptionFilter)}`;
  if (cardShopStatusFilter) url += `&shop_status=${encodeURIComponent(cardShopStatusFilter)}`;
  const shopGroup = cardShopPriceGroups.find(function(row) { return row.key === cardShopGroupFilter; });
  if (shopGroup) {
    url += `&shop_price=${shopGroup.amount}`;
    url += `&shop_subscription=${encodeURIComponent(shopGroup.subscription)}`;
    url += `&shop_account_count=${shopGroup.accountCount}`;
  }

  const r = await api('GET', url);
  const tbody = document.getElementById('cardsBody');
  if (!tbody) return;
  if (r.code === 0 && r.data?.filters) updateCardFilterOptions(r.data.filters);
  if (r.code !== 0 || !r.data?.list?.length) {
    tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;padding:40px;color:var(--text-muted)">无卡密记录</td></tr>';
    renderPagination('cardsPagination', 0, size, 1, loadCards, 'cards');
    updateCardBatchBtn();
    return;
  }

  tbody.innerHTML = r.data.list.map(function(c) {
    const checked = selectedCardIds.has(c.ID) ? 'checked' : '';
    const multiLabel = c.AccountCount > 1 ? `<span class="k-badge" style="background:#eff6ff;color:#1d4ed8">${c.AccountCount}号</span>` : '';
    const subscription = cardSubscriptionLabel(c.Subscription || '');
    const status = c.Status || (c.UsedAt ? 'active' : 'unused');
    return `<tr>
      <td data-label="选择"><input type="checkbox" class="k-checkbox" ${checked} onchange="toggleCardSelect(${c.ID}, this.checked)"></td>
      <td data-label="ID">${c.ID}</td>
      <td data-label="序列号"><code style="background:#f1f1f1;padding:2px 4px;white-space:nowrap">${escapeHtml(c.Code)}</code></td>
      <td data-label="账号订阅" style="font-size:12px;white-space:nowrap">${escapeHtml(subscription)} ${multiLabel}</td>
      <td data-label="状态">${cardStatusBadge(status)}</td>
      <td data-label="商城">${renderCardShopCell(c)}</td>
      <td data-label="操作">
        <div style="display:flex;gap:6px;flex-wrap:wrap">
          <button class="ui-btn ui-btn-secondary ui-btn-sm" onclick="showCardLogs(${c.ID}, '${escapeAttr(c.Code)}')">详情</button>
          <button class="ui-btn ui-btn-primary ui-btn-sm" onclick="copyCardFormatted('${escapeAttr(c.Code)}')">复制</button>
          ${status === 'active' ? `<button class="ui-btn ui-btn-sm" style="background:#10b981;color:#fff" onclick="checkCardHealth(${c.ID}, '${escapeAttr(c.Code)}')">检测</button>` : ''}
          <button class="ui-btn ui-btn-danger ui-btn-sm" onclick="deleteCard(${c.ID})">删除</button>
        </div>
      </td>
    </tr>`;
  }).join('');

  renderPagination('cardsPagination', r.data.total, size, page, loadCards, 'cards');
  updateCardBatchBtn();

  const selectAll = document.getElementById('selectAllCards');
  if (selectAll) {
    const checkboxes = tbody.querySelectorAll('input[type="checkbox"]');
    selectAll.checked = checkboxes.length > 0 && [...checkboxes].every(cb => cb.checked);
  }
}

function clearCardSelectionForFilter() {
  selectedCardIds.clear();
  updateCardBatchBtn();
  const selectAll = document.getElementById('selectAllCards');
  if (selectAll) selectAll.checked = false;
}

function selectCardSubscription(value, text, item) {
  cardSubscriptionFilter = value;
  document.getElementById('cardSubscriptionText').textContent = text;
  document.querySelectorAll('#cardSubscriptionDropdown .k-dropdown-item').forEach(function(row) { row.classList.remove('selected'); });
  if (item) item.classList.add('selected');
  toggleDropdown('cardSubscriptionDropdown');
  clearCardSelectionForFilter();
  loadCards(1);
}

function selectCardShopStatus(value, text, item) {
  cardShopStatusFilter = value;
  document.getElementById('cardShopStatusText').textContent = text;
  document.querySelectorAll('#cardShopStatusDropdown .k-dropdown-item').forEach(function(row) { row.classList.remove('selected'); });
  if (item) item.classList.add('selected');
  toggleDropdown('cardShopStatusDropdown');
  clearCardSelectionForFilter();
  loadCards(1);
}

function selectCardShopGroup(value, text, item) {
  cardShopGroupFilter = value || '';
  document.getElementById('cardShopGroupText').textContent = text;
  document.querySelectorAll('#cardShopGroupDropdown .k-dropdown-item').forEach(function(row) { row.classList.remove('selected'); });
  if (item) item.classList.add('selected');
  toggleDropdown('cardShopGroupDropdown');
  updateDelistShopGroupButton();
  clearCardSelectionForFilter();
  loadCards(1);
}

function updateCardFilterOptions(filters) {
  const subscriptions = Array.isArray(filters.subscriptions) ? filters.subscriptions : [];
  const subscriptionMenu = document.querySelector('#cardSubscriptionDropdown .k-dropdown-menu');
  if (subscriptionMenu) {
    subscriptionMenu.innerHTML = '';
    [''].concat(subscriptions).forEach(function(value) {
      const item = document.createElement('div');
      const text = value || '全部订阅';
      item.className = 'k-dropdown-item' + (value === cardSubscriptionFilter ? ' selected' : '');
      item.textContent = text;
      item.onclick = function() { selectCardSubscription(value, text, item); };
      subscriptionMenu.appendChild(item);
    });
  }

  cardShopPriceGroups = Array.isArray(filters.shopPriceGroups) ? filters.shopPriceGroups : [];
  const groupMenu = document.querySelector('#cardShopGroupDropdown .k-dropdown-menu');
  if (groupMenu) {
    groupMenu.innerHTML = '';
    const allItem = document.createElement('div');
    allItem.className = 'k-dropdown-item' + (!cardShopGroupFilter ? ' selected' : '');
    allItem.textContent = '全部售价档位';
    allItem.onclick = function() { selectCardShopGroup('', '全部售价档位', allItem); };
    groupMenu.appendChild(allItem);
    cardShopPriceGroups.forEach(function(group) {
      const item = document.createElement('div');
      const state = group.active ? '' : ' · 已下架';
      const batchText = Number(group.products || 0) > 1 ? ' · ' + group.products + ' 个批次' : '';
      const text = formatCardPrice(group.amount) + ' · ' + group.subscription + ' · ' + group.accountCount + ' 个账号 · ' + group.available + ' 可售' + batchText + state;
      item.className = 'k-dropdown-item' + (group.key === cardShopGroupFilter ? ' selected' : '');
      item.textContent = text;
      item.onclick = function() { selectCardShopGroup(group.key, text, item); };
      groupMenu.appendChild(item);
      if (group.key === cardShopGroupFilter) document.getElementById('cardShopGroupText').textContent = text;
    });
  }
  updateDelistShopGroupButton();
}

function updateDelistShopGroupButton() {
  const button = document.getElementById('delistShopGroupBtn');
  if (!button) return;
  const group = cardShopPriceGroups.find(function(row) { return row.key === cardShopGroupFilter; });
  button.style.display = group && group.active ? '' : 'none';
}

async function delistSelectedShopGroup() {
  const group = cardShopPriceGroups.find(function(row) { return row.key === cardShopGroupFilter; });
  if (!group) return;
  const message = '确认下架“' + group.subscription + ' · ' + group.accountCount + ' 个账号 · ' + formatCardPrice(group.amount) + '”的全部 ' + group.products + ' 个批次，并删除 ' + group.available + ' 张未售卡密？已售卡密会保留。';
  if (!confirm(message)) return;
  const r = await api('POST', '/admin/cards/shop-products/delist-group', {
    amount: Number(group.amount),
    subscription: group.subscription,
    account_count: Number(group.accountCount)
  });
  if (r.code === 0) {
    showToast('售价档位已下架，共处理 ' + (r.data?.products || 0) + ' 个批次，删除未售卡密 ' + (r.data?.deleted || 0) + ' 张，保留已售卡密 ' + (r.data?.preservedSold || 0) + ' 张', 'success');
    clearCardSelectionForFilter();
    loadCards(1);
    loadStats();
  } else {
    showToast('下架失败：' + (r.message || r.msg || '未知错误'), 'error');
  }
}

function selectCardFilter(value, text, item) {
  cardStatusFilter = value;
  document.getElementById('cardFilterText').textContent = text;
  document.querySelectorAll('#cardFilterDropdown .k-dropdown-item').forEach(function(item) {
    item.classList.remove('selected');
  });
  if (item) item.classList.add('selected');
  toggleDropdown('cardFilterDropdown');
  clearCardSelectionForFilter();
  loadCards(1);
}

function selectGenSubscription(value, text) {
  const subscription = String(value || '').trim();
  if (!subscription) return;
  genSubscription = subscription;
  document.getElementById('genSubscriptionText').textContent = text || cardSubscriptionLabel(subscription);
  document.querySelectorAll('#genSubscriptionDropdown .k-dropdown-item').forEach(function(item) {
    item.classList.toggle('selected', item.getAttribute('data-subscription') === genSubscription);
  });
  toggleDropdown('genSubscriptionDropdown');
  updateModeHint();
}

function getGenAccountCount(normalizeInput) {
  const input = document.getElementById('genAccountCount');
  let count = parseInt(input?.value, 10);
  if (!Number.isFinite(count) || count < 1) count = 1;
  if (normalizeInput && input) input.value = count;
  return count;
}

function updateModeHint() {
  const count = parseInt(document.getElementById('genCount')?.value) || 1;
  const accountCount = getGenAccountCount(false);
  const hint = document.getElementById('genModeHint');
  if (!hint) return;
  const accountText = `每张绑定 ${accountCount} 个账号`;
  const subscriptionText = genSubscription ? cardSubscriptionLabel(genSubscription) : '请先选择账号订阅';
  hint.textContent = `将生成 ${count} 张卡密，${accountText}，账号订阅：${subscriptionText}。`;
}

async function showGenerateModal() {
  document.getElementById('generateModal').classList.add('active');
  await loadCardSubscriptionStats();
  updateModeHint();
}

async function loadCardSubscriptionStats() {
  const r = await api('GET', '/admin/accounts/subscription-stats');
  const dropdown = document.getElementById('genSubscriptionDropdown');
  if (!dropdown) return;
  const menu = dropdown.querySelector('.k-dropdown-menu');
  if (!menu) return;
  const text = document.getElementById('genSubscriptionText');

  if (r.code !== 0 || !Array.isArray(r.data)) {
    genSubscription = '';
    if (text) text.textContent = '订阅加载失败';
    menu.innerHTML = '<div class="k-dropdown-item disabled">订阅加载失败</div>';
    updateModeHint();
    return;
  }

  const items = r.data.map(function(it) {
    return {
      subscription: String(it.subscription || '').trim(),
      label: String(it.subscription || '').trim(),
      unusedCount: it.unusedCount || 0,
      totalCount: it.totalCount || 0
    };
  }).filter(function(it) {
    return !!it.subscription;
  });

  if (!items.length) {
    genSubscription = '';
    if (text) text.textContent = '暂无订阅类型';
    menu.innerHTML = '<div class="k-dropdown-item disabled">暂无订阅类型</div>';
    updateModeHint();
    return;
  }

  genSubscription = items[0].subscription;
  const selectedItem = items[0];
  if (text) text.textContent = selectedItem.label;

  menu.innerHTML = items.map(function(it) {
    const selected = genSubscription === it.subscription ? 'selected' : '';
    const countColor = it.unusedCount > 0 ? '#999' : '#dc2626';
    return '<div class="k-dropdown-item ' + selected + '" ' +
      'data-subscription="' + escapeAttr(it.subscription) + '" data-label="' + escapeAttr(it.label) + '">' +
      escapeHtml(it.label) + ' <span style="color:' + countColor + ';font-size:12px">(' + it.unusedCount + ' 可用)</span>' +
      '</div>';
  }).join('');

  menu.querySelectorAll('.k-dropdown-item').forEach(function(item) {
    item.addEventListener('click', function() {
      selectGenSubscription(this.getAttribute('data-subscription') || '', this.getAttribute('data-label') || '');
    });
  });
}

function closeGenerateModal() {
  document.getElementById('generateModal').classList.remove('active');
  document.getElementById('generateResult').innerHTML = '';
}

function toggleGenerateShopFields() {
  const listed = document.getElementById('genListOnShop').checked;
  document.getElementById('genShopFields').hidden = !listed;
  if (!listed) clearGenerateShopImage();
}

function clearGenerateShopImage() {
  genShopImageData = '';
  const input = document.getElementById('genShopImage');
  const preview = document.getElementById('genShopImagePreview');
  const clearButton = document.getElementById('clearGenShopImageBtn');
  if (input) input.value = '';
  if (preview) {
    preview.removeAttribute('src');
    preview.hidden = true;
  }
  if (clearButton) clearButton.hidden = true;
}

function handleGenerateShopImage(input) {
  const file = input.files && input.files[0];
  if (!file) {
    clearGenerateShopImage();
    return;
  }
  if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) {
    clearGenerateShopImage();
    showToast('商城商品图片仅支持 PNG、JPEG 或 WebP', 'error');
    return;
  }
  if (file.size > 2 * 1024 * 1024) {
    clearGenerateShopImage();
    showToast('商城商品图片不能超过 2 MB', 'error');
    return;
  }
  const reader = new FileReader();
  reader.onload = function() {
    genShopImageData = String(reader.result || '');
    const preview = document.getElementById('genShopImagePreview');
    preview.src = genShopImageData;
    preview.hidden = false;
    document.getElementById('clearGenShopImageBtn').hidden = false;
  };
  reader.onerror = function() {
    clearGenerateShopImage();
    showToast('商城商品图片读取失败', 'error');
  };
  reader.readAsDataURL(file);
}

function yuanToCents(value) {
  const raw = String(value == null ? '' : value).trim();
  if (!/^\d+(?:\.\d{1,2})?$/.test(raw)) return null;
  const parts = raw.split('.');
  const whole = Number(parts[0]);
  const fraction = Number(((parts[1] || '') + '00').slice(0, 2));
  const cents = whole * 100 + fraction;
  return Number.isSafeInteger(cents) ? cents : null;
}

async function doGenerate() {
  const count = parseInt(document.getElementById('genCount').value) || 1;
  const accountCount = getGenAccountCount(true);
  const listOnShop = document.getElementById('genListOnShop').checked;
  const shopPrice = yuanToCents(document.getElementById('genShopPrice').value);
  const resultEl = document.getElementById('generateResult');
  if (!genSubscription) {
    const msg = '请先选择账号订阅';
    resultEl.innerHTML = '<span style="color:red">' + msg + '</span>';
    showToast(msg, 'error');
    return;
  }
  if (listOnShop && (!Number.isFinite(shopPrice) || shopPrice < 0)) {
    showToast('请填写正确的人民币售价', 'error');
    return;
  }

  const r = await api('POST', '/admin/cards/generate', {
    count,
    account_count: accountCount,
    subscription: genSubscription,
    list_on_shop: listOnShop,
    price: listOnShop ? shopPrice : 0,
    image_data: listOnShop ? genShopImageData : ''
  });
  if (r.code === 0) {
    const codes = (r.data?.codes || []).join('\n');
    resultEl.innerHTML = `<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
        <span style="font-size:13px;color:var(--text-muted)">生成成功，共 ${r.data?.codes?.length ?? count} 张：</span>
        <div style="display:flex;gap:6px">
          <button class="ui-btn ui-btn-secondary ui-btn-sm" onclick="copyToClipboard(document.getElementById('generatedCodes').value)">一键复制全部</button>
          <button class="ui-btn ui-btn-primary ui-btn-sm" onclick="copyAllFormatted()">格式化复制全部</button>
        </div>
      </div>
      <textarea class="k-input" id="generatedCodes" rows="8" readonly style="font-family:monospace;font-size:12px">${escapeHtml(codes)}</textarea>`;
    loadCards(1);
    showToast(`成功生成 ${r.data?.codes?.length ?? count} 张卡密`, 'success');
  } else {
    const msg = r.message || r.msg || '未知错误';
    resultEl.innerHTML = '<span style="color:red">生成失败：' + escapeHtml(msg) + '</span>';
    showToast('生成失败：' + msg, 'error');
  }
}

async function deleteCard(id) {
  if (!confirm('确认删除该卡密？')) return;
  const r = await api('DELETE', '/admin/cards/' + id);
  if (r.code === 0) {
    showToast('卡密删除成功', 'success');
    loadCards(1);
  } else {
    showToast('删除失败：' + (r.message || r.msg || '未知错误'), 'error');
  }
}

function resetCardFilters() {
  cardStatusFilter = '';
  cardKeyword = '';
  cardSubscriptionFilter = '';
  cardShopStatusFilter = '';
  cardShopGroupFilter = '';

  var keywordInput = document.getElementById('cardKeyword');
  if (keywordInput) keywordInput.value = '';
  var cf = document.getElementById('cardCreatedFrom');
  if (cf) cf.value = '';
  var ct = document.getElementById('cardCreatedTo');
  if (ct) ct.value = '';

  document.getElementById('cardFilterText').textContent = '全部状态';
  document.getElementById('cardSubscriptionText').textContent = '全部订阅';
  document.getElementById('cardShopStatusText').textContent = '全部商城状态';
  document.getElementById('cardShopGroupText').textContent = '全部售价档位';
  document.querySelectorAll('#cardFilterDropdown .k-dropdown-item').forEach(function(item) {
    item.classList.remove('selected');
  });
  document.querySelector('#cardFilterDropdown .k-dropdown-item:first-child')?.classList.add('selected');
  document.querySelectorAll('#cardShopStatusDropdown .k-dropdown-item').forEach(function(item, index) { item.classList.toggle('selected', index === 0); });
  updateDelistShopGroupButton();
  clearCardSelectionForFilter();
  loadCards(1);
}

function toggleCardSelect(id, checked) {
  if (checked) selectedCardIds.add(id);
  else selectedCardIds.delete(id);
  updateCardBatchBtn();
  const checkboxes = document.querySelectorAll('#cardsBody input[type="checkbox"]');
  const selectAll = document.getElementById('selectAllCards');
  if (selectAll) {
    selectAll.checked = checkboxes.length > 0 && [...checkboxes].every(cb => cb.checked);
  }
}

function toggleSelectAllCards(checked) {
  const checkboxes = document.querySelectorAll('#cardsBody input[type="checkbox"]');
  checkboxes.forEach(function(cb) {
    cb.checked = checked;
    const id = parseInt(cb.closest('tr').querySelector('td:nth-child(2)').textContent);
    if (checked) selectedCardIds.add(id);
    else selectedCardIds.delete(id);
  });
  updateCardBatchBtn();
}

function updateCardBatchBtn() {
  const deleteBtn = document.getElementById('batchDeleteCardsBtn');
  const healthBtn = document.getElementById('batchCheckHealthBtn');
  const deleteCount = document.getElementById('selectedCardCount');
  const healthCount = document.getElementById('selectedCardCountHealth');
  
  if (deleteBtn && deleteCount) {
    deleteCount.textContent = selectedCardIds.size;
    deleteBtn.style.display = selectedCardIds.size > 0 ? '' : 'none';
  }
  
  if (healthBtn && healthCount) {
    healthCount.textContent = selectedCardIds.size;
    // 只在"使用中"状态时显示批量检测按钮
    healthBtn.style.display = (selectedCardIds.size > 0 && cardStatusFilter === 'active') ? '' : 'none';
  }
}

async function batchDeleteCards() {
  if (selectedCardIds.size === 0) return;
  if (!confirm(`确认删除选中的 ${selectedCardIds.size} 张卡密？`)) return;

  const r = await api('POST', '/admin/cards/batch-delete', { ids: [...selectedCardIds] });
  if (r.code === 0) {
    showToast(`成功删除 ${r.data?.deleted || selectedCardIds.size} 张卡密`, 'success');
    selectedCardIds.clear();
    updateCardBatchBtn();
    const selectAll = document.getElementById('selectAllCards');
    if (selectAll) selectAll.checked = false;
    loadCards(1);
    loadStats();
  } else {
    showToast('批量删除失败：' + (r.message || r.msg || '未知错误'), 'error');
  }
}

async function showCardLogs(cardId, code) {
  var r = await api('GET', '/admin/cards/' + cardId + '/logs');
  var logs = (r.code === 0 && r.data) ? r.data : [];

  var old = document.getElementById('cardLogModal');
  if (old) old.remove();

  var overlay = document.createElement('div');
  overlay.id = 'cardLogModal';
  overlay.className = 'modal-overlay active card-log-modal';

  var content = '<div class="modal-content card-log-content">';
  content += '<div class="modal-header"><span class="modal-title">卡密使用记录 - ' + escapeHtml(code) + '</span>';
  content += '<button type="button" class="modal-close" aria-label="关闭" title="关闭" onclick="document.getElementById(\'cardLogModal\').remove()">&times;</button></div>';
  content += '<div class="modal-body card-log-body">';

  if (!logs.length) {
    content += '<div style="text-align:center;color:#999;padding:40px;font-size:13px">暂无使用记录</div>';
  } else {
    content += '<table class="card-log-table"><thead><tr><th>操作</th><th>账号邮箱</th><th>客户端 IP</th><th>时间</th></tr></thead><tbody>';
    logs.forEach(function(log) {
      var actionLabel = log.Action === 'activate' ? '激活' : log.Action;
      var timeStr = new Date(log.CreatedAt).toLocaleString('zh-CN', {hour12: false});
      content += '<tr>';
      content += '<td data-label="操作" class="card-log-action" style="font-size:13px">' + escapeHtml(actionLabel) + '</td>';
      content += '<td data-label="账号邮箱" class="card-log-email" style="font-size:12px;font-family:monospace">' + escapeHtml(log.Email || ('ID:' + log.AccountID)) + '</td>';
      content += '<td data-label="客户端 IP" class="card-log-ip" style="font-size:12px;color:#999">' + escapeHtml(log.ClientIP || '-') + '</td>';
      content += '<td data-label="时间" class="card-log-time" style="font-size:12px;color:#999;white-space:nowrap">' + escapeHtml(timeStr) + '</td>';
      content += '</tr>';
    });
    content += '</tbody></table>';
  }
  content += '</div></div>';
  overlay.innerHTML = content;
  document.body.appendChild(overlay);
}

// 检测单个卡密健康状态（使用SSE流式更新）
async function checkCardHealth(cardId, cardCode) {
  // 第一步：立即显示缓存数据
  const quickResult = await api('GET', `/admin/cards/${cardId}/health/quick`);
  
  if (quickResult.code !== 0) {
    showToast('获取数据失败：' + (quickResult.message || quickResult.msg || '未知错误'), 'error');
    return;
  }

  // 立即显示缓存数据
  const initialData = quickResult.data;
  showCardHealthModal(initialData, true); // 传入 true 表示正在刷新

  // 第二步：建立SSE连接实时更新（通过URL参数传递Token）
  const token = localStorage.getItem('adminToken');
  if (!token) {
    showToast('未登录或登录已过期', 'error');
    return;
  }
  const eventSource = new EventSource(`/admin/cards/${cardId}/health?token=${encodeURIComponent(token)}`);

  let accountsMap = {}; // 用于存储账号更新
  initialData.accounts.forEach((acc, idx) => {
    accountsMap[idx] = acc;
  });

  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      
      if (data.type === 'account') {
        // 更新单个账号数据
        accountsMap[data.index] = data.account;
        
        // 重新计算统计信息
        const accounts = Object.values(accountsMap);
        const updatedData = calculateHealthStats(initialData.card_id, initialData.card_code, accounts);
        updatedData.progress = `${data.index + 1}/${data.total}`;
        
        // 增量更新显示（不重新创建弹框）
        updateCardHealthModal(updatedData, true);
        
      } else if (data.type === 'complete') {
        // 全部完成
        eventSource.close();
        const accounts = Object.values(accountsMap);
        const finalData = calculateHealthStats(initialData.card_id, initialData.card_code, accounts);
        updateCardHealthModal(finalData, false); // 刷新完成
        showToast('健康检测完成', 'success');
      }
    } catch (err) {
      console.error('SSE parse error:', err);
    }
  };

  eventSource.onerror = (err) => {
    console.error('SSE error:', err);
    eventSource.close();
    showToast('实时更新连接断开', 'warning');
  };
}

// 计算健康统计信息
function calculateHealthStats(cardId, cardCode, accounts) {
  let healthy = 0, used = 0, suspended = 0;
  let totalCredit = 0, usedCredit = 0;

  accounts.forEach(acc => {
    totalCredit += acc.credit_limit || 0;
    usedCredit += acc.credit_used || 0;

    if (acc.used) {
      used++;
    } else if (acc.status === 'suspended') {
      suspended++;
    } else if (acc.status === 'active') {
      healthy++;
    }
  });

  return {
    card_id: cardId,
    card_code: cardCode,
    total_bound: accounts.length,
    healthy: healthy,
    used: used,
    suspended: suspended,
    total_credit: totalCredit,
    used_credit: usedCredit,
    avg_credit_pct: totalCredit > 0 ? (usedCredit / totalCredit * 100) : 0,
    accounts: accounts
  };
}

// 批量检测卡密健康状态
async function batchCheckCardsHealth() {
  if (selectedCardIds.size === 0) {
    showToast('请先选择要检测的卡密', 'warning');
    return;
  }

  if (selectedCardIds.size > 100) {
    showToast('单次最多检测100张卡密', 'error');
    return;
  }

  showToast('正在检测中...', 'info');
  const r = await api('POST', '/admin/cards/batch-health', { card_ids: [...selectedCardIds] });
  
  if (r.code !== 0) {
    showToast('批量检测失败：' + (r.message || r.msg || '未知错误'), 'error');
    return;
  }

  showBatchHealthModal(r.data);
}

// 显示单个卡密健康检测结果
function showCardHealthModal(data, isRefreshing = false) {
  // 如果模态框已存在，先移除
  const existingModal = document.getElementById('cardHealthModal');
  if (existingModal) {
    existingModal.remove();
  }

  const modal = document.createElement('div');
  modal.id = 'cardHealthModal';
  modal.className = 'modal-overlay active';

  const accounts = Array.isArray(data.accounts) ? data.accounts : [];
  const creditPct = Number(data.avg_credit_pct || 0).toFixed(1);

  // 刷新状态提示
  let refreshHint = '';
  if (isRefreshing) {
    const progress = data.progress || '正在加载';
    refreshHint = `<div id="refreshHint" style="background:#f59e0b20;color:#f59e0b;padding:8px 12px;border-radius:6px;margin-bottom:16px;text-align:center;font-size:13px">⏳ 正在实时刷新... ${progress}</div>`;
  }

  let content = `
    <div class="modal-content" style="max-width: 900px">
      <div class="modal-header">
        <span class="modal-title">账号健康检测 - ${escapeHtml(data.card_code)}</span>
        <button type="button" class="modal-close" onclick="document.getElementById('cardHealthModal').remove()">&times;</button>
      </div>
      <div class="modal-body">
        ${refreshHint}
        <div id="statsGrid" style="display:grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 16px; margin-bottom: 24px">
          <div class="k-stat-card">
            <div class="k-stat-label">总绑定账号</div>
            <div class="k-stat-value" data-stat="total">${data.total_bound || 0}</div>
          </div>
          <div class="k-stat-card">
            <div class="k-stat-label">健康账号</div>
            <div class="k-stat-value" style="color:#22c55e" data-stat="healthy">${data.healthy || 0}</div>
          </div>
          <div class="k-stat-card">
            <div class="k-stat-label">已使用</div>
            <div class="k-stat-value" style="color:#f59e0b" data-stat="used">${data.used || 0}</div>
          </div>
          <div class="k-stat-card">
            <div class="k-stat-label">已封禁</div>
            <div class="k-stat-value" style="color:#dc2626" data-stat="suspended">${data.suspended || 0}</div>
          </div>
          <div class="k-stat-card">
            <div class="k-stat-label">已删除</div>
            <div class="k-stat-value" style="color:#9ca3af" data-stat="deleted">${data.deleted || 0}</div>
          </div>
          <div class="k-stat-card">
            <div class="k-stat-label">平均额度使用</div>
            <div class="k-stat-value" data-stat="credit">${creditPct}%</div>
          </div>
        </div>
  `;

  if (accounts.length > 0) {
    content += `
      <div style="margin-top: 20px">
        <h4 style="margin-bottom: 12px; font-size: 14px; font-weight: 600">账号详情</h4>
        <div style="max-height: 400px; overflow-y: auto">
          <table class="k-table">
            <thead>
              <tr>
                <th>邮箱</th>
                <th>状态</th>
                <th>额度使用</th>
                <th>更新时间</th>
                <th>数据来源</th>
              </tr>
            </thead>
            <tbody id="accountsTableBody">
    `;

    accounts.forEach((acc, idx) => {
      content += generateAccountRow(acc, idx);
    });

    content += `
            </tbody>
          </table>
        </div>
      </div>
    `;
  }

  content += `
        <div style="margin-top: 20px; text-align: right">
          <button class="ui-btn ui-btn-secondary" onclick="document.getElementById('cardHealthModal').remove()">关闭</button>
        </div>
      </div>
    </div>
  `;

  modal.innerHTML = content;
  document.body.appendChild(modal);
}

// 增量更新健康检测弹框（不重新创建，只更新内容）
function updateCardHealthModal(data, isRefreshing = false) {
  const modal = document.getElementById('cardHealthModal');
  if (!modal) return; // 弹框已关闭

  const accounts = Array.isArray(data.accounts) ? data.accounts : [];
  const creditPct = Number(data.avg_credit_pct || 0).toFixed(1);

  // 更新刷新提示
  const refreshHint = modal.querySelector('#refreshHint');
  if (refreshHint) {
    if (isRefreshing) {
      const progress = data.progress || '正在加载';
      refreshHint.textContent = `⏳ 正在实时刷新... ${progress}`;
    } else {
      refreshHint.remove(); // 完成后移除提示
    }
  }

  // 更新统计数据
  const totalEl = modal.querySelector('[data-stat="total"]');
  const healthyEl = modal.querySelector('[data-stat="healthy"]');
  const usedEl = modal.querySelector('[data-stat="used"]');
  const suspendedEl = modal.querySelector('[data-stat="suspended"]');
  const deletedEl = modal.querySelector('[data-stat="deleted"]');
  const creditEl = modal.querySelector('[data-stat="credit"]');

  if (totalEl) totalEl.textContent = data.total_bound || 0;
  if (healthyEl) healthyEl.textContent = data.healthy || 0;
  if (usedEl) usedEl.textContent = data.used || 0;
  if (suspendedEl) suspendedEl.textContent = data.suspended || 0;
  if (deletedEl) deletedEl.textContent = data.deleted || 0;
  if (creditEl) creditEl.textContent = creditPct + '%';

  // 更新账号表格（只更新变化的行）
  const tbody = modal.querySelector('#accountsTableBody');
  if (tbody && accounts.length > 0) {
    accounts.forEach((acc, idx) => {
      let row = tbody.querySelector(`tr[data-account-idx="${idx}"]`);
      if (!row) {
        // 新行，直接添加
        row = document.createElement('tr');
        row.setAttribute('data-account-idx', idx);
        tbody.appendChild(row);
      }
      // 更新行内容
      row.innerHTML = generateAccountRowContent(acc, idx);
    });
  }
}

// 生成账号表格行（完整）
function generateAccountRow(acc, idx) {
  return `<tr data-account-idx="${idx}">${generateAccountRowContent(acc, idx)}</tr>`;
}

// 生成账号表格行内容（仅内容，不含 tr 标签）
function generateAccountRowContent(acc, idx) {
  const statusBadge = acc.used ? 
    '<span class="k-badge" style="background:#f59e0b;color:#fff">已使用</span>' :
    (acc.status === 'suspended' ? 
      '<span class="k-badge" style="background:#dc2626;color:#fff">已封禁</span>' :
      '<span class="k-badge k-badge-success">健康</span>');
  
  const creditUsed = Number(acc.credit_used || 0).toFixed(2);
  const creditLimit = Number(acc.credit_limit || 0).toFixed(2);
  const creditPct = creditLimit > 0 ? ((acc.credit_used / acc.credit_limit) * 100).toFixed(1) : '0.0';

  // 格式化更新时间
  const updatedAt = acc.updated_at ? formatDateTime(acc.updated_at) : '-';
  
  // 数据来源标记
  const dataSourceBadge = acc.data_source === 'realtime' ? 
    '<span class="k-badge" style="background:#10b981;color:#fff">实时</span>' :
    '<span class="k-badge" style="background:#6b7280;color:#fff">缓存</span>';

  return `
    <td data-label="邮箱" style="font-size:12px;font-family:monospace">${escapeHtml(acc.email || 'ID:' + acc.id)}</td>
    <td data-label="状态">${statusBadge}</td>
    <td data-label="额度使用" style="font-size:12px">${creditUsed}/${creditLimit} (${creditPct}%)</td>
    <td data-label="更新时间" style="font-size:11px;color:#6b7280">${updatedAt}</td>
    <td data-label="数据来源">${dataSourceBadge}</td>
  `;
}

// 格式化日期时间（辅助函数）
function formatDateTime(dateStr) {
  if (!dateStr) return '-';
  try {
    const date = new Date(dateStr);
    const now = new Date();
    const diffMs = now - date;
    const diffMins = Math.floor(diffMs / 60000);
    
    // 1小时内显示相对时间
    if (diffMins < 60) {
      if (diffMins < 1) return '刚刚';
      return `${diffMins}分钟前`;
    }
    
    // 24小时内显示小时
    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) {
      return `${diffHours}小时前`;
    }
    
    // 否则显示日期时间
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    const hours = String(date.getHours()).padStart(2, '0');
    const minutes = String(date.getMinutes()).padStart(2, '0');
    
    // 今年的话省略年份
    if (year === now.getFullYear()) {
      return `${month}-${day} ${hours}:${minutes}`;
    }
    return `${year}-${month}-${day} ${hours}:${minutes}`;
  } catch (e) {
    return dateStr;
  }
}

// 显示批量检测结果
function showBatchHealthModal(results) {
  const modal = document.createElement('div');
  modal.id = 'batchHealthModal';
  modal.className = 'modal-overlay active';

  let totalBound = 0;
  let totalHealthy = 0;
  let totalUsed = 0;
  let totalSuspended = 0;
  let totalDeleted = 0;

  results.forEach(r => {
    totalBound += r.total_bound || 0;
    totalHealthy += r.healthy || 0;
    totalUsed += r.used || 0;
    totalSuspended += r.suspended || 0;
    totalDeleted += r.deleted || 0;
  });

  let content = `
    <div class="modal-content" style="max-width: 1000px">
      <div class="modal-header">
        <span class="modal-title">批量健康检测结果 (${results.length} 张卡密)</span>
        <button type="button" class="modal-close" onclick="document.getElementById('batchHealthModal').remove()">&times;</button>
      </div>
      <div class="modal-body">
        <div style="display:grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 12px; margin-bottom: 24px; padding: 16px; background: #f9fafb; border-radius: 8px">
          <div style="text-align:center">
            <div style="font-size:12px;color:#6b7280;margin-bottom:4px">总绑定</div>
            <div style="font-size:24px;font-weight:600">${totalBound}</div>
          </div>
          <div style="text-align:center">
            <div style="font-size:12px;color:#6b7280;margin-bottom:4px">健康</div>
            <div style="font-size:24px;font-weight:600;color:#22c55e">${totalHealthy}</div>
          </div>
          <div style="text-align:center">
            <div style="font-size:12px;color:#6b7280;margin-bottom:4px">已使用</div>
            <div style="font-size:24px;font-weight:600;color:#f59e0b">${totalUsed}</div>
          </div>
          <div style="text-align:center">
            <div style="font-size:12px;color:#6b7280;margin-bottom:4px">已封禁</div>
            <div style="font-size:24px;font-weight:600;color:#dc2626">${totalSuspended}</div>
          </div>
          <div style="text-align:center">
            <div style="font-size:12px;color:#6b7280;margin-bottom:4px">已删除</div>
            <div style="font-size:24px;font-weight:600;color:#9ca3af">${totalDeleted}</div>
          </div>
        </div>

        <div style="max-height: 500px; overflow-y: auto">
          <table class="k-table">
            <thead>
              <tr>
                <th>卡密</th>
                <th>总绑定</th>
                <th>健康</th>
                <th>已使用</th>
                <th>已封禁</th>
                <th>已删除</th>
                <th>平均额度</th>
              </tr>
            </thead>
            <tbody>
  `;

  results.forEach(r => {
    const creditPct = Number(r.avg_credit_pct || 0).toFixed(1);
    const hasIssues = (r.deleted > 0 || r.suspended > 0 || r.used > 0);
    const rowStyle = hasIssues ? 'background:#fef2f2' : '';

    content += `
      <tr style="${rowStyle}">
        <td data-label="卡密" style="font-size:12px;font-family:monospace">${escapeHtml(r.card_code || r.card_id)}</td>
        <td data-label="总绑定">${r.total_bound || 0}</td>
        <td data-label="健康" style="color:#22c55e;font-weight:600">${r.healthy || 0}</td>
        <td data-label="已使用" style="color:#f59e0b">${r.used || 0}</td>
        <td data-label="已封禁" style="color:#dc2626">${r.suspended || 0}</td>
        <td data-label="已删除" style="color:#9ca3af">${r.deleted || 0}</td>
        <td data-label="平均额度" style="font-size:12px">${creditPct}%</td>
      </tr>
    `;
  });

  content += `
            </tbody>
          </table>
        </div>

        <div style="margin-top: 20px; text-align: right">
          <button class="ui-btn ui-btn-secondary" onclick="document.getElementById('batchHealthModal').remove()">关闭</button>
        </div>
      </div>
    </div>
  `;

  modal.innerHTML = content;
  document.body.appendChild(modal);
}
