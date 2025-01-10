package vod

import (
	"fmt"
	"path/filepath"
	"strconv"
)

func (s *Stream) buildFFMpegArgs(start int, transcode bool, format string, pipe bool) []string {
	hwAccel, ok := allHWInfos[s.context.contextConfig.HWAccel]
	if !ok {
		hwAccel = allHWInfos[HWAccelNone]
	}

	args := []string{
		"-loglevel", "debug", // set log level to debug, only debug level could get ts ended.
		"-noaccurate_seek",
		"-noautorotate",
	}

	if start > 0 {
		ss := float64(start)
		if s.format == FormatHLS {
			ss = ss * float64(s.context.contextConfig.ChunkDuration)
		}

		args = append(args, "-ss", fmt.Sprintf("%.6f", ss))
	}

	if len(hwAccel.decoderArgs) != 0 && transcode {
		args = append(args, "-hwaccel")
		args = append(args, hwAccel.decoderArgs...)
	}

	args = append(args, "-i", s.context.path)
	args = append(args, "-y", "-copyts", "-fflags", "+genpts")
	// args = append(args, "-start_at_zero")
	if format == FormatHLS {
		args = append(args, "-f", "mpegts")
	}
	if !transcode {
		args = append(args, "-c", "copy")
	} else {
		// if transcode, we only support h264 and aac
		args = append(args, "-c:v")
		args = append(args, hwAccel.encoderArgs...)

		if s.spec.Bitrate > 0 {
			args = append(args, "-b:v", strconv.Itoa(int(float64(s.spec.Bitrate)*hwAccel.encodeFactor)))
		}

		args = append(args, "-c:a", "aac")
		if s.spec.Width > 0 {
			// if width>0, we must set height already
			args = append(args, hwAccel.scaleArgs(s.spec.Width, s.spec.Height)...)
		}
	}

	if format == FormatMP4 {
		args = append(args, "-movflags", "frag_keyframe+empty_moov")
	}
	if format == FormatMP4 {
		args = append(args, "-f", format)
	}
	if format == FormatHLS {
		args = append(args, s.buildHLSArgs(start, transcode)...)
	}

	if pipe {
		args = append(args, "pipe:1")
	}

	return args
}

func (s *Stream) buildHLSArgs(start int, transcode bool) []string {
	args := []string{
		"-max_delay", "5000000",
		"-avoid_negative_ts", "disabled",
		"-f", "segment",
		"-segment_format", "mpegts",
		"-segment_list", filepath.Join(s.context.contextConfig.TmpPath, "index.m3u8"),
		"-segment_list_type", "m3u8",
		"-segment_time", formatTime(s.context.contextConfig.ChunkDuration),
		"-segment_start_number", strconv.Itoa(start),
		"-break_non_keyframes", "1",
		"-individual_header_trailer", "0",
		"-write_header_trailer", "0",
		filepath.Join(s.context.contextConfig.TmpPath, "%d.ts"),
	}
	if transcode {
		args = append(args, "-force_key_frames", "expr:gte(t,n_forced*3)")
	}

	return args
}

//
// func (s *Stream) buildM4SFFMpegArgs(start int, transcode bool, format string, pipe bool) []string {
//	hw, ok := allHWInfos[s.context.contextConfig.HWAccel]
//	if !ok {
//		hw = allHWInfos[HWAccelNone]
//	}
//	args := []string{
//		"-loglevel", "debug", // set log level to debug, only debug level could get ts ended.
//		"-report",
//		"-noaccurate_seek",
//		"-noautorotate",
//	}
//	if start > 0 {
//		ss := float64(start)
//		if s.format == FormatHLS {
//			ss = ss * float64(s.context.contextConfig.ChunkDuration)
//		}
//		args = append(args, "-ss", fmt.Sprintf("%.6f", ss))
//	}
//	if hw.decoder != "" && transcode {
//		args = append(args, "-hwaccel", hw.decoder)
//	}
//	args = append(args, "-i", s.context.path)
//	args = append(args, "-y")
//	args = append(args, "-copyts")
//	args = append(args, "-c", "copy")
//	//-f hls \
//	//-hls_fmp4_init_filename pelicans-128k-IS.mp4 \
//	//-hls_segment_filename pelicans-128k-%d.m4s \
//	//-hls_segment_type fmp4 \
//	//-hls_time 6 \
//	//temp.m3u8
//	args = append(args, "-f", "hls")
//	args = append(args, "-hls_fmp4_init_filename", "init.mp4")
//	args = append(args, "-hls_segment_filename", "%d.m4s")
//	args = append(args, "-hls_segment_type", "fmp4")
//	args = append(args, "-hls_time", "6")
//	args = append(args, "temp.m3u8")
//
//	return args
//}
