//go:build ignore

// Command generate_banner_cmd generates an SVG banner for a release version.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/narvanalabs/control-plane/scripts"
)

func main() {
	var (
		version    string
		outputFile string
		outputDir  string
		width      int
		height     int
		size       string
		both       bool
	)

	flag.StringVar(&version, "version", "", "Version number (e.g., 1.0.0)")
	flag.StringVar(&outputFile, "output", "", "Output file path (for single banner)")
	flag.StringVar(&outputDir, "output-dir", "", "Output directory (for -both mode)")
	flag.IntVar(&width, "width", 800, "Banner width in pixels")
	flag.IntVar(&height, "height", 200, "Banner height in pixels")
	flag.StringVar(&size, "size", "content", "Banner size preset: 'content' (800x200) or 'social' (1200x630)")
	flag.BoolVar(&both, "both", false, "Generate both content and social media banners")
	flag.Parse()

	if version == "" {
		fmt.Fprintln(os.Stderr, "Error: -version is required")
		os.Exit(1)
	}

	// Handle -both mode: generate both banner sizes
	if both {
		if outputDir == "" {
			outputDir = filepath.Join("changelog", "src", "assets")
		}
		contentPath, socialPath, err := scripts.GenerateBothBanners(version, "Narvana", outputDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully generated banners:\n")
		fmt.Printf("  Content (800x200): %s\n", contentPath)
		fmt.Printf("  Social (1200x630): %s\n", socialPath)
		return
	}

	// Single banner mode
	if outputFile == "" {
		fmt.Fprintln(os.Stderr, "Error: -output is required (or use -both with -output-dir)")
		os.Exit(1)
	}

	// Apply size preset if specified
	if size == "social" {
		width, height = scripts.GetBannerDimensions(scripts.BannerSizeSocial)
	} else if size == "content" {
		width, height = scripts.GetBannerDimensions(scripts.BannerSizeContent)
	}

	config := scripts.BannerConfig{
		Version:    version,
		Width:      width,
		Height:     height,
		BrandName:  "Narvana",
		OutputPath: outputFile,
	}

	if err := scripts.WriteBannerToFile(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to generate banner: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated banner at %s\n", outputFile)
}
