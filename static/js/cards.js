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
  let url = `/admin/cards?page=${page}&size=15`;
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
          <button class="ui-btn ui-btn-danger ui-btn-sm" onclick="deleteCard(${c.ID})">删除</button>
        </div>
      </td>
    </tr>`;
  }).join('');

  renderPagination('cardsPagination', r.data.total, 15, page, loadCards);
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
        <button class="ui-btn ui-btn-secondary ui-btn-sm" onclick="copyToClipboard(document.getElementById('generatedCodes').value)">一键复制全部</button>
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
  const btn = document.getElementById('batchDeleteCardsBtn');
  const count = document.getElementById('selectedCardCount');
  if (btn && count) {
    count.textContent = selectedCardIds.size;
    btn.style.display = selectedCardIds.size > 0 ? '' : 'none';
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
