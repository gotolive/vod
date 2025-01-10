package vod

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TODO hook exec.Command to test the detect functions
func TestDetectVTBReturnsTrueOnMacWithVideoToolbox(t *testing.T) {
	result := detectVTB("ffmpeg")
	t.Log("vtb result: ", result)
}

func TestDetectQSVReturnsTrueOnSuccess(t *testing.T) {
	result := detectQSV("ffmpeg")
	t.Log("qsv result: ", result)
}

func TestDetectAMFReturnsTrueOnSuccess(t *testing.T) {
	result := detectAMF("ffmpeg")
	t.Log("amf result: ", result)
}

func TestDetectVAAPIReturnsTrueOnSuccess(t *testing.T) {
	result := detectVAAPI("ffmpeg")
	t.Log("vaapi result: ", result)
}

func TestDetectNVENCReturnsTrueOnSuccess(t *testing.T) {
	result := detectNVENC("ffmpeg")
	t.Log("nvenc result: ", result)
}

func TestScaleArgsReturnsCorrectFormatAndScale(t *testing.T) {
	args := scaleArgs(1280, 720)
	assert.Equal(t, []string{"-vf", "format=nv12,scale=force_original_aspect_ratio=decrease:w=1280:h=720"}, args)
}

func TestScaleArgsHandlesZeroWidthAndHeight(t *testing.T) {
	args := scaleArgs(0, 0)
	assert.Equal(t, []string{"-vf", "format=nv12,scale=force_original_aspect_ratio=decrease:w=0:h=0"}, args)
}

func TestScaleVAAPIReturnsCorrectFormatAndScale(t *testing.T) {
	args := scaleVAAPI(1280, 720)
	assert.Equal(t, []string{"-vf", "format=nv12|vaapi,hwupload,scale_vaapi=force_original_aspect_ratio=decrease:format=nv12:w=1280:h=720"}, args)
}

func TestScaleVAAPIHandlesZeroWidthAndHeight(t *testing.T) {
	args := scaleVAAPI(0, 0)
	assert.Equal(t, []string{"-vf", "format=nv12|vaapi,hwupload,scale_vaapi=force_original_aspect_ratio=decrease:format=nv12:w=0:h=0"}, args)
}

func TestScaleNVENCReturnsCorrectFormatAndScale(t *testing.T) {
	args := scaleNVENC(1280, 720)
	assert.Equal(t, []string{"-vf", "format=nv12|cuda,hwupload,scale_cuda=force_original_aspect_ratio=decrease:passthrough=0:w=1280:h=720"}, args)
}

func TestScaleNVENCHandlesZeroWidthAndHeight(t *testing.T) {
	args := scaleNVENC(0, 0)
	assert.Equal(t, []string{"-vf", "format=nv12|cuda,hwupload,scale_cuda=force_original_aspect_ratio=decrease:passthrough=0:w=0:h=0"}, args)
}
