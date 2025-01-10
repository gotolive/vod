package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gotolive/vod"
)

var (
	videoService *vod.Service
)

func main() {
	var err error
	videoService, err = vod.NewService(vod.ContextConfig{
		//FFMpegPath:        "D:\\ffmpeg.exe",
		//FFProbePath:       "D:\\ffprobe.exe",
		Logger:            vod.NewSTDLogger(),
		Format:            vod.FormatHLS,
		SupportVideoCodec: []string{"h264"},
		IdleTimeout:       30,
	})
	if err != nil {
		panic(err)
	}
	http.HandleFunc("/play", play)
	http.HandleFunc("/video/index.m3u8", m3u8)
	http.HandleFunc("/video/ts", ts)
	http.HandleFunc("/video/mp4", mp4)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "examples/index.html")
	})
	s := http.Server{
		Addr:        ":18081",
		ReadTimeout: time.Minute,
		Handler:     http.DefaultServeMux,
	}
	if err = s.ListenAndServe(); err != nil {
		panic(err)
	}
}

type Request struct {
	Force   bool   `json:"force"`
	HWAccel int    `json:"hwaccel"`
	Path    string `json:"path"`
	Hevc    bool   `json:"hevc"`
	Format  string `json:"format"`
}
type Response struct {
	ID        string        `json:"id"`
	HWAccel   string        `json:"hwAccel"`
	URL       string        `json:"url"`
	Transcode bool          `json:"transcode"`
	Error     string        `json:"error"`
	Format    string        `json:"format"`
	Info      vod.ProbeInfo `json:"info"`
}

func play(writer http.ResponseWriter, request *http.Request) {
	var req Request
	var res Response
	err := json.NewDecoder(request.Body).Decode(&req)
	if err != nil {
		res.Error = err.Error()
		json.NewEncoder(writer).Encode(&res)
	}
	id := strconv.FormatInt(time.Now().UnixMilli(), 10)
	config := vod.ContextConfig{
		Format:  req.Format,
		HWAccel: vod.HWAccel(req.HWAccel),
	}
	if req.Hevc {
		config.SupportVideoCodec = []string{"h264", "hevc"}
	}
	if req.Force {
		config.StreamSpec = []vod.StreamSpec{
			{
				Name:  "force",
				Force: true,
			},
		}
	}
	ctx, err := videoService.CreateContext(id, req.Path, &config)
	if err != nil {
		res.Error = err.Error()
		json.NewEncoder(writer).Encode(&res)
		return
	}
	res.ID = ctx.ID()
	res.Format = req.Format
	if req.Format == "mp4" {
		res.URL = "/video/mp4?id=" + res.ID
	} else {
		res.URL = "/video/index.m3u8?id=" + res.ID
	}

	json.NewEncoder(writer).Encode(&res)
	return
}

func ts(writer http.ResponseWriter, request *http.Request) {
	var err error
	queries := request.URL.Query()
	id := queries.Get("id")
	context := videoService.Context(id)
	if context == nil {
		_, _ = writer.Write([]byte("context is nil"))

		return
	}

	spec := queries.Get("spec")
	stream := context.Stream(spec)
	if stream == nil {
		_, _ = writer.Write([]byte("stream not found"))

		return
	}

	indexStr := request.URL.Query().Get("index")
	if indexStr == "" {
		_, _ = writer.Write([]byte("index is required"))

		return
	}

	var index int
	index, err = strconv.Atoi(indexStr)

	if err != nil {
		_, _ = writer.Write([]byte(err.Error()))

		return
	}

	var readCloser io.ReadCloser
	readCloser, err = stream.Chunk(index, index)

	defer func() {
		_ = readCloser.Close()
	}()

	if err != nil {
		_, _ = writer.Write([]byte(err.Error()))

		return
	}

	writer.Header().Set("Content-Type", "video/MP2T")
	_, _ = io.Copy(writer, readCloser)
}

func m3u8(writer http.ResponseWriter, request *http.Request) {

	queries := request.URL.Query()
	spec := queries.Get("spec")
	id := queries.Get("id")
	c := videoService.Context(id)
	if c == nil {
		_, _ = writer.Write([]byte("context is nil"))

		return
	}

	if spec != "" {
		stream := c.Stream(spec)
		if stream == nil {
			_, _ = writer.Write([]byte("stream not found"))

			return
		}
		content, err := stream.Content()
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))

			return
		}

		writer.Header().Set("Content-Type", c.MimeType())
		_, _ = io.Copy(writer, content)

		return
	} else {
		content, err := c.Content()
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))

			return
		}

		writer.Header().Set("Content-Type", c.MimeType())
		_, _ = io.Copy(writer, content)
	}
}

func mp4(writer http.ResponseWriter, request *http.Request) {
	var err error
	queries := request.URL.Query()
	id := queries.Get("id")
	context := videoService.Context(id)
	if context == nil {
		_, _ = writer.Write([]byte("context is nil"))

		return
	}

	var readCloser io.ReadCloser
	readCloser, err = context.Content()
	if err != nil {
		_, _ = writer.Write([]byte(err.Error()))

		return
	}

	defer readCloser.Close()
	writer.Header().Set("Content-Type", context.MimeType())
	const (
		bufLen = 1024 * 1024 // 1Mb
	)
	buf := make([]byte, bufLen)
	stdoutReader := bufio.NewReader(readCloser)
	flusher, ok := writer.(http.Flusher)
	if !ok {
		_, _ = io.Copy(writer, readCloser)

		return
	}

	for {
		var n int
		n, err = stdoutReader.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			break
		}

		_, err = writer.Write(buf[:n])
		if err != nil {
			log.Println(err)

			break
		}

		flusher.Flush()
	}
}
