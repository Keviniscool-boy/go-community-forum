# 问见

<div align="center">

_一个让好问题遇见懂行回答者的知识社区_

[![license](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](go.mod)
[![Vue Version](https://img.shields.io/badge/Vue-3.5-4FC08D.svg)](frontend/package.json)
[![CI](https://github.com/Keviniscool-boy/go-community-forum/actions/workflows/ci.yml/badge.svg)](https://github.com/Keviniscool-boy/go-community-forum/actions/workflows/ci.yml)

[项目简介](#项目简介) · [核心能力](#核心能力) · [快速开始](#快速开始) · [技术栈](#技术栈)

</div>

## 项目简介

问见是一个以问题为入口的中文知识社区。用户可以围绕话题提出问题、分享回答、撰写文章与专栏，也可以关注感兴趣的作者和话题，通过推荐、最新和热榜内容持续发现值得讨论的问题。

提问者可以关注问题进展、采纳有帮助的回答；创作者可以管理自己的内容并查看互动。项目还包含审核、举报、通知、搜索与移动端导航等社区能力。

> **项目状态：**本项目仍在持续开发，适合学习和体验知识社区的前后端实现。演示或自托管部署前，请按部署文档配置密钥、邮件服务、数据库备份和审核策略；默认配置不代表生产环境安全基线。

后端基于 Go、Gin 和 MySQL，Redis 用于缓存与互动数据；前端使用 Vue 3、Vite 和 TypeScript。可通过 Docker Compose 启动，也可按下方说明本地运行。

## 核心能力

### 用户

- 邮箱验证码注册（注册成功直接登录）、邮箱密码登录，密码 bcrypt 加密存储
- 验证码存 Redis（5 分钟有效），同邮箱 60 秒限发、单 IP 每小时上限
- JWT 无状态鉴权，登录后可访问受保护接口
- 用户主页：资料卡、提问/回答列表、关注者统计

### 问答与互动

- 问题（标题 + 补充说明 + 话题）与回答
- 回答投票（赞同/反对/取消，幂等）、收藏、评论
- 评论为两级楼中楼：一级评论 + 其下回复，回复可以再回复另一条回复（展示「回复 @xxx」）；问题与每条回答各有独立评论区
- 评论点赞（幂等）与删除（作者或管理员/审核员；软删除，其下回复保留并以「该评论已删除」占位）
- 关注用户、关注问题、关注话题
- 互动产生站内通知，支持未读数与全部已读；回复、评论点赞会通知被回复人 / 评论作者，回答类通知可跳转到所属问题并定位到该回答

### Feed

- `/feed/recommend | /feed/latest | /feed/following | /feed/hot`，统一 cursor 游标分页（opaque cursor，前端不解析）
- 推荐分 = 互动分（赞同×2 + 评论×3 + 收藏×4）+ 新鲜度 ×100，关注作者额外 +20
- 热榜与推荐走 Redis 缓存（TTL 10 分钟），最新与关注流走 MySQL 时间游标

### 话题

- 话题列表、话题详情与话题下的问题
- 提问时可关联话题

### 治理与安全（第三阶段）

- 内容治理状态机：moderation_status（pending/approved/rejected/quarantined）+ visibility + 软删除，统一公开可见条件覆盖 Feed/详情/搜索/评论
- 发布审核流：敏感词检查（词表存 DB + 缓存版本失效），拒绝词进人工队列、普通词隔离
- 举报：同用户同目标幂等 + 每小时限流；管理员审核队列（乐观锁防并发覆盖，举报成立内容隔离下架）
- 角色权限：user/moderator/admin，角色每次查库即时生效；管理员初始账号通过 admin.user_ids 配置
- 审计日志：只追加，记录管理员动作与 request_id

### 搜索

- 统一搜索（问题/回答/用户/话题），scope/sort/cursor 分页；只返回公开可见内容
- SearchIndex 接口抽象：开发环境 MySQL fallback，生产可接 Elasticsearch 适配器

### 媒体与编辑

- 图片上传：类型白名单（魔数检测，禁 SVG）、5/10MB 限制、解码炸弹防护、sha256 去重、media_asset 资产表
- 安全 Markdown 子集：服务端清洗（白名单标签、https URL），原始 HTML/脚本不可能输出
- 草稿箱：自动保存带版本乐观锁，发布重新过审核流

### 产品化

- 收藏夹（默认"我的收藏"）、浏览历史（可清空）、目标删除显示"内容已不可用"
- 推荐事件埋点（曝光/点击/停留等，client event_id 幂等）与推荐规则版本
- Request ID 贯穿日志与审计；/healthz /readyz 与 /health/live /health/ready 就绪探针

### 社区

- 社区列表与详情查询

### 帖子

- 发布帖子、查看帖子详情
- 分页列表，支持按发布时间 / 热度排序

### 投票与排行

- 基于 Redis 的帖子投票：赞同（+1）/ 反对（-1）/ 取消（0）
- 发帖一周内可投票，过期自动锁定
- 热度算法：分数 = 发帖时间 + 票数 × 432（约 200 票可抵消一天的帖子年龄）

### 工程化

- Swagger 接口文档：`/swagger/index.html`
- Docker Compose 一键部署（MySQL + Redis + 后端 + 前端）
- 优雅关机、配置文件热更新、接口限流、pprof 性能分析

## 技术栈

| 层     | 技术                                        |
| ------ | ------------------------------------------- |
| 后端   | Go 1.25 · Gin · sqlx · go-redis/v9 · Viper · Zap |
| 数据   | MySQL 8.0 · Redis 7                         |
| 鉴权/ID | JWT · Snowflake 分布式 ID                   |
| 前端   | Vue 3 · Vite · TypeScript · Vue Router · Pinia · Element Plus |
| 工程   | Docker Compose · Swagger · GitHub Actions   |

## 快速开始

### 方式一：Docker Compose（推荐，想快速体验用这个）

前置要求：Docker Desktop（含 Docker Compose）、Git。

1. 创建本地环境变量文件：

   ```powershell
   Copy-Item .env.example .env
   ```

2. 编辑 `.env`，填入 3 个随机值（这些是密码，**不要**提交到仓库）：

   ```text
   MYSQL_ROOT_PASSWORD=一段随机字符串
   MYSQL_PASSWORD=另一段随机字符串
   JWT_SECRET=至少 32 位的随机字符串
   ```

   可以用在线密码生成器，或在 PowerShell 里生成：

   ```powershell
   -join ((48..57)+(65..90)+(97..122) | Get-Random -Count 32 | ForEach-Object { [char]$_ })
   ```

3. 构建并启动全部服务（MySQL 首次启动会自动执行 `models/create_table.sql` 建表并写入示例社区）：

   ```powershell
   docker compose up --build -d
   ```

4. 查看服务状态：

   ```powershell
   docker compose ps
   ```

启动后访问：

| 服务          | 地址                                    |
| ------------- | --------------------------------------- |
| Web 应用      | http://localhost:8080                   |
| 后端 API      | http://localhost:8888                   |
| Swagger 文档  | http://localhost:8888/swagger/index.html |

停止服务：`docker compose down`。
**警告**：`docker compose down -v` 会删除 `mysql_data` / `redis_data` 数据卷，数据库将清空恢复到初始状态，仅在确定不要数据时使用。

### 数据库迁移与数据持久化

- **版本化迁移**：服务启动时自动执行 `models/migrations/` 迁移链（版本记录在 `schema_migrations` 表，`GET_LOCK` 防多实例并发）。空库、Compose 初始化库（仅基线）、旧生产库都走同一条链。
- **旧库升级**：旧用户的 NULL 邮箱自动清洗为 `legacy_<user_id>@placeholder.invalid`（RFC 2606 保留 TLD，不可作为登录邮箱），`email_verified` 保持 0。
- **存量评论升级（008）**：楼中楼统一为两级结构——历史数据里直接挂在「回复」下的评论会自动上提到所属一级评论下，并记录被回复人；`reply_count` 按升级后的回复关系回填，点赞数从 0 起算。
- **关闭自动迁移**：配置 `mysql.auto_migrate: false`（或环境变量 `MYSQL_AUTO_MIGRATE=false`）后启动不再执行迁移，需要人工运行 SQL。
- **备份**：`docker exec go-community-forum-mysql8019-1 mysqldump -uroot -p$MYSQL_ROOT_PASSWORD bluebell > backup.sql`。
- **恢复**：先建空库再导入备份，服务启动时迁移链会补齐缺失的结构变更。
- **重复执行安全**：迁移按版本记录跳过已应用项，服务重启不会重复执行。

### 管理后台

- 管理员初始账号：`.env` 配置 `ADMIN_USER_IDS=<雪花ID>`（逗号分隔），启动时同步角色
- 管理后台入口 `/admin`（侧栏仅管理员/审核员可见）：指标总览、举报队列、内容审核、用户禁用/启用、审计日志

### 旧接口兼容说明

旧的用户名注册/登录接口（`/signup`、`/login`）在过渡期保留：旧用户注册时邮箱为占位地址（未验证），需要重置邮箱才能使用邮箱登录。计划在邮箱绑定流程上线后废弃旧接口。

### 方式二：本地开发

需要：Go 1.25+、MySQL 8、Redis 7、Node.js。

1. **准备数据库**
   - 启动本地 MySQL，创建一个名为 `bluebell` 的数据库
   - 导入表结构与示例社区数据（新装环境可一次建齐全部表；存量环境请按顺序执行 `models/migrations/` 下的迁移）：

     ```powershell
     mysql -uroot -p bluebell -e "source models/create_table.sql"
     ```

   - 启动本地 Redis（默认端口 6379 即可）

2. **填写配置**
   - 复制 `settings/config.example.yaml` 为 `settings/config.yaml`
   - 填入你的 MySQL 连接信息，以及一个至少 32 位的 `jwt.secret`

3. **启动后端**（默认监听 8081 端口）：

   ```powershell
   go run .
   ```

4. **启动前端**（开发服务器会把 `/api/v1` 请求代理到本地后端）：

   ```powershell
   Set-Location frontend
   npm ci
   npm run serve
   ```

   > 💡 如果前端页面请求报网络错误，先检查 `frontend/vue.config.js` 里代理的 `target` 端口是否与后端实际端口一致。

## API 接口一览

> 全部接口前缀为 `/api/v1`，🔒 表示需要请求头携带 `Authorization: Bearer <token>`，🔓 表示可选登录（登录后返回个性化状态）

### 评论

| 方法 | 路径 | 说明 | 鉴权 |
| --- | --- | --- | --- |
| GET | `/comments?target_type=&target_id=&cursor=&limit=` | 一级评论列表（最新在前；返回点赞状态、回复数、是否可删除） | 🔓 |
| GET | `/comments/:id/replies?cursor=&limit=` | 楼中楼回复列表（最早在前） | 🔓 |
| POST | `/comments` | 发表评论 / 回复（`parent_id` 为被回复的评论，可是一级评论或其下回复） | 🔒 |
| POST | `/comments/:id/like` | 点赞评论（幂等） | 🔒 |
| DELETE | `/comments/:id/like` | 取消点赞 | 🔒 |
| DELETE | `/comments/:id` | 删除评论（作者或管理员/审核员；其下回复保留） | 🔒 |

### 旧接口（过渡期保留，新前端已不再使用）

| 方法 | 路径                                    | 说明                                                         | 鉴权 |
| ---- | --------------------------------------- | ------------------------------------------------------------ | ---- |
| POST | `/signup`                               | 用户注册                                                     |      |
| POST | `/login`                                | 用户登录                                                     |      |
| GET  | `/ping`                                 | 连通性测试                                                   | 🔒   |
| GET  | `/community`                            | 社区列表                                                     |      |
| GET  | `/community/:id`                        | 社区详情                                                     |      |
| POST | `/post`                                 | 发布帖子                                                     | 🔒   |
| GET  | `/post/:id`                             | 帖子详情                                                     |      |
| GET  | `/posts/?page=&size=`                   | 帖子列表（分页）                                             |      |
| GET  | `/posts2/?order=time\|score&page=&size=` | 帖子列表（按时间 / 热度排序）                                |      |
| POST | `/vote`                                 | 帖子投票（direction：1 赞同 / -1 反对 / 0 取消）             | 🔒   |

完整的请求 / 响应示例见 Swagger 文档。

## 项目结构

```text
bluebell/
├── controller/   # HTTP 参数解析与统一响应
├── dao/          # 数据访问层（mysql / redis）
├── logic/        # 业务逻辑层
├── middlewares/  # JWT 鉴权、接口限流
├── models/       # 数据模型与建表 SQL
├── pkg/          # 通用包（JWT、Snowflake）
├── routes/       # Gin 路由注册
├── settings/     # 配置加载与示例配置
├── logger/       # Zap 日志封装
├── docs/         # Swagger 自动生成文件
├── frontend/     # Vue 3 + Vite + TS 前端
├── Dockerfile    # 后端镜像
├── docker-compose.yml
└── main.go       # 程序入口
```

## 质量检查

```powershell
go test ./...
go vet ./...

Set-Location frontend
npm ci
npm run lint
npm run build
```

## 参与贡献

从 Issue 建立上下文 → 在独立分支完成修改 → 通过 Pull Request 合并到 `main`。

## 许可证

本项目基于 [Apache License 2.0](LICENSE) 开源。
