package scripts

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

			// Check for orange color values (hex codes for the gradient)
			return strings.Contains(svg, "#ff6b35") || // bright orange
				strings.Contains(svg, "#f7931e") // golden orange
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


// **Feature: intelligent-release-notes, Property 12: Banner SVG is valid**
// For any generated banner, the SVG output SHALL start with `<svg`, contain
// `xmlns="http://www.w3.org/2000/svg"`, and end with `</svg>`.
// **Validates: Requirements 6.1**

func TestPropertyBannerSVGValidity(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 12.1: Generated SVG starts with <svg
	properties.Property("generated SVG starts with <svg", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			config := BannerConfig{
				Version: version,
			}

			svg, err := GenerateBannerSVG(config)
			if err != nil {
				return false
			}

			return strings.HasPrefix(svg, "<svg")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 12.2: Generated SVG contains xmlns declaration
	properties.Property("generated SVG contains xmlns declaration", prop.ForAll(
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

	// Property 12.3: Generated SVG ends with </svg>
	properties.Property("generated SVG ends with </svg>", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			config := BannerConfig{
				Version: version,
			}

			svg, err := GenerateBannerSVG(config)
			if err != nil {
				return false
			}

			return strings.HasSuffix(svg, "</svg>")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 12.4: ValidateBannerSVG passes for all generated banners
	properties.Property("ValidateBannerSVG passes for all generated banners", prop.ForAll(
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

	// Property 12.5: All three validity conditions hold simultaneously
	properties.Property("all three validity conditions hold simultaneously", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			config := BannerConfig{
				Version: version,
			}

			svg, err := GenerateBannerSVG(config)
			if err != nil {
				return false
			}

			// Check all three conditions from Property 12
			startsWithSvg := strings.HasPrefix(svg, "<svg")
			containsXmlns := strings.Contains(svg, `xmlns="http://www.w3.org/2000/svg"`)
			endsWithSvg := strings.HasSuffix(svg, "</svg>")

			return startsWithSvg && containsXmlns && endsWithSvg
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	properties.TestingRun(t)
}

// **Feature: intelligent-release-notes, Property 13: Banner paths use correct relative format**
// For any release entry, the image.src path SHALL start with "../../assets/" and end with ".svg".
// **Validates: Requirements 6.2, 6.4**

func TestPropertyBannerRelativePaths(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 13.1: GenerateBannerRelativePath starts with ../../assets/
	properties.Property("GenerateBannerRelativePath starts with ../../assets/", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path := GenerateBannerRelativePath(version)

			return strings.HasPrefix(path, "../../assets/")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 13.2: GenerateBannerRelativePath ends with .svg
	properties.Property("GenerateBannerRelativePath ends with .svg", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path := GenerateBannerRelativePath(version)

			return strings.HasSuffix(path, ".svg")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 13.3: GenerateCustomBannerRelativePath starts with ../../assets/custom/
	properties.Property("GenerateCustomBannerRelativePath starts with ../../assets/custom/", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path := GenerateCustomBannerRelativePath(version)

			return strings.HasPrefix(path, "../../assets/custom/")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 13.4: GenerateCustomBannerRelativePath ends with .svg
	properties.Property("GenerateCustomBannerRelativePath ends with .svg", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path := GenerateCustomBannerRelativePath(version)

			return strings.HasSuffix(path, ".svg")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 13.5: ResolveBannerRelativePath always starts with ../../assets/
	properties.Property("ResolveBannerRelativePath always starts with ../../assets/", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path := ResolveBannerRelativePath(version)

			return strings.HasPrefix(path, "../../assets/")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 13.6: ResolveBannerRelativePath always ends with .svg
	properties.Property("ResolveBannerRelativePath always ends with .svg", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path := ResolveBannerRelativePath(version)

			return strings.HasSuffix(path, ".svg")
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 13.7: ValidateBannerRelativePath passes for generated paths
	properties.Property("ValidateBannerRelativePath passes for generated paths", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path := GenerateBannerRelativePath(version)

			return ValidateBannerRelativePath(path) == nil
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 13.8: ValidateBannerRelativePath passes for custom paths
	properties.Property("ValidateBannerRelativePath passes for custom paths", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path := GenerateCustomBannerRelativePath(version)

			return ValidateBannerRelativePath(path) == nil
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 13.9: ValidateBannerRelativePath passes for resolved paths
	properties.Property("ValidateBannerRelativePath passes for resolved paths", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path := ResolveBannerRelativePath(version)

			return ValidateBannerRelativePath(path) == nil
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 13.10: Both conditions (starts with ../../assets/ AND ends with .svg) hold
	properties.Property("both path format conditions hold simultaneously", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path := GenerateBannerRelativePath(version)

			// Check both conditions from Property 13
			startsCorrectly := strings.HasPrefix(path, "../../assets/")
			endsCorrectly := strings.HasSuffix(path, ".svg")

			return startsCorrectly && endsCorrectly
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	// Property 13.11: Relative path contains version in filename
	properties.Property("relative path contains version in filename", prop.ForAll(
		func(major, minor, patch int) bool {
			version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
			path := GenerateBannerRelativePath(version)

			// The path should contain the version with underscores
			expectedVersionPart := fmt.Sprintf("%d_%d_%d", major, minor, patch)
			return strings.Contains(path, expectedVersionPart)
		},
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
		gen.IntRange(0, 999),
	))

	properties.TestingRun(t)
}
