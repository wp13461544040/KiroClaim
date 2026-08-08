# Git Tag发布与Docker镜像构建指南

## 一、当前版本状态检查

```bash
# 查看所有tag
git tag -l

# 查看最新tag
git describe --tags --abbrev=0 2>/dev/null || echo "当前commit未打tag"

# 查看当前分支
git branch --show-current

# 查看commit历史
git log --oneline -10

# 查看远程仓库
git remote -v
```

---

## 二、创建并推送Tag（触发Docker镜像构建）

### 方式1：快速发布（推荐）

```bash
# 1. 确保在main分支且代码已提交
git status

# 2. 添加新文件并提交（如果有未提交的改动）
git add .
git commit -m "docs: add deployment guide"
git push origin main

# 3. 创建新tag（语义化版本号）
git tag v0.2.2

# 4. 推送tag到远程（触发GitHub Actions自动构建镜像）
git push origin v0.2.2

# 5. 验证tag是否推送成功
git ls-remote --tags origin
```

### 方式2：带注释的Tag（推荐生产环境）

```bash
# 创建带注释的tag
git tag -a v0.2.2 -m "Release v0.2.2: Add deployment guide and bug fixes"

# 推送到远程
git push origin v0.2.2
```

### 方式3：一次性推送所有tag

```bash
# 推送所有本地tag
git push origin --tags
```

---

## 三、版本号规范（语义化版本）

### 格式：`vMAJOR.MINOR.PATCH[-PRERELEASE]`

- **MAJOR（主版本号）**: 不兼容的API变更
- **MINOR（次版本号）**: 向下兼容的功能新增
- **PATCH（修订号）**: 向下兼容的bug修复
- **PRERELEASE（预发布版本）**: `-alpha`, `-beta`, `-rc1`

### 示例：

```bash
# 正式版本
v1.0.0    # 首个稳定版
v1.1.0    # 新增功能
v1.1.1    # bug修复
v2.0.0    # 重大更新

# 预发布版本
v1.0.0-alpha    # Alpha测试版
v1.0.0-beta     # Beta测试版
v1.0.0-rc1      # Release Candidate 1
```

### 当前项目版本建议：

```bash
# 已有版本：v0.1.0, v0.1.1, v0.1.2, v0.1.3, v0.2.1-beta

# 推荐下一个版本：
v0.2.2         # 如果是bug修复或小改进
v0.3.0         # 如果新增了重要功能
v1.0.0         # 如果准备发布正式稳定版
```

---

## 四、GitHub Actions自动化流程

### 推送tag后会自动触发：

1. **Docker镜像构建** (`.github/workflows/docker.yml`)
   - 构建多架构镜像（linux/amd64, linux/arm64）
   - 推送到GitHub Container Registry
   - 镜像标签：
     - `ghcr.io/wp13461544040/kiroclaim:latest`
     - `ghcr.io/wp13461544040/kiroclaim:v0.2.2`

2. **Release发布** (`.github/workflows/release.yml`)
   - 编译多平台二进制文件
   - 打包release压缩包
   - 创建GitHub Release
   - 上传附件：
     - `KiroClaim_v0.2.2_linux_amd64.tar.gz`
     - `KiroClaim_v0.2.2_linux_arm64.tar.gz`
     - `KiroClaim_v0.2.2_windows_amd64.zip`
     - `KiroClaim_v0.2.2_docker.zip`

### 查看构建状态：

- GitHub Actions: https://github.com/wp13461544040/KiroClaim/actions
- Container Registry: https://github.com/wp13461544040/KiroClaim/pkgs/container/kiroclaim
- Releases: https://github.com/wp13461544040/KiroClaim/releases

---

## 五、完整发版操作流程

### Step 1: 更新代码版本号（可选）

```bash
# 编辑 utils/version.go
nano utils/version.go

# 修改版本号
var AppVersion = "v0.2.2"
```

### Step 2: 提交所有改动

```bash
# 查看当前状态
git status

# 添加所有改动
git add .

# 提交
git commit -m "release: v0.2.2 - Add deployment documentation"

# 推送到main分支
git push origin main
```

### Step 3: 创建并推送tag

```bash
# 创建tag
git tag -a v0.2.2 -m "Release v0.2.2

## Changes
- Add comprehensive deployment documentation
- Add deployment automation script
- Improve Docker compose configuration
- Bug fixes and performance improvements
"

# 推送tag
git push origin v0.2.2
```

### Step 4: 等待自动构建完成

```bash
# 查看Actions构建状态（在浏览器打开）
# https://github.com/wp13461544040/KiroClaim/actions

# 或使用GitHub CLI查看
gh run list --workflow=docker.yml
gh run list --workflow=release.yml

# 构建完成后验证镜像
docker pull ghcr.io/wp13461544040/kiroclaim:v0.2.2
docker pull ghcr.io/wp13461544040/kiroclaim:latest

# 验证镜像是否是最新的
docker inspect ghcr.io/wp13461544040/kiroclaim:latest | grep Created
```

### Step 5: 验证Release发布

访问：https://github.com/wp13461544040/KiroClaim/releases/tag/v0.2.2

检查是否包含：
- Release Notes
- 二进制压缩包
- Docker compose配置包

---

## 六、Tag管理常用命令

### 查看tag

```bash
# 列出所有tag
git tag

# 列出tag（带通配符）
git tag -l "v0.2.*"

# 查看tag详细信息
git show v0.2.2

# 查看tag对应的commit
git rev-list -n 1 v0.2.2
```

### 删除tag

```bash
# 删除本地tag
git tag -d v0.2.2

# 删除远程tag（慎用！会影响已构建的镜像）
git push origin --delete v0.2.2

# 或者
git push origin :refs/tags/v0.2.2
```

### 给历史commit打tag

```bash
# 查找要打tag的commit hash
git log --oneline

# 给指定commit打tag
git tag v0.2.2 a3a4619

# 推送到远程
git push origin v0.2.2
```

### 修改tag

```bash
# Git不支持直接修改tag，需要删除后重新创建
git tag -d v0.2.2
git tag -a v0.2.2 -m "New message"
git push origin v0.2.2 --force
```

---

## 七、故障排查

### 问题1：tag推送后Actions没有触发

```bash
# 检查tag是否推送成功
git ls-remote --tags origin

# 检查workflows配置
cat .github/workflows/docker.yml | grep -A 5 "on:"

# 手动触发workflow
gh workflow run docker.yml
```

### 问题2：镜像构建失败

```bash
# 查看Actions日志
gh run list --workflow=docker.yml
gh run view <run-id> --log

# 常见原因：
# 1. Dockerfile语法错误
# 2. 依赖下载失败
# 3. 权限不足（GITHUB_TOKEN）
```

### 问题3：无法拉取镜像

```bash
# 检查镜像是否存在
docker manifest inspect ghcr.io/wp13461544040/kiroclaim:v0.2.2

# 登录GitHub Container Registry（私有镜像需要）
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin

# 检查镜像权限设置
# 访问：https://github.com/users/wp13461544040/packages/container/kiroclaim/settings
```

### 问题4：tag与镜像版本不匹配

```bash
# 查看镜像构建时使用的版本
docker inspect ghcr.io/wp13461544040/kiroclaim:v0.2.2 | grep -A 10 Labels

# 查看镜像内应用版本
docker run --rm ghcr.io/wp13461544040/kiroclaim:v0.2.2 ./kiroclaim --version
```

---

## 八、最佳实践

### 1. 发版前检查清单

- [ ] 所有改动已提交并推送到main
- [ ] 更新了CHANGELOG（如果有）
- [ ] 更新了版本号（utils/version.go）
- [ ] 本地测试通过
- [ ] 确定版本号符合语义化规范

### 2. 发版流程

```bash
# 1. 更新代码
git add .
git commit -m "release: prepare for v0.2.2"
git push origin main

# 2. 打tag
git tag -a v0.2.2 -m "Release v0.2.2"
git push origin v0.2.2

# 3. 等待构建（约5-10分钟）
# 4. 验证镜像
docker pull ghcr.io/wp13461544040/kiroclaim:v0.2.2

# 5. 更新生产环境
cd ~/kiroclaim
docker compose pull
docker compose up -d
```

### 3. 版本策略建议

- **开发版本**: 直接push到main，使用`latest`镜像
- **测试版本**: 使用`-beta`, `-alpha`后缀
- **生产版本**: 使用正式版本号，无后缀
- **Hotfix**: 增加PATCH版本号（如v1.0.0 -> v1.0.1）

### 4. 自动化建议

创建发版脚本 `release.sh`：

```bash
#!/bin/bash
set -e

if [ -z "$1" ]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 v0.2.2"
    exit 1
fi

VERSION=$1

echo "🚀 Releasing $VERSION..."

# 更新版本号
sed -i "s/var AppVersion = \".*\"/var AppVersion = \"$VERSION\"/" utils/version.go

# 提交改动
git add .
git commit -m "release: $VERSION"
git push origin main

# 打tag
git tag -a "$VERSION" -m "Release $VERSION"
git push origin "$VERSION"

echo "✅ Tag $VERSION pushed!"
echo "📦 Building Docker images..."
echo "🔍 Check status: https://github.com/wp13461544040/KiroClaim/actions"
```

使用方法：
```bash
chmod +x release.sh
./release.sh v0.2.2
```

---

## 九、快速参考

```bash
# 创建并推送tag（最常用）
git tag v0.2.2
git push origin v0.2.2

# 查看所有tag
git tag -l

# 查看远程tag
git ls-remote --tags origin

# 删除tag
git tag -d v0.2.2              # 删除本地
git push origin --delete v0.2.2  # 删除远程

# 拉取镜像
docker pull ghcr.io/wp13461544040/kiroclaim:latest
docker pull ghcr.io/wp13461544040/kiroclaim:v0.2.2

# 查看镜像版本
docker run --rm ghcr.io/wp13461544040/kiroclaim:latest ./kiroclaim --version
```

---

**文档版本**: v1.0  
**更新日期**: 2026-08-08  
**作者**: SeaGull Security Lab
