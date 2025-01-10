package vod

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v4/process"
)

type Stream struct {
	spec    StreamSpec
	context *Context
	format  string
	probe   *ProbeInfo
	bitrate int
	width   int
	height  int

	m      sync.Mutex
	chunks map[int]*tsChunk
	goal   int
	cmd    *exec.Cmd
	logger Logger
}

func newStream(spec StreamSpec, context *Context, info *ProbeInfo, logger Logger) *Stream {
	return &Stream{
		logger:  logger,
		spec:    spec,
		probe:   info,
		context: context,
		format:  context.contextConfig.Format,
		chunks:  map[int]*tsChunk{},
	}
}

func (s *Stream) Content() (io.ReadCloser, error) {
	switch s.format {
	case FormatMP4:
		return s.content()
	case FormatHLS:
		return s.contentHLS()
	}

	return nil, ErrInvalidFormat
}

func (s *Stream) Chunk(start, end int) (io.ReadCloser, error) {
	s.context.access()

	switch s.format {
	case FormatMP4:
	case FormatHLS:
		return s.serveChunk(start)
	}

	return nil, ErrInvalidFormat
}

func (s *Stream) ChunkLength() int {
	return len(s.generateChunks())
}

// Seek to the timestamp, expected used for HTTP range request
// TODO NOT IMPLEMENTED YET
//func (s *Stream) Seek(timestamp int64) (io.ReadCloser, error) {
//	return nil, nil
//}

// content return the FormatMP4 or FormatFLV content
func (s *Stream) content() (io.ReadCloser, error) {
	format := s.context.contextConfig.Format
	if !s.needTranscode() && s.probe.Format == format {
		file, err := os.Open(s.context.path)

		return file, err
	}

	args := s.buildFFMpegArgs(0, s.needTranscode(), format, true)
	contentCMD := exec.Command(s.context.contextConfig.FFMpegPath, args...)
	s.logger.Debugf("content command: %v", contentCMD.String())
	stdOut, err := contentCMD.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdErr, err := contentCMD.StderrPipe()
	if err != nil {
		return nil, err
	}

	go s.debugFFMpeg(stdErr)
	err = contentCMD.Start()
	if err != nil {
		return nil, err
	}

	return stdOut, nil
}

func (s *Stream) supportVideoCodec(codec string) bool {
	for _, c := range s.context.contextConfig.SupportVideoCodec {
		if c == codec {
			return true
		}
	}

	return false
}

func (s *Stream) supportAudioCodec(codec string) bool {
	for _, c := range s.context.contextConfig.SupportAudioCodec {
		if c == codec {
			return true
		}
	}

	return false
}

func (s *Stream) contentHLS() (io.ReadCloser, error) {
	buf := &bytes.Buffer{}
	buf.WriteString("#EXTM3U\n")
	buf.WriteString("#EXT-X-VERSION:4\n")
	buf.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")
	buf.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	// chunk max duration, this is a config value.
	buf.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", s.context.contextConfig.ChunkDuration))

	for i, c := range s.generateChunks() {
		buf.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", c.duration))
		buf.WriteString(s.context.contextConfig.TSGenerator(i, s, s.context))
	}

	buf.WriteString("#EXT-X-ENDLIST\n")

	return io.NopCloser(buf), nil
}

func (s *Stream) generateChunks() []tsChunk {
	duration := s.probe.Duration
	chunks := make([]tsChunk, 0)
	id := 0

	for duration > 0 {
		size := float64(s.context.contextConfig.ChunkDuration)
		if duration < size {
			size = duration
		}

		chunks = append(chunks, tsChunk{id: id, duration: size})
		id++
		duration -= size
	}

	return chunks
}

func (s *Stream) serveChunk(index int) (io.ReadCloser, error) {
	s.checkGoal(index)
	s.m.Lock()
	c, ok := s.chunks[index]
	s.m.Unlock()
	if ok {
		<-c.done

		return c, nil
	}
	// almost there, 3 should be configurable
	for i := index - 3; i < index; i++ {
		s.m.Lock()
		_, ok = s.chunks[i]
		s.m.Unlock()
		if ok {
			return s.waitForChunk(index)
		}
	}

	return s.restartAtChunk(index)
}

func (s *Stream) waitForChunk(index int) (io.ReadCloser, error) {
	s.m.Lock()
	chunk := s.chunks[index]
	if chunk == nil {
		chunk = &tsChunk{id: index, done: make(chan bool)}
		s.chunks[index] = chunk
	}
	s.m.Unlock()
	<-chunk.done

	return chunk, nil
}

func (s *Stream) restartAtChunk(index int) (io.ReadCloser, error) {
	s.stopProcess()
	s.goal = index + s.context.contextConfig.MaxBuffer

	args := s.buildFFMpegArgs(index, s.needTranscode(), s.context.contextConfig.Format, false)

	restartCMD := exec.Command(s.context.contextConfig.FFMpegPath, args...)
	s.logger.Debugf("restart command: %v", restartCMD.String())
	// Capture standard error
	stderr, err := restartCMD.StderrPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := restartCMD.StdoutPipe()
	if err != nil {
		return nil, err
	}

	s.m.Lock()
	c := &tsChunk{id: index, done: make(chan bool)}
	s.chunks[index] = c
	s.m.Unlock()

	if err = restartCMD.Start(); err != nil {
		return nil, err
	}

	go s.monitorChunk(stdout, stderr)
	go s.monitorProcess(restartCMD)
	s.cmd = restartCMD

	return s.waitForChunk(index)
}

func (s *Stream) needTranscode() bool {
	if s.spec.Force {
		return true
	}
	if !s.supportVideoCodec(s.probe.VideoCodec) || !s.supportAudioCodec(s.probe.AudioCodec) {
		return true
	}
	if s.spec.Width > 0 && s.spec.Width != s.probe.Width {
		return true
	}
	if s.spec.Height > 0 && s.spec.Height != s.probe.Height {
		return true
	}
	if s.spec.Bitrate > 0 && s.spec.Bitrate != s.bitrate {
		return true
	}
	if s.spec.Scale > 0 {
		return true
	}

	return false
}

var ErrNoChunkID = errors.New("no chunk id found")

func (s *Stream) resolveChunkID(line []byte) (int, string, error) {
	id := bytes.Index(line, []byte("segment:"))
	if id < 0 {
		return -1, "", ErrNoChunkID
	}
	start := id + len("segment:'")
	end := bytes.Index(line[start:], []byte("'"))
	if end < 0 {
		return -1, "", ErrNoChunkID
	}
	segment := string(line[start : start+end])
	parts := strings.Split(filepath.Base(segment), ".")
	if len(parts) != 2 {
		return -1, "", ErrNoChunkID
	}
	index := parts[0]
	if i, err := strconv.Atoi(index); err == nil {
		return i, segment, nil
	}

	return -1, "", ErrNoChunkID
}

func (s *Stream) monitorChunk(_ io.ReadCloser, stderr io.ReadCloser) {
	out := bufio.NewReader(stderr)

	for {
		line, err := out.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			s.logger.Infof("ffmpeg-error: %v", err)

			break
		}

		if !bytes.Contains(line, []byte(".ts")) || !bytes.Contains(line, []byte("ended")) {
			continue
		}
		// ffmpeg-error: [segment @ 0x15b004080] segment:'0.ts' count:0 ended
		// ffmpeg-error: [segment @ 0x146e05e50] segment:'/tmp/0.ts' count:0 ended
		id, segment, err := s.resolveChunkID(line)
		if err != nil {
			s.logger.Errorf("failed to resolve chunk id: %v", err)
			// todo should we break?
			continue
		}

		s.m.Lock()
		chunk, ok := s.chunks[id]

		if ok {
			chunk.path = segment
			s.logger.Infof("chunk %d is ready with file:%s", id, segment)
			close(chunk.done)
		} else {
			chunk = &tsChunk{id: id, path: segment, done: make(chan bool)}
			s.chunks[id] = chunk
			close(chunk.done)
		}
		if id >= s.goal {
			err = suspendProcess(s.cmd.Process.Pid)
			// pause the process
			if err != nil {
				s.logger.Error(err)
			}
		}

		for id := range s.chunks {
			if id < s.goal-s.context.contextConfig.MaxBuffer {
				co := s.chunks[id]
				co.destroy()
				delete(s.chunks, id)
			}
		}
		s.m.Unlock()
	}
}

// monitor unexpected exit
func (s *Stream) monitorProcess(cmd *exec.Cmd) {
	err := cmd.Wait()
	if err != nil {
		s.logger.Error(err)
	}
}

func (s *Stream) stopProcess() {
	s.m.Lock()
	chunks := s.chunks
	s.chunks = map[int]*tsChunk{}
	s.m.Unlock()

	for _, c := range chunks {
		c.destroy()
	}
	if s.cmd != nil {
		// TODO it may already exit, but we kill it anyway
		_ = s.cmd.Process.Kill()
		// TODO data race
		//_ = s.cmd.Wait()
		s.cmd = nil
	}
}

func (s *Stream) Close() error {
	// we don't remove the tmp file, let context to do it.
	s.stopProcess()
	return nil
}

func (s *Stream) checkGoal(index int) {
	goal := index + s.context.contextConfig.MinBuffer
	if goal > s.goal {
		s.goal = index + s.context.contextConfig.MaxBuffer
	}
	if s.cmd != nil {
		// TODO if there is an error, we should restart the process
		err := resumeProcess(s.cmd.Process.Pid)
		if err != nil {
			s.logger.Error("failed to resume ffmpeg process", err)
		}
	}
}

func suspendProcess(pid int) error {
	process, err := process.NewProcess(int32(pid))
	if err != nil {
		return err
	}

	return process.Suspend()
}

func resumeProcess(pid int) error {
	process, err := process.NewProcess(int32(pid))
	if err != nil {
		return err
	}

	return process.Resume()
}

func (s *Stream) debugFFMpeg(r io.ReadCloser) {
	br := bufio.NewReader(r)

	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			break
		}
		_ = line
	}
}

type TSGenerator func(index int, stream *Stream, context *Context) string

func DefaultTSGenerator(index int, stream *Stream, context *Context) string {
	return fmt.Sprintf("/video/ts?id=%s&index=%d&spec=%s\n", context.ID(), index, stream.spec.Name)
}

func formatTime(seconds int) string {
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60

	return fmt.Sprintf("%02d:%02d:%02d.000", h, m, s)
}
