package hls

type MasterPlaylist struct {
	Variants  []Variant
	Audio     []Rendition
	Subtitles []Rendition
}

type Variant struct {
	URI        string
	Bandwidth  int64
	Width      int
	Height     int
	Codecs     string
	AudioGroup string
}

type Rendition struct {
	URI      string
	GroupID  string
	Name     string
	Language string
	Default  bool
}

type MediaPlaylist struct {
	TargetDuration        int
	MediaSequence         uint64
	DiscontinuitySequence uint64
	Segments              []Segment
	DateRanges            []string
	EndList               bool
}

type Segment struct {
	URI           string
	Duration      float64
	Title         string
	Discontinuity bool
}
