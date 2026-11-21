# 基线验收清单

> 验收对象：`frontend-v3` @ `e477dc161e33c0ac7580d07474a3214dd86d9bd9`（干净克隆核对 SHA 一致，工作树零改动起点）
> 验收日期：2026-11-21。验收环境：真实 MySQL（compose mysql8019，端口 33061）+ Redis（redis507，端口 26379），Playwright Chromium。
> 本清单是 `e477dc1` 的可复验证据记录；质量闭环修复在其后的提交中（见 §4），合并后以新 HEAD 为冻结基线。

## 1. 三处验收修复在 `e477dc1` 中的核对

| 项 | 核对方式 | 结果 |
| --- | --- | --- |
| P1-1 主页可见性（user.go 四查询统一公开条件） | `grep moderation_status dao/mysql/user.go` = 6 处 | ✅ 在提交内 |
| P1-2 搜索 keyset 游标（decodeSearchCursor / 确定性排序） | `grep decodeSearchCursor\|item_type ASC pkg/search/mysql.go` = 6 处 | ✅ 在提交内 |
| P2-5 /me/content 专栏类型 | `MeContentColumn` 于 models/content.go:123、mycontent.go:75 | ✅ 在提交内 |

## 2. 自动化验证（全部在干净副本上执行）

| 项 | 命令 | 结果 |
| --- | --- | --- |
| 空库迁移链 000–014 | 新建 `bluebell` 库 + 后端启动 auto_migrate → `/readyz` | ✅ `migration_version: 14` |
| Go 全量测试（-count=1，真实 MySQL/Redis） | `go test ./... -count=1` | ✅ dao/mysql 23.6s、logic 37.2s、pkg/search 14.8s、dao/redis、pkg/sanitizer 全绿 |
| `go vet ./...` | — | ✅ |
| 前端构建（vue-tsc + vite） | `npm run build` | ✅（非阻塞提示：element-plus chunk > 500KB，已知） |
| 浏览器 E2E | `npx playwright test`（BASE_URL=8081，E2E_REDIS_CMD 注入验证码） | ✅ 2/2（详见 §3） |

## 3. E2E 覆盖

1. `register → login → ask → answer → logout`（原有用例）✅
   - 修复：原用例先调 `POST /auth/send-code`，但无 SMTP 时该接口 fail-closed 会**删除**验证码，
     导致从 Redis GET 取到空串——CI 的 e2e 实际从未可能通过。改为按文档约定直接 Redis 注入。
2. `resolution loop → solved badge → search → my content`（本轮新增）✅
   - 双用户 API 注册 → A 提问 → B 回答 → A 采纳（响应 `status=solved`）
   - 游客 375px UI：问题页「已解决」标识 + 回答「✓ 已采纳」可见
   - 游客搜索：结果带「已解决」标注
   - 作者 UI 登录 → `/me/content?type=question` 命中且状态公开

## 4. 质量闭环发现与修复（本清单验收过程中发现，已提交）

### 4.1 可见性统一性审计矩阵

| 入口 | 审核状态 | 可见性 | 删除 | 作者状态 | 父对象 | 结果 |
| --- | --- | --- | --- | --- | --- | --- |
| 问题详情（公开） | ✅ | ✅ | ✅ | ✅(JOIN user) | — | ✅ 原有 |
| 回答列表（问题详情页） | ❌→✅ | ❌→✅ | ❌→✅ | （父问题可见） | ✅ | 🔧 已修复 |
| 单条回答（互动入口） | ❌→✅ | ❌→✅ | ❌→✅ | — | — | 🔧 已修复 |
| 评论列表 | ❌→✅ | n/a | n/a | — | — | 🔧 已修复 |
| 用户主页问题/回答 + 计数 | ✅(e477dc1) | ✅ | ✅ | ✅ | 回答含父问题 ✅ | ✅ 上轮修复 |
| 用户主页文章/专栏 | ✅ | ✅ | ✅ | ✅ | — | ✅ |
| Feed（三段 UNION） | ✅ | ✅ | ✅ | ✅ | — | ✅ 原有 |
| 搜索（全 scope） | ✅ | ✅ | ✅ | ✅ | 回答含父问题 ✅ | ✅ |
| 公开专栏列表 / 专栏文章（读者视图） | ✅ | ✅ | ✅ | ✅ | — | ✅ 原有 |
| 收藏/历史列表 | 只存 (type,id)，展示经 `TargetExists` 完整条件标注"不可用" | | | | | ✅ 设计正确 |

### 4.2 修复内容

1. **回答可见性泄露**（问题详情页回答列表 / 单条回答 / 评论列表只查 `status=1`）——补齐统一条件；
   隔离回答的互动链路（投票/收藏/评论）随之 fail-closed。
2. **派生计数口径**——`CountAnswersByQuestion` 与列表口径一致；审核状态变化后重算
   `RecountAnswerCountByAnswer`；`SoftDeleteAnswer` 重算同口径。
3. **存量真 bug：回答软删把 answer_count 清零**——重算 SQL 的内层 `question_id = ?` 被绑成 answerID
   （非关联子查询，恒计 0），自基线即存在且无测试断言；改为关联外层行的子查询。
4. **keyset 游标稳定性**——新增测试：翻页中途软删一条未展示结果，游标仍有效，无重复、无遗漏、无已删项。

### 4.3 已知残留（登记，不在本轮回圈）

- `answer_count` 冗余列在"审核隔离"路径之外的历史数据可能存在口径漂移（派生字段可重算；对账工具为既有 PRD 待办）。
- 评论计数（`comment_count`）为创建时累加口径，隔离评论不自动递减（同上，派生字段）。
- 通知聚合跨页边界不合并（规格允许：聚合不改变底层事实）。
- Element Plus chunk > 500KB 构建提示（非阻塞，代码分割为后续优化项）。

## 5. 冻结声明

- `e477dc1`：已按 §2/§3 完成验收，三处修复核对在位。
- 质量闭环修复（§4.2）作为独立提交进入同一分支；冻结基线 = 该提交的 SHA（见提交记录）。
- 后续开发（含"未回答问题发现 + 回答邀请闭环"任务卡）一律以冻结基线为起点，`Documents\New project\repo-frontend-v3-audit-48a1d47` 下的未提交工作树不再作为实现来源。
