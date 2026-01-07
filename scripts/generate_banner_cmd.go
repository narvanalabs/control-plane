//go:build ignore

// Command generate_banner_cmd generates an SVG banner for a release version.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var (
		version    string
		outputFile string
		width      int
		height     int
	)

	flag.StringVar(&version, "version", "", "Version number (e.g., 1.0.0)")
	flag.StringVar(&outputFile, "output", "", "Output file path")
	flag.IntVar(&width, "width", 1200, "Banner width in pixels")
	flag.IntVar(&height, "height", 630, "Banner height in pixels")
	flag.Parse()

	if version == "" {
		fmt.Fprintln(os.Stderr, "Error: -version is required")
		os.Exit(1)
	}

	if outputFile == "" {
		fmt.Fprintln(os.Stderr, "Error: -output is required")
		os.Exit(1)
	}

	config := BannerConfig{
		Version:    version,
		Width:      width,
		Height:     height,
		BrandName:  "Narvana",
		OutputPath: outputFile,
	}

	if err := WriteBannerToFile(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to generate banner: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated banner at %s\n", outputFile)
}
