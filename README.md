<p align="center">
  <img src="static/apple-touch-icon.png" width="96" height="96" alt="KiroClaim 图标">
</p>

<h1 align="center">KiroClaim</h1>

<p align="center">一个轻量的 Kiro 账号发卡、兑换与管理系统。</p>

<p align="center">
  <img alt="GitHub Tag" src="https://img.shields.io/github/v/tag/wp13461544040/KiroClaim?label=tag&sort=semver">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white">
  <img alt="Gin" src="https://img.shields.io/badge/Gin-HTTP-00ADD8">
  <img alt="SQLite" src="https://img.shields.io/badge/SQLite-Dev-003B57?logo=sqlite&logoColor=white">
  <img alt="MySQL" src="https://img.shields.io/badge/MySQL-Prod-4479A1?logo=mysql&logoColor=white">
  <img alt="Docker" src="https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker&logoColor=white">
</p>

## 项目简介

KiroClaim 面向 Kiro 账号发卡兑换场景，提供账号导入、上游健康检查、卡密生成、客户端兑换、管理员初始化、系统配置和日志记录等功能。

本地开发默认使用 SQLite，Docker 生产部署默认使用容器内 MySQL。管理员账号、JWT 密钥和账号凭证加密密钥都通过初始化流程生成。

## 功能特性

- 首次启动引导创建管理员账号
- 本地环境检测与初始化确认
- Kiro 账号导入、刷新 token、用量检查、模型列表检查
- 导入与发货时进行上游健康检查，403 判定为封禁
- 卡密生成与兑换，支持 SSE 流式发货
- 管理后台设置项写入 KV 表
- SQLite 开发环境，MySQL 生产环境
- 文件日志与日志切片配置
- Docker / Docker Compose 部署
- 版本检测、手动 Docker 更新与自动更新

## 一键部署


```bash
mkdir -p kiroclaim && cd kiroclaim && curl -fsSL https://raw.githubusercontent.com/wp13461544040/KiroClaim/main/docker-compose.yml
 -o docker-compose.yml && { echo "PORT=9527"; echo "MYSQL_DATABASE=kiroclaim"; echo "MYSQL_USER=kiroclaim"; echo "MYSQL_PASSWORD=$(openssl rand -hex 24)"; echo "MYSQL_ROOT_PASSWORD=$(openssl rand -hex 24)"; } > .env && docker compose up -d
```

启动后访问：

- 初始化页面：`http://服务器IP:9527/setup`
- 兑换中心：`http://服务器IP:9527/redeem`

默认镜像：

```text
ghcr.io/wp13461544040/KiroClaim:latest
```

查看服务：

```bash
docker compose ps
docker compose logs -f kiroclaim
```

停止服务：

```bash
docker compose down
```

完整删除数据：

```bash
docker compose down -v
```

## Docker Compose 说明

`docker-compose.yml` 包含两个容器：

- `kiroclaim`：应用服务
- `kiroclaim-mysql`：MySQL 8.4 数据库

数据卷：

- `mysql-data`：MySQL 数据
- `kiroclaim-logs`：应用日志

后台内置版本检测。若要使用后台里的"更新到最新 Docker"和"系统自动更新"，Compose 部署会默认挂载宿主 Docker socket；这等同于授予容器管理宿主 Docker 的权限，请只在可信服务器上开启自动更新。

## 本地开发

```bash
cp .env.example .env
go mod download
go run .
```

本地开发默认使用 SQLite：

```env
DB_TYPE=sqlite
DB_PATH=app.db
```

## 二进制资源包

每个 `v*` 版本 tag 会自动发布 GitHub Release，并附带：

- `KiroClaim_<version>_linux_amd64.tar.gz`
- `KiroClaim_<version>_linux_arm64.tar.gz`
- `KiroClaim_<version>_windows_amd64.zip`
- `KiroClaim_<version>_docker.zip`

二进制包内包含运行所需的 `static` 静态资源和 `.env.example`。

## 配置说明

| 配置项 | 默认值 | 说明 |
| --- | --- | --- |
| `PORT` | `9527` | HTTP 监听端口 |
| `DB_TYPE` | `sqlite` | 本地开发数据库类型，支持 `sqlite` / `mysql` |
| `DB_PATH` | `app.db` | SQLite 数据库文件路径 |
| `DB_DSN` | 空 | 外部 MySQL DSN，`DB_TYPE=mysql` 时使用 |
| `MYSQL_DATABASE` | `kiroclaim` | Compose 内置 MySQL 数据库名 |
| `MYSQL_USER` | `kiroclaim` | Compose 内置 MySQL 用户 |
| `MYSQL_PASSWORD` | 自动生成 | Compose 内置 MySQL 用户密码 |
| `MYSQL_ROOT_PASSWORD` | 自动生成 | Compose 内置 MySQL root 密码 |
| `METRICS_USER` | 空 | Prometheus 指标接口 Basic Auth 用户名 |
| `METRICS_PASS` | 空 | Prometheus 指标接口 Basic Auth 密码 |

运行期配置，例如请求限流、上游检查并发、日志切片、人机验证、自动更新等，通过管理后台写入 KV 表。

## 推送到 GitHub

```bash
git init
git remote add origin https://github.com/wp13461544040/KiroClaim.git
git add .
git commit -m "Initial commit"
git branch -M main
git push -u origin main
```

`.gitignore` 已忽略本地运行文件，包括 `.env`、SQLite 数据库、WAL 文件、日志目录和构建产物。