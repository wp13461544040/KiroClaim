// API 请求封装
async function api(method, path, body) {
  var opts = { method: method, headers: { 'Content-Type': 'application/json' } };

  // 管理员接口附加 JWT token
  if (ADMIN_TOKEN && path.startsWith('/admin/')) {
    opts.headers['Authorization'] = 'Bearer ' + ADMIN_TOKEN;
  }
  if (body) opts.body = JSON.stringify(body);

  var r = await fetch(path, opts);

  // token 验证失败，跳转登录页
  if ((r.status === 401 || r.status === 403) && path.startsWith('/admin/')) {
    localStorage.removeItem('adminToken');
    ADMIN_TOKEN = null;
    showLogin();
    return { code: 1, message: 'Token 已失效' };
  }

  return parseApiResponse(r);
}

// 解析响应体：服务端正常返回 JSON，但反向代理/网关出错时会返回 HTML 错误页，
// 直接调用 r.json() 只会抛出 "Unexpected token '<'" 这种无法定位问题的报错。
async function parseApiResponse(r) {
  var text = await r.text();
  var ct = (r.headers.get('Content-Type') || '').toLowerCase();

  if (ct.indexOf('json') >= 0 || looksLikeJson(text)) {
    try {
      return JSON.parse(text);
    } catch (e) {
      throw new Error('服务器返回的数据无法解析（HTTP ' + r.status + '）');
    }
  }

  throw new Error(describeNonJsonResponse(r.status, text));
}

function looksLikeJson(text) {
  var s = (text || '').trim();
  return s.charAt(0) === '{' || s.charAt(0) === '[';
}

function describeNonJsonResponse(status, text) {
  if (status === 413) {
    return '请求体过大被网关拒绝（HTTP 413）。请在反向代理（Nginx）中调大 client_max_body_size，或减少单次导入的数据量';
  }
  if (status === 502 || status === 503 || status === 504) {
    return '网关错误（HTTP ' + status + '），后端服务可能未启动或响应超时';
  }
  if (status === 404) {
    return '接口不存在（HTTP 404），请确认后端版本与前端匹配';
  }

  var snippet = (text || '').replace(/<[^>]*>/g, ' ').replace(/\s+/g, ' ').trim().slice(0, 120);
  return '服务器返回了非 JSON 响应（HTTP ' + status + '）' + (snippet ? '：' + snippet : '');
}
