package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: release-changelog-cicd, Property 3: Banner SVG Generation**
// For any version string, the banner generator SHALL produce valid SVG content
// that contains the version number as visible text element.
// **Validates: Requirements 4.1, 4.3**

func TestPropertyBannerSVGGeneration(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 3.1: Generated SVG is valid XML with svg root element
	properties.Property("generated SVG is valid with svg root element", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			config := BannerConfig{
				Version: version,
			}

			svg, err := GenerateBannerSVG(config)
			if err != nil {
				return false
			}

			return strings.HasPrefix(svg, "<svg") && strings.Contains(svg, "</svg>")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 3.2: Generated SVG contains xmlns attribute
	properties.Property("generated SVG contains xmlns attribute", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			config := BannerConfig{
				Version: version,
			}

			svg, err := GenerateBannerSVG(config)
			if err != nil {
				return false
			}

			return strings.Contains(svg, `xmlns="http://www.w3.org/2000/svg"`)
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 3.3: Generated SVG contains version number as visible text
	properties.Property("generated SVG contains version number as visible text", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			config := BannerConfig{
				Version: version,
			}

			svg, err := GenerateBannerSVG(config)
			if err != nil {
				return false
			}

			return BannerContainsVersion(svg, version)
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 3.4: Generated SVG contains gradient background
	properties.Property("generated SVG contains gradient background", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			config := BannerConfig{
				Version: version,
			}

			svg, err := GenerateBannerSVG(config)
			if err != nil {
				return false
			}

			return BannerHasGradient(svg)
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 3.5: Generated SVG contains Narvana branding by default
	properties.Property("generated SVG contains Narvana branding by default", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			config := BannerConfig{
				Version: version,
			}

			svg, err := GenerateBannerSVG(config)
			if err != nil {
				return false
			}

			return BannerContainsBranding(svg, "Narvana")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 3.6: Generated SVG passes validation
	properties.Property("generated SVG passes validation", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			config := BannerConfig{
				Version: version,
			}

			svg, err := GenerateBannerSVG(config)
			if err != nil {
				return false
			}

			return ValidateBannerSVG(svg) == nil
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 3.7: Generated SVG has correct dimensions
	properties.Property("generated SVG has correct dimensions", prop.ForAll(
		func(major, minor, patch, width, height int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			config := BannerConfig{
				Version: version,
				Width:   width,
				Height:  height,
			}

			svg, err := GenerateBannerSVG(config)
			if err != nil {
				return false
			}

			// Check that width and height attributes are present
			expectedWidth := fmt.Sprintf(`width="%d"`, width)
			expectedHeight := fmt.Sprintf(`height="%d"`, height)
			return strings.Contains(svg, expectedWidth) && strings.Contains(svg, expectedHeight)
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(100, 2000),
		gen.IntRange(100, 1000),
	))

	// Property 3.8: Generated SVG uses orange theme colors
	properties.Property("generated SVG uses orange theme colors", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			config := BannerConfig{
				Version: version,
			}

			svg, err := GenerateBannerSVG(config)
			if err != nil {
				return false
			}

			// Check for orange color values (hex codes for orange-500, orange-600, orange-700)
			return strings.Contains(svg, "#f97316") || // orange-500
				strings.Contains(svg, "#ea580c") || // orange-600
				strings.Contains(svg, "#c2410c") // orange-700
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 3.9: Empty version returns error
	properties.Property("empty version returns error", prop.ForAll(
		func(_ int) bool {
			config := BannerConfig{
				Version: "",
			}

			_, err := GenerateBannerSVG(config)
			return err != nil
		},
		gen.IntRange(0, 1),
	))

	// Property 3.10: GenerateBannerFilename produces correct format
	properties.Property("GenerateBannerFilename produces correct format", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			filename := GenerateBannerFilename(version)

			expected := fmt.Sprintf("release-%d_%d_%d.svg", major, minor, patch)
			return filename == expected
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 3.11: GenerateBannerPath produces correct path
	properties.Property("GenerateBannerPath produces correct path", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path := GenerateBannerPath(version)

			// Should contain changelog/src/assets and the filename
			return strings.Contains(path, "changelog") &&
				strings.Contains(path, "assets") &&
				strings.HasSuffix(path, ".svg")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 3.12: Custom brand name is included in SVG
	properties.Property("custom brand name is included in SVG", prop.ForAll(
		func(major, minor, patch int, brandName string) bool {
			if brandName == "" {
				brandName = "TestBrand"
			}
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			config := BannerConfig{
				Version:   version,
				BrandName: brandName,
			}

			svg, err := GenerateBannerSVG(config)
			if err != nil {
				return false
			}

			return BannerContainsBranding(svg, brandName)
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	properties.TestingRun(t)
}


// **Feature: release-changelog-cicd, Property 4: Custom Banner Precedence**
// For any release version, if a custom banner file exists at the expected path,
// the changelog site SHALL reference the custom file; otherwise it SHALL reference
// the generated banner.
// **Validates: Requirements 4.4**

func TestPropertyCustomBannerPrecedence(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 4.1: GetCustomBannerPath returns path in custom directory
	properties.Property("GetCustomBannerPath returns path in custom directory", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path := GetCustomBannerPath(version)

			return strings.Contains(path, "custom") &&
				strings.Contains(path, "changelog") &&
				strings.HasSuffix(path, ".svg")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 4.2: Custom path differs from generated path
	properties.Property("custom path differs from generated path", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			customPath := GetCustomBannerPath(version)
			generatedPath := GenerateBannerPath(version)

			return customPath != generatedPath
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 4.3: When no custom banner exists, GetBannerPath returns generated path
	properties.Property("when no custom banner exists, GetBannerPath returns generated path", prop.ForAll(
		func(major, minor, patch int) bool {
			// Use a version that definitely won't have a custom banner
			version := fmt.Sprintf("%d.%d.%d", major+10000, minor+10000, patch+10000)
			path := GetBannerPath(version)
			generatedPath := GenerateBannerPath(version)

			return path == generatedPath
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 4.4: ResolveBannerPath returns isCustom=false when no custom banner exists
	properties.Property("ResolveBannerPath returns isCustom=false when no custom banner exists", prop.ForAll(
		func(major, minor, patch int) bool {
			// Use a version that definitely won't have a custom banner
			version := fmt.Sprintf("%d.%d.%d", major+10000, minor+10000, patch+10000)
			path, isCustom := ResolveBannerPath(version)

			return !isCustom && path == GenerateBannerPath(version)
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 4.5: Custom and generated paths have same filename
	properties.Property("custom and generated paths have same filename", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			customPath := GetCustomBannerPath(version)
			generatedPath := GenerateBannerPath(version)

			// Extract filenames
			customParts := strings.Split(customPath, "/")
			generatedParts := strings.Split(generatedPath, "/")

			customFilename := customParts[len(customParts)-1]
			generatedFilename := generatedParts[len(generatedParts)-1]

			return customFilename == generatedFilename
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 4.6: CustomBannerExists returns false for non-existent versions
	properties.Property("CustomBannerExists returns false for non-existent versions", prop.ForAll(
		func(major, minor, patch int) bool {
			// Use a version that definitely won't have a custom banner
			version := fmt.Sprintf("%d.%d.%d", major+10000, minor+10000, patch+10000)
			return !CustomBannerExists(version)
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 4.7: GetBannerPath always returns a valid path format
	properties.Property("GetBannerPath always returns a valid path format", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path := GetBannerPath(version)

			// Path should be non-empty and end with .svg
			return len(path) > 0 && strings.HasSuffix(path, ".svg")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 4.8: ResolveBannerPath always returns a valid path
	properties.Property("ResolveBannerPath always returns a valid path", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path, _ := ResolveBannerPath(version)

			return len(path) > 0 && strings.HasSuffix(path, ".svg")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	properties.TestingRun(t)
}
