// 操作日志模块

var logActionFilter = '';

// 操作类型中文映射
var actionLabels = {
  import: '导入',
  delete: '删除',
  generate: '生成卡密',
  activate: '激活',
  clear: '清空',
  refresh: '刷新',
  logout: '登出',
  settings: '设置',
  export: '导出'
};

// 加载日志列表
async function loadLogs(page) {
  if (!page) page = 1;
  var url = '/admin/oplogs?page=' + page + '&size=20';
  if (logActionFilter) url += '&action=' + logActionFilter;

  var r = await api('GET', url);
  var tbody = document.getElementById('logsBody');
  if (r.code !== 0 || !r.data.list || !r.data.list.length) {
    tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:#999;padding:40px">暂无日志记录</td></tr>';
    return;
  }
  tbody.innerHTML = r.data.list.map(function(log) {
    var label = actionLabels[log.Action] || log.Action;
    var ip = log.ClientIP || '-';
    var ua = log.UserAgent || '';
    var ipTooltip = ua ? ' title="' + ua.replace(/"/g, '&quot;') + '"' : '';
    return '<tr>' +
      '<td data-label="ID" style="color:#999">' + log.ID + '</td>' +
      '<td data-label="操作类型">' + label + '</td>' +
      '<td data-label="详情" style="font-size:13px;word-break:break-all">' + (log.Detail || '-') + '</td>' +
      '<td data-label="操作者">' + (log.Operator || '-') + '</td>' +
      '<td data-label="客户端 IP" style="font-size:12px;color:#999;font-family:monospace"' + ipTooltip + '>' + ip + '</td>' +
      '<td data-label="时间" style="color:#999;font-size:12px">' + new Date(log.CreatedAt).toLocaleString('zh-CN', {hour12:false}) + '</td>' +
    '</tr>';
  }).join('');
  renderPagination('logsPagination', r.data.total, 20, page, loadLogs);
}

// 日志类型筛选
function selectLogAction(value, text) {
  logActionFilter = value;
  document.getElementById('logActionText').textContent = text;
  document.querySelectorAll('#logActionDropdown .k-dropdown-item').forEach(function(item) {
    item.classList.remove('selected');
  });
  event.target.classList.add('selected');
  toggleDropdown('logActionDropdown');
  loadLogs(1);
}
