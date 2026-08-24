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

// 导出账号，仅支持 JSON 格式
// exportType: 'selected' 导出选中账号, 'all' 导出全部筛选结果
async function exportAccounts(format, exportType) {
  // 只支持 JSON 格式
  if (format !== 'json') {
    showToast('仅支持 JSON 格式导出', 'error');
    return;
  }

  let list = [];
  let description = '';

  if (exportType === 'selected') {
    // 导出选中的账号
    if (selectedAccountIds.size === 0) {
      showToast('请先选择要导出的账号', 'info');
      return;
    }
    
    // 构造查询 URL，使用 IDs 筛选
    const ids = Array.from(selectedAccountIds);
    const url = '/admin/accounts?used=false&size=1000';
    const r = await api('GET', url);
    
    if (r.code === 0 && r.data && r.data.list) {
      // 过滤出选中的账号
      list = r.data.list.filter(a => ids.includes(a.ID));
    }
    
    description = '选中的';
  } else {
    // 导出全部账号（按筛选条件）
    var url = '/admin/accounts?used=false';
    if (accountStatusFilter) url += '&status=' + accountStatusFilter;
    if (accountSubscriptionFilter) url += '&subscription=' + encodeURIComponent(accountSubscriptionFilter);
    if (accountKeyword) url += '&keyword=' + encodeURIComponent(accountKeyword);

    list = await fetchAllPages(url, 500);
    description = '全部';
  }

  if (!list.length) {
    showToast('没有可导出的数据', 'info');
    return;
  }

  var dateStr = new Date().toISOString().slice(0, 10);

  // 导出为 JSON 格式（与导入格式保持一致）
  var jsonData = list.map(function(a) {
    return {
      accessToken: a.AccessToken || '',
      refreshToken: a.RefreshToken || '',
      clientId: a.ClientId || '',
      clientSecret: a.ClientSecret || ''
    };
  });
  var jsonStr = JSON.stringify(jsonData, null, 2);
  downloadFile(jsonStr, 'accounts_' + dateStr + '.json', 'application/json;charset=utf-8');

  showToast(`导出${description}账号成功，共 ${list.length} 条`, 'success');
}

// 导出已分配账号，仅支持 JSON 格式
// exportType: 'selected' 导出选中账号, 'all' 导出全部筛选结果
async function exportAssignedAccounts(format, exportType) {
  // 只支持 JSON 格式
  if (format !== 'json') {
    showToast('仅支持 JSON 格式导出', 'error');
    return;
  }

  let list = [];
  let description = '';

  if (exportType === 'selected') {
    // 导出选中的已分配账号
    if (selectedAssignedIds.size === 0) {
      showToast('请先选择要导出的账号', 'info');
      return;
    }
    
    // 构造查询 URL
    const ids = Array.from(selectedAssignedIds);
    const url = '/admin/accounts?used=true&size=1000';
    const r = await api('GET', url);
    
    if (r.code === 0 && r.data && r.data.list) {
      // 过滤出选中的账号
      list = r.data.list.filter(a => ids.includes(a.ID));
    }
    
    description = '选中的';
  } else {
    // 导出全部已分配账号（按筛选条件）
    var url = '/admin/accounts?used=true';
    if (assignedStatusFilter) url += '&status=' + assignedStatusFilter;
    if (assignedKeyword) url += '&keyword=' + encodeURIComponent(assignedKeyword);

    list = await fetchAllPages(url, 500);
    description = '全部';
  }

  if (!list.length) {
    showToast('没有可导出的数据', 'info');
    return;
  }

  var dateStr = new Date().toISOString().slice(0, 10);

  // 导出为 JSON 格式（与未分配账号格式一致）
  var jsonData = list.map(function(a) {
    return {
      accessToken: a.AccessToken || '',
      refreshToken: a.RefreshToken || '',
      clientId: a.ClientId || '',
      clientSecret: a.ClientSecret || ''
    };
  });
  var jsonStr = JSON.stringify(jsonData, null, 2);
  downloadFile(jsonStr, 'assigned_accounts_' + dateStr + '.json', 'application/json;charset=utf-8');

  showToast(`导出${description}已分配账号成功，共 ${list.length} 条`, 'success');
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
