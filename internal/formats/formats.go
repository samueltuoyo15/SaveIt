package formats

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type rawFormat struct {
	FormatID       string  `json:"format_id"`
	Ext            string  `json:"ext"`
	Protocol       string  `json:"protocol"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Acodec         string  `json:"acodec"`
	Vcodec         string  `json:"vcodec"`
	ABR            float64 `json:"abr"`
	TBR            float64 `json:"tbr"`
	Filesize       int64   `json:"filesize"`
	FilesizeApprox int64   `json:"filesize_approx"`
}

type rawInfo struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Uploader   string      `json:"uploader"`
	Thumbnail  string      `json:"thumbnail"`
	WebpageURL string      `json:"webpage_url"`
	Formats    []rawFormat `json:"formats"`
}

type Format struct {
	FormatID       string `json:"format_id"`
	Label          string `json:"label"`
	Ext            string `json:"ext"`
	Height         int    `json:"height"`
	Filesize       int64  `json:"filesize"`
	FilesizeApprox bool   `json:"filesize_approx"`
	HasVideo       bool   `json:"has_video"`
	HasAudio       bool   `json:"has_audio"`
}

type VideoInfo struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Author    string   `json:"author"`
	Thumbnail string   `json:"thumbnail"`
	URL       string   `json:"url"`
	Formats   []Format `json:"formats"`
}

func hasVideo(f rawFormat) bool { return f.Vcodec != "" && f.Vcodec != "none" }
func hasAudio(f rawFormat) bool { return f.Acodec != "" && f.Acodec != "none" }

func extFamily(ext string) string {
	switch ext {
	case "mp4", "m4a", "m4v":
		return "mp4"
	case "webm":
		return "webm"
	default:
		return ext
	}
}

func effectiveSize(filesize, approx int64) (int64, bool) {
	if filesize > 0 {
		return filesize, false
	}
	if approx > 0 {
		return approx, true
	}
	return 0, true
}

func fmtSize(bytes int64, approx bool) string {
	prefix := ""
	if approx {
		prefix = "~"
	}
	const mb = 1024 * 1024
	const gb = 1024 * mb
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%s%.1f GB", prefix, float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%s%.0f MB", prefix, float64(bytes)/float64(mb))
	default:
		return fmt.Sprintf("%s%.0f KB", prefix, float64(bytes)/1024)
	}
}

// codecRank orders video codecs by how widely they play: H.264 works on
// everything, VP9 on most devices, AV1 lacks hardware decoding on many phones.
func codecRank(vcodec string) int {
	v := strings.ToLower(vcodec)
	switch {
	case strings.HasPrefix(v, "avc"), strings.HasPrefix(v, "h264"):
		return 0
	case strings.HasPrefix(v, "vp9"), strings.HasPrefix(v, "vp09"):
		return 1
	case strings.HasPrefix(v, "av01"), strings.HasPrefix(v, "av1"):
		return 2
	default:
		return 3
	}
}

// isStream reports HLS/DASH-manifest formats. YouTube lists these next to the
// plain files, but they have no known size and advertise an inflated bitrate.
func isStream(f rawFormat) bool {
	return strings.HasPrefix(f.Protocol, "m3u8") || strings.Contains(f.Protocol, "dash")
}

func betterVideo(a, b rawFormat) bool {
	if ra, rb := codecRank(a.Vcodec), codecRank(b.Vcodec); ra != rb {
		return ra < rb
	}
	if sa, sb := isStream(a), isStream(b); sa != sb {
		return sb
	}
	return a.TBR > b.TBR
}

// isDRC reports YouTube's "dynamic range compression" tracks (IDs like
// "140-drc"): loudness-squashed variants, so prefer the original audio.
func isDRC(f rawFormat) bool { return strings.HasSuffix(f.FormatID, "-drc") }

func betterAudio(a, b rawFormat) bool {
	if da, db := isDRC(a), isDRC(b); da != db {
		return db
	}
	if a.ABR != b.ABR {
		return a.ABR > b.ABR
	}
	return a.TBR > b.TBR
}

func ParseInfo(data []byte) (*VideoInfo, error) {
	var raw rawInfo
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	var videoOnly, audioOnly, combined []rawFormat
	for _, f := range raw.Formats {
		hv, ha := hasVideo(f), hasAudio(f)
		if !hv && !ha {
			continue
		}
		switch {
		case hv && ha:
			combined = append(combined, f)
		case hv:
			videoOnly = append(videoOnly, f)
		case ha:
			audioOnly = append(audioOnly, f)
		}
	}

	bestAudio := map[string]rawFormat{}
	for _, a := range audioOnly {
		fam := extFamily(a.Ext)
		existing, ok := bestAudio[fam]
		if !ok || betterAudio(a, existing) {
			bestAudio[fam] = a
		}
	}

	// One option per resolution. YouTube offers most heights in several codecs
	// (H.264, VP9, AV1); listing them all just looks like duplicates to users.
	type candidate struct {
		f        rawFormat
		combined bool
	}
	best := map[int]candidate{}
	consider := func(f rawFormat, combined bool) {
		if f.Height <= 0 {
			return
		}
		if c, ok := best[f.Height]; !ok || betterVideo(f, c.f) {
			best[f.Height] = candidate{f, combined}
		}
	}
	for _, f := range combined {
		consider(f, true)
	}
	for _, f := range videoOnly {
		consider(f, false)
	}

	var formats []Format
	for _, c := range best {
		f := c.f
		if c.combined {
			sz, approx := effectiveSize(f.Filesize, f.FilesizeApprox)
			label := fmt.Sprintf("%dp %s", f.Height, strings.ToUpper(f.Ext))
			if sz > 0 {
				label += " " + fmtSize(sz, approx)
			}
			formats = append(formats, Format{
				FormatID: f.FormatID, Label: label, Ext: f.Ext,
				Height: f.Height, Filesize: sz, FilesizeApprox: approx,
				HasVideo: true, HasAudio: true,
			})
			continue
		}

		mergedID := f.FormatID
		outExt := f.Ext
		var sz int64
		approx := true

		for _, pref := range []string{"mp4", "webm"} {
			if a, ok := bestAudio[pref]; ok {
				mergedID = f.FormatID + "+" + a.FormatID
				vsz, _ := effectiveSize(f.Filesize, f.FilesizeApprox)
				asz, _ := effectiveSize(a.Filesize, a.FilesizeApprox)
				if vsz > 0 && asz > 0 {
					sz = vsz + asz // unknown video size: show nothing rather than just the audio size
				}
				approx = f.FilesizeApprox > 0 || a.FilesizeApprox > 0
				outExt = "mp4"
				break
			}
		}

		label := fmt.Sprintf("%dp %s", f.Height, strings.ToUpper(outExt))
		if sz > 0 {
			label += " " + fmtSize(sz, approx)
		}
		formats = append(formats, Format{
			FormatID: mergedID, Label: label, Ext: outExt,
			Height: f.Height, Filesize: sz, FilesizeApprox: approx,
			HasVideo: true, HasAudio: len(bestAudio) > 0,
		})
	}

	added := 0
	for _, pref := range []string{"mp4", "webm", "mp3"} {
		if added >= 2 {
			break
		}
		a, ok := bestAudio[pref]
		if !ok {
			continue
		}
		sz, approxSz := effectiveSize(a.Filesize, a.FilesizeApprox)
		abrStr := ""
		if a.ABR > 0 {
			abrStr = fmt.Sprintf(" %.0fk", a.ABR)
		}
		label := fmt.Sprintf("Audio only (%s%s)", strings.ToUpper(a.Ext), abrStr)
		if sz > 0 {
			label += " " + fmtSize(sz, approxSz)
		}
		formats = append(formats, Format{
			FormatID: a.FormatID, Label: label, Ext: a.Ext,
			Filesize: sz, FilesizeApprox: approxSz, HasAudio: true,
		})
		added++
	}

	sort.SliceStable(formats, func(i, j int) bool {
		return formats[i].Height > formats[j].Height
	})

	return &VideoInfo{
		ID:        raw.ID,
		Title:     raw.Title,
		Author:    raw.Uploader,
		Thumbnail: raw.Thumbnail,
		URL:       raw.WebpageURL,
		Formats:   formats,
	}, nil
}
