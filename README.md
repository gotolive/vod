# VOD

A go video on demand server. Inspired by [go-vod](https://github.com/pulsejet/go-vod).

## Features

- [x] Support output HLS and MP4.
- [x] Support hevc.
- [x] Support hardware acceleration.
- [ ] Support multiple audio.
- [ ] Support subtitles.  

## Usage

FFmpeg is required and can be set in the PATH, through environment variables(FFMPEG_PATH and FFPROBE_PATH), or specified in the code.

If you need hardware acceleration, the FFMpeg must compiled with the hardware acceleration support.

See [examples](examples) for more usage.

## Thanks

- [go-vod](https://github.com/pulsejet/go-vod) for the inspiration.
- [jellyfin](https://github.com/jellyfin/jellyfin)

