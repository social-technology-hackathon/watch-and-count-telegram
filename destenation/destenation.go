package destenation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go-v2/aws"
)

type Destenation interface {
	Store(ctx context.Context, f io.Reader, ext string) (string, error)
}

type FSDestenation struct {
	basePath string
}

func NewFSDestenation(basePath string) (*FSDestenation, error) {
	err := os.MkdirAll(basePath, 0755)
	if err != nil {
		if !os.IsExist(err) {
			return nil, err
		}
	}
	return &FSDestenation{basePath: basePath}, nil
}

func (fs *FSDestenation) Store(_ context.Context, f io.Reader, ext string) (string, error) {
	name := uuid.New().String()
	p := filepath.Join(fs.basePath, fmt.Sprintf("%s.%s", name, ext))
	file, err := os.Create(p)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, f); err != nil {
		return "", err
	}

	return p, file.Close()
}

type S3Destenation struct {
	cli      *s3.Client
	bucket   string
	endpoint aws.EndpointResolver
}

type S3DestenationOption func(*S3Destenation)

func WithCustomEndpoint(uri string) S3DestenationOption {
	return func(dst *S3Destenation) {
		dst.endpoint = aws.ResolveWithEndpointURL(uri)
	}
}

func NewS3Destenation(bucket, key, secret, region string, options ...S3DestenationOption) (*S3Destenation, error) {
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		return nil, err
	}

	s3dst := S3Destenation{
		bucket:   bucket,
		endpoint: cfg.EndpointResolver,
	}
	for _, opt := range options {
		opt(&s3dst)
	}

	cfg.EndpointResolver = s3dst.endpoint
	cfg.Region = region
	cfg.Credentials = aws.NewStaticCredentialsProvider(key, secret, "")
	cli := s3.New(cfg)
	_, err = cli.HeadBucketRequest(&s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	}).Send(context.Background())
	if err != nil {
		return nil, err
	}
	return &S3Destenation{
		cli:    cli,
		bucket: bucket,
	}, nil
}

func (s *S3Destenation) abortMultipartUpload(ctx context.Context, key string, uploadID *string) error {
	_, err := s.cli.AbortMultipartUploadRequest(&s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: uploadID,
	}).Send(ctx)
	return err
}

func (s *S3Destenation) Store(ctx context.Context, f io.Reader, ext string) (string, error) {
	fname := uuid.New()
	path := fmt.Sprintf("%s.%s", fname, ext)
	resp, err := s.cli.CreateMultipartUploadRequest(&s3.CreateMultipartUploadInput{
		Bucket: aws.String(s.bucket),
		ACL:    s3.ObjectCannedACLPrivate,
		Key:    aws.String(path),
	}).Send(ctx)
	if err != nil {
		return "", err
	}

	size := 5 * 1024 * 1024
	buffer := make([]byte, size)
	buf := make([]byte, size)
	done := false
	var part int64 = 1
	parts := make([]s3.CompletedPart, 0)
	for !done {
		buffer = buffer[:0]
		readed := 0
		for readed < size && !done {
			nr, err := f.Read(buf)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					return "", err
				}
				logrus.Debugf("EOF reached at part %d", part)
				done = true
			}
			readed += nr
			buffer = append(buffer, buf[:nr]...)
		}
		if readed == 0 {
			continue
		}

		upResp, err := s.cli.UploadPartRequest(&s3.UploadPartInput{
			Body:       bytes.NewReader(buffer[:readed]),
			Bucket:     aws.String(s.bucket),
			Key:        aws.String(path),
			UploadId:   resp.UploadId,
			PartNumber: aws.Int64(part),
		}).Send(ctx)
		if err != nil {
			return "", err
		}

		parts = append(parts, s3.CompletedPart{
			ETag:       upResp.ETag,
			PartNumber: aws.Int64(part),
		})
		part++
	}

	_, err = s.cli.CompleteMultipartUploadRequest(&s3.CompleteMultipartUploadInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
		MultipartUpload: &s3.CompletedMultipartUpload{
			Parts: parts,
		},
		UploadId: resp.UploadId,
	}).Send(ctx)
	if err != nil {
		return "", err
	}
	return path, nil
}

func (s *S3Destenation) PublicURL(ctx context.Context, p string) (string, error) {
	req := s.cli.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(p),
	})
	return req.Presign(time.Hour)
}
