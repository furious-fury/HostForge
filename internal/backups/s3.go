package backups

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	transfermanagertypes "github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Destination struct {
	Endpoint, Region, Bucket, AccessKey, SecretKey string
	ServerSideEncryption, SSEKMSKeyID              string
	PathStyle                                      bool
}

type Client struct {
	s3                   *s3.Client
	upload               *transfermanager.Client
	bucket               string
	serverSideEncryption transfermanagertypes.ServerSideEncryption
	sseKMSKeyID          string
}

func NewClient(ctx context.Context, destination Destination) (*Client, error) {
	return newClient(ctx, destination, nil)
}

func newClient(ctx context.Context, destination Destination, httpClient aws.HTTPClient) (*Client, error) {
	if strings.TrimSpace(destination.Endpoint) == "" || strings.TrimSpace(destination.Region) == "" || strings.TrimSpace(destination.Bucket) == "" || strings.TrimSpace(destination.AccessKey) == "" || strings.TrimSpace(destination.SecretKey) == "" {
		return nil, fmt.Errorf("incomplete S3 destination")
	}
	configOptions := []func(*config.LoadOptions) error{
		config.WithRegion(strings.TrimSpace(destination.Region)),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(strings.TrimSpace(destination.AccessKey), strings.TrimSpace(destination.SecretKey), "")),
	}
	if httpClient != nil {
		configOptions = append(configOptions, config.WithHTTPClient(httpClient))
	}
	cfg, err := config.LoadDefaultConfig(ctx, configOptions...)
	if err != nil {
		return nil, fmt.Errorf("load S3 client configuration: %w", err)
	}
	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(strings.TrimRight(strings.TrimSpace(destination.Endpoint), "/"))
		options.UsePathStyle = destination.PathStyle
	})
	return &Client{s3: client, upload: transfermanager.New(client), bucket: strings.TrimSpace(destination.Bucket), serverSideEncryption: transfermanagertypes.ServerSideEncryption(strings.TrimSpace(destination.ServerSideEncryption)), sseKMSKeyID: strings.TrimSpace(destination.SSEKMSKeyID)}, nil
}

func (c *Client) Put(ctx context.Context, key string, body io.Reader, contentType string) error {
	input := &transfermanager.UploadObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(strings.TrimLeft(key, "/")), Body: body, ContentType: aws.String(contentType), ServerSideEncryption: c.serverSideEncryption}
	if c.sseKMSKeyID != "" {
		input.SSEKMSKeyID = aws.String(c.sseKMSKeyID)
	}
	_, err := c.upload.UploadObject(ctx, input)
	if err != nil {
		return fmt.Errorf("upload backup object: %w", err)
	}
	return nil
}

func (c *Client) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	result, err := c.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(strings.TrimLeft(key, "/"))})
	if err != nil {
		return nil, fmt.Errorf("read backup object: %w", err)
	}
	return result.Body, nil
}

func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(strings.TrimLeft(key, "/"))})
	if err != nil {
		return fmt.Errorf("delete backup object: %w", err)
	}
	return nil
}

func (c *Client) Size(ctx context.Context, key string) (int64, error) {
	result, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(strings.TrimLeft(key, "/"))})
	if err != nil {
		return 0, fmt.Errorf("inspect backup object: %w", err)
	}
	if result.ContentLength == nil {
		return 0, fmt.Errorf("backup object size unavailable")
	}
	return *result.ContentLength, nil
}

func (c *Client) Test(ctx context.Context, key string) error {
	payload := []byte("hostforge-backup-destination-probe")
	if err := c.Put(ctx, key, bytes.NewReader(payload), "application/octet-stream"); err != nil {
		return err
	}
	defer c.Delete(context.WithoutCancel(ctx), key)
	body, err := c.Get(ctx, key)
	if err != nil {
		return err
	}
	defer body.Close()
	read, err := io.ReadAll(io.LimitReader(body, int64(len(payload)+1)))
	if err != nil || !bytes.Equal(read, payload) {
		return fmt.Errorf("backup destination probe content mismatch")
	}
	if err := c.Delete(ctx, key); err != nil {
		return err
	}
	return nil
}
