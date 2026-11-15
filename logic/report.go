package logic

import (
	"strconv"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/snowflake"

	"go.uber.org/zap"
)

// 举报限流：每用户每小时最多 N 条（防利用举报接口骚扰）
const reportHourlyLimit = 20

// CreateReport 创建举报（同用户同目标幂等；每小时限流）
func CreateReport(reporterID int64, p *models.ParamCreateReport) (duplicated bool, err error) {
	targetType := models.ReportTargetTypeToCode(p.TargetType)
	if targetType == 0 {
		return false, ErrCommentInvalid
	}
	targetID, err := strconv.ParseInt(p.TargetID, 10, 64)
	if err != nil || targetID <= 0 {
		return false, ErrCommentInvalid
	}
	// 限流（Redis INCR + 1h TTL）
	if err = checkReportRateLimit(reporterID); err != nil {
		return false, err
	}
	r := &models.Report{
		ReportID:    snowflake.GenerateID(),
		ReporterID:  reporterID,
		TargetType:  targetType,
		TargetID:    targetID,
		ReasonCode:  p.ReasonCode,
		Description: p.Description,
	}
	return mysql.CreateReport(r)
}

func checkReportRateLimit(userID int64) error {
	cnt, err := redis.IncrReportLimit(userID)
	if err != nil {
		// Redis 故障不阻塞举报（审核兜底），仅告警
		zap.L().Error("report rate limit incr failed", zap.Error(err))
		return nil
	}

	if cnt > reportHourlyLimit {
		return redis.ErrCodeSendTooFrequent
	}
	return nil
}

