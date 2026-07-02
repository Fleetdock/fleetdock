// Package storage is a thin S3-compatible object-storage adapter (AWS S3,
// Cloudflare R2, MinIO, ...) used for backup upload/download and presigning
// URLs handed to agents.
package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config identifies one bucket + credential pair.
type Config struct {
	Endpoint  string // empty = AWS S3
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
}

// Client wraps a minio client bound to one bucket.
type Client struct {
	mc     *minio.Client
	bucket string
}

// New builds a client for the given destination config.
func New(cfg Config) (*Client, error) {
	endpoint := strings.TrimPrefix(strings.TrimPrefix(cfg.Endpoint, "https://"), "http://")
	secure := true
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	} else if strings.HasPrefix(cfg.Endpoint, "http://") {
		secure = false
	}
	region := cfg.Region
	if region == "" {
		region = "auto"
	}
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: secure,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: new client: %w", err)
	}
	return &Client{mc: mc, bucket: cfg.Bucket}, nil
}

// TestAccess verifies the bucket is reachable with the credentials.
func (c *Client) TestAccess(ctx context.Context) error {
	_, err := c.mc.BucketExists(ctx, c.bucket)
	return err
}

// PresignPut returns a URL an agent can HTTP PUT an object to.
func (c *Client) PresignPut(ctx context.Context, key string, expiry time.Duration) (string, error) {
	u, err := c.mc.PresignedPutObject(ctx, c.bucket, key, expiry)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// PresignGet returns a URL an agent can HTTP GET an object from.
func (c *Client) PresignGet(ctx context.Context, key string, expiry time.Duration) (string, error) {
	u, err := c.mc.PresignedGetObject(ctx, c.bucket, key, expiry, url.Values{})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// Upload streams an object of known size into the bucket.
func (c *Client) Upload(ctx context.Context, key string, r io.Reader, size int64) error {
	_, err := c.mc.PutObject(ctx, c.bucket, key, r, size, minio.PutObjectOptions{ContentType: "application/gzip"})
	return err
}

// Download opens an object for reading.
func (c *Client) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	return c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
}
