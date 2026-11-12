package storage

import (
	"context"
	"fmt"
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
