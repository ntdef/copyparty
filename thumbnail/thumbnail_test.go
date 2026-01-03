package thumbnail

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// createTestImage creates a simple test image
func createTestImage(path string, width, height int) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with a gradient
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := color.RGBA{
				R: uint8(x * 255 / width),
				G: uint8(y * 255 / height),
				B: 128,
				A: 255,
			}
			img.Set(x, y, c)
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Width != 320 {
		t.Errorf("Expected width 320, got %d", config.Width)
	}
	if config.Height != 256 {
		t.Errorf("Expected height 256, got %d", config.Height)
	}
	if config.Quality != 40 {
		t.Errorf("Expected quality 40, got %d", config.Quality)
	}
	if config.Format != FormatJPEG {
		t.Errorf("Expected JPEG format, got %v", config.Format)
	}
}

func TestCachePathGeneration(t *testing.T) {
	cache := NewCache(".test-cache")

	mtime := time.Unix(1234567890, 0)
	path := cache.GetCachePath("/path/to/image.jpg", mtime, FormatJPEG)

	// Check that path contains expected components
	if !filepath.IsAbs(path) {
		if filepath.Base(filepath.Dir(filepath.Dir(path))) == "" {
			t.Error("Expected nested directory structure")
		}
	}

	// Check file extension
	if filepath.Ext(path) != ".jpg" {
		t.Errorf("Expected .jpg extension, got %s", filepath.Ext(path))
	}
}

func TestHashPathConsistency(t *testing.T) {
	// Same input should produce same hash
	hash1 := hashPath("/path/to/file.jpg")
	hash2 := hashPath("/path/to/file.jpg")

	if hash1 != hash2 {
		t.Error("Hash function not consistent")
	}

	// Different input should produce different hash
	hash3 := hashPath("/different/path.jpg")
	if hash1 == hash3 {
		t.Error("Different paths produced same hash")
	}
}

func TestOrientationRotation(t *testing.T) {
	tests := []struct {
		orientation Orientation
		angle       int
		needsRotate bool
	}{
		{OrientationNormal, 0, false},
		{OrientationRotate90, 90, true},
		{OrientationRotate180, 180, true},
		{OrientationRotate270, 270, true},
	}

	for _, tt := range tests {
		if tt.orientation.needsRotation() != tt.needsRotate {
			t.Errorf("Orientation %d: expected needsRotation=%v, got %v",
				tt.orientation, tt.needsRotate, tt.orientation.needsRotation())
		}

		if tt.orientation.rotationAngle() != tt.angle {
			t.Errorf("Orientation %d: expected angle=%d, got %d",
				tt.orientation, tt.angle, tt.orientation.rotationAngle())
		}
	}
}

func TestThumbnailGeneration(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "thumbnail-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test image
	testImage := filepath.Join(tmpDir, "test.jpg")
	if err := createTestImage(testImage, 800, 600); err != nil {
		t.Fatal(err)
	}

	// Create generator with test cache directory
	config := DefaultConfig()
	config.CacheDir = filepath.Join(tmpDir, ".cache")
	config.Width = 160
	config.Height = 120
	config.Quality = 75

	gen := New(config)

	// Generate thumbnail
	thumbPath, err := gen.Generate(testImage)
	if err != nil {
		t.Fatalf("Failed to generate thumbnail: %v", err)
	}

	// Check that thumbnail exists
	if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
		t.Errorf("Thumbnail file does not exist: %s", thumbPath)
	}

	// Generate again (should use cache)
	thumbPath2, err := gen.Generate(testImage)
	if err != nil {
		t.Fatalf("Failed to generate cached thumbnail: %v", err)
	}

	if thumbPath != thumbPath2 {
		t.Error("Cached thumbnail path differs from original")
	}
}

func TestCrop(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "thumbnail-crop-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create wide test image (2:1 aspect ratio)
	testImage := filepath.Join(tmpDir, "wide.jpg")
	if err := createTestImage(testImage, 1000, 500); err != nil {
		t.Fatal(err)
	}

	// Test cropped mode
	cropConfig := DefaultConfig()
	cropConfig.CacheDir = filepath.Join(tmpDir, ".cache-crop")
	cropConfig.Width = 200
	cropConfig.Height = 200
	cropConfig.Crop = true

	cropGen := New(cropConfig)
	cropThumb, err := cropGen.Generate(testImage)
	if err != nil {
		t.Fatalf("Failed to generate cropped thumbnail: %v", err)
	}

	// Test fit mode (preserve aspect ratio)
	fitConfig := DefaultConfig()
	fitConfig.CacheDir = filepath.Join(tmpDir, ".cache-fit")
	fitConfig.Width = 200
	fitConfig.Height = 200
	fitConfig.Crop = false

	fitGen := New(fitConfig)
	fitThumb, err := fitGen.Generate(testImage)
	if err != nil {
		t.Fatalf("Failed to generate fitted thumbnail: %v", err)
	}

	// Both should exist
	if _, err := os.Stat(cropThumb); os.IsNotExist(err) {
		t.Error("Cropped thumbnail does not exist")
	}
	if _, err := os.Stat(fitThumb); os.IsNotExist(err) {
		t.Error("Fitted thumbnail does not exist")
	}
}

func TestOutputFormats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "thumbnail-format-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	testImage := filepath.Join(tmpDir, "test.jpg")
	if err := createTestImage(testImage, 400, 300); err != nil {
		t.Fatal(err)
	}

	formats := []struct {
		format OutputFormat
		ext    string
	}{
		{FormatJPEG, ".jpg"},
		{FormatPNG, ".png"},
		{FormatWebP, ".webp"},
	}

	for _, tt := range formats {
		config := DefaultConfig()
		config.CacheDir = filepath.Join(tmpDir, ".cache-"+tt.format.String())
		config.Format = tt.format

		gen := New(config)
		thumbPath, err := gen.Generate(testImage)
		if err != nil {
			t.Errorf("Failed to generate %s thumbnail: %v", tt.format.String(), err)
			continue
		}

		if filepath.Ext(thumbPath) != tt.ext {
			t.Errorf("Expected extension %s, got %s", tt.ext, filepath.Ext(thumbPath))
		}
	}
}

func BenchmarkThumbnailGeneration(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "thumbnail-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	testImage := filepath.Join(tmpDir, "test.jpg")
	if err := createTestImage(testImage, 2000, 1500); err != nil {
		b.Fatal(err)
	}

	config := DefaultConfig()
	config.CacheDir = filepath.Join(tmpDir, ".cache")
	gen := New(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clear cache between runs
		os.RemoveAll(config.CacheDir)

		_, err := gen.Generate(testImage)
		if err != nil {
			b.Fatal(err)
		}
	}
}
