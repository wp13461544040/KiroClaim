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

  return r.json();
}
