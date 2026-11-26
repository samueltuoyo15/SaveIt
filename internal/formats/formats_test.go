package formats

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseInfoOneOptionPerHeight(t *testing.T) {
	raw := map[string]any{
		"id": "x", "title": "T",
		"formats": []map[string]any{
			{"format_id": "137", "ext": "mp4", "height": 1080, "vcodec": "avc1.640028", "acodec": "none", "tbr": 4000, "protocol": "https", "filesize": 1000},
			{"format_id": "312", "ext": "mp4", "height": 1080, "vcodec": "avc1.64002a", "acodec": "none", "tbr": 7957, "protocol": "m3u8_native"},
			{"format_id": "248", "ext": "webm", "height": 1080, "vcodec": "vp9", "acodec": "none", "tbr": 3000},
			{"format_id": "399", "ext": "mp4", "height": 1080, "vcodec": "av01.0.08M.08", "acodec": "none", "tbr": 2000},
			{"format_id": "313", "ext": "webm", "height": 2160, "vcodec": "vp9", "acodec": "none", "tbr": 20000},
			{"format_id": "401", "ext": "mp4", "height": 2160, "vcodec": "av01.0.12M.08", "acodec": "none", "tbr": 15000},
			{"format_id": "18", "ext": "mp4", "height": 360, "vcodec": "avc1.42001E", "acodec": "mp4a.40.2", "tbr": 500},
			{"format_id": "134", "ext": "mp4", "height": 360, "vcodec": "avc1.4d401e", "acodec": "none", "tbr": 700},
			{"format_id": "140-drc", "ext": "m4a", "vcodec": "none", "acodec": "mp4a.40.2", "abr": 129},
			{"format_id": "140", "ext": "m4a", "vcodec": "none", "acodec": "mp4a.40.2", "abr": 129, "filesize": 500},
			{"format_id": "251", "ext": "webm", "vcodec": "none", "acodec": "opus", "abr": 138},
		},
	}
	data, _ := json.Marshal(raw)
	info, err := ParseInfo(data)
	if err != nil {
		t.Fatal(err)
	}

	got := map[int]string{}
	videos := 0
	for _, f := range info.Formats {
		if !f.HasVideo {
			continue
		}
		videos++
		if prev, dup := got[f.Height]; dup {
			t.Fatalf("duplicate %dp: %s and %s", f.Height, prev, f.FormatID)
		}
		got[f.Height] = f.FormatID
	}
	want := map[int]string{
		2160: "313+140", // VP9 beats AV1 for compatibility
		1080: "137+140", // H.264 beats VP9/AV1; plain file beats the HLS stream
		360:  "134+140", // higher bitrate H.264 wins over the progressive one
	}
	if videos != len(want) {
		t.Fatalf("want %d video options, got %d: %+v", len(want), videos, info.Formats)
	}
	for h, id := range want {
		if got[h] != id {
			t.Errorf("%dp: want %s, got %s", h, id, got[h])
		}
	}
	for _, f := range info.Formats {
		if strings.HasSuffix(f.FormatID, "-drc") {
			t.Errorf("DRC track should not be chosen: %s", f.FormatID)
		}
	}
	for _, f := range info.Formats {
		if f.Height == 1080 && f.Filesize != 1500 {
			t.Errorf("1080p size should be video+audio=1500, got %d", f.Filesize)
		}
		if f.Height == 2160 && f.Filesize != 0 {
			t.Errorf("unknown video size must not show the audio size, got %d", f.Filesize)
		}
	}
	if info.Formats[0].Height != 2160 {
		t.Errorf("formats should be sorted highest first")
	}
}

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		title  string
		author string
		want   string
	}{
		{
			title:  "288K views · 19K reactions | What each layer is actually doing ↓ 1️⃣ Raw This is just you talking. | Kienobi",
			author: "Kienobi",
			want:   "What each layer is actually doing",
		},
		{
			title:  "Weekend side project: https://saveit.pxxl.pro | Samuel Tuoyo",
			author: "Samuel Tuoyo",
			want:   "Weekend side project: https://saveit.pxxl.pro",
		},
		{
			title:  "Programming Legends Are at War Over AI",
			author: "MeshPit",
			want:   "Programming Legends Are at War Over AI",
		},
		{
			title:  "",
			author: "",
			want:   "video",
		},
	}

	for _, tt := range tests {
		got := cleanTitle(tt.title, tt.author)
		if got != tt.want {
			t.Errorf("cleanTitle(%q, %q) = %q; want %q", tt.title, tt.author, got, tt.want)
		}
	}
}

func TestParseTwitterFormats(t *testing.T) {
	// Twitter/X formats where vcodec and acodec are null on progressive MP4s
	raw := map[string]any{
		"id": "2124971768184233984", "title": "GeekPark video",
		"formats": []map[string]any{
			{"format_id": "hls-audio-32000-Audio", "ext": "mp4", "vcodec": "none", "acodec": nil, "protocol": "m3u8_native"},
			{"format_id": "hls-audio-128000-Audio", "ext": "mp4", "vcodec": "none", "acodec": nil, "protocol": "m3u8_native", "abr": 128},
			{"format_id": "http-632", "ext": "mp4", "height": 538, "vcodec": nil, "acodec": nil, "tbr": 632, "protocol": "https"},
			{"format_id": "hls-256", "ext": "mp4", "height": 538, "vcodec": "avc1.4D401F", "acodec": "none", "tbr": 257, "protocol": "m3u8_native"},
			{"format_id": "http-950", "ext": "mp4", "height": 808, "vcodec": nil, "acodec": nil, "tbr": 950, "protocol": "https"},
			{"format_id": "hls-486", "ext": "mp4", "height": 808, "vcodec": "avc1.4D401F", "acodec": "none", "tbr": 486, "protocol": "m3u8_native"},
			{"format_id": "http-2176", "ext": "mp4", "height": 1212, "vcodec": nil, "acodec": nil, "tbr": 2176, "protocol": "https"},
			{"format_id": "hls-987", "ext": "mp4", "height": 1212, "vcodec": "avc1.640020", "acodec": "none", "tbr": 987, "protocol": "m3u8_native"},
			{"format_id": "http-10368", "ext": "mp4", "height": 2222, "vcodec": nil, "acodec": nil, "tbr": 10368, "protocol": "https"},
			{"format_id": "hls-5718", "ext": "mp4", "height": 2222, "vcodec": "avc1.640033", "acodec": "none", "tbr": 5719, "protocol": "m3u8_native"},
		},
	}
	data, _ := json.Marshal(raw)
	info, err := ParseInfo(data)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the progressive MP4 formats (with audio) are chosen over the video-only HLS streams
	expected := map[int]string{
		2222: "http-10368",
		1212: "http-2176",
		808:  "http-950",
		538:  "http-632",
	}

	foundVideos := 0
	for _, f := range info.Formats {
		if !f.HasVideo {
			continue
		}
		foundVideos++
		expID, ok := expected[f.Height]
		if !ok {
			t.Errorf("unexpected height %d: %s", f.Height, f.FormatID)
			continue
		}
		if f.FormatID != expID {
			t.Errorf("height %d: got format %s, want %s", f.Height, f.FormatID, expID)
		}
		if !f.HasAudio {
			t.Errorf("format %s at height %d should have audio", f.FormatID, f.Height)
		}
	}

	if foundVideos != len(expected) {
		t.Fatalf("expected %d video formats, got %d", len(expected), foundVideos)
	}
}
