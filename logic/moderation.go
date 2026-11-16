package logic

import (
	"errors"
	"strings"
	"sync"

	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/sanitizer"
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

// loadSensitiveWords 加载启用的敏感词（缓存 + 版本比对自动失效）
// 先轻查询当前版本号；与缓存版本不一致（管理端/外部更新词表）则重新加载
func loadSensitiveWords() ([]sensitiveWord, int64, error) {
	curVer, err := mysql.SensitiveWordsVersion()
	if err != nil {
		return nil, 0, err
	}
	wordCacheMu.RLock()
	if wordCacheValid && wordCacheVer == curVer {
		words := wordCache
		wordCacheMu.RUnlock()
		return words, curVer, nil
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

// ErrModerationUnavailable 审核组件不可用（fail-closed，规格 15.1：避免未审核内容直接放出）
var ErrModerationUnavailable = errors.New("审核服务暂时不可用，请稍后重试")

// CheckContentModeration 发布前敏感词检查
// 返回决策：命中拒绝词 -> pending；命中普通词 -> quarantined；未命中 -> approved。
// 词表加载失败时 fail-closed：拒绝本次发布并提示重试——宁可发布失败也不放出未审核内容
// （管理员绕过路径：临时停用全部敏感词或通过审核后台人工放行）
func CheckContentModeration(texts ...string) (ModerationDecision, error) {
	words, _, err := loadSensitiveWords()
	if err != nil {
		zap.L().Error("load sensitive words failed, fail-closed", zap.Error(err))
		return ModerationDecision{}, ErrModerationUnavailable
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

// RenderMarkdownSafe 安全 Markdown 渲染（服务端清洗，输出白名单 HTML）
func RenderMarkdownSafe(markdown string) string {
	return sanitizer.RenderMarkdown(markdown)
}

// StripMarkdownText 提取纯文本摘要
func StripMarkdownText(markdown string) string {
	return sanitizer.StripMarkdown(markdown)
}

// InvalidateSensitiveWords 已在上方定义

// ListSensitiveWords 导出词表（管理视图，含版本）
type SensitiveWordRow struct {
	Word    string `json:"word"`
	Type    int32  `json:"word_type"`
	Enabled bool   `json:"enabled"`
	Version int64  `json:"version"`
}

func ListSensitiveWords() ([]SensitiveWordRow, int64, error) {
	words, ver, err := loadSensitiveWords()
	if err != nil {
		return nil, 0, err
	}
	rows := make([]SensitiveWordRow, 0, len(words))
	for _, w := range words {
		rows = append(rows, SensitiveWordRow{Word: w.Word, Type: w.Type, Enabled: true, Version: ver})
	}
	return rows, ver, nil
}
