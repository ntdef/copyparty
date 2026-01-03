# Thumbnail - Go Thumbnail Generation Library

A Go implementation of the copyparty thumbnail generation system. This library provides efficient, cached thumbnail generation with support for multiple image formats, EXIF orientation handling, and flexible configuration options.

## Features

- **Hash-based caching system** - Uses SHA512 hashing for efficient thumbnail storage and retrieval
- **EXIF orientation support** - Automatically rotates images based on EXIF data
- **Multiple output formats** - JPEG, PNG (WebP support coming soon)
- **Quality control** - Configurable quality settings (10-90)
- **Flexible resizing** - Crop to fill or preserve aspect ratio
- **Atomic file operations** - Thread-safe thumbnail generation
- **Cache management** - Automatic cleanup of old thumbnails
- **High-quality resampling** - Lanczos, Linear, and Box filters

## Installation

```bash
go get github.com/copyparty/thumbnail
```

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    "github.com/copyparty/thumbnail"
)

func main() {
    // Create generator with default settings
    gen := thumbnail.New(nil)

    // Generate thumbnail
    thumbPath, err := gen.Generate("photo.jpg")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Thumbnail: %s\n", thumbPath)
}
```

## Configuration

### Default Configuration

```go
config := thumbnail.DefaultConfig()
// Width: 320
// Height: 256
// Quality: 40
// Format: JPEG
// Crop: true
// CacheDir: ".hist/th"
// MaxAge: 7 days
// ResamplingFilter: Lanczos
```

### Custom Configuration

```go
config := &thumbnail.Config{
    Width:            640,
    Height:           480,
    Quality:          85,
    Format:           thumbnail.FormatJPEG,
    Crop:             false, // Preserve aspect ratio
    CacheDir:         ".thumbnails",
    MaxAge:           30 * 24 * time.Hour,
    ResamplingFilter: thumbnail.FilterLanczos,
}

gen := thumbnail.New(config)
```

## Configuration Options

### Size

- `Width` - Thumbnail width in pixels (default: 320)
- `Height` - Thumbnail height in pixels (default: 256)

### Quality

- `Quality` - Output quality (10-90)
  - 10: Small file size, fast generation, lower quality
  - 40: Balanced (default)
  - 90: Large file size, slower generation, highest quality

### Output Format

```go
thumbnail.FormatJPEG  // JPEG format
thumbnail.FormatPNG   // PNG format
thumbnail.FormatWebP  // WebP format (not yet implemented)
```

### Cropping Modes

- `Crop: true` - Crop to fill the entire target size (default)
- `Crop: false` - Preserve aspect ratio, may have letterboxing

### Resampling Filters

```go
thumbnail.FilterLanczos  // High quality (default, similar to Pillow's LANCZOS)
thumbnail.FilterLinear   // Faster, good quality
thumbnail.FilterBox      // Fastest, lower quality
```

## Cache System

The library implements a hash-based caching system similar to copyparty:

### Cache Path Structure

```
.hist/th/{d1}/{d2}/{dirHash}{fileHash}.{mtime}.{ext}
```

Example:
```
.hist/th/ab/cd/abcdef1234567890abcdef12.5f3e2a1b.jpg
         ↑   ↑  ↑                        ↑         ↑
       cat  d1 d2  hash (24 chars)     mtime    format
```

- **Directory hash**: SHA512 of directory path (first 24 chars, base64-url)
- **File hash**: SHA512 of filename (first 24 chars, base64-url)
- **Mtime**: File modification time (prevents stale cache)
- **Format**: Output format extension (.jpg, .png, .webp)

### Cache Management

```go
// Clean thumbnails older than MaxAge
err := gen.CleanCache()

// Check if cached thumbnail exists
exists := cache.Exists(cachePath)
```

## EXIF Orientation

The library automatically handles EXIF orientation:

- Reads orientation tag from JPEG images
- Rotates images appropriately (90°, 180°, 270°)
- Supports all 8 EXIF orientation values
- Adjusts dimensions when transposing

## Advanced Usage

### Generate with Custom Config

```go
gen := thumbnail.New(thumbnail.DefaultConfig())

customConfig := &thumbnail.Config{
    Width:   800,
    Height:  600,
    Quality: 90,
    Format:  thumbnail.FormatPNG,
}

thumbPath, err := gen.GenerateWithConfig("photo.jpg", customConfig)
```

### Batch Processing

```go
gen := thumbnail.New(nil)

images := []string{"img1.jpg", "img2.png", "img3.jpg"}

for _, img := range images {
    thumbPath, err := gen.Generate(img)
    if err != nil {
        log.Printf("Error for %s: %v\n", img, err)
        continue
    }
    fmt.Printf("Generated: %s\n", thumbPath)
}
```

## Comparison with copyparty (Python)

This Go implementation mirrors the core functionality of copyparty's thumbnail system:

| Feature | copyparty (Python) | This Library (Go) |
|---------|-------------------|-------------------|
| Hash-based caching | ✅ SHA512 | ✅ SHA512 |
| EXIF orientation | ✅ PIL/ExifTags | ✅ goexif |
| Multiple formats | ✅ JPG, WebP, PNG | ✅ JPG, PNG (WebP planned) |
| Quality settings | ✅ 10-90 | ✅ 10-90 |
| Cropping modes | ✅ Crop/Fit | ✅ Crop/Fit |
| Resampling | ✅ Lanczos (PIL) | ✅ Lanczos (imaging) |
| Atomic writes | ✅ Temp + rename | ✅ Temp + rename |
| Cache cleanup | ✅ Age-based | ✅ Age-based |
| Video thumbnails | ✅ FFmpeg | ❌ Not implemented |
| RAW images | ✅ rawpy | ❌ Not implemented |
| Audio spectrograms | ✅ FFmpeg | ❌ Not implemented |

## Architecture

### Key Components

1. **Generator** - Main thumbnail generation engine
2. **Cache** - Hash-based caching with atomic writes
3. **Config** - Flexible configuration system
4. **EXIF Handler** - Orientation detection and correction

### Process Flow

```
1. Request thumbnail for file
2. Compute cache path (SHA512 hash)
3. Check if cached version exists
   ├─ Yes → Return cached path
   └─ No  → Generate new thumbnail
4. Read EXIF orientation
5. Decode image
6. Apply EXIF rotation if needed
7. Resize to 2x target (high quality)
8. Resize to final target
9. Encode in target format
10. Atomic write to cache
11. Return thumbnail path
```

## Limitations

- WebP encoding not yet implemented (requires libwebp bindings)
- Video thumbnail generation not supported
- RAW image formats not supported
- Audio waveform/spectrogram generation not supported

These features may be added in future versions.

## Dependencies

- `github.com/disintegration/imaging` - Image resizing and manipulation
- `github.com/rwcarlsen/goexif` - EXIF data parsing
- `golang.org/x/image` - Extended image format support

## License

This library is part of the copyparty project.

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.

## See Also

- [copyparty](https://github.com/9001/copyparty) - The original Python implementation
- [copyparty thumbnail documentation](https://github.com/9001/copyparty/blob/hovudstraum/docs/thumbnails.md)
