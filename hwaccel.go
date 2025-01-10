package vod

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

type HWAccel int

const (
	// HWAccelNone if setting is not supported, it will fallback to HWAccelNone rather than HWAccelAuto or return an error
	// NOTICE: Except HWAccelNone, all other HWAccel require ffmpeg to be compiled with the corresponding codec.
	// WARNING: Except HWAccelVTB, all other HWAccel are not tested yet.
	HWAccelNone    = HWAccel(-1)
	HWAccelAuto    = HWAccel(0)
	HWAccelNVENC   = HWAccel(1) // linux/windows nvidia gpu required
	HWAccelQSV     = HWAccel(2) // linux/windows, intel gpu required
	HWAccelVAAPI   = HWAccel(3) // linux only, amd/intel gpu required
	HWAccelVAAPILP = HWAccel(4) // linux only, intel gpu required
	HWAccelVTB     = HWAccel(5) // mac only.
	HWAccelAMF     = HWAccel(7) // linux/windows, amd gpu required
)

func (h HWAccel) String() string {
	return allHWInfos[h].name
}

type hwInfo struct {
	codec        HWAccel
	name         string
	decoderArgs  []string
	encoderArgs  []string
	encodeFactor float64
	detector     hwDetectFunc
	scaleArgs    func(w, h int) []string
}

//	func (h *HWAccel) String() string {
//		return h.Name
//	}
type hwDetectFunc func(string) bool

var allHWInfos = map[HWAccel]hwInfo{
	HWAccelNone: {
		HWAccelNone,
		"none",
		nil,
		[]string{"libx264", "-preset", "fast", "-crf", "23"},
		1,
		func(string) bool { return true },
		scaleArgs,
	},
	HWAccelAuto: {
		HWAccelAuto,
		"auto",
		nil, nil,
		1,
		func(string) bool { return false },
		scaleArgs,
	},
	HWAccelNVENC: {
		HWAccelNVENC,
		"NVEnc",
		[]string{"cuda"},
		[]string{
			"h264_nvenc", "-preset", "p6",
			"-tune", "ll",
			"-rc", "vbr",
			"-rc-lookahead", "30",
			"-cq", "23",
			"-temporal-aq", "1",
		},
		2,
		detectNVENC,
		scaleNVENC,
	},
	HWAccelVTB: {
		HWAccelVTB,
		"VideoToolBox",
		[]string{"videotoolbox"},
		[]string{"h264_videotoolbox", "-q:v", "50"},
		2,
		detectVTB,
		scaleArgs,
	},
	HWAccelQSV: {
		HWAccelQSV,
		"QSV",
		[]string{"qsv"},
		[]string{"h264_qsv"},
		2,
		detectQSV,
		scaleArgs,
	},
	HWAccelAMF: {
		HWAccelAMF,
		"AMF",
		[]string{},
		[]string{"h264_amf"},
		2,
		detectAMF,
		scaleArgs,
	},
	HWAccelVAAPI: {
		HWAccelVAAPI,
		"VAAPI",
		[]string{"vaapi", "-hwaccel_device", "/dev/dri/renderD128", "-hwaccel_output_format", "vaapi"},
		[]string{"h264_vaapi", "-global_quality", "21"},
		2,
		detectVAAPI,
		scaleVAAPI,
	},
	HWAccelVAAPILP: {
		HWAccelVAAPILP,
		"VAAPI Low Power",
		[]string{"vaapi", "-hwaccel_device", "/dev/dri/renderD128", "-hwaccel_output_format", "vaapi"},
		[]string{"h264_vaapi", "-low_power", "1"},
		2,
		detectVAAPI,
		scaleVAAPI,
	},
}

func scaleArgs(w, h int) []string {
	return []string{
		"-vf",
		fmt.Sprintf("format=nv12,scale=force_original_aspect_ratio=decrease:w=%d:h=%d", w, h),
		// transpose?
	}
}

func scaleVAAPI(w, h int) []string {
	return []string{
		"-vf",
		fmt.Sprintf("format=nv12|vaapi,hwupload,scale_vaapi=force_original_aspect_ratio=decrease:format=nv12:w=%d:h=%d", w, h),
	}
}

func scaleNVENC(w, h int) []string {
	return []string{
		"-vf",
		fmt.Sprintf("format=nv12|cuda,hwupload,scale_cuda=force_original_aspect_ratio=decrease:passthrough=0:w=%d:h=%d", w, h),
	}
}

func detectVTB(ffmpeg string) bool {
	if runtime.GOOS == "darwin" {
		cmd := exec.Command(ffmpeg, "-hide_banner", "-hwaccels")
		result, err := cmd.Output()
		if err != nil {
			return false
		}

		return strings.Contains(string(result), "videotoolbox")
	}

	return false
}

func detectQSV(ffmpeg string) bool {
	cmd := exec.Command(ffmpeg, "-hide_banner", "-f",
		"lavfi",
		"-i",
		"color=c=black:s=1280x720:d=1",
		"-c:v",
		"h264_qsv",
		"-f",
		"null",
		"-")
	if _, err := cmd.Output(); err != nil {
		return false
	}

	return true
}

func detectAMF(ffmpeg string) bool {
	cmd := exec.Command(ffmpeg, "-hide_banner", "-f",
		"lavfi",
		"-i",
		"color=c=black:s=1280x720:d=1",
		"-c:v",
		"h264_amf",
		"-f",
		"null",
		"-")
	if _, err := cmd.Output(); err != nil {
		return false
	}

	return true
}

func detectVAAPI(ffmpeg string) bool {
	cmd := exec.Command(ffmpeg, "-hide_banner", "-f",
		"lavfi",
		"-vaapi_device",
		"/dev/dri/renderD128", // TODO: find a way to get the device
		"-i",
		"color=c=black:s=1280x720:d=1",
		"-c:v",
		"h264_vaapi",
		"-f",
		"null",
		"-")
	if _, err := cmd.Output(); err != nil {
		return false
	}

	return true
}

func detectNVENC(ffmpeg string) bool {
	cmd := exec.Command(ffmpeg, "-hide_banner",
		"-f",
		"lavfi",
		"-i",
		"color=c=black:s=1280x720:d=1",
		"-c:v",
		"h264_nvenc",
		"-f",
		"null",
		"-")
	if _, err := cmd.Output(); err != nil {
		return false
	}

	return true
}
