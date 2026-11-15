package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/snowflake"
	"bluebell/pkg/storage"
)

// 上传限制（规格 8.2）
const (
	maxAvatarBytes = 5 << 20  // 5 MB
	maxImageBytes  = 10 << 20 // 10 MB
	maxImagePixels = 40_000_000 // 解码炸弹防护：最大像素总数
	mediaHourlyLimit = 30
)

var storageInstance storage.ObjectStorage

// InitStorage 初始化本地存储（生产替换 S3 适配器）
func InitStorage(root string) error {
	s, err := storage.NewLocalStorage(root)
	if err != nil {
		return err
	}
	storageInstance = s
	return nil
}

// allowedImageType 通过文件魔数判断真实类型，不信任扩展名和 Content-Type
func detectImageType(r io.Reader) (ext string, size int64, err error) {
	buf := make([]byte, 512)
	n, err := io.ReadFull(io.MultiReader(io.LimitReader(r, 512), r), buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", 0, fmt.Errorf("read file: %w", err)
	}
	head := buf[:n]
	switch {
	case len(head) >= 8 && head[0] == 0x89 && head[1] == 'P' && head[2] == 'N' && head[3] == 'G':
		return "png", int64(n), nil
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		return "jpg", int64(n), nil
	case len(head) >= 6 && (string(head[0:6]) == "GIF87a" || string(head[0:6]) == "GIF89a"):
		return "gif", int64(n), nil
	case len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return "webp", int64(n), nil
	}
	return "", 0, fmt.Errorf("不支持的图片类型（允许 png/jpeg/gif/webp）")
}

// decodeImageBounds 解码图片头部获取尺寸（防解码炸弹：限制总像素）
func decodeImageBounds(c *gin.Context, file multipart.File) (width, height int, err error) {
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return 0, 0, err
	}
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, fmt.Errorf("图片解码失败（文件可能损坏或伪装）")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 10000 || cfg.Height > 10000 ||
		int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return 0, 0, fmt.Errorf("图片尺寸超出限制")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

// checkMediaRateLimit 上传限流（每小时）
func checkMediaRateLimit(userID int64) error {
	cnt, err := redis.IncrRateLimit("media:limit:", userID)
	if err != nil {
		return nil
	}
	if cnt > mediaHourlyLimit {
		return fmt.Errorf("上传过于频繁，请稍后再试")
	}
	return nil
}

// UploadMediaHandler 图片上传
// @Summary 上传图片
// @Description multipart 上传（field: file）；类型白名单 png/jpeg/gif/webp（魔数检测）；头像 5MB、内容图 10MB；服务端解码校验与去重
// @Tags 媒体相关接口
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param purpose query string false "avatar（5MB）| content（10MB，默认）"
// @Param file formData file true "图片文件"
// @Success 200 {object} controller.Response{data=map[string]string}
// @Router /media/upload [post]
func UploadMediaHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if storageInstance == nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	if err := checkMediaRateLimit(userID); err != nil {
		ResponseErrorWithMsg(c, CodeSendTooFrequent, err.Error())
		return
	}
	purpose := c.DefaultQuery("purpose", "content")
	maxBytes := int64(maxImageBytes)
	if purpose == "avatar" {
		maxBytes = maxAvatarBytes
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, "缺少 file 字段")
		return
	}
	if fileHeader.Size > maxBytes {
		ResponseErrorWithMsg(c, CodeInvalidParams, fmt.Sprintf("文件超过大小限制（%d MB）", maxBytes>>20))
		return
	}

	f, err := fileHeader.Open()
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	defer f.Close()

	ext, _, err := detectImageType(f)
	if err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, err.Error())
		return
	}
	width, height, err := decodeImageBounds(c, f)
	if err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, err.Error())
		return
	}

	// 读取全部内容计算 sha256（大小受上限约束）
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes {
		ResponseErrorWithMsg(c, CodeInvalidParams, "读取文件失败或超过大小限制")
		return
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	objectKey := path.Join(strconv.FormatInt(userID, 10),
		time.Now().UTC().Format("200601"), hash+"."+ext)
	// 去重：同人同内容直接返回已有 URL
	if err = storageInstance.Put(context.Background(), storage.UploadRequest{
		ObjectKey: objectKey,
		Reader:    strings.NewReader(string(data)),
		Size:      int64(len(data)),
	}); err != nil {
		zap.L().Error("storage put failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	asset := &models.MediaAsset{
		AssetID:  snowflake.GenerateID(),
		OwnerID:  userID,
		ObjectKey: objectKey,
		MimeType:  "image/" + strings.TrimPrefix(ext, "jpg"),
		ByteSize:  int64(len(data)),
		SHA256:    hash,
		Width:     int32(width),
		Height:    int32(height),
		Status:    1, // ready
	}
	if err = mysql.UpsertMediaAsset(asset); err != nil {
		zap.L().Error("upsert media asset failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	url, _ := storageInstance.ReadURL(context.Background(), objectKey, time.Hour)
	ResponseSuccess(c, gin.H{
		"asset_id": strconv.FormatInt(asset.AssetID, 10),
		"url":      url,
		"width":    width,
		"height":   height,
	})
}

// MediaServeHandler 本地媒体静态服务（生产由对象存储/CDN 承担）
func MediaServeHandler(c *gin.Context) {
	if storageInstance == nil {
		c.Status(404)
		return
	}
	key := c.Param("key")
	// 路径安全由 storage.safePath 校验；只读
	url, err := storageInstance.ReadURL(context.Background(), key, time.Minute)
	if err != nil {
		c.Status(404)
		return
	}
	// LocalStorage 的 URL 是 /media/<key>，这里实际读文件
	local, ok := storageInstance.(interface {
		ReadFile(key string) ([]byte, error)
	})
	if !ok {
		c.Redirect(302, url)
		return
	}
	data, err := local.ReadFile(key)
	if err != nil {
		c.Status(404)
		return
	}
	ext := strings.ToLower(path.Ext(key))
	ctype := map[string]string{
		".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
		".gif": "image/gif", ".webp": "image/webp",
	}[ext]
	if ctype == "" {
		c.Status(404)
		return
	}
	c.Data(200, ctype, data)
}
