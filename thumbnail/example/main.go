package main

import (
	"fmt"
	"log"
	"time"

	"github.com/copyparty/thumbnail"
)

func main() {
	// Example 1: Basic usage with default configuration
	basicExample()

	// Example 2: Custom configuration
	customExample()

	// Example 3: Different output formats
	formatExample()

	// Example 4: Cache cleanup
	cleanupExample()
}

func basicExample() {
	fmt.Println("=== Basic Example ===")

	// Create generator with default settings
	gen := thumbnail.New(nil)

	// Generate thumbnail for an image
	thumbPath, err := gen.Generate("sample.jpg")
	if err != nil {
		log.Printf("Error generating thumbnail: %v\n", err)
		return
	}

	fmt.Printf("Thumbnail generated at: %s\n\n", thumbPath)
}

func customExample() {
	fmt.Println("=== Custom Configuration Example ===")

	// Create custom configuration
	config := &thumbnail.Config{
		Width:            640,  // Larger thumbnail
		Height:           480,
		Quality:          80,   // Higher quality
		Format:           thumbnail.FormatJPEG,
		Crop:             false, // Preserve aspect ratio
		CacheDir:         ".thumbnails",
		MaxAge:           30 * 24 * time.Hour, // Keep for 30 days
		ResamplingFilter: thumbnail.FilterLanczos,
	}

	gen := thumbnail.New(config)
	thumbPath, err := gen.Generate("photo.jpg")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Custom thumbnail at: %s\n\n", thumbPath)
}

func formatExample() {
	fmt.Println("=== Different Format Example ===")

	images := []string{"image1.jpg", "image2.png", "image3.jpg"}

	for _, imgPath := range images {
		// JPEG format
		jpegConfig := thumbnail.DefaultConfig()
		jpegConfig.Format = thumbnail.FormatJPEG
		jpegConfig.Quality = 85
		jpegGen := thumbnail.New(jpegConfig)

		jpegThumb, err := jpegGen.Generate(imgPath)
		if err != nil {
			log.Printf("JPEG error for %s: %v\n", imgPath, err)
			continue
		}
		fmt.Printf("JPEG: %s -> %s\n", imgPath, jpegThumb)

		// PNG format
		pngConfig := thumbnail.DefaultConfig()
		pngConfig.Format = thumbnail.FormatPNG
		pngGen := thumbnail.New(pngConfig)

		pngThumb, err := pngGen.Generate(imgPath)
		if err != nil {
			log.Printf("PNG error for %s: %v\n", imgPath, err)
			continue
		}
		fmt.Printf("PNG:  %s -> %s\n", imgPath, pngThumb)
	}
	fmt.Println()
}

func cleanupExample() {
	fmt.Println("=== Cache Cleanup Example ===")

	config := thumbnail.DefaultConfig()
	config.MaxAge = 7 * 24 * time.Hour // 7 days

	gen := thumbnail.New(config)

	// Clean old thumbnails
	if err := gen.CleanCache(); err != nil {
		log.Printf("Error cleaning cache: %v\n", err)
		return
	}

	fmt.Println("Cache cleaned successfully\n")
}
