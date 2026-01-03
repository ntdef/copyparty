package thumbnail

import (
	"time"
)

// Config holds thumbnail generation configuration
type Config struct {
	// Size is the target thumbnail size (width x height)
	Width  int
	Height int

	// Quality controls the output quality (10-90)
	// 10 = small/fast, 90 = large/slow
	Quality int

	// Format specifies the output format
	Format OutputFormat

	// Crop determines if the thumbnail should be cropped to fill the target size
	// If false, the thumbnail will preserve aspect ratio
	Crop bool

	// CacheDir is the directory where thumbnails are cached
	CacheDir string

	// MaxAge is how long to keep unused thumbnails
	MaxAge time.Duration

	// ResamplingFilter specifies the resampling algorithm
	ResamplingFilter ResamplingFilter
}

// OutputFormat specifies the thumbnail output format
type OutputFormat int

const (
	FormatJPEG OutputFormat = iota
	FormatWebP
	FormatPNG
)

// String returns the string representation of the output format
func (f OutputFormat) String() string {
	switch f {
	case FormatJPEG:
		return "jpg"
	case FormatWebP:
		return "webp"
	case FormatPNG:
		return "png"
	default:
		return "jpg"
	}
}

// Extension returns the file extension for the format
func (f OutputFormat) Extension() string {
	return "." + f.String()
}

// ResamplingFilter specifies the image resampling algorithm
type ResamplingFilter int

const (
	// FilterLanczos is a high-quality resampling filter (similar to Pillow's LANCZOS)
	FilterLanczos ResamplingFilter = iota
	// FilterLinear is a faster but lower quality filter
	FilterLinear
	// FilterBox is the fastest but lowest quality filter
	FilterBox
)

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Width:            320,
		Height:           256,
		Quality:          40,
		Format:           FormatJPEG,
		Crop:             true,
		CacheDir:         ".hist/th",
		MaxAge:           7 * 24 * time.Hour,
		ResamplingFilter: FilterLanczos,
	}
}
