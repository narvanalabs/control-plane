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
// Uses 800x200 dimensions optimized for content banners.
func DefaultBannerConfig() BannerConfig {
	return BannerConfig{
		Width:     800,
		Height:    200,
		BrandName: "Narvana",
	}
}

// SocialMediaBannerConfig returns a BannerConfig optimized for social media sharing.
// Uses 1200x630 dimensions (Open Graph standard for Twitter/Facebook/LinkedIn).
func SocialMediaBannerConfig() BannerConfig {
	return BannerConfig{
		Width:     1200,
		Height:    630,
		BrandName: "Narvana",
	}
}

// BannerSize represents predefined banner size options.
type BannerSize string

const (
	// BannerSizeContent is the default size for content banners (800x200)
	BannerSizeContent BannerSize = "content"
	// BannerSizeSocial is the size for social media sharing (1200x630)
	BannerSizeSocial BannerSize = "social"
)

// GetBannerDimensions returns width and height for a given banner size.
func GetBannerDimensions(size BannerSize) (width, height int) {
	switch size {
	case BannerSizeSocial:
		return 1200, 630
	case BannerSizeContent:
		fallthrough
	default:
		return 800, 200
	}
}

// bannerTemplate is the SVG template for release banners.
// Uses orange gradient theme matching control-plane design with animated pixel glitter effects.
// Designed to look good on both dark and light mode backgrounds.
const bannerTemplate = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {{.Width}} {{.Height}}" width="{{.Width}}" height="{{.Height}}">
  <defs>
    <!-- Main background gradient -->
    <linearGradient id="bgGradient" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" style="stop-color:#ff6b35;stop-opacity:1" />
      <stop offset="100%" style="stop-color:#f7931e;stop-opacity:1" />
    </linearGradient>
    <!-- Shine overlay gradient -->
    <linearGradient id="shineGradient" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" style="stop-color:#ffffff;stop-opacity:0.3" />
      <stop offset="50%" style="stop-color:#ffffff;stop-opacity:0.1" />
      <stop offset="100%" style="stop-color:#ffffff;stop-opacity:0" />
    </linearGradient>
    <!-- Glitter animations -->
    <style>
      @keyframes glitter1 { 0%, 100% { opacity: 0.1; } 50% { opacity: 0.4; } }
      @keyframes glitter2 { 0%, 100% { opacity: 0.15; } 50% { opacity: 0.5; } }
      @keyframes glitter3 { 0%, 100% { opacity: 0.2; } 50% { opacity: 0.6; } }
      .pixel-glitter-1 { animation: glitter1 2s ease-in-out infinite; }
      .pixel-glitter-2 { animation: glitter2 2.5s ease-in-out infinite; }
      .pixel-glitter-3 { animation: glitter3 1.8s ease-in-out infinite; }
    </style>
  </defs>
  
  <!-- Main background with rounded corners -->
  <rect width="{{.Width}}" height="{{.Height}}" fill="url(#bgGradient)" rx="12"/>
  
  <!-- Pixelated glitter pattern -->
  <g>
    <!-- Top left cluster (white pixels) -->
    <rect x="{{.PixelTL1X}}" y="{{.PixelTL1Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#ffffff" class="pixel-glitter-1"/>
    <rect x="{{.PixelTL2X}}" y="{{.PixelTL2Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#ffffff" class="pixel-glitter-2"/>
    <rect x="{{.PixelTL3X}}" y="{{.PixelTL3Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#ffffff" class="pixel-glitter-3"/>
    <rect x="{{.PixelTL4X}}" y="{{.PixelTL4Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#ffffff" class="pixel-glitter-3"/>
    <rect x="{{.PixelTL5X}}" y="{{.PixelTL5Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#ffffff" class="pixel-glitter-1"/>
    
    <!-- Top right cluster (white pixels) -->
    <rect x="{{.PixelTR1X}}" y="{{.PixelTR1Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#ffffff" class="pixel-glitter-2"/>
    <rect x="{{.PixelTR2X}}" y="{{.PixelTR2Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#ffffff" class="pixel-glitter-1"/>
    <rect x="{{.PixelTR3X}}" y="{{.PixelTR3Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#ffffff" class="pixel-glitter-3"/>
    <rect x="{{.PixelTR4X}}" y="{{.PixelTR4Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#ffffff" class="pixel-glitter-3"/>
    
    <!-- Bottom right cluster (dark pixels for contrast) -->
    <rect x="{{.PixelBR1X}}" y="{{.PixelBR1Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#000000" class="pixel-glitter-1"/>
    <rect x="{{.PixelBR2X}}" y="{{.PixelBR2Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#000000" class="pixel-glitter-3"/>
    <rect x="{{.PixelBR3X}}" y="{{.PixelBR3Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#000000" class="pixel-glitter-2"/>
    
    <!-- Bottom left cluster (dark pixels for contrast) -->
    <rect x="{{.PixelBL1X}}" y="{{.PixelBL1Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#000000" class="pixel-glitter-2"/>
    <rect x="{{.PixelBL2X}}" y="{{.PixelBL2Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#000000" class="pixel-glitter-1"/>
    <rect x="{{.PixelBL3X}}" y="{{.PixelBL3Y}}" width="{{.PixelSize}}" height="{{.PixelSize}}" fill="#000000" class="pixel-glitter-3"/>
  </g>
  
  <!-- Shine overlay (top half) -->
  <rect width="{{.Width}}" height="{{.ShineHeight}}" fill="url(#shineGradient)" rx="12"/>
  
  <!-- Version badge -->
  <g transform="translate({{.BadgeX}}, {{.BadgeY}})">
    <!-- Badge shadow -->
    <rect width="{{.BadgeWidth}}" height="{{.BadgeHeight}}" rx="{{.BadgeRadius}}" fill="#000000" opacity="0.2"/>
    <!-- Badge background -->
    <rect x="2" y="2" width="{{.BadgeInnerWidth}}" height="{{.BadgeInnerHeight}}" rx="{{.BadgeInnerRadius}}" fill="#ffffff"/>
    <rect x="4" y="4" width="{{.BadgeInnerWidth2}}" height="{{.BadgeInnerHeight2}}" rx="{{.BadgeInnerRadius2}}" fill="url(#bgGradient)"/>
    <!-- Badge glass shine -->
    <rect x="4" y="4" width="{{.BadgeInnerWidth2}}" height="{{.BadgeShineHeight}}" rx="{{.BadgeInnerRadius2}}" fill="url(#shineGradient)" opacity="0.6"/>
    <!-- Version text -->
    <text x="{{.BadgeCenterX}}" y="{{.BadgeTextY}}" font-family="Arial, sans-serif" font-size="{{.BadgeFontSize}}" font-weight="bold" fill="#ffffff" text-anchor="middle">v{{.Version}}</text>
  </g>
  
  <!-- Main brand text -->
  <text x="{{.BrandX}}" y="{{.BrandY}}" font-family="Arial, sans-serif" font-size="{{.BrandFontSize}}" font-weight="bold" fill="#ffffff" text-anchor="start">{{.BrandName}}</text>
  
  <!-- Subtitle -->
  <text x="{{.SubtitleX}}" y="{{.SubtitleY}}" font-family="Arial, sans-serif" font-size="{{.SubtitleFontSize}}" fill="#ffffff" opacity="0.9" text-anchor="start">New version released</text>
</svg>`

// bannerTemplateData holds computed values for the banner template.
type bannerTemplateData struct {
	Width     int
	Height    int
	Version   string
	BrandName string

	// Pixel glitter positions (scaled based on dimensions)
	PixelSize int
	// Top left cluster
	PixelTL1X, PixelTL1Y int
	PixelTL2X, PixelTL2Y int
	PixelTL3X, PixelTL3Y int
	PixelTL4X, PixelTL4Y int
	PixelTL5X, PixelTL5Y int
	// Top right cluster
	PixelTR1X, PixelTR1Y int
	PixelTR2X, PixelTR2Y int
	PixelTR3X, PixelTR3Y int
	PixelTR4X, PixelTR4Y int
	// Bottom right cluster
	PixelBR1X, PixelBR1Y int
	PixelBR2X, PixelBR2Y int
	PixelBR3X, PixelBR3Y int
	// Bottom left cluster
	PixelBL1X, PixelBL1Y int
	PixelBL2X, PixelBL2Y int
	PixelBL3X, PixelBL3Y int

	// Shine overlay
	ShineHeight int

	// Version badge
	BadgeX, BadgeY                   int
	BadgeWidth, BadgeHeight          int
	BadgeRadius                      int
	BadgeInnerWidth, BadgeInnerHeight int
	BadgeInnerRadius                 int
	BadgeInnerWidth2, BadgeInnerHeight2 int
	BadgeInnerRadius2                int
	BadgeShineHeight                 int
	BadgeCenterX, BadgeTextY         int
	BadgeFontSize                    int

	// Brand text
	BrandX, BrandY   int
	BrandFontSize    int

	// Subtitle
	SubtitleX, SubtitleY int
	SubtitleFontSize     int
}

// GenerateBannerSVG generates an SVG banner for a release version.
func GenerateBannerSVG(config BannerConfig) (string, error) {
	if config.Version == "" {
		return "", fmt.Errorf("version is required")
	}
	if config.Width <= 0 {
		config.Width = 800
	}
	if config.Height <= 0 {
		config.Height = 200
	}
	if config.BrandName == "" {
		config.BrandName = "Narvana"
	}

	// Scale factor based on reference design (800x200)
	scaleX := float64(config.Width) / 800.0
	scaleY := float64(config.Height) / 200.0

	// Pixel size scales with the smaller dimension
	pixelSize := int(15 * min(scaleX, scaleY))
	if pixelSize < 8 {
		pixelSize = 8
	}

	// Compute layout values based on dimensions
	data := bannerTemplateData{
		Width:     config.Width,
		Height:    config.Height,
		Version:   config.Version,
		BrandName: strings.ToUpper(config.BrandName),

		// Pixel size
		PixelSize: pixelSize,

		// Top left cluster (white pixels)
		PixelTL1X: int(20 * scaleX), PixelTL1Y: int(20 * scaleY),
		PixelTL2X: int(45 * scaleX), PixelTL2Y: int(20 * scaleY),
		PixelTL3X: int(70 * scaleX), PixelTL3Y: int(20 * scaleY),
		PixelTL4X: int(20 * scaleX), PixelTL4Y: int(45 * scaleY),
		PixelTL5X: int(45 * scaleX), PixelTL5Y: int(45 * scaleY),

		// Top right cluster (white pixels)
		PixelTR1X: int(690 * scaleX), PixelTR1Y: int(25 * scaleY),
		PixelTR2X: int(715 * scaleX), PixelTR2Y: int(25 * scaleY),
		PixelTR3X: int(740 * scaleX), PixelTR3Y: int(25 * scaleY),
		PixelTR4X: int(705 * scaleX), PixelTR4Y: int(50 * scaleY),

		// Bottom right cluster (dark pixels)
		PixelBR1X: int(720 * scaleX), PixelBR1Y: int(145 * scaleY),
		PixelBR2X: int(745 * scaleX), PixelBR2Y: int(145 * scaleY),
		PixelBR3X: int(735 * scaleX), PixelBR3Y: int(170 * scaleY),

		// Bottom left cluster (dark pixels)
		PixelBL1X: int(25 * scaleX), PixelBL1Y: int(150 * scaleY),
		PixelBL2X: int(50 * scaleX), PixelBL2Y: int(150 * scaleY),
		PixelBL3X: int(35 * scaleX), PixelBL3Y: int(175 * scaleY),

		// Shine overlay (top half)
		ShineHeight: config.Height / 2,

		// Version badge
		BadgeX:            int(50 * scaleX),
		BadgeY:            int(75 * scaleY),
		BadgeWidth:        int(160 * scaleX),
		BadgeHeight:       int(60 * scaleY),
		BadgeRadius:       int(30 * min(scaleX, scaleY)),
		BadgeInnerWidth:   int(156 * scaleX),
		BadgeInnerHeight:  int(56 * scaleY),
		BadgeInnerRadius:  int(28 * min(scaleX, scaleY)),
		BadgeInnerWidth2:  int(152 * scaleX),
		BadgeInnerHeight2: int(52 * scaleY),
		BadgeInnerRadius2: int(26 * min(scaleX, scaleY)),
		BadgeShineHeight:  int(26 * scaleY),
		BadgeCenterX:      int(80 * scaleX),
		BadgeTextY:        int(38 * scaleY),
		BadgeFontSize:     int(24 * min(scaleX, scaleY)),

		// Brand text (positioned to the right of badge)
		BrandX:        int(250 * scaleX),
		BrandY:        int(105 * scaleY),
		BrandFontSize: int(48 * min(scaleX, scaleY)),

		// Subtitle
		SubtitleX:        int(250 * scaleX),
		SubtitleY:        int(140 * scaleY),
		SubtitleFontSize: int(20 * min(scaleX, scaleY)),
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

// min returns the smaller of two float64 values.
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
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

// GenerateSocialBannerFilename generates the filename for a social media release banner.
// Example: "1.0.0" -> "release-1_0_0-social.svg"
func GenerateSocialBannerFilename(version string) string {
	return "release-" + VersionToUnderscore(version) + "-social.svg"
}

// GenerateBannerPath generates the full path for a release banner in the changelog assets.
// Example: "1.0.0" -> "changelog/src/assets/release-1_0_0.svg"
func GenerateBannerPath(version string) string {
	return filepath.Join("changelog", "src", "assets", GenerateBannerFilename(version))
}

// GenerateSocialBannerPath generates the full path for a social media banner.
// Example: "1.0.0" -> "changelog/src/assets/release-1_0_0-social.svg"
func GenerateSocialBannerPath(version string) string {
	return filepath.Join("changelog", "src", "assets", GenerateSocialBannerFilename(version))
}

// GenerateBothBanners generates both content and social media banners for a version.
// Returns the paths to both generated files.
func GenerateBothBanners(version, brandName, outputDir string) (contentPath, socialPath string, err error) {
	if version == "" {
		return "", "", fmt.Errorf("version is required")
	}
	if brandName == "" {
		brandName = "Narvana"
	}
	if outputDir == "" {
		outputDir = filepath.Join("changelog", "src", "assets")
	}

	// Generate content banner (800x200)
	contentConfig := BannerConfig{
		Version:    version,
		Width:      800,
		Height:     200,
		BrandName:  brandName,
		OutputPath: filepath.Join(outputDir, GenerateBannerFilename(version)),
	}
	if err := WriteBannerToFile(contentConfig); err != nil {
		return "", "", fmt.Errorf("failed to generate content banner: %w", err)
	}

	// Generate social media banner (1200x630)
	socialConfig := BannerConfig{
		Version:    version,
		Width:      1200,
		Height:     630,
		BrandName:  brandName,
		OutputPath: filepath.Join(outputDir, GenerateSocialBannerFilename(version)),
	}
	if err := WriteBannerToFile(socialConfig); err != nil {
		return "", "", fmt.Errorf("failed to generate social banner: %w", err)
	}

	return contentConfig.OutputPath, socialConfig.OutputPath, nil
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
// The check is case-insensitive since the brand name may be uppercased in the SVG.
func BannerContainsBranding(svg, brandName string) bool {
	// Check for brand name in text element (may be uppercased)
	return strings.Contains(strings.ToUpper(svg), ">"+strings.ToUpper(brandName)+"<")
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

// GenerateBannerRelativePath generates the relative path for a release banner
// from the content directory (changelog/src/content/releases/).
// This path format is compatible with Astro's image optimization.
// Example: "1.0.0" -> "../../assets/release-1_0_0.svg"
func GenerateBannerRelativePath(version string) string {
	return "../../assets/" + GenerateBannerFilename(version)
}

// GenerateCustomBannerRelativePath generates the relative path for a custom release banner
// from the content directory (changelog/src/content/releases/).
// This path format is compatible with Astro's image optimization.
// Example: "1.0.0" -> "../../assets/custom/release-1_0_0.svg"
func GenerateCustomBannerRelativePath(version string) string {
	return "../../assets/custom/" + GenerateBannerFilename(version)
}

// ResolveBannerRelativePath resolves the banner relative path with custom banner precedence.
// Returns the relative path suitable for use in markdown frontmatter.
// If a custom banner exists, returns the custom path; otherwise returns the generated path.
// The returned path is relative to the content directory and compatible with Astro image optimization.
func ResolveBannerRelativePath(version string) string {
	if CustomBannerExists(version) {
		return GenerateCustomBannerRelativePath(version)
	}
	return GenerateBannerRelativePath(version)
}

// ValidateBannerRelativePath checks if a banner relative path has the correct format
// for Astro image optimization. The path should:
// - Start with "../../assets/"
// - End with ".svg"
func ValidateBannerRelativePath(path string) error {
	if path == "" {
		return fmt.Errorf("banner path is empty")
	}
	if !strings.HasPrefix(path, "../../assets/") {
		return fmt.Errorf("banner path does not start with '../../assets/': %s", path)
	}
	if !strings.HasSuffix(path, ".svg") {
		return fmt.Errorf("banner path does not end with '.svg': %s", path)
	}
	return nil
}
