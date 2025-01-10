package vod

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

var ErrFFMpegNotFound = errors.New("ffmpeg not found")

const (
	ffmpeg  = "ffmpeg"
	ffprobe = "ffprobe"
)

func NewService(config ContextConfig) (*Service, error) {
	if config.Logger == nil {
		config.Logger = NewEmptyLogger()
	}
	logger := config.Logger

	err := resolveFFMpeg(&config)
	if err != nil {
		return nil, err
	}
	if config.Format == FormatHLS {
		setHLSDefaultValue(&config)
	}

	accel := probeHWAccel(config.FFMpegPath, config.HWAccel)
	if accel != config.HWAccel {
		if config.HWAccel == HWAccelAuto {
			logger.Infof("auto detected hardware acceleration: %s", accel)
		} else {
			logger.Infof("hardware acceleration changed from %s to %s", config.HWAccel, accel)
		}

		config.HWAccel = accel
	}

	if len(config.SupportVideoCodec) == 0 {
		config.SupportVideoCodec = []string{"h264"}
	}
	if len(config.SupportAudioCodec) == 0 {
		config.SupportAudioCodec = []string{"aac"}
	}

	if config.TmpPath == "" {
		config.TmpPath = filepath.Join(os.TempDir(), "fily-vod")
	}
	// We will clean the tmp path. So if have multiple services, should use different tmp path.
	_ = os.RemoveAll(config.TmpPath)
	_ = os.MkdirAll(config.TmpPath, os.ModePerm)

	if len(config.StreamSpec) == 0 {
		config.StreamSpec = []StreamSpec{
			Origin,
		}
	}
	if config.Logger == nil {
		config.Logger = NewEmptyLogger()
	}

	return &Service{
		logger:   logger,
		config:   config,
		contexts: make(map[string]*Context),
	}, nil
}

func setHLSDefaultValue(config *ContextConfig) {
	if config.ListGenerator == nil {
		config.ListGenerator = DefaultListGenerator
	}
	if config.TSGenerator == nil {
		config.TSGenerator = DefaultTSGenerator
	}
	if config.ChunkDuration == 0 {
		config.ChunkDuration = defaultChunkDuration
	}

	if config.MaxBuffer == 0 {
		config.MaxBuffer = defaultMaxBuffer
	}
	if config.MinBuffer == 0 {
		config.MinBuffer = defaultMinBuffer
	}
}

func resolveFFMpeg(config *ContextConfig) error {
	if config.FFMpegPath == "" {
		config.FFMpegPath = findExecutable(ffmpeg)
	}
	if config.FFProbePath == "" {
		config.FFProbePath = findExecutable(ffprobe)
	}
	if config.FFMpegPath == "" || config.FFProbePath == "" {
		return ErrFFMpegNotFound
	}
	// get the version and check the path is valid
	_, err := ffmpegVersion(config.FFMpegPath)
	if err != nil {
		return err
	}
	_, err = ffprobeVersion(config.FFProbePath)
	if err != nil {
		return err
	}

	return nil
}

type Service struct {
	m        sync.Mutex
	contexts map[string]*Context
	config   ContextConfig
	logger   Logger
}

const (
	FormatMP4 = "mp4"
	FormatHLS = "hls"
	FormatTS  = "ts"
)

func supportedFormat(format string) bool {
	return format == FormatMP4 || format == FormatHLS || format == FormatTS
}

var ErrInvalidFormat = errors.New("invalid format")

func (s *Service) CreateContext(id, path string, config *ContextConfig) (*Context, error) {
	config = s.mergeConfig(config)
	if err := config.valid(); err != nil {
		return nil, err
	}

	if !supportedFormat(config.Format) {
		return nil, ErrInvalidFormat
	}

	info, err := s.Probe(path)
	if err != nil {
		return nil, err
	}

	// even if the same path, we still create a new context
	config.TmpPath = filepath.Join(config.TmpPath, id)
	_ = os.RemoveAll(config.TmpPath)
	err = os.MkdirAll(config.TmpPath, os.ModePerm)
	if err != nil {
		return nil, err
	}
	context, err := newContext(id, path, config, info, s.stopContext, s.logger)
	if err != nil {
		return nil, err
	}

	s.m.Lock()
	s.contexts[id] = context
	s.m.Unlock()

	return context, nil
}

func (s *Service) mergeConfig(config *ContextConfig) *ContextConfig {
	if config == nil {
		config = &ContextConfig{}
	}
	if config.ListGenerator == nil {
		config.ListGenerator = s.config.ListGenerator
	}
	if config.TSGenerator == nil {
		config.TSGenerator = s.config.TSGenerator
	}
	if config.TmpPath == "" {
		config.TmpPath = s.config.TmpPath
	}
	if config.FFMpegPath == "" {
		config.FFMpegPath = s.config.FFMpegPath
	}
	if config.FFProbePath == "" {
		config.FFProbePath = s.config.FFProbePath
	}
	if config.ChunkDuration == 0 {
		config.ChunkDuration = s.config.ChunkDuration
	}
	if config.Format == "" {
		config.Format = s.config.Format
	}
	if config.StreamSpec == nil {
		config.StreamSpec = s.config.StreamSpec
	}
	if len(config.SupportVideoCodec) == 0 {
		config.SupportVideoCodec = s.config.SupportVideoCodec
	}
	if len(config.SupportAudioCodec) == 0 {
		config.SupportAudioCodec = s.config.SupportAudioCodec
	}
	if config.HWAccel == HWAccelAuto {
		config.HWAccel = s.config.HWAccel
	}
	if config.MaxBuffer == 0 {
		config.MaxBuffer = s.config.MaxBuffer
	}
	if config.MinBuffer == 0 {
		config.MinBuffer = s.config.MinBuffer
	}

	return config
}

type AudioTrack struct {
	Index int
	Codec string
	Key   string
}

type ProbeInfo struct {
	Duration     float64
	VideoBitrate int
	AudioBitrate int
	Bitrate      int
	HasBFrame    bool
	FrameRate    float64
	Width        int
	Height       int
	AudioCodec   string
	VideoCodec   string

	// AudioBitrate int // Not always available
	Format      string
	AudioTracks []AudioTrack
}

func (s *Service) Stop() error {
	s.m.Lock()
	contexts := s.contexts
	s.m.Unlock()

	for _, c := range contexts {
		err := c.Close()
		if err != nil {
			return err
		}
	}

	_ = os.RemoveAll(s.config.TmpPath)

	return nil
}

var (
	ErrStreamNotFound = errors.New("stream not found")
	ErrNoVideoFound   = errors.New("no video stream found")
)

func (s *Service) Probe(path string) (*ProbeInfo, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	args := []string{
		"-v", "error", "-show_entries", "format:stream", "-of", "json", path,
	}

	probeCmd := exec.Command(s.config.FFProbePath, args...)
	s.logger.Debugf("Running command: %s", probeCmd.String())
	output, err := probeCmd.Output()
	if err != nil {
		return nil, err
	}
	info := &ProbeResult{}
	err = json.Unmarshal(output, info)
	if err != nil {
		return nil, err
	}

	if len(info.Streams) == 0 {
		return nil, ErrNoVideoFound
	}

	return s.resolveProbeResult(info)
}

func (s *Service) resolveProbeResult(info *ProbeResult) (*ProbeInfo, error) {
	var (
		videoCount int
		err        error
		probe      ProbeInfo
	)
	// TODO should be for range streams here
	videoCount = s.resolveProbeStream(info, videoCount, &probe)
	if videoCount == 0 {
		return nil, ErrNoVideoFound
	}

	bitrate, err := strconv.Atoi(info.Format.BitRate)
	if err != nil {
		s.logger.Warnf("failed to parse bitrate: %v", err)
	}
	// if format has no duration, try use the first video stream duration
	duration, err := strconv.ParseFloat(info.Format.Duration, 64)
	if err != nil {
		return nil, err
	}

	probe.Duration = duration
	probe.Format = info.Format.FormatName
	probe.Bitrate = bitrate

	return &probe, nil
}

func (s *Service) resolveProbeStream(info *ProbeResult, videoCount int, probe *ProbeInfo) int {
	for _, stream := range info.Streams {
		// if we have more than one video stream, we only use the first one.
		// Some video files have multiple video streams, one is the main video stream, the other is the thumbnail.
		if stream.CodecType == "video" {
			videoCount++
			if videoCount > 1 {
				continue
			}
			probe.HasBFrame = stream.HasBFrames > 0
			// we try
			if bitrate, err := strconv.Atoi(stream.BitRate); err == nil && bitrate > 0 {
				probe.VideoBitrate = bitrate
			}
			probe.VideoCodec = stream.CodecName
			probe.Width = stream.Width
			probe.Height = stream.Height
			probe.FrameRate = s.resolveFrameRate(stream.RFrameRate)
			if probe.FrameRate == 0 {
				probe.FrameRate = s.resolveFrameRate(stream.AvgFrameRate)
			}
		}
		if stream.CodecType == "audio" {
			probe.AudioCodec = stream.CodecName
		}
	}

	return videoCount
}

func probeHWAccel(ffmpeg string, accel HWAccel) HWAccel {
	switch accel {
	case HWAccelNone:
		return HWAccelNone
	case HWAccelAuto:
		switch runtime.GOOS {
		case "linux":
			for _, a := range []HWAccel{HWAccelQSV, HWAccelVAAPI, HWAccelNVENC, HWAccelAMF} {
				if allHWInfos[a].detector(ffmpeg) {
					return a
				}
			}
		case "darwin":
			// on Mac, vtb should be supported.
			if allHWInfos[HWAccelVTB].detector(ffmpeg) {
				return HWAccelVTB
			}
		case "windows":
			for _, a := range []HWAccel{HWAccelQSV, HWAccelNVENC, HWAccelAMF} {
				if allHWInfos[a].detector(ffmpeg) {
					return a
				}
			}
		}
	default:
		if allHWInfos[accel].detector(ffmpeg) {
			return accel
		}
	}

	return HWAccelNone
}

// Why do we keep the contexts any way.
func (s *Service) stopContext(id string, normal CloseReason) {
	s.m.Lock()
	defer s.m.Unlock()
	delete(s.contexts, id)
}

// TODO could be nil, should we return error?
func (s *Service) Context(id string) *Context {
	s.m.Lock()
	defer s.m.Unlock()

	return s.contexts[id]
}

//nolint:tagliatelle
type ProbeResult struct {
	Streams []struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		BitRate      string `json:"bit_rate"`
		Width        int    `json:"width,omitempty"`
		Height       int    `json:"height,omitempty"`
		HasBFrames   int    `json:"has_b_frames,omitempty"`
		RFrameRate   string `json:"r_frame_rate"`
		AvgFrameRate string `json:"avg_frame_rate"`
	} `json:"streams"`
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
}

func (s *Service) resolveFrameRate(rate string) float64 {
	parts := strings.Split(rate, "/")
	if len(parts) == 2 {
		num, err := strconv.Atoi(parts[0])
		if err != nil {
			s.logger.Warnf("failed to parse frame rate: %v", err)
		}
		den, err := strconv.Atoi(parts[1])
		if err != nil {
			s.logger.Warnf("failed to parse frame rate: %v", err)
		}

		return float64(num) / float64(den)
	}

	return 0
}

func ffmpegVersion(path string) (string, error) {
	out, err := exec.Command(path, "-version").Output()
	if err != nil {
		return "", err
	}

	return string(out), nil
}

func ffprobeVersion(path string) (string, error) {
	out, err := exec.Command(path, "-version").Output()
	if err != nil {
		return "", err
	}

	return string(out), nil
}
