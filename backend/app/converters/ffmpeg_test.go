package converters

import (
	"testing"
)

func init() {

}

// test ParseDuration

func TestParseDuration(t *testing.T) {
	// test cases
	cases := []struct {
		input    string
		expected string
	}{
		{
			input:    "size=      13kB time=00:00:01.63 bitrate=  67.5kbits/s speed=6.97x",
			expected: "1.63s",
		},
		{
			input: `ffmpeg version 4.2.7-0ubuntu0.1 Copyright (c) 2000-2022 the FFmpeg developers
			...
			size=      13kB time=00:00:01.6323234 bitrate=  67.5kbits/s speed=6.97x
			video:0kB audio:12kB subtitle:0kB other streams:0kB global headers:0kB muxing overhead: 9.588932%
			`,
			expected: "1.6323234s",
		},
		{
			input: `ffmpeg version 5.1.2 Copyright (c) 2000-2022 the FFmpeg developers
			built with gcc 12.2.1 (Alpine 12.2.1_git20220924-r3) 20220924
			configuration: --prefix=/usr --enable-avfilter --enable-libfontconfig --enable-libfreetype --enable-libfribidi --enable-gnutls --enable-gpl --enable-libass --enable-libmp3lame --enable-libpulse --enable-libvorbis --enable-libvpx --enable-libxvid --enable-libx264 --enable-libx265 --enable-libtheora --enable-libv4l2 --enable-libdav1d --enable-lto --enable-postproc --enable-pic --enable-pthreads --enable-shared --enable-libxcb --enable-librist --enable-libsrt --enable-libssh --enable-libvidstab --disable-stripping --disable-static --disable-librtmp --disable-lzma --enable-libaom --enable-libopus --enable-libsoxr --enable-libwebp --enable-vaapi --enable-vdpau --enable-vulkan --enable-libdrm --enable-libzmq --optflags=-O2 --disable-debug --enable-libsvtav1
  libavutil      57. 28.100 / 57. 28.100
  libavcodec     59. 37.100 / 59. 37.100
  libavformat    59. 27.100 / 59. 27.100
			  libavdevice    59.  7.100 / 59.  7.100
  libavfilter     8. 44.100 /  8. 44.100
  libswscale      6.  7.100 /  6.  7.100
  libswresample   4.  7.100 /  4.  7.100
  libpostproc    56.  6.100 / 56.  6.100
[ogg @ 0x7ff66a672100] Page at 47 is missing granule
Input #0, ogg, from '/data/819adad0-f5f4-413c-a0c6-3ab61858118b.oga':
  Duration: 00:00:02.06, start: 0.020000, bitrate: 47 kb/s
  Stream #0:0: Audio: opus, 48000 Hz, mono, fltp
Stream mapping:
  Stream #0:0 -\u003e #0:0 (opus (native) -\u003e opus (libopus))
Press [q] to stop, [?] for help
[libopus @ 0x7ff667157940] No bit rate set. Defaulting to 64000 bps.
Output #0, webm, to '/data/819adad0-f5f4-413c-a0c6-3ab61858118b.webm':
  Metadata:
    encoder         : Lavf59.27.100
  Stream #0:0: Audio: opus, 48000 Hz, mono, flt, 64 kb/s
    Metadata:
      encoder         : Lavc59.37.100 libopus
size=       0kB time=00:00:00.01 bitrate= 308.3kbits/s speed=87.8x    \rsize=      16kB time=00:00:01.97 bitrate=  64.7kbits/s speed=18.5x    
video:0kB audio:14kB subtitle:0kB other streams:0kB global headers:0kB muxing overhead: 8.208601%
`,
			expected: "1.97s",
		},
	}
	for _, c := range cases {
		actual, err := ParseDuration(c.input)
		if err != nil || actual.String() != c.expected {
			t.Errorf("ParseDuration(%q) == %q, want %q. Error: %v", c.input, actual, c.expected, err)
		}
	}
}

func TestParseDurationFailure(t *testing.T) {
	actual, err := ParseDuration("")
	if err == nil || actual.String() != "0s" {
		t.Errorf("ParseDuration(%q) == %q, want %q. Error: %v", "", actual, "0s", err)
	}
}
