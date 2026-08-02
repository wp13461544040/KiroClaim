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

var xlsxLoadPromise = null;

function ensureXlsxLoaded() {
  if (window.XLSX) return Promise.resolve();
  if (xlsxLoadPromise) return xlsxLoadPromise;

  xlsxLoadPromise = new Promise(function(resolve, reject) {
    var script = document.createElement('script');
    script.src = 'https://cdn.jsdelivr.net/npm/xlsx@0.18.5/dist/xlsx.full.min.js';
    script.async = true;
    script.onload = function() { resolve(); };
    script.onerror = function() { reject(new Error('XLSX 组件加载失败')); };
    document.head.appendChild(script);
  });
  return xlsxLoadPromise;
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
      refreshToken: a.RefreshToken || '',
      accessToken: a.AccessToken || '',
      clientId: a.ClientId || '',
      clientSecret: a.ClientSecret || '',
      provider: a.Provider || '',
      region: a.Region || ''
    };
  });
  var jsonStr = JSON.stringify(jsonData, null, 2);
  downloadFile(jsonStr, 'accounts_' + dateStr + '.json', 'application/json;charset=utf-8');

  showToast(`导出${description}账号成功，共 ${list.length} 条`, 'success');
}

// 导出卡密，支持 txt / csv / xlsx 格式
async function exportCards(format) {
  var url = '/admin/cards';
  var qs = [];
  if (cardStatusFilter) qs.push('status=' + cardStatusFilter);
  if (cardKeyword) qs.push('keyword=' + encodeURIComponent(cardKeyword));
  if (qs.length) url += '?' + qs.join('&');

  var list = await fetchAllPages(url, 500);
  if (!list.length) {
    showToast('没有可导出的数据', 'info');
    return;
  }
  var dateStr = new Date().toISOString().slice(0, 10);

  if (format === 'txt') {
    // 纯文本：每行一个卡密
    var txt = list.map(function(c) { return c.Code; }).join('\n');
    downloadFile(txt, 'cards_' + dateStr + '.txt', 'text/plain;charset=utf-8');
  } else if (format === 'csv') {
    // CSV 格式
    var csv = '卡号,密码,成本单价,账号订阅,账号数量,状态\n';
    list.forEach(function(c) {
      csv += (c.ID || '') + ',' +
        '"' + (c.Code || '') + '",' +
        ',' +
        '"' + (c.Subscription || '') + '",' +
        (c.AccountCount || 1) + ',' +
        (c.Status || '') + '\n';
    });
    downloadFile('\uFEFF' + csv, 'cards_' + dateStr + '.csv', 'text/csv;charset=utf-8');
  } else if (format === 'xlsx') {
    try {
      await ensureXlsxLoaded();
    } catch (e) {
      showToast(e.message || 'XLSX 组件加载失败', 'error');
      return;
    }
    exportCardsXlsx(list, dateStr);
  }
  showToast('导出成功，共 ' + list.length + ' 张', 'success');
}

// 生成卡密 XLSX 文件
function exportCardsXlsx(list, dateStr) {
  var data = [['卡号', '密码', '成本单价', '账号订阅', '账号数量']];
  list.forEach(function(c) {
    data.push([c.ID || '', c.Code || '', '', c.Subscription || '', c.AccountCount || 1]);
  });

  var ws = XLSX.utils.aoa_to_sheet(data);

  // F 列注意事项（红色文字）
  var notes = [
    '注意事项:',
    '1. 表头不要做任何修改!!!',
    '2. 卡密和密码至少填一个，账号订阅按后台卡密配置匹配',
    '3. 成本非必填',
    '4. 最多可导入50000张卡密'
  ];
  for (var i = 0; i < notes.length; i++) {
    var cell = { v: notes[i], t: 's', s: { font: { color: { rgb: 'FF0000' } } } };
    ws[XLSX.utils.encode_cell({ r: i, c: 5 })] = cell;
  }

  // 设置列宽
  ws['!cols'] = [
    { wch: 10 },  // A: 卡号
    { wch: 40 },  // B: 串码
    { wch: 12 },  // C: 成本单价
    { wch: 18 },  // D: 账号订阅
    { wch: 10 },  // E: 账号数量
    { wch: 40 }   // F: 注意事项
  ];

  // 更新工作表范围
  var range = XLSX.utils.decode_range(ws['!ref'] || 'A1');
  if (range.e.c < 5) range.e.c = 5;
  if (range.e.r < notes.length - 1) range.e.r = Math.max(range.e.r, notes.length - 1);
  ws['!ref'] = XLSX.utils.encode_range(range);

  var wb = XLSX.utils.book_new();
  XLSX.utils.book_append_sheet(wb, ws, 'Sheet1');
  XLSX.writeFile(wb, 'cards_' + dateStr + '.xlsx');
}
