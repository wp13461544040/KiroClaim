# ✅ KiroClaim 双环境自动部署配置完成

## 📦 已完成的工作

### 1. GitHub Actions Workflow
创建了 `.github/workflows/deploy-dual-env.yml`，实现：
- ✅ 当 Docker 镜像构建成功后自动部署到两个环境
- ✅ 国内环境：111.229.254.24:/home/ubuntu/kiroclaim/
- ✅ US 环境：64.83.17.68:~/kiroclaim
- ✅ 支持手动选择部署环境（both/china/us）
- ✅ 串行部署避免冲突，带部署结果通知

### 2. 配置文档
创建了 `docs/GITHUB_ACTIONS_DEPLOY.md`，包含：
- ✅ GitHub Secrets 详细配置步骤
- ✅ 自动和手动部署说明
- ✅ SSH 密钥认证升级指南（安全建议）
- ✅ 故障排查和监控方案
- ✅ 维护清单和最佳实践

---

## 🚀 接下来需要做的事

### 第一步：推送代码到 GitHub

```bash
cd ~/Desktop/KiroClaim  # 或你的项目路径

# 代码已经提交到本地 Git
# 你只需要推送到远程仓库
git push origin main
```

**注意**: 由于网络连接问题，代码还没有推送到 GitHub。请在网络恢复后执行上述命令。

### 第二步：配置 GitHub Secrets

推送成功后，前往 GitHub 配置 8 个必需的 Secrets：

**访问链接**: https://github.com/wp13461544040/KiroClaim/settings/secrets/actions

#### 国内环境（4 个）
```
DEPLOY_CHINA_HOST = 111.229.254.24
DEPLOY_CHINA_USER = root
DEPLOY_CHINA_PASSWORD = Wp156174225.
DEPLOY_CHINA_PORT = 22
```

#### US 环境（4 个）
```
DEPLOY_US_HOST = 64.83.17.68
DEPLOY_US_USER = root
DEPLOY_US_PASSWORD = Wp156174225.
DEPLOY_US_PORT = 22
```

**快速配置方法（使用 GitHub CLI）**:
```bash
# 确保已登录
gh auth login

# 配置所有 Secrets
gh secret set DEPLOY_CHINA_HOST --body "111.229.254.24"
gh secret set DEPLOY_CHINA_USER --body "root"
gh secret set DEPLOY_CHINA_PASSWORD --body "Wp156174225."
gh secret set DEPLOY_CHINA_PORT --body "22"
gh secret set DEPLOY_US_HOST --body "64.83.17.68"
gh secret set DEPLOY_US_USER --body "root"
gh secret set DEPLOY_US_PASSWORD --body "Wp156174225."
gh secret set DEPLOY_US_PORT --body "22"

# 验证配置
gh secret list
```

### 第三步：测试部署

#### 方法一：自动部署测试
推送任意代码到 `main` 分支，会触发：
1. Docker 镜像构建
2. 镜像构建成功后自动部署到两个环境

```bash
# 测试推送
git commit --allow-empty -m "test: 测试自动部署"
git push origin main
```

#### 方法二：手动部署测试
1. 访问: https://github.com/wp13461544040/KiroClaim/actions
2. 选择 **Deploy to Dual Environments**
3. 点击 **Run workflow**
4. 选择环境：`both`（测试两个环境）
5. 点击 **Run workflow**

或使用命令行：
```bash
gh workflow run deploy-dual-env.yml -f environment=both
```

### 第四步：验证部署

查看 GitHub Actions 日志：
```bash
# 查看最近的运行
gh run list --workflow=deploy-dual-env.yml

# 查看详细日志
gh run view --log
```

登录服务器检查：
```bash
# 国内服务器
ssh root@111.229.254.24
cd /home/ubuntu/kiroclaim/
docker ps | grep kiroclaim
docker logs kiroclaim --tail 50

# US 服务器
ssh root@64.83.17.68
cd ~/kiroclaim
docker ps | grep kiroclaim
docker logs kiroclaim --tail 50
```

浏览器访问：
- 国内: http://111.229.254.24:9527
- US: http://64.83.17.68:9527

---

## 📋 部署流程图

```
代码推送到 main 分支
        ↓
  触发 Docker workflow
        ↓
   构建 Docker 镜像
        ↓
  上传到 GHCR (ghcr.io)
        ↓
  ✅ 构建成功
        ↓
 自动触发部署 workflow
        ↓
    ┌─────────┬─────────┐
    ↓         ↓         ↓
国内环境   US环境   部署通知
    ↓         ↓         ↓
  更新完成   更新完成   结果汇总
```

---

## 🔐 安全建议（重要）

当前配置使用 **SSH 密码认证**，建议升级到 **SSH 密钥认证**：

### 升级步骤
1. 生成 SSH 密钥对
2. 添加公钥到两个服务器
3. 测试密钥登录
4. 更新 GitHub Secrets（删除 PASSWORD，添加 KEY）
5. 修改 workflow 文件（password → key）
6. 服务器上禁用密码登录

**详细步骤**: 查看 `docs/GITHUB_ACTIONS_DEPLOY.md` 的"安全建议"章节

---

## 📖 完整文档

- **部署配置文档**: [docs/GITHUB_ACTIONS_DEPLOY.md](docs/GITHUB_ACTIONS_DEPLOY.md)
  - GitHub Secrets 配置步骤
  - 部署方式说明
  - SSH 密钥认证升级
  - 故障排查
  - 监控和维护

- **服务器部署文档**: [DEPLOYMENT.md](DEPLOYMENT.md)
  - 服务器环境准备
  - Docker 安装配置
  - 手动部署步骤
  - 日常运维命令

- **Workflow 配置**: [.github/workflows/deploy-dual-env.yml](.github/workflows/deploy-dual-env.yml)
  - 自动部署逻辑
  - 部署脚本详情

---

## ✅ 配置检查清单

部署前请确认：

- [ ] 代码已推送到 GitHub (`git push origin main`)
- [ ] 8 个 GitHub Secrets 已配置完成
- [ ] 两个服务器都已安装 Docker 和 Docker Compose
- [ ] 两个服务器的 `docker-compose.yml` 和 `.env` 已配置
- [ ] 服务器防火墙已开放 SSH 端口 (22)
- [ ] 已测试手动 SSH 登录两个服务器
- [ ] 已手动拉取过一次镜像测试网络连接

首次部署建议：
- [ ] 先手动触发部署测试单个环境
- [ ] 验证成功后再测试自动部署
- [ ] 观察 GitHub Actions 日志确认无报错
- [ ] 登录服务器验证容器运行正常

---

## 🎯 快速命令参考

```bash
# 推送代码（首次需要）
git push origin main

# 配置 Secrets（使用 GitHub CLI）
gh secret set DEPLOY_CHINA_HOST --body "111.229.254.24"
gh secret set DEPLOY_CHINA_USER --body "root"
gh secret set DEPLOY_CHINA_PASSWORD --body "Wp156174225."
gh secret set DEPLOY_CHINA_PORT --body "22"
gh secret set DEPLOY_US_HOST --body "64.83.17.68"
gh secret set DEPLOY_US_USER --body "root"
gh secret set DEPLOY_US_PASSWORD --body "Wp156174225."
gh secret set DEPLOY_US_PORT --body "22"

# 验证 Secrets
gh secret list

# 手动触发部署
gh workflow run deploy-dual-env.yml -f environment=both

# 查看部署状态
gh run list --workflow=deploy-dual-env.yml
gh run view --log

# 测试服务器连接
ssh root@111.229.254.24 "cd /home/ubuntu/kiroclaim && docker ps"
ssh root@64.83.17.68 "cd ~/kiroclaim && docker ps"
```

---

## 📞 遇到问题？

1. 查看 `docs/GITHUB_ACTIONS_DEPLOY.md` 的"故障排查"章节
2. 检查 GitHub Actions 日志找出具体错误
3. 登录服务器查看 Docker 日志
4. 确认网络连接和防火墙配置
5. 验证 GitHub Secrets 配置是否正确

---

**🎉 配置已完成，等待你推送代码并测试！**

**下一步**: 
1. 在网络恢复后执行 `git push origin main`
2. 配置 GitHub Secrets
3. 手动触发一次部署测试

**祝部署顺利！** 🚀
