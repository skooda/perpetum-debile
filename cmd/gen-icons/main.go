package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	// Create assets directory if it doesn't exist
	assetsDir := "assets"
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating assets directory: %v\n", err)
		os.Exit(1)
	}

	// Define icons: name and RGBA color
	icons := []struct {
		name string
		r, g, b, a uint8
	}{
		{"flame1.png", 255, 80, 0, 255},
		{"flame2.png", 255, 120, 0, 255},
		{"flame3.png", 255, 180, 0, 255},
		{"flame4.png", 255, 220, 50, 255},
		{"check.png", 0, 200, 80, 255},
		{"bang.png", 220, 30, 30, 255},
	}

	// Create 44x44 PNG images
	for _, icon := range icons {
		img := image.NewRGBA(image.Rect(0, 0, 44, 44))
		col := color.RGBA{icon.r, icon.g, icon.b, icon.a}
		draw.Draw(img, img.Bounds(), &image.Uniform{col}, image.Point{}, draw.Src)

		filePath := filepath.Join(assetsDir, icon.name)
		file, err := os.Create(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating file %s: %v\n", filePath, err)
			os.Exit(1)
		}
		defer file.Close()

		if err := png.Encode(file, img); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding PNG %s: %v\n", filePath, err)
			os.Exit(1)
		}

		fmt.Printf("Created %s\n", filePath)
	}

	fmt.Println("All icons generated successfully!")
}
