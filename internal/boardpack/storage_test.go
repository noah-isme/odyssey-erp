package boardpack

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

func TestLocalStorageSaveAndOpen(t *testing.T) {
	storage, err := NewStorage(t.Context(), StorageConfig{Driver: "local", LocalDir: t.TempDir()})
	require.NoError(t, err)

	key, err := storage.Save(t.Context(), 42, []byte("pdf-data"))
	require.NoError(t, err)

	file, err := storage.Open(t.Context(), key)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })
	content, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, []byte("pdf-data"), content)
}

func TestS3StorageSaveAndOpen(t *testing.T) {
	client := &fakeS3{objects: make(map[string]string)}
	storage := &s3Storage{client: client, bucket: "boardpacks"}

	key, err := storage.Save(t.Context(), 7, []byte("generated-pdf"))
	require.NoError(t, err)
	require.Equal(t, "board-packs/board-pack-7.pdf", key)

	file, err := storage.Open(t.Context(), key)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })
	content, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, "generated-pdf", string(content))
}

type fakeS3 struct {
	objects map[string]string
}

func (f *fakeS3) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	content, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	f.objects[aws.ToString(input.Key)] = string(content)
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(f.objects[aws.ToString(input.Key)]))}, nil
}

func (f *fakeS3) HeadBucket(context.Context, *s3.HeadBucketInput, ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	return &s3.HeadBucketOutput{}, nil
}

func (f *fakeS3) CreateBucket(context.Context, *s3.CreateBucketInput, ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	return &s3.CreateBucketOutput{}, nil
}
