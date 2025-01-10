package vod

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

var (
	Origin          = StreamSpec{Name: "Origin", Width: 0, Height: 0, Force: false}
	Compatible      = StreamSpec{Name: "Compatible", Width: 0, Height: 0, Force: true}
	Resolution2160P = StreamSpec{Name: "2160P", Width: 3840, Height: 2160, Force: false}
	Resolution1080P = StreamSpec{Name: "1080P", Width: 1920, Height: 1080, Force: false}

	Resolution1080P10M = StreamSpec{Name: "1080P10M", Width: 1920, Height: 1080, Force: false, Bitrate: 10000000}
	Resolution1080P5M  = StreamSpec{Name: "1080P5M", Width: 1920, Height: 1080, Force: false, Bitrate: 5000000}
	Resolution720P     = StreamSpec{Name: "720P", Width: 1280, Height: 720, Force: false}
	Resolution480P     = StreamSpec{Name: "480P", Width: 854, Height: 480, Force: false}
	Scale75            = StreamSpec{Name: "Scale75", Scale: 0.75, Force: false}
	Scale50            = StreamSpec{Name: "Scale50", Scale: 0.5, Force: false}
	Scale25            = StreamSpec{Name: "Scale25", Scale: 0.25, Force: false}
)

// StreamSpec is the spec for the stream.
// Width and Height is the target resolution.
// If the width and height are both 0, it means the original resolution.
// Force will force transcode.
// if Force is false, it will only transcode when needed.
// Let's say the original video is 480p, but the resolution is 720p, if Force is true, it will transcode the video to 720p,
// otherwise, it will ignore the spec.
// if scale and width are set both, the width and height will be ignored.
// if width/height does not fit the aspect ratio, the height will be adjusted.
type StreamSpec struct {
	Name    string
	Width   int
	Height  int
	Force   bool
	Bitrate int     // not used yet
	Scale   float64 // not used yet
}

type ContextConfig struct {
	Logger     Logger
	StreamSpec []StreamSpec // ffmpeg stream spec
	Format     string

	// TODO browser support codec
	SupportVideoCodec []string // default is h264
	SupportAudioCodec []string // default is aac

	// hls only config
	ListGenerator ListGenerator
	TSGenerator   TSGenerator
	ChunkDuration int
	MaxBuffer     int
	MinBuffer     int
	// TODO do we give user a callback when the context is idle for a while
	IdleTimeout int // in second

	// The config below should be service level
	// HWAccel is the hardware acceleration, default is auto
	// On Mac, it's VTB.
	// On Windows, TBD
	// On Linux, it's VAAPI > NV > None
	// Only set it to service so maybe should change
	HWAccel HWAccel // default is auto
	TmpPath string
	// Allowing the user to override the default tmp path and ffmpeg path
	FFMpegPath  string
	FFProbePath string
}

var (
	ErrEmptyConfig     = errors.New("config is nil")
	ErrFFMpegPath      = errors.New("ffmpeg path is empty")
	ErrFormat          = errors.New("format is empty")
	ErrListGenerator   = errors.New("list generator is nil")
	ErrTSGenerator     = errors.New("ts generator is nil")
	ErrChunkDuration   = errors.New("chunk duration is 0")
	ErrMaxBuffer       = errors.New("max buffer is 0")
	ErrMinBuffer       = errors.New("min buffer is 0")
	ErrTmpPath         = errors.New("tmp path is empty")
	ErrUnknownStrategy = errors.New("strategy is unknown")
	ErrStreamSpec      = errors.New("stream spec is empty")
)

func (c *ContextConfig) valid() error {
	if c == nil {
		return ErrEmptyConfig
	}

	if c.FFMpegPath == "" || c.FFProbePath == "" {
		return ErrFFMpegPath
	}

	if c.Format == "" {
		return ErrFormat
	}

	if c.Format == FormatHLS {
		switch {
		case c.ListGenerator == nil:
			return ErrListGenerator
		case c.TSGenerator == nil:
			return ErrTSGenerator
		case c.ChunkDuration == 0:
			return ErrChunkDuration
		case c.MaxBuffer == 0:
			return ErrMaxBuffer
		case c.MinBuffer == 0:
			return ErrMinBuffer
		case c.TmpPath == "":
			return ErrTmpPath
		}
	}

	if len(c.StreamSpec) == 0 {
		return ErrStreamSpec
	}

	return nil
}

func MimeType(format string) string {
	switch format {
	case FormatHLS:
		return "application/x-mpegURL"
	case FormatMP4:
		return "video/mp4"
	case FormatTS:
		return "video/MP2T"
	}

	return "application/octet-stream"
}

type Context struct {
	id            string
	contextConfig *ContextConfig
	streams       []*Stream
	path          string
	lastAccess    int64
	onClose       func(id string, reason CloseReason)
	closed        chan bool
	err           error
	info          *ProbeInfo
	logger        Logger
}

func newContext(id, path string, config *ContextConfig, info *ProbeInfo, onClose func(string, CloseReason), logger Logger) (*Context, error) {
	context := &Context{
		id:            id,
		path:          path,
		logger:        logger,
		lastAccess:    time.Now().Unix(),
		contextConfig: config,
		closed:        make(chan bool),
		onClose:       onClose,
		info:          info,
	}
	// TODO not valid config, seems to be valid in caller,
	// if true, consider move mkdir to caller too.
	if config.Format == FormatHLS {
		if err := os.MkdirAll(filepath.Join(config.TmpPath, id), os.ModePerm); err != nil {
			return nil, err
		}
	}

	streams := make([]*Stream, 0, len(config.StreamSpec))

	for _, spec := range config.StreamSpec {
		if fit(spec, info) {
			// spec.Bitrate = 0 calculate the bitrate
			// No adjustment needed
			// TODO codec may cause the bitrate to be different
			//if spec.Width == 0 && spec.Height == 0 && spec.Scale == 0 {
			//	spec.Bitrate = info.VideoBitrate
			//}
			streams = append(streams, newStream(adjustSpec(spec, info), context, info, logger))
		}
	}

	context.streams = streams
	if context.contextConfig.Format == FormatHLS {
		if context.contextConfig.IdleTimeout > 0 {
			go context.checkAlive()
		}
	}

	return context, nil
}

type CloseReason string

const (
	Normal = CloseReason("normal")
)

func (c *Context) access() {
	atomic.StoreInt64(&c.lastAccess, time.Now().Unix())
}

// Close all ffmpeg process, we don't monitor the process, Close must be called by the user
// TODO not called by user, multiple Close will cause error
func (c *Context) Close() error {
	select {
	case <-c.closed:
		return c.err
	default:
	}
	var err error
	for _, s := range c.streams {
		err = s.Close()
	}

	if err != nil {
		return err
	}

	_ = os.RemoveAll(filepath.Join(c.contextConfig.TmpPath, c.id))

	if c.onClose == nil {
		c.onClose(c.id, Normal)
	}

	close(c.closed)

	return nil
}

func (c *Context) MimeType() string {
	return MimeType(c.contextConfig.Format)
}

// Content return the content of the context
// if the format is HLS, it will return the m3u8 file
// otherwise, it will return the content of the first stream
func (c *Context) Content() (io.ReadCloser, error) {
	c.access()

	if c.contextConfig.Format == FormatHLS {
		if len(c.streams) == 1 {
			return c.streams[0].Content()
		}

		return c.multiStreamContent()
	}

	return c.streams[0].Content()
}

func (c *Context) Stream(name string) *Stream {
	c.access()

	for _, s := range c.streams {
		if s.spec.Name == name {
			return s
		}
	}

	return nil
}

func (c *Context) multiStreamContent() (io.ReadCloser, error) {
	buf := &bytes.Buffer{}
	buf.WriteString("#EXTM3U\n")

	for i, s := range c.streams {
		buf.WriteString(DefaultListGenerator(i, s))
	}

	buf.WriteString("#EXT-X-ENDLIST\n")

	return io.NopCloser(buf), nil
}

func (c *Context) ID() string {
	return c.id
}

func (c *Context) ProbeInfo() *ProbeInfo {
	return c.info
}

const (
	defaultChunkDuration = 6
	defaultMaxBuffer     = 10
	defaultMinBuffer     = 3
)

type ListGenerator func(index int, stream *Stream) string

func DefaultListGenerator(index int, stream *Stream) string {
	// TODO bitrate w,d should not be empty
	l := fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d,CODECS=\"avc1.42e00a,mp4a.40.2\"\n", index*5000, stream.width, stream.height)
	l = l + fmt.Sprintf("index.m3u8?spec=%s\n", stream.spec.Name)

	return l
}

func fit(spec StreamSpec, info *ProbeInfo) bool {
	if spec.Force {
		return true
	}

	if spec.Width == 0 && spec.Height == 0 {
		return true
	}

	if spec.Width < info.Width {
		return true
	}

	return false
}

var ErrIdleTimeout = errors.New("context is idle")

func (c *Context) checkAlive() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// TODO consider using service to check, save a lot of goroutine
	for {
		select {
		case <-ticker.C:
			now := time.Now().Unix()
			if now-atomic.LoadInt64(&c.lastAccess) > int64(c.contextConfig.IdleTimeout) {
				c.err = ErrIdleTimeout
				_ = c.Close()
			}
		case <-c.closed:
			ticker.Stop()
			return
		}
	}
}

func adjustSpec(spec StreamSpec, info *ProbeInfo) StreamSpec {
	if spec.Force && spec.Bitrate > 0 {
		// they know what they are doing, we ignore the spec.
		return spec
	}

	originBitrate := info.VideoBitrate

	// try give it more bitrate if it's not h264
	if info.VideoCodec != "h264" {
		originBitrate = originBitrate * 2
	}

	// keep the original resolution
	if spec.Width == 0 && spec.Height == 0 && spec.Scale == 0 {
		return spec
	}
	// 19395
	// we always keep the aspect ratio
	switch {
	case spec.Scale != 0:
		spec.Width = int(float64(info.Width) * spec.Scale)
		spec.Height = int(float64(info.Height) * spec.Scale)
	case spec.Width != 0:
		spec.Scale = float64(spec.Width) / float64(info.Width)
		spec.Height = int(float64(info.Height) * spec.Scale)
	case spec.Height != 0:
		spec.Scale = float64(spec.Height) / float64(info.Height)
		spec.Width = int(float64(info.Width) * spec.Scale)
	}

	if spec.Height%2 != 0 {
		spec.Height++
	}

	if spec.Width%2 != 0 {
		spec.Width++
	}

	spec.Bitrate = (spec.Width * spec.Height) / (info.Width * info.Height) * originBitrate

	return spec
}
