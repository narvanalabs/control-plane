// Package scripts provides banner generation utilities for release management.
package scripts

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// BannerConfig holds configuration for banner generation.
type BannerConfig struct {
	Version     string // Version number to display (e.g., "1.0.0")
	Width       int    // SVG width in pixels
	Height      int    // SVG height in pixels
	BrandName   string // Brand name to display (e.g., "Narvana")
	OutputPath  string // Output file path
}

// DefaultBannerConfig returns a BannerConfig with default values.
func DefaultBannerConfig() BannerConfig {
	return BannerConfig{
		Width:     1200,
		Height:    630,
		BrandName: "Narvana",
	}
}

// bannerTemplate is the SVG template for release banners.
// Uses orange gradient theme matching control-plane design (oklch 0.705 0.213 47.604).
const bannerTemplate = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {{.Width}} {{.Height}}" width="{{.Width}}" height="{{.Height}}">
  <defs>
    <linearGradient id="bgGradient" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" style="stop-color:#f97316;stop-opacity:1" />
      <stop offset="50%" style="stop-color:#ea580c;stop-opacity:1" />
      <stop offset="100%" style="stop-color:#c2410c;stop-opacity:1" />
    </linearGradient>
    <linearGradient id="textGradient" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" style="stop-color:#ffffff;stop-opacity:1" />
      <stop offset="100%" style="stop-color:#fed7aa;stop-opacity:1" />
    </linearGradient>
  </defs>
  
  <!-- Background -->
  <rect width="{{.Width}}" height="{{.Height}}" fill="url(#bgGradient)"/>
  
  <!-- Decorative elements -->
  <circle cx="{{.CircleX1}}" cy="{{.CircleY1}}" r="{{.CircleR1}}" fill="rgba(255,255,255,0.1)"/>
  <circle cx="{{.CircleX2}}" cy="{{.CircleY2}}" r="{{.CircleR2}}" fill="rgba(255,255,255,0.05)"/>
  
  <!-- Brand name -->
  <text x="{{.CenterX}}" y="{{.BrandY}}" 
        font-family="system-ui, -apple-system, sans-serif" 
        font-size="{{.BrandFontSize}}" 
        font-weight="600"
        fill="rgba(255,255,255,0.9)" 
        text-anchor="middle">{{.BrandName}}</text>
  
  <!-- Version number (prominent, centered) -->
  <text x="{{.CenterX}}" y="{{.VersionY}}" 
        font-family="system-ui, -apple-system, sans-serif" 
        font-size="{{.VersionFontSize}}" 
        font-weight="700"
        fill="url(#textGradient)" 
        text-anchor="middle">v{{.Version}}</text>
  
  <!-- Release label -->
  <text x="{{.CenterX}}" y="{{.LabelY}}" 
        font-family="system-ui, -apple-system, sans-serif" 
        font-size="{{.LabelFontSize}}" 
        font-weight="400"
        fill="rgba(255,255,255,0.7)" 
        text-anchor="middle">Release</text>
</svg>`

// bannerTemplateData holds computed values for the banner template.
type bannerTemplateData struct {
	Width           int
	Height          int
	Version         string
	BrandName       string
	CenterX         int
	BrandY          int
	VersionY        int
	LabelY          int
	BrandFontSize   int
	VersionFontSize int
	LabelFontSize   int
	CircleX1        int
	CircleY1        int
	CircleR1        int
	CircleX2        int
	CircleY2        int
	CircleR2        int
}

// GenerateBannerSVG generates an SVG banner for a release version.
func GenerateBannerSVG(config BannerConfig) (string, error) {
	if config.Version == "" {
		return "", fmt.Errorf("version is required")
	}
	if config.Width <= 0 {
		config.Width = 1200
	}
	if config.Height <= 0 {
		config.Height = 630
	}
	if config.BrandName == "" {
		config.BrandName = "Narvana"
	}

	// Compute layout values based on dimensions
	data := bannerTemplateData{
		Width:           config.Width,
		Height:          config.Height,
		Version:         config.Version,
		BrandName:       config.BrandName,
		CenterX:         config.Width / 2,
		BrandY:          config.Height/2 - config.Height/6,
		VersionY:        config.Height/2 + config.Height/12,
		LabelY:          config.Height/2 + config.Height/4,
		BrandFontSize:   config.Height / 10,
		VersionFontSize: config.Height / 4,
		LabelFontSize:   config.Height / 15,
		CircleX1:        config.Width / 6,
		CircleY1:        config.Height / 4,
		CircleR1:        config.Height / 3,
		CircleX2:        config.Width * 5 / 6,
		CircleY2:        config.Height * 3 / 4,
		CircleR2:        config.Height / 2,
	}

	tmpl, err := template.New("banner").Parse(bannerTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// WriteBannerToFile generates a banner and writes it to a file.
func WriteBannerToFile(config BannerConfig) error {
	if config.OutputPath == "" {
		return fmt.Errorf("output path is required")
	}

	svg, err := GenerateBannerSVG(config)
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(config.OutputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(config.OutputPath, []byte(svg), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// GenerateBannerFilename generates the filename for a release banner.
// Example: "1.0.0" -> "release-1_0_0.svg"
func GenerateBannerFilename(version string) string {
	return "release-" + VersionToUnderscore(version) + ".svg"
}

// GenerateBannerPath generates the full path for a release banner in the changelog assets.
// Example: "1.0.0" -> "changelog/src/assets/release-1_0_0.svg"
func GenerateBannerPath(version string) string {
	return filepath.Join("changelog", "src", "assets", GenerateBannerFilename(version))
}

// ValidateBannerSVG checks if the generated SVG is valid and contains required elements.
func ValidateBannerSVG(svg string) error {
	if svg == "" {
		return fmt.Errorf("SVG content is empty")
	}
	if !strings.HasPrefix(svg, "<svg") {
		return fmt.Errorf("SVG does not start with <svg tag")
	}
	if !strings.Contains(svg, "</svg>") {
		return fmt.Errorf("SVG does not contain closing </svg> tag")
	}
	if !strings.Contains(svg, "xmlns=") {
		return fmt.Errorf("SVG does not contain xmlns attribute")
	}
	return nil
}

// BannerContainsVersion checks if the SVG banner contains the version number as visible text.
func BannerContainsVersion(svg, version string) bool {
	// Check for version in text element (with 'v' prefix as displayed)
	return strings.Contains(svg, ">v"+version+"<")
}

// BannerContainsBranding checks if the SVG banner contains the brand name.
func BannerContainsBranding(svg, brandName string) bool {
	return strings.Contains(svg, ">"+brandName+"<")
}

// BannerHasGradient checks if the SVG banner has a gradient background.
func BannerHasGradient(svg string) bool {
	return strings.Contains(svg, "linearGradient") && strings.Contains(svg, "url(#bgGradient)")
}

// GetCustomBannerPath returns the path where a custom banner would be located.
func GetCustomBannerPath(version string) string {
	return filepath.Join("changelog", "src", "assets", "custom", GenerateBannerFilename(version))
}

// CustomBannerExists checks if a custom banner exists for the given version.
func CustomBannerExists(version string) bool {
	customPath := GetCustomBannerPath(version)
	_, err := os.Stat(customPath)
	return err == nil
}

// GetBannerPath returns the appropriate banner path, preferring custom over generated.
// If a custom banner exists, returns the custom path; otherwise returns the generated path.
func GetBannerPath(version string) string {
	if CustomBannerExists(version) {
		return GetCustomBannerPath(version)
	}
	return GenerateBannerPath(version)
}

// ResolveBannerPath resolves the banner path with custom banner precedence.
// Returns (path, isCustom) where isCustom indicates if a custom banner was found.
func ResolveBannerPath(version string) (string, bool) {
	customPath := GetCustomBannerPath(version)
	if _, err := os.Stat(customPath); err == nil {
		return customPath, true
	}
	return GenerateBannerPath(version), false
}
