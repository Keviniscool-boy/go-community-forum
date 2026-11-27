# 问见

> 一个让好问题遇见懂行回答者的知识社区。

[![license](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](go.mod)
[![Vue](https://img.shields.io/badge/Vue-3.5-4FC08D.svg)](frontend/package.json)
[![CI](https://github.com/Keviniscool-boy/go-community-forum/actions/workflows/ci.yml/badge.svg)](https://github.com/Keviniscool-boy/go-community-forum/actions/workflows/ci.yml)

问见是一个中文问答社区，支持提问与回答、话题发现、内容互动和社区治理。后端使用 Go、Gin、MySQL 和 Redis，前端使用 Vue 3、Vite 和 TypeScript。

## 功能

- **问答**：发布问题和回答，编辑或删除自己的内容。
- **发现**：推荐、最新、关注和热榜 Feed；按问题、回答、用户和话题搜索。
- **互动**：关注用户、问题和话题；回答投票、收藏；评论支持楼中楼回复、点赞和删除。
- **创作**：草稿箱、图片上传、安全 Markdown 预览、收藏夹和浏览历史。
- **社区治理**：举报、内容审核、角色权限、审计日志和管理指标。
- **兼容接口**：保留早期帖子与社区 API，供旧客户端过渡使用。

## 快速开始

需要 Docker Desktop 和 Docker Compose。

1. 创建本地环境文件：

   ```powershell
   Copy-Item .env.example .env
   ```

2. 在 `.env` 中设置 `MYSQL_ROOT_PASSWORD`、`MYSQL_PASSWORD` 和至少 32 位的 `JWT_SECRET`。邮件验证码需要发送时，再配置 SMTP 变量。

3. 构建并启动：

   ```powershell
   docker compose up --build -d
   ```

访问前端 `http://localhost:8080`，API `http://localhost:8888`。停止服务运行 `docker compose down`。执行 `docker compose down -v` 会删除数据库和 Redis 数据卷。

## 本地开发

需要 Go 1.25+、MySQL 8、Redis 7 和 Node.js。创建 `bluebell` 数据库并导入基线结构：

```sql
CREATE DATABASE bluebell;
```

```powershell
mysql -uroot -p bluebell -e "source models/create_table.sql"
```

启动 Redis，将 `settings/config.example.yaml` 复制为 `settings/config.yaml` 并填写连接信息，再分别启动后端和前端：

```powershell
go run .
```

```powershell
Set-Location frontend
npm ci
npm run dev
```

开发服务器默认运行在 `http://localhost:8080`，并将 `/api` 请求代理至本地后端 `http://127.0.0.1:8081`。

## API 与配置

- Swagger UI：Compose 部署访问 `http://localhost:8888/swagger/index.html`；本地运行访问 `http://localhost:8081/swagger/index.html`。
- API 前缀：`/api/v1`。
- 本地配置示例：[`settings/config.example.yaml`](settings/config.example.yaml)
- Compose 环境变量示例：[`.env.example`](.env.example)
- 活跃产品规格：[`docs/product/unanswered-question-answer-invitation-prd.md`](docs/product/unanswered-question-answer-invitation-prd.md)

## 技术栈

| 部分 | 技术 |
| --- | --- |
| 后端 | Go、Gin、sqlx、MySQL、Redis、Viper、Zap |
| 前端 | Vue 3、Vite、TypeScript、Vue Router、Pinia、Element Plus |
| 工程 | Docker Compose、Swagger、GitHub Actions |

## 质量检查

```powershell
go test ./...
go vet ./...
Set-Location frontend
npm ci
npm run lint
npm run build
```

## 许可证

[Apache License 2.0](LICENSE)
