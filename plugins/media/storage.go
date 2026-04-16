package media

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/wangling-miao/aroute/core"
)

type StorageBackend interface {
	Save(ctx context.Context, data []byte, path string) error
	Get(ctx context.Context, path string) ([]byte, error)
	Delete(ctx context.Context, path string) error
	GetURL(ctx context.Context, path string) (string, error)
	Type() string
}

func NewStorageBackend(storageType string, dataDir string, cfg core.ConfigProvider) (StorageBackend, error) {
	switch storageType {
	case "local":
		return NewLocalStorage(dataDir)
	case "s3":
		return NewS3Storage(cfg)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", storageType)
	}
}

type LocalStorage struct {
	baseDir string
}

func NewLocalStorage(dataDir string) (*LocalStorage, error) {
	baseDir := filepath.Join(dataDir, "uploads")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create uploads directory: %w", err)
	}
	return &LocalStorage{baseDir: baseDir}, nil
}

func (s *LocalStorage) Save(_ context.Context, data []byte, path string) error {
	fullPath := filepath.Join(s.baseDir, path)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func (s *LocalStorage) Get(_ context.Context, path string) ([]byte, error) {
	fullPath := filepath.Join(s.baseDir, path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return data, nil
}

func (s *LocalStorage) Delete(_ context.Context, path string) error {
	fullPath := filepath.Join(s.baseDir, path)
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}

func (s *LocalStorage) GetURL(_ context.Context, path string) (string, error) {
	return "/uploads/" + filepath.ToSlash(path), nil
}

func (s *LocalStorage) Type() string { return "local" }

type S3Storage struct {
	client *minio.Client
	bucket string
}

func NewS3Storage(cfg core.ConfigProvider) (*S3Storage, error) {
	endpoint := cfg.GetString("endpoint")
	accessKey := cfg.GetString("access_key")
	secretKey := cfg.GetString("secret_key")
	bucket := cfg.GetString("bucket")
	useSSL := cfg.GetBool("use_ssl")
	region := cfg.GetString("region")

	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		return nil, fmt.Errorf("s3 storage requires endpoint, access_key, secret_key, and bucket configuration")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("create s3 client: %w", err)
	}

	return &S3Storage{client: client, bucket: bucket}, nil
}

func (s *S3Storage) Save(ctx context.Context, data []byte, path string) error {
	contentType := "application/octet-stream"
	_, err := s.client.PutObject(ctx, s.bucket, path, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return fmt.Errorf("s3 put object: %w", err)
	}
	return nil
}

func (s *S3Storage) Get(ctx context.Context, path string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, path, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("s3 get object: %w", err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("s3 read object: %w", err)
	}
	return data, nil
}

func (s *S3Storage) Delete(ctx context.Context, path string) error {
	err := s.client.RemoveObject(ctx, s.bucket, path, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("s3 remove object: %w", err)
	}
	return nil
}

func (s *S3Storage) GetURL(ctx context.Context, path string) (string, error) {
	reqParams := url.Values{}
	presignedURL, err := s.client.PresignedGetObject(ctx, s.bucket, path, 1*time.Hour, reqParams)
	if err != nil {
		return "", fmt.Errorf("s3 presigned url: %w", err)
	}
	return presignedURL.String(), nil
}

func (s *S3Storage) Type() string { return "s3" }
