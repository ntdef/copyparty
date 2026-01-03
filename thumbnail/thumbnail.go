package thumbnail

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"

	"github.com/disintegration/imaging"
	"golang.org/x/image/webp"
)

// Generator handles thumbnail generation
type Generator struct {
	config *Config
	cache  *Cache
}

// New creates a new thumbnail generator with the given configuration
func New(config *Config) *Generator {
	if config == nil {
		config = DefaultConfig()
	}

	return &Generator{
		config: config,
		cache:  NewCache(config.CacheDir),
	}
}

// Generate creates a thumbnail for the given file
// Returns the path to the generated thumbnail or an error
func (g *Generator) Generate(filePath string) (string, error) {
	// Get file info
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}

	// Compute cache path
	cachePath := g.cache.GetCachePath(filePath, info.ModTime(), g.config.Format)

	// Check if cached thumbnail exists
	if g.cache.Exists(cachePath) {
		return cachePath, nil
	}

	// Generate new thumbnail
	if err := g.generateThumbnail(filePath, cachePath); err != nil {
		// Create empty file to mark failed conversion
		g.cache.EnsureDir(cachePath)
		os.WriteFile(cachePath, []byte{}, 0644)
		return "", fmt.Errorf("generate thumbnail: %w", err)
	}

	return cachePath, nil
}

// generateThumbnail performs the actual thumbnail generation
func (g *Generator) generateThumbnail(srcPath, dstPath string) error {
	// Ensure cache directory exists
	if err := g.cache.EnsureDir(dstPath); err != nil {
		return fmt.Errorf("ensure cache dir: %w", err)
	}

	// Read orientation from EXIF
	orientation := getOrientationFromFile(srcPath)

	// Open and decode the image
	img, err := g.openImage(srcPath)
	if err != nil {
		return fmt.Errorf("open image: %w", err)
	}

	// Apply EXIF rotation if needed
	if orientation.needsRotation() {
		img = g.rotateImage(img, orientation.rotationAngle())
	}

	// Resize image to 2x target resolution first (for better quality)
	// This matches the Python implementation's approach
	targetWidth := g.config.Width * 2
	targetHeight := g.config.Height * 2

	var resized image.Image
	if g.config.Crop {
		// Crop to fill the entire target size
		resized = imaging.Fill(img, targetWidth, targetHeight, imaging.Center, g.getFilter())
	} else {
		// Preserve aspect ratio
		resized = imaging.Fit(img, targetWidth, targetHeight, g.getFilter())
	}

	// Resize down to final target size
	resized = imaging.Resize(resized, g.config.Width, g.config.Height, g.getFilter())

	// Encode and save
	data, err := g.encodeImage(resized)
	if err != nil {
		return fmt.Errorf("encode image: %w", err)
	}

	// Atomic write
	if err := atomicWrite(dstPath, data); err != nil {
		return fmt.Errorf("write thumbnail: %w", err)
	}

	return nil
}

// openImage opens and decodes an image file
func (g *Generator) openImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Try to decode as different formats
	// First, try standard image.Decode which handles JPEG, PNG, GIF
	img, format, err := image.Decode(f)
	if err == nil {
		return img, nil
	}

	// Reset file position
	f.Seek(0, 0)

	// Try WebP
	if format == "webp" {
		img, err := webp.Decode(f)
		if err == nil {
			return img, nil
		}
	}

	return nil, fmt.Errorf("unsupported format or decode error: %w", err)
}

// rotateImage rotates an image by the specified angle
func (g *Generator) rotateImage(img image.Image, angle int) image.Image {
	switch angle {
	case 90:
		return imaging.Rotate90(img)
	case 180:
		return imaging.Rotate180(img)
	case 270:
		return imaging.Rotate270(img)
	default:
		return img
	}
}

// getFilter returns the imaging filter based on configuration
func (g *Generator) getFilter() imaging.ResampleFilter {
	switch g.config.ResamplingFilter {
	case FilterLanczos:
		return imaging.Lanczos
	case FilterLinear:
		return imaging.Linear
	case FilterBox:
		return imaging.Box
	default:
		return imaging.Lanczos
	}
}

// encodeImage encodes an image to the configured format
func (g *Generator) encodeImage(img image.Image) ([]byte, error) {
	buf := new(bytes.Buffer)

	switch g.config.Format {
	case FormatJPEG:
		// Map quality (10-90) to JPEG quality
		// Python uses quality directly, so we do the same
		quality := g.config.Quality
		if quality < 1 {
			quality = 1
		}
		if quality > 100 {
			quality = 100
		}

		opts := &jpeg.Options{Quality: quality}
		if err := jpeg.Encode(buf, img, opts); err != nil {
			return nil, fmt.Errorf("encode JPEG: %w", err)
		}

	case FormatPNG:
		encoder := &png.Encoder{
			CompressionLevel: png.DefaultCompression,
		}
		if err := encoder.Encode(buf, img); err != nil {
			return nil, fmt.Errorf("encode PNG: %w", err)
		}

	case FormatWebP:
		// For WebP, we use imaging library's encoding
		// Note: Go's imaging library doesn't have native WebP encoding,
		// so we fall back to JPEG for now
		// In production, you'd use a library like libwebp bindings
		return nil, fmt.Errorf("WebP encoding not yet implemented - use JPEG or PNG")

	default:
		return nil, fmt.Errorf("unsupported format: %v", g.config.Format)
	}

	return buf.Bytes(), nil
}

// CleanCache removes old thumbnails from the cache
func (g *Generator) CleanCache() error {
	return g.cache.CleanOldThumbnails(g.config.MaxAge)
}

// GenerateWithConfig generates a thumbnail with a custom configuration
func (g *Generator) GenerateWithConfig(filePath string, config *Config) (string, error) {
	oldConfig := g.config
	g.config = config
	defer func() { g.config = oldConfig }()

	return g.Generate(filePath)
}
