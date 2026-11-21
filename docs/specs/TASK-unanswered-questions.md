# 任务卡：未回答问题发现 + 回答邀请闭环

> 版本：1.0（2026-11-21）。前置：基线验收清单 `docs/verification/ACCEPTANCE-baseline.md` 通过后的冻结基线。
> 目标：让问题更快得到好回答。本卡是"选一个核心增长闭环做下一迭代"的落地规格，先写清页面/接口/通知规则/指标，再实现前后端。
> 明确非目标：不做推荐算法升级、不做私信/群聊扩展、不做积分声望体系。

## 0. 现状缺口（代码核实）

1. `logic/answer.go CreateAnswer` **不产生任何通知**——提问者和问题关注者都不知道有新回答（通知类型 1–7 中无"新回答"）。
2. 问题关注表 `question_follow` 已存在（可复用），但关注后没有更新触达。
3. 无"邀请回答"机制；提问者只能被动等待。
4. 无未回答问题的发现入口；发现页只有话题列表。

## 1. 功能范围

| # | 能力 | 说明 |
| --- | --- | --- |
| F1 | 未回答/低回答问题发现 | 公开问题按 `answer_count` 筛选（0 = 未回答、1–2 = 低回答），可按话题过滤，keyset 分页 |
| F2 | 新回答通知（提问者 + 问题关注者） | 回答发布同事务写入通知；聚合规则见 §4 |
| F3 | 邀请回答 | 提问者可邀请最多 5 人/问题；被邀请人收到通知；同一问题同一被邀请人幂等 |
| F4 | 通知可控 | 通知偏好最小实现：`notify_answer_update`（新回答/邀请）每用户可关，默认开 |
| F5 | 创作侧入口 | 创作者中心增加"适合回答的问题"：本人关注话题下未回答问题，按新鲜度排序 |

## 2. 数据模型（迁移 015，禁止修改历史迁移）

```sql
-- 邀请回答（幂等：同一问题+被邀请人唯一；同一问题总数 ≤5 在业务事务内校验）
CREATE TABLE `answer_invitation` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `invitation_id` bigint NOT NULL COMMENT '雪花id',
  `question_id` bigint NOT NULL,
  `inviter_id` bigint NOT NULL COMMENT '提问者',
  `invitee_id` bigint NOT NULL,
  `status` tinyint NOT NULL DEFAULT 0 COMMENT '0=pending 1=answered 2=declined 3=expired',
  `answered_answer_id` bigint NULL,
  `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `update_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_invitation_id` (`invitation_id`),
  UNIQUE KEY `uk_question_invitee` (`question_id`, `invitee_id`),
  KEY `idx_invitee_status` (`invitee_id`, `status`, `create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- 通知偏好（F4 最小实现；后续通知类型扩展复用此表）
ALTER TABLE `user` ADD COLUMN `notify_answer_update` tinyint NOT NULL DEFAULT 1
  COMMENT '0=关闭新回答/邀请通知 1=开启';

-- 未回答问题查询索引（EXPLAIN 验证后允许调整）
CREATE INDEX `idx_question_answer_count_time` ON `question` (`answer_count`, `status`, `create_time`);
```

不新增"问题更新通知开关"独立表之外的表；`notify_answer_update=0` 时 F2 对**非提问者**（关注者）不发，F3 邀请通知不受该开关控制（直接对话性质），但被邀请人已有 pending 邀请时不再重复通知。

## 3. API 契约（沿用 `/api/v1`、envelope、字符串 ID、稳定错误码追加）

| 方法 | 路径 | 鉴权 | 语义 |
| --- | --- | --- | --- |
| GET | `/questions/unanswered?topic_id=&sort=hot\|new&cursor=&limit=` | 公开（OptionalAuth） | `answer_count=0` 且统一公开可见条件；`hot` 按浏览+关注加权、`new` 按发布时间；keyset 游标 `(sort_key, id)` |
| POST | `/questions/:id/invitations` | 提问者 | body `{ invitee_id }`；校验：问题作者、问题公开、被邀请人有效未禁用、非自己、该问题 pending+answered 邀请总数 <5、未被屏蔽（双向任一屏蔽 → 统一不可联系）；幂等：同问题同人重复邀请返回原记录（`duplicated=true`），不重复通知 |
| GET | `/questions/:id/invitations` | 提问者 | 本人问题的邀请列表（invitee 摘要 + 状态） |
| POST | `/invitations/:id/decline` | 被邀请人 | 幂等；pending→declined；不通知提问者（v1 简化，避免打扰） |
| GET | `/me/answer-invitations` | 登录 | 我收到的邀请（pending 优先，含问题摘要），供"适合回答"入口聚合 |
| PUT | `/me/notification-preferences` | 登录 | body `{ notify_answer_update: bool }`；GET 同路径读取 |

错误码追加（不重排）：`1045 CodeInvitationLimit`（每问题邀请数超限）、`1046 CodeInvitationState`（状态不允许该操作）。通知类型追加：`8 NotifyTypeNewAnswer`（你的问题有新回答 / 你关注的问题有新回答）、`9 NotifyTypeAnswerInvite`（邀请你回答）。前端 http.ts 与通知文案同步。

## 4. 通知规则（与业务写入同事务；参照 §14 事件语义）

| 触发 | 接收者 | 类型 | 聚合/去重 |
| --- | --- | --- | --- |
| 新回答发布（审核通过即写入；隔离/删除路径回收见下） | 提问者（除非回答者=提问者） | 8 | 同问题同提问者按批次聚合 `agg_count`，不改底层事实 |
| 新回答发布 | 问题关注者（排除提问者与回答者；`notify_answer_update=0` 跳过；上限每回答 50 人，超出截断并记日志——v1 简化，后续改 outbox 批投递） | 8 | 不聚合 |
| 邀请回答 | 被邀请人（每次新邀请；幂等重试不重发；屏蔽联动拒绝邀请） | 9 | 不聚合 |
| 回答被隔离/删除 | 不撤回已发通知（历史事实）；对应 invitation 若 pending → `expired`（回答已不可见） | — | — |
| 被邀请人回答了受邀问题 | invitation `pending→answered`（在 CreateAnswer 同事务内按 `uk_question_invitee` 命中更新）；不再单独通知提问者（新回答通知已覆盖） | — | — |

邮件/推送不在本卡范围（站内通知已满足闭环）；outbox 批量投递作为 F2 超过 50 人场景的后续项登记。

## 5. 页面（前端，沿用现有 token 与组件）

1. **发现页**：新增"等你来答"区块（未回答问题 Top N，按 hot/new 切换）；空态给"去提问"入口，不用假数据。
2. **问题详情页**：提问者看到"邀请回答"入口（选择用户弹层；显示已邀请列表与状态）；被邀请人进入问题页时顶部轻提示"xx 邀请你回答这个问题"（来自 `/me/answer-invitations`）。
3. **通知中心**：类型 8/9 的图标与文案；点击跳转对应问题。
4. **创作者中心**："适合回答的问题"列表（关注话题下的未回答问题 + 我收到的 pending 邀请）。
5. **推荐设置页**：新增"新回答与邀请通知"开关（PUT /me/notification-preferences）。
6. 移动端：以上入口在 375px 底部导航可达（发现 → 等你来答）。

## 6. 成功指标（先采集基线，不设虚构目标值）

| 指标 | 口径 | 数据来源 |
| --- | --- | --- |
| 首答时间 P50/P90 | 问题发布 → 第一个非作者公开回答 | `question.create_time` vs `answer.create_time`（需回答表按问题取最早公开回答，现有数据可算） |
| 问题回答率 | 观察窗口内 ≥1 个非作者公开回答的问题 / 公开可回答问题 | question/answer 联表 |
| 邀请转化率 | 被邀请后 7 天内产生回答的邀请 / 全部邀请 | `answer_invitation.status=1` 且 `answered_answer_id` 非空 |
| 通知→回访 | 发出 type=8 通知后 24h 内该用户会话有提问/回答动作的比例 | notification + 用户行为（v1 用最小口径：通知点击跳转计一次） |
| 退订率 | `notify_answer_update=0` 的用户 / 活跃用户 | user 列 |

指标实现为只读 SQL/聚合接口（`/creator/insights` 已有创作者统计入口可扩展），不以模拟数据宣称达成。

## 7. 验收标准

- 权限：非提问者不能邀请；自邀请拒绝；屏蔽任一方统一不可联系；`notify_answer_update=0` 时关注者不再收到 type=8（提问者仍收到——提问者对自己的问题默认在意，开关为后续细分项登记）。
- 幂等：同问题同人重复邀请返回原记录且不重复通知；同问题 >5 个有效邀请返回 1045；回答后邀请状态自动 answered。
- 事务：新回答通知与 CreateAnswer 同事务；通知写入失败回滚回答（与现有互动通知语义一致）。
- 分页：`/questions/unanswered` keyset 翻页无重复无遗漏；隔离/删除问题即时消失。
- 集成测试：并发同问题 6 次邀请恰好 5 成功 1 超限；退订后不发；屏蔽后邀请拒绝；聚合通知 `agg_count` 正确。
- E2E：提问者邀请 → 被邀请人通知可见 → 回答 → 提问者收到新回答通知 → 采纳（串联 A2 已有闭环）。
