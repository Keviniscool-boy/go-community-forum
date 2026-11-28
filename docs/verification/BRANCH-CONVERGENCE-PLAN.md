# frontend-v3 与 main 迁移线合流规划

> 版本：1.0（2026-11-28）
> 现状：两条线已分叉，各自有对方没有的迁移与功能；再不合并，同名迁移编号冲突会持续恶化。
> 本文给出迁移重编号映射、冲突清单、建议合流步骤与验收标准。**本文是规划，不代表已完成合并。**

## 1. 两条线的现状

| 线 | HEAD（2026-11-28） | 已有迁移 | 独有功能 |
| --- | --- | --- | --- |
| `frontend-v3` | `2b103bb` | `000`–`015`（008=articles_columns） | 邮箱认证全链路增强、增长/个性化（009）、私信 PRD 全套状态机（010）、协作创作（011）、举报证据/outbox/聚合（012）、冷却+pending 唯一（013）、问答解决闭环采纳（014）、回答邀请闭环（015）、跨内容搜索（文章/专栏）、统一可见性收尾、第五阶段审计 PRD 文档 |
| `main`（含 `feat/phase5-social-dm`） | `2142e6f`（phase5 未推送） | `000`–`011`（008=comment_enhance、009=social、010=messages、011=m3） | 第四阶段评论体系（楼中楼/评论点赞/回复通知）、品牌「问见」、第五阶段 M1/M2/M3（关注粉丝列表、拉黑、通知单条已读+偏好、评论编辑、私信简化版、头像校验、问题重定向、搜索历史） |

两线共同祖先是 `c8aa855`（第三阶段产品化）。

## 2. 核心冲突：迁移编号表

| 版本号 | frontend-v3 内容 | main 内容 | 合流后归属（建议） |
| --- | --- | --- | --- |
| 008 | articles_columns | comment_enhance | **保持 008 不能有两个**：重编号 main 的 comment_enhance → 见下 |
| 009 | growth_personalization | social_phase5（user_block+comment.update_time） | 重编号 |
| 010 | direct_messages（PRD 全套状态机版） | direct_messages（PRD-第五阶段简化版） | **语义冲突最重的一对**，二选一或人工融合 |
| 011 | collaboration | phase5_m3（notify_settings+redirect_to） | 重编号 |
| 012–015 | privacy_outbox / dm_cooldown / question_resolution / answer_invitations | （无） | 顺延 |

### 2.1 建议的合流后迁移序列（新统一链）

以 frontend-v3 为主干（功能更全、审计更严），将 main 线的三个迁移**重新编号并适配**后追加：

```text
000–015  保持 frontend-v3 原序
016      comment_enhance（main 008 改造：把「ALTER TABLE comment」合入——注意 frontend-v3
         的 comment 已有 012/013 增列，需核对 like_count/reply_count/reply_to_user_id
         不与其冲突；存量上提 UPDATE 语句原样保留）
017      social_phase5（main 009：user_block + comment.update_time——comment.update_time
         需确认 016 未重复添加；user_block 无冲突）
018      direct_messages（main 010 简化版）——**决策点**：frontend-v3 的 010 已交付完整
         私信 PRD（会话状态机/隐私策略/冷却/幂等键/证据加密）。简化版 message_thread/message
         与其语义不同，建议**丢弃简化版**，保留 frontend-v3 版本（功能是超集）；
         main 的私信前端页面按 frontend-v3 的 API 改写
019      phase5_m3（main 011：notify_settings + redirect_to，无冲突）
```

### 2.2 通知类型编号冲突（最需要先定契约）

| 类型 | frontend-v3 | main |
| --- | --- | --- |
| 5 | 私信（outbox 投递） | 赞同了你的评论 |
| 6 | 回答被采纳 | 回复了你的评论 |
| 7 | 采纳被撤销 | 私信 |
| 8–11 | 8=新回答 9=邀请回答 10/11=邀请响应 | （无） |

合流后统一分配建议：**以 frontend-v3 为准保留 5–11 原义，main 的 comment_like/reply 追加为 12/13**，
main 侧 `txCreateNotification` 调用点与前端 `typeIcon/targetLink` 随之改号。前端通知文案映射合并两线字典。

### 2.3 代码级冲突清单（按文件，按 git diff 规模排序）

| 文件 | 冲突性质 | 处理建议 |
| --- | --- | --- |
| `routes/routes.go` | 两线各加 ~100 行路由；私信两套 API 并存 | 以 frontend-v3 路由为基座，追加 main 的 phase5 路由（messages 简化版不并入） |
| `logic/notification.go` | 通知文案/type 映射/主页聚合分叉 | 手工合并：frontend-v3 的偏好+邀请+新回答文案，叠加 main 的评论类文案 |
| `models/interaction.go` | 通知常量、Comment/Question 字段、偏好结构 | 取并集；Question 用 frontend-v3 版（含 resolution/accepted）+ main 的 redirect_to |
| `dao/mysql/interaction.go` | comment 两级结构（main）vs 互动通知偏好跳过（两线都改了 txCreateNotification） | 以 main 的评论体系为主体，frontend-v3 的偏好检查/屏蔽跳过逻辑叠加 |
| `dao/mysql/comment.go` / `logic/comment.go` | main 独有（楼中楼/点赞/编辑） | 全量取 main，再叠 frontend-v3 的可见性过滤（FilterBlockViewerSQL） |
| `dao/mysql/question.go` / `logic/content.go` | frontend-v3 有解决闭环/采纳；main 有 redirect | 取 frontend-v3 版 + main 的 redirect 三函数 |
| `pkg/search/mysql.go` | frontend-v3 重写过（keyset/专栏/已解决标注） | 取 frontend-v3 版 |
| `e2e/e2e.spec.ts` | frontend-v3 有 4 用例，main 线裁剪到 2 | 合并两线用例集（采纳/邀请/第五阶段），本地跑通为准 |

## 3. 建议合流步骤（每步可独立验收）

1. **冻结窗口**：两线停止新迁移；本规划评审通过后开合流分支 `merge/lines-2026-11`（基于 main）。
2. **迁移链改造**：按 §2.1 生成新链 000–019；在空库跑全链 + 在「已执行至 008_comment_enhance 的库」演练升级（main 生产/测试库的真实状态）+ 在「frontend-v3 库」演练回放；三路径全绿才继续。
3. **通知类型契约定稿**：按 §2.2 的编号表更新两线常量与前端映射，作为独立提交（先于代码合并），避免合并中途改语义。
4. **git merge frontend-v3**：逐文件按 §2.3 的建议取合；`routes.go`/`interaction.go`/`notification.go` 人工合并；每合并一个域跑一次 `go build && go test ./dao/mysql ./logic`。
5. **私信取舍落地**：丢弃 main 简化版表（018 不迁移），前端 messages 页改接 frontend-v3 的会话 API（字段名不同：conversation/thread 等）。
6. **全量验收**：frontend-v3 的 23+ 集成测试、main 的 6 项 phase5 测试、合并后新增冲突专项测试（评论点赞通知 type 改号后的映射、偏好键位覆盖）、e2e 全集 4+2 用例、375px 走查。
7. **废弃清理**：main 侧被替代的 messages DAO/控制器删除；`frontend-v3` 分支在合流 PR 合并后归档。

## 4. 验收标准

- 空库、main 现网库（至 008_comment_enhance）、frontend-v3 库（至 015）三条升级路径全部通过。
- 通知类型编号唯一且两线功能的通知都能送达（评论点赞用 12、回复用 13 验证）。
- 私信只保留一套事实表（frontend-v3 conversation 系）。
- 采纳闭环、邀请闭环、第五阶段社交/私信/偏好/重定向全部回归通过。
- CI（Go + 前端 + e2e）在合流 PR 上全绿。

## 5. 风险

- **010 语义冲突**是最大风险：两线都叫 direct_messages 但事实表完全不同；若 main 线已有真实用户数据落在 message_thread，需要数据迁移脚本（当前判断：main 的新私信尚无生产数据，丢弃成本为零——需负责人确认）。
- 通知改号会影响已发出通知的 type 值（存量行），前端按新编号渲染前需要一次性数据订正（UPDATE notification SET type=12 WHERE type=5 AND ...）——具体订正语句在合并 PR 中给出。
- 两线的 `user` 表在合流后同时具备 notify_settings（main 011）与 growth_personalization 的偏好列（frontend-v3 009），字段无冲突但设置页需合并展示。
