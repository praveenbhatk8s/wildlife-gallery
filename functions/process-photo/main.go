package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Larger originals are rejected rather than risking an out-of-memory crash.
const maxOriginalBytes = 80 << 20

// Photo is the DynamoDB item.
type Photo struct {
	PhotoID     string `dynamodbav:"photoId"`
	Album       string `dynamodbav:"album"`
	SortKey     string `dynamodbav:"sortKey"` // takenAt if known, else processedAt; GSI range key
	OriginalKey string `dynamodbav:"originalKey"`
	ThumbKey    string `dynamodbav:"thumbKey"`
	WebKey      string `dynamodbav:"webKey"`
	Width       int    `dynamodbav:"width"`
	Height      int    `dynamodbav:"height"`
	ProcessedAt string `dynamodbav:"processedAt"`
	Metadata
}

type handler struct {
	s3            *s3.Client
	ddb           *dynamodb.Client
	derivedBucket string
	table         string
	log           *slog.Logger
}

func (h *handler) Handle(ctx context.Context, evt events.S3Event) error {
	var errs []error
	for _, rec := range evt.Records {
		if err := h.processRecord(ctx, rec); err != nil {
			errs = append(errs, err)
		}
	}
	// Returning an error makes Lambda retry the event, then send it to the DLQ.
	return errors.Join(errs...)
}

func (h *handler) processRecord(ctx context.Context, rec events.S3EventRecord) error {
	bucket := rec.S3.Bucket.Name
	key, err := url.QueryUnescape(rec.S3.Object.Key) // event keys are URL-encoded ("+" for spaces)
	if err != nil {
		return fmt.Errorf("unescape key %q: %w", rec.S3.Object.Key, err)
	}
	log := h.log.With("bucket", bucket, "key", key)
	start := time.Now()

	if rec.S3.Object.Size > maxOriginalBytes {
		log.Warn("skipping oversized original", "bytes", rec.S3.Object.Size)
		return nil
	}

	obj, err := h.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return fmt.Errorf("get %s: %w", key, err)
	}
	original, err := io.ReadAll(io.LimitReader(obj.Body, maxOriginalBytes+1))
	obj.Body.Close()
	if err != nil {
		return fmt.Errorf("read %s: %w", key, err)
	}

	id := PhotoID(bucket, key)
	meta := ExtractMetadata(original)
	rendered, size, err := RenderVariants(original)
	if err != nil {
		return fmt.Errorf("render %s: %w", key, err)
	}

	keys := map[string]string{}
	for _, r := range rendered {
		k := fmt.Sprintf("photos/%s/%s.jpg", id, r.Name)
		_, err := h.s3.PutObject(ctx, &s3.PutObjectInput{
			Bucket:       &h.derivedBucket,
			Key:          &k,
			Body:         bytes.NewReader(r.Body),
			ContentType:  aws.String("image/jpeg"),
			CacheControl: aws.String("public, max-age=31536000, immutable"),
		})
		if err != nil {
			return fmt.Errorf("put %s: %w", k, err)
		}
		keys[r.Name] = k
	}

	now := time.Now().UTC().Format(time.RFC3339)
	sortKey := meta.TakenAt
	if sortKey == "" {
		sortKey = now
	}
	item, err := attributevalue.MarshalMap(Photo{
		PhotoID: id, Album: Album(key), SortKey: sortKey, OriginalKey: key,
		ThumbKey: keys["thumb"], WebKey: keys["web"],
		Width: size.X, Height: size.Y, ProcessedAt: now, Metadata: meta,
	})
	if err != nil {
		return fmt.Errorf("marshal item: %w", err)
	}
	if _, err := h.ddb.PutItem(ctx, &dynamodb.PutItemInput{TableName: &h.table, Item: item}); err != nil {
		return fmt.Errorf("put item %s: %w", id, err)
	}

	log.Info("photo processed", "photoId", id, "album", Album(key), "hadGps", meta.HadGPS,
		"originalBytes", len(original), "durationMs", time.Since(start).Milliseconds())
	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Error("load aws config", "err", err)
		os.Exit(1)
	}
	h := &handler{
		s3:            s3.NewFromConfig(cfg),
		ddb:           dynamodb.NewFromConfig(cfg),
		derivedBucket: os.Getenv("DERIVED_BUCKET"),
		table:         os.Getenv("TABLE_NAME"),
		log:           logger,
	}
	lambda.Start(h.Handle)
}
