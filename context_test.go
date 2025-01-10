package vod

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdjustSpecReturnsOriginalSpecWhenForceAndBitrateSet(t *testing.T) {
	spec := StreamSpec{Force: true, Bitrate: 5000}
	info := &ProbeInfo{VideoBitrate: 1000}
	result := adjustSpec(spec, info)
	assert.Equal(t, spec, result)
}

func TestAdjustSpecCalculatesHeightFromWidth(t *testing.T) {
	spec := StreamSpec{Width: 960}
	info := &ProbeInfo{Width: 1920, Height: 1080}
	result := adjustSpec(spec, info)
	assert.Equal(t, 960, result.Width)
	assert.Equal(t, 540, result.Height)
}

func TestAdjustSpecCalculatesWidthFromHeight(t *testing.T) {
	spec := StreamSpec{Height: 540}
	info := &ProbeInfo{Width: 1920, Height: 1080}
	result := adjustSpec(spec, info)
	assert.Equal(t, 960, result.Width)
	assert.Equal(t, 540, result.Height)
}

func TestAdjustSpecRoundsUpOddDimensions(t *testing.T) {
	spec := StreamSpec{Width: 961, Height: 541}
	info := &ProbeInfo{Width: 1920, Height: 1080}
	result := adjustSpec(spec, info)
	assert.Equal(t, 962, result.Width)
	assert.Equal(t, 540, result.Height)
}
