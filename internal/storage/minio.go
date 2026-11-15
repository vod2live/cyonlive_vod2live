package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	Minio  *minio.Client
	Bucket string
}

func New(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Client, error) {
	m, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: useSSL})
	if err != nil {
		return nil, err
	}
	return &Client{Minio: m, Bucket: bucket}, nil
}

func (c *Client) EnsureBucket(ctx context.Context) error {
	exists, err := c.Minio.BucketExists(ctx, c.Bucket)
	if err != nil {
		return err
	}
	if !exists {
		return c.Minio.MakeBucket(ctx, c.Bucket, minio.MakeBucketOptions{})
	}
	return nil
}

func (c *Client) PresignedPut(ctx context.Context, key string, expiry time.Duration) (*url.URL, error) {
	return c.Minio.PresignedPutObject(ctx, c.Bucket, cleanKey(key), expiry)
}

func (c *Client) PresignedGet(ctx context.Context, key string, expiry time.Duration) (*url.URL, error) {
	return c.Minio.PresignedGetObject(ctx, c.Bucket, cleanKey(key), expiry, nil)
}

func (c *Client) Stat(ctx context.Context, key string) (int64, string, error) {
	info, err := c.Minio.StatObject(ctx, c.Bucket, cleanKey(key), minio.StatObjectOptions{})
	return info.Size, info.ContentType, err
}

func (c *Client) Download(ctx context.Context, key, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return c.Minio.FGetObject(ctx, c.Bucket, cleanKey(key), destination, minio.GetObjectOptions{})
}

func (c *Client) Read(ctx context.Context, key string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = 8 << 20
	}
	object, err := c.Minio.GetObject(ctx, c.Bucket, cleanKey(key), minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer object.Close()
	info, err := object.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size > maxBytes {
		return nil, fmt.Errorf("object exceeds the %d-byte read limit", maxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(object, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("object exceeds the %d-byte read limit", maxBytes)
	}
	return body, nil
}

func (c *Client) UploadFile(ctx context.Context, key, filename, contentType string) error {
	_, err := c.Minio.FPutObject(ctx, c.Bucket, cleanKey(key), filename, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (c *Client) Delete(ctx context.Context, key string) error {
	return c.Minio.RemoveObject(ctx, c.Bucket, cleanKey(key), minio.RemoveObjectOptions{})
}

func (c *Client) ObjectURI(key string) string {
	return fmt.Sprintf("s3://%s/%s", c.Bucket, cleanKey(key))
}

func ParseURI(raw string) (bucket, key string, err error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "s3" || u.Host == "" || strings.Trim(u.Path, "/") == "" {
		return "", "", fmt.Errorf("invalid storage URI")
	}
	return u.Host, strings.TrimPrefix(u.Path, "/"), nil
}

func cleanKey(key string) string {
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+key)), "/")
}
