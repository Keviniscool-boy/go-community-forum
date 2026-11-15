package logic

import (
	"strings"
	"sync"

	"bluebell/dao/mysql"
	"bluebell/models"
	"go.uber.org/zap"
)

// 敏感词审核：词表存 MySQL，进程内缓存 + 版本号失效。
// 发布流程（规格 7.1）：命中拒绝词 -> pending（人工队列）；命中普通词 -> quarantined（隔离复核）；未命中 -> approved。

var (
	wordCacheMu    sync.RWMutex
	wordCache      []sensitiveWord
	wordCacheVer   int64
	wordCacheValid bool
)

type sensitiveWord = struct {
	Word string
	Type int32 // 0 普通词（隔离） 1 拒绝词（待人工审核）
}

// loadSensitiveWords 加载启用的敏感词（带缓存）
func loadSensitiveWords() ([]sensitiveWord, int64, error) {
	wordCacheMu.RLock()
	if wordCacheValid {
		words, ver := wordCache, wordCacheVer
		wordCacheMu.RUnlock()
		return words, ver, nil
	}
	wordCacheMu.RUnlock()

	words, ver, err := mysql.LoadSensitiveWords()
	if err != nil {
		return nil, 0, err
	}
	wordCacheMu.Lock()
	wordCache, wordCacheVer, wordCacheValid = words, ver, true
	wordCacheMu.Unlock()
	return words, ver, nil
}

// InvalidateSensitiveWords 词表更新后失效缓存（管理端调用）
func InvalidateSensitiveWords() {
	wordCacheMu.Lock()
	wordCacheValid = false
	wordCacheMu.Unlock()
}

// ModerationDecision 发布审核决策
type ModerationDecision struct {
	Status int32  // models.ModerationApproved / Pending / Quarantined
	Reason string // 命中描述（不落敏感词原文，只落命中类型与规则 ID）
}

// CheckContentModeration 发布前敏感词检查
// 返回决策：命中拒绝词 -> pending；命中普通词 -> quarantined；未命中 -> approved
func CheckContentModeration(texts ...string) (ModerationDecision, error) {
	words, _, err := loadSensitiveWords()
	if err != nil {
		// 词表不可用：按失败开放处理（approved）并告警——不能因审核组件故障阻塞主功能；
		// 管理员可通过审计与举报链路兜底
		zap.L().Error("load sensitive words failed, fallback to approved", zap.Error(err))
		return ModerationDecision{Status: models.ModerationApproved}, nil
	}
	if len(words) == 0 {
		return ModerationDecision{Status: models.ModerationApproved}, nil
	}
	joined := strings.ToLower(strings.Join(texts, "\n"))
	decision := ModerationDecision{Status: models.ModerationApproved}
	for _, w := range words {
		if !strings.Contains(joined, strings.ToLower(w.Word)) {
			continue
		}
		if w.Type == 1 {
			// 拒绝词：进入人工审核队列，覆盖普通词结果
			return ModerationDecision{Status: models.ModerationPending, Reason: "reject-word"}, nil
		}
		if decision.Status == models.ModerationApproved {
			decision = ModerationDecision{Status: models.ModerationQuarantined, Reason: "sensitive-word"}
		}
	}
	return decision, nil
}
