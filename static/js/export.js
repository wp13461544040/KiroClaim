// 导出功能模块

// 将 ISO 时间字符串格式化为 "YYYY-MM-DD HH:mm:ss"
function formatExportTime(isoStr) {
  if (!isoStr) return '';
  var d = new Date(isoStr);
  if (isNaN(d.getTime())) return '';
  var pad = function(n) { return n < 10 ? '0' + n : '' + n; };
  return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) +
    ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds());
}

// 通用文件下载
function downloadFile(content, filename, mimeType) {
  var blob = new Blob([content], { type: mimeType });
  var url = URL.createObjectURL(blob);
  var a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

// 按页拉取全部数据，直到后端返回的列表少于当前页大小
async function fetchAllPages(baseUrl, pageSize) {
  var all = [];
  var page = 1;
  while (true) {
    var sep = baseUrl.indexOf('?') >= 0 ? '&' : '?';
    var r = await api('GET', baseUrl + sep + 'page=' + page + '&size=' + pageSize);
    if (r.code !== 0 || !r.data || !r.data.list || !r.data.list.length) break;
    all = all.concat(r.data.list);
    if (r.data.list.length < pageSize) break;
    page++;
  }
  return all;
}

// 直接下载后端流式导出的文件。
// 账号导出必须走这个通道：列表接口不返回凭证字段，只有导出端点才带 token。
// 后端边查边写 response，一次请求拿完，不再按页累积到内存。
async function downloadExport(path, fallbackName) {
  var opts = { method: 'GET', headers: {} };
  if (typeof ADMIN_TOKEN !== 'undefined' && ADMIN_TOKEN) {
    opts.headers['Authorization'] = 'Bearer ' + ADMIN_TOKEN;
  }

  var resp = await fetch(path, opts);
  if (resp.status === 401 || resp.status === 403) {
    localStorage.removeItem('adminToken');
    if (typeof ADMIN_TOKEN !== 'undefined') ADMIN_TOKEN = null;
    if (typeof showLogin === 'function') showLogin();
    throw new Error('Token 已失效');
  }
  if (!resp.ok) {
    throw new Error('导出失败：HTTP ' + resp.status);
  }

  // 用 blob 直接落盘，不把内容解析成 JS 对象再序列化一遍
  var blob = await resp.blob();
  var name = fallbackName;
  var disposition = resp.headers.get('Content-Disposition') || '';
  var matched = disposition.match(/filename=([^;]+)/);
  if (matched) name = matched[1].trim().replace(/^"|"$/g, '');

  var url = URL.createObjectURL(blob);
  var a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
  return blob.size;
}

// 导出账号，仅支持 JSON 格式
// exportType: 'selected' 导出选中账号, 'all' 导出全部筛选结果
async function exportAccounts(format, exportType) {
  // 只支持 JSON 格式
  if (format !== 'json') {
    showToast('仅支持 JSON 格式导出', 'error');
    return;
  }

  var url = '/admin/accounts/export?used=false';
  var description = '全部';

  if (exportType === 'selected') {
    if (selectedAccountIds.size === 0) {
      showToast('请先选择要导出的账号', 'info');
      return;
    }
    // 按 ID 精确导出，不再受"只取第一页"的限制
    url += '&ids=' + Array.from(selectedAccountIds).join(',');
    description = '选中的';
  } else {
    if (accountStatusFilter) url += '&status=' + accountStatusFilter;
    if (accountSubscriptionFilter) url += '&subscription=' + encodeURIComponent(accountSubscriptionFilter);
    if (accountKeyword) url += '&keyword=' + encodeURIComponent(accountKeyword);
  }

  var dateStr = new Date().toISOString().slice(0, 10);
  try {
    showToast('正在导出，请稍候...', 'info');
    await downloadExport(url, 'accounts_' + dateStr + '.json');
    showToast('导出' + description + '账号完成', 'success');
  } catch (e) {
    showToast(e.message || '导出失败', 'error');
  }
}

// 导出已分配账号，仅支持 JSON 格式
// exportType: 'selected' 导出选中账号, 'all' 导出全部筛选结果
async function exportAssignedAccounts(format, exportType) {
  // 只支持 JSON 格式
  if (format !== 'json') {
    showToast('仅支持 JSON 格式导出', 'error');
    return;
  }

  var url = '/admin/accounts/export?used=true';
  var description = '全部';

  if (exportType === 'selected') {
    if (selectedAssignedIds.size === 0) {
      showToast('请先选择要导出的账号', 'info');
      return;
    }
    url += '&ids=' + Array.from(selectedAssignedIds).join(',');
    description = '选中的';
  } else {
    if (assignedStatusFilter) url += '&status=' + assignedStatusFilter;
    if (assignedKeyword) url += '&keyword=' + encodeURIComponent(assignedKeyword);
  }

  var dateStr = new Date().toISOString().slice(0, 10);
  try {
    showToast('正在导出，请稍候...', 'info');
    await downloadExport(url, 'assigned_accounts_' + dateStr + '.json');
    showToast('导出' + description + '已分配账号完成', 'success');
  } catch (e) {
    showToast(e.message || '导出失败', 'error');
  }
}


// 导出卡密，支持 txt / csv / xlsx 格式
// 导出卡密，仅支持 JSON 格式
// exportType: 'selected' 导出选中卡密, 'all' 导出全部筛选结果
async function exportCards(format, exportType) {
  // 只支持 JSON 格式
  if (format !== 'json') {
    showToast('仅支持 JSON 格式导出', 'error');
    return;
  }

  let list = [];
  let description = '';

  if (exportType === 'selected') {
    // 导出选中的卡密
    if (selectedCardIds.size === 0) {
      showToast('请先选择要导出的卡密', 'info');
      return;
    }
    
    // 构造查询 URL
    var url = '/admin/cards?size=1000';
    const r = await api('GET', url);
    
    if (r.code === 0 && r.data && r.data.list) {
      // 过滤出选中的卡密
      const ids = Array.from(selectedCardIds);
      list = r.data.list.filter(c => ids.includes(c.ID));
    }
    
    description = '选中的';
  } else {
    // 导出全部卡密（按筛选条件）
    var url = '/admin/cards';
    var qs = [];
    if (cardStatusFilter) qs.push('status=' + cardStatusFilter);
    if (cardKeyword) qs.push('keyword=' + encodeURIComponent(cardKeyword));
    if (qs.length) url += '?' + qs.join('&');

    list = await fetchAllPages(url, 500);
    description = '全部';
  }

  if (!list.length) {
    showToast('没有可导出的数据', 'info');
    return;
  }

  var dateStr = new Date().toISOString().slice(0, 10);

  // 导出为 JSON 格式
  var jsonData = list.map(function(c) {
    return {
      id: c.ID || 0,
      code: c.Code || '',
      subscription: c.Subscription || '',
      accountCount: c.AccountCount || 1,
      status: c.Status || '',
      createdAt: c.CreatedAt || ''
    };
  });
  var jsonStr = JSON.stringify(jsonData, null, 2);
  downloadFile(jsonStr, 'cards_' + dateStr + '.json', 'application/json;charset=utf-8');

  showToast(`导出${description}卡密成功，共 ${list.length} 张`, 'success');
}

// 显示自定义导出数量模态框
function showExportCustomModal() {
  document.getElementById('exportCustomModal').style.display = 'flex';
  document.getElementById('exportCustomCount').value = '10';
  document.getElementById('exportCustomCount').focus();
}

// 关闭自定义导出数量模态框
function closeExportCustomModal() {
  document.getElementById('exportCustomModal').style.display = 'none';
}

// 执行自定义数量导出（先选中再导出）
async function doExportCustom() {
  const countInput = document.getElementById('exportCustomCount');
  const count = parseInt(countInput.value);
  
  if (!count || count < 1) {
    showToast('请输入有效的导出数量（至少为1）', 'error');
    return;
  }
  
  if (count > 10000) {
    showToast('单次导出数量不能超过 10000', 'error');
    return;
  }
  
  closeExportCustomModal();

  // 按当前筛选条件导出指定数量，由后端 limit 截断，直接下载
  var url = '/admin/accounts/export?used=false&limit=' + count;
  if (accountStatusFilter) url += '&status=' + accountStatusFilter;
  if (accountSubscriptionFilter) url += '&subscription=' + encodeURIComponent(accountSubscriptionFilter);
  if (accountKeyword) url += '&keyword=' + encodeURIComponent(accountKeyword);

  var dateStr = new Date().toISOString().slice(0, 10);
  try {
    showToast('正在导出，请稍候...', 'info');
    await downloadExport(url, 'accounts_' + dateStr + '.json');
    showToast('导出完成，最多 ' + count + ' 条', 'success');
  } catch (e) {
    showToast(e.message || '导出失败', 'error');
  }
}
