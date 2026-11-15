// Package storage 对象存储抽象（规格 8.1）。
// 开发环境使用本地目录适配器；生产环境可接 S3/OSS/MinIO，业务层只依赖接口。
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrObjectNotFound 对象不存在
var ErrObjectNotFound = errors.New("object not found")

// UploadRequest 上传请求
type UploadRequest struct {
	ObjectKey string // 服务端生成，禁止客户端控制（防路径穿越）
	Reader    io.Reader
	Size      int64
}

// ObjectStorage 对象存储接口
type ObjectStorage interface {
	Put(ctx context.Context, req UploadRequest) error
	Delete(ctx context.Context, objectKey string) error
	ReadURL(ctx context.Context, objectKey string, ttl time.Duration) (string, error)
}

// LocalStorage 本地目录适配器（开发环境）
// root 为上传根目录；所有 key 经过安全校验（防路径穿越）
type LocalStorage struct {
	Root string
}

// NewLocalStorage 创建本地存储并确保目录存在
func NewLocalStorage(root string) (*LocalStorage, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &LocalStorage{Root: root}, nil
}

// safePath 校验 objectKey 并返回磁盘路径；拒绝 ..、绝对路径与非法字符
func (l *LocalStorage) safePath(objectKey string) (string, error) {
	if strings.Contains(objectKey, "..") || strings.HasPrefix(objectKey, "/") || strings.ContainsAny(objectKey, "\\\x00") {
		return "", fmt.Errorf("invalid object key")
	}
	clean := filepath.Clean(filepath.Join(l.Root, objectKey))
	if !strings.HasPrefix(clean, filepath.Clean(l.Root)+string(filepath.Separator)) &&
		clean != filepath.Clean(l.Root) {
		return "", fmt.Errorf("invalid object key")
	}
	return clean, nil
}

// Put 写入对象
func (l *LocalStorage) Put(ctx context.Context, req UploadRequest) error {
	path, err := l.safePath(req.ObjectKey)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	if _, err = io.Copy(f, req.Reader); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	f.Close()
	// 原子替换
	return os.Rename(f.Name(), path)
}

// Delete 删除对象
func (l *LocalStorage) Delete(ctx context.Context, objectKey string) error {
	path, err := l.safePath(objectKey)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return ErrObjectNotFound
	}
	return err
}

// ReadURL 本地模式返回一个内部路径（由 /media/ 静态路由服务）
func (l *LocalStorage) ReadURL(ctx context.Context, objectKey string, ttl time.Duration) (string, error) {
	if _, err := l.safePath(objectKey); err != nil {
		return "", err
	}
	return "/media/" + objectKey, nil
}

// ReadFile 本地读取对象内容（静态服务用）
func (l *LocalStorage) ReadFile(objectKey string) ([]byte, error) {
	path, err := l.safePath(objectKey)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
