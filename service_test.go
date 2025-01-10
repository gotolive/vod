package vod

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	m.Run()
}

// This test requires ffmpeg and ffprobe to be installed on the system.
func TestNewServiceReturnsServiceInstance(t *testing.T) {

	config := ContextConfig{
		Format:        FormatHLS,
		ChunkDuration: 10,
		MaxBuffer:     100,
		MinBuffer:     10,
		HWAccel:       HWAccelAuto,
	}

	service, err := NewService(config)
	assert.NoError(t, err)
	assert.NotNil(t, service)
	assert.NoError(t, service.Stop())
}

func TestCreateContextReturnsContextInstance(t *testing.T) {
	config := ContextConfig{
		Format:        FormatHLS,
		ChunkDuration: 10,
		MaxBuffer:     100,
		MinBuffer:     10,
		HWAccel:       HWAccelAuto,
	}
	service, err := NewService(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	context, err := service.CreateContext(fmt.Sprintf("%s-%d", t.Name(), time.Now().Unix()), "testdata/test.flv", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if context == nil {
		t.Error("expected context instance, got nil")
	}

	content, err := context.Content()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if content == nil {
		t.Error("expected content instance, got nil")
	}
	assert.NoError(t, service.Stop())
}

func TestCreateContextMP4(t *testing.T) {
	config := ContextConfig{
		Format:  FormatMP4,
		HWAccel: HWAccelAuto,
	}
	service, err := NewService(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	context, err := service.CreateContext(fmt.Sprintf("%s-%d", t.Name(), time.Now().Unix()), "testdata/test.flv", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if context == nil {
		t.Error("expected context instance, got nil")
	}

	content, err := context.Content()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if content == nil {
		t.Error("expected content instance, got nil")
	}
	defer content.Close()
	defer content.Close()
	var c []byte
	buf := bytes.NewBuffer(c)
	_, err = io.Copy(buf, content)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected content to be written, got empty")
	}
	assert.NoError(t, context.Close())
	assert.NoError(t, service.Stop())
}

func TestCreateContextHLS(t *testing.T) {
	config := ContextConfig{
		Format:  FormatHLS,
		HWAccel: HWAccelAuto,
	}
	service, err := NewService(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	context, err := service.CreateContext(fmt.Sprintf("%s-%d", t.Name(), time.Now().Unix()), "testdata/test.flv", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if context == nil {
		t.Error("expected context instance, got nil")
	}
	assert.Equal(t, "application/x-mpegURL", context.MimeType())
	content, err := context.Content()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if content == nil {
		t.Error("expected content instance, got nil")
	}

	defer content.Close()
	defer content.Close()
	var c []byte
	buf := bytes.NewBuffer(c)
	_, err = io.Copy(buf, content)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected content to be written, got empty")
	}

	stream := context.Stream("Origin")
	if stream == nil {
		t.Error("expected stream instance, got nil")
	}
	ch, err := stream.Chunk(0, 0)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	chunkBuf := &bytes.Buffer{}
	_, err = io.Copy(chunkBuf, ch)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if chunkBuf.Len() == 0 {
		t.Error("expected chunk to be written, got empty")
	}
	assert.NoError(t, context.Close())
	assert.NoError(t, service.Stop())
}

func TestCreateContextHLSWithIdle(t *testing.T) {
	config := ContextConfig{
		Format:      FormatHLS,
		HWAccel:     HWAccelAuto,
		IdleTimeout: 2, // TODO default value does not work
	}
	service, err := NewService(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	context, err := service.CreateContext(fmt.Sprintf("%s-%d", t.Name(), time.Now().Unix()), "testdata/test.flv", &config)
	assert.NoError(t, err)
	assert.NotNil(t, context)
	time.Sleep(time.Second * 5)
	assert.Equal(t, ErrIdleTimeout, context.Close())
	// TODO close service will have error
}

func TestMultiStreamMP4CreatesMultipleStreams(t *testing.T) {
	config := ContextConfig{
		Format:  FormatMP4,
		HWAccel: HWAccelAuto,
		StreamSpec: []StreamSpec{
			Origin,
			{
				Name:  "Force",
				Force: true,
			},
		},
	}
	service, err := NewService(config)
	assert.NoError(t, err)

	context, err := service.CreateContext(fmt.Sprintf("%s-%d", t.Name(), time.Now().Unix()), "testdata/test.flv", nil)
	assert.NoError(t, err)
	assert.NotNil(t, context)

	content, err := context.Content()
	assert.NoError(t, err)
	assert.NotNil(t, content)

	assert.Equal(t, context.MimeType(), "video/mp4")

	stream1 := context.Stream("Origin")
	assert.NotNil(t, stream1)
	stream2 := context.Stream("Force")
	assert.NotNil(t, stream2)
	assert.NoError(t, context.Close())
	assert.NoError(t, service.Stop())
}

func TestMultiStreamHLSCreatesMultipleStreams(t *testing.T) {
	config := ContextConfig{
		Format:        FormatHLS,
		ChunkDuration: 10,
		MaxBuffer:     100,
		MinBuffer:     10,
		StreamSpec: []StreamSpec{
			Origin,
			{
				Name:  "Force",
				Force: true,
			},
		},
	}
	service, err := NewService(config)
	assert.NoError(t, err)

	context, err := service.CreateContext(fmt.Sprintf("%s-%d", t.Name(), time.Now().Unix()), "testdata/test.flv", nil)
	assert.NoError(t, err)
	assert.NotNil(t, context)
	content, err := context.Content()
	assert.NoError(t, err)
	assert.NotNil(t, content)
	stream1 := context.Stream("Origin")
	assert.NotNil(t, stream1)
	stream2 := context.Stream("Force")
	assert.NotNil(t, stream2)
	assert.NoError(t, context.Close())
	assert.NoError(t, service.Stop())
}

func TestCreateContextReturnsErrorForInvalidPath(t *testing.T) {
	config := ContextConfig{
		Format:        FormatHLS,
		ChunkDuration: 10,
		MaxBuffer:     100,
		MinBuffer:     10,
		HWAccel:       HWAccelAuto,
	}
	service, err := NewService(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	context, err := service.CreateContext(fmt.Sprintf("%s-%d", t.Name(), time.Now().Unix()), "/invalid/path/to/video.mp4", nil)
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
	if context != nil {
		t.Error("expected nil context, got instance")
	}
	assert.NoError(t, service.Stop())
}

func TestCreateContextReturnsErrorForNonMediaFile(t *testing.T) {
	config := ContextConfig{
		Format:        FormatHLS,
		ChunkDuration: 10,
		MaxBuffer:     100,
		MinBuffer:     10,
		HWAccel:       HWAccelAuto,
	}
	service, err := NewService(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	_, err = service.Probe("testdata/test.txt")
	// TODO it's only return exit status 1, may ffmpeg error
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
	assert.NoError(t, service.Stop())
}

func TestProbeReturnsProbeInfo(t *testing.T) {
	config := ContextConfig{
		Format:        FormatHLS,
		ChunkDuration: 10,
		MaxBuffer:     100,
		MinBuffer:     10,
		HWAccel:       HWAccelAuto,
	}

	service, err := NewService(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	probeInfo, err := service.Probe("testdata/test.flv")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if probeInfo == nil {
		t.Error("expected probe info, got nil")
	}
	if probeInfo.Format != "flv" {
		t.Errorf("expected format mp4, got %s", probeInfo.Format)
	}
	assert.NoError(t, service.Stop())
}

func TestProbeReturnsErrorForInvalidPath(t *testing.T) {
	config := ContextConfig{
		Format:        FormatHLS,
		ChunkDuration: 10,
		MaxBuffer:     100,
		MinBuffer:     10,
		HWAccel:       HWAccelAuto,
	}

	service, err := NewService(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	_, err = service.Probe("/invalid/path/to/video.mp4")
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
	assert.NoError(t, service.Stop())
}

func TestProbeReturnsErrorForNoVideo(t *testing.T) {
	config := ContextConfig{
		Format:        FormatHLS,
		ChunkDuration: 10,
		MaxBuffer:     100,
		MinBuffer:     10,
		HWAccel:       HWAccelAuto,
	}

	service, err := NewService(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	_, err = service.Probe("testdata/test.mp3")
	if !errors.Is(err, ErrNoVideoFound) {
		t.Errorf("expected ErrStreamNotFound, got %v", err)
	}
	assert.NoError(t, service.Stop())
}

func TestProbeParsesVideoStreamInfo(t *testing.T) {
	config := ContextConfig{
		Format:        FormatHLS,
		ChunkDuration: 10,
		MaxBuffer:     100,
		MinBuffer:     10,
		HWAccel:       HWAccelAuto,
	}

	service, err := NewService(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	probeInfo, err := service.Probe("testdata/test.flv")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if probeInfo.VideoCodec != "h264" {
		t.Error("expected video codec, got empty")
	}
	if probeInfo.Width != 768 || probeInfo.Height != 320 {
		t.Error("expected non-zero width and height")
	}
	assert.NoError(t, service.Stop())
}

func TestProbeParsesAudioStreamInfo(t *testing.T) {
	config := ContextConfig{
		Format:        FormatHLS,
		ChunkDuration: 10,
		MaxBuffer:     100,
		MinBuffer:     10,
		HWAccel:       HWAccelAuto,
	}

	service, err := NewService(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	probeInfo, err := service.Probe("testdata/test.flv")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if probeInfo.AudioCodec != "aac" {
		t.Error("expected audio codec, got empty")
	}
	assert.NoError(t, service.Stop())
}
