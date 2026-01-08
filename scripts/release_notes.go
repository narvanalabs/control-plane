// Package scripts provides release notes generation utilities for the changelog site.
package scripts

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// CommitType represents the type of change in a conventional commit.
type CommitType string

const (
	CommitTypeFeat     CommitType = "feat"
	CommitTypeFix      CommitType = "fix"
	CommitTypeDocs     CommitType = "docs"
	CommitTypeStyle    CommitType = "style"
	CommitTypeRefactor CommitType = "refactor"
	CommitTypePerf     CommitType = "perf"
	CommitTypeTest     CommitType = "test"
	CommitTypeBuild    CommitType = "build"
	CommitTypeCI       CommitType = "ci"
	CommitTypeChore    CommitType = "chore"
	CommitTypeOther    CommitType = "other"
)

// AllCommitTypes returns all valid commit types.
func AllCommitTypes() []CommitType {
	return []CommitType{
		CommitTypeFeat,
		CommitTypeFix,
		CommitTypeDocs,
		CommitTypeStyle,
		CommitTypeRefactor,
		CommitTypePerf,
		CommitTypeTest,
		CommitTypeBuild,
		CommitTypeCI,
		CommitTypeChore,
		CommitTypeOther,
	}
}

// IsValidCommitType checks if a string is a valid commit type.
func IsValidCommitType(t string) bool {
	switch CommitType(t) {
	case CommitTypeFeat, CommitTypeFix, CommitTypeDocs, CommitTypeStyle,
		CommitTypeRefactor, CommitTypePerf, CommitTypeTest, CommitTypeBuild,
		CommitTypeCI, CommitTypeChore:
		return true
	default:
		return false
	}
}

// ParsedCommit represents a parsed conventional commit.
type ParsedCommit struct {
	Type           CommitType // The type of commit (feat, fix, etc.)
	Scope          string     // Optional scope in parentheses
	Description    string     // The commit description
	Body           string     // Optional commit body
	BreakingChange bool       // Whether this is a breaking change
	Hash           string     // Short commit hash (for reference only, not displayed)
	Raw            string     // Original raw commit message
}


// conventionalCommitRegex matches the conventional commit format:
// type(scope)!: description
// where scope and ! are optional
var conventionalCommitRegex = regexp.MustCompile(`^(\w+)(?:\(([^)]*)\))?(!)?\s*:\s*(.*)$`)

// ParseCommit parses a raw commit message into structured form.
// It handles the conventional commit format: type(scope): description
// If the message doesn't match the format, it returns a commit with type "other".
func ParseCommit(raw string) ParsedCommit {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ParsedCommit{
			Type:        CommitTypeOther,
			Description: "",
			Raw:         raw,
		}
	}

	// Split into first line and body
	lines := strings.SplitN(raw, "\n", 2)
	firstLine := strings.TrimSpace(lines[0])
	var body string
	if len(lines) > 1 {
		body = strings.TrimSpace(lines[1])
	}

	// Check for BREAKING CHANGE in body
	breakingChange := strings.Contains(strings.ToUpper(body), "BREAKING CHANGE")

	// Try to match conventional commit format
	matches := conventionalCommitRegex.FindStringSubmatch(firstLine)
	if matches == nil {
		// Not a conventional commit, categorize as "other"
		return ParsedCommit{
			Type:           CommitTypeOther,
			Description:    firstLine,
			Body:           body,
			BreakingChange: breakingChange,
			Raw:            raw,
		}
	}

	// Extract components
	commitType := strings.ToLower(matches[1])
	scope := matches[2]
	bangIndicator := matches[3]
	description := strings.TrimSpace(matches[4])

	// Check for breaking change indicator (!)
	if bangIndicator == "!" {
		breakingChange = true
	}

	// Validate commit type
	var parsedType CommitType
	if IsValidCommitType(commitType) {
		parsedType = CommitType(commitType)
	} else {
		parsedType = CommitTypeOther
	}

	return ParsedCommit{
		Type:           parsedType,
		Scope:          scope,
		Description:    description,
		Body:           body,
		BreakingChange: breakingChange,
		Raw:            raw,
	}
}


// FormatCommit formats a ParsedCommit back to conventional commit string.
// This enables round-trip testing: parse(format(commit)) should equal commit.
func FormatCommit(commit ParsedCommit) string {
	var sb strings.Builder

	// Write type
	sb.WriteString(string(commit.Type))

	// Write scope if present
	if commit.Scope != "" {
		sb.WriteString("(")
		sb.WriteString(commit.Scope)
		sb.WriteString(")")
	}

	// Write breaking change indicator if applicable
	if commit.BreakingChange {
		sb.WriteString("!")
	}

	// Write description
	sb.WriteString(": ")
	sb.WriteString(commit.Description)

	// Write body if present
	if commit.Body != "" {
		sb.WriteString("\n\n")
		sb.WriteString(commit.Body)
	}

	return sb.String()
}

// hashInParensRegex matches commit hashes in parentheses like (abc123) or (abcd1234)
var hashInParensRegex = regexp.MustCompile(`\s*\([a-fA-F0-9]{6,8}\)`)

// leadingPrefixRegex matches leading "Add ", "Update ", "Fix " prefixes (case-insensitive)
var leadingPrefixRegex = regexp.MustCompile(`^(?i)(add|update|fix)\s+`)

// CleanDescription processes a commit description for display.
// - Removes commit hashes in parentheses (abc123)
// - Removes leading "Add ", "Update ", "Fix " prefixes
// - Capitalizes first letter
// - Adds period if missing punctuation at end
func CleanDescription(desc string) string {
	if desc == "" {
		return ""
	}

	// Step 1: Remove commit hashes in parentheses
	cleaned := hashInParensRegex.ReplaceAllString(desc, "")

	// Step 2: Trim any leading/trailing whitespace after hash removal
	cleaned = strings.TrimSpace(cleaned)

	if cleaned == "" {
		return ""
	}

	// Step 3: Remove leading "Add ", "Update ", "Fix " prefixes
	cleaned = leadingPrefixRegex.ReplaceAllString(cleaned, "")

	// Step 4: Trim again after prefix removal
	cleaned = strings.TrimSpace(cleaned)

	if cleaned == "" {
		return ""
	}

	// Step 5: Capitalize first letter
	runes := []rune(cleaned)
	if len(runes) > 0 {
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	}
	cleaned = string(runes)

	// Step 6: Add period if missing punctuation at end
	if len(cleaned) > 0 && !endsWithPunctuation(cleaned) {
		cleaned += "."
	}

	return cleaned
}

// endsWithPunctuation checks if a string ends with common punctuation marks.
func endsWithPunctuation(s string) bool {
	if len(s) == 0 {
		return false
	}
	lastChar := s[len(s)-1]
	return lastChar == '.' || lastChar == '!' || lastChar == '?' || lastChar == ':' || lastChar == ';'
}

// ContainsHash checks if description contains a commit hash pattern in parentheses.
func ContainsHash(desc string) bool {
	return hashInParensRegex.MatchString(desc)
}

// NoiseFilterConfig holds configuration for filtering noise commits.
type NoiseFilterConfig struct {
	ExcludedTypes []CommitType // Types to always exclude (chore, style, ci, test)
	NoisePatterns []string     // Regex patterns for noise descriptions
	PreserveStats bool         // Whether to track original count
}

// FilterResult contains filtered commits and statistics.
type FilterResult struct {
	Commits       []ParsedCommit // Commits that passed the filter
	OriginalCount int            // Original number of commits before filtering
	FilteredCount int            // Number of commits that passed the filter
	NoiseCommits  []ParsedCommit // Commits that were filtered out
}

// DefaultNoiseFilterConfig returns the default noise filter configuration.
// Excludes chore, style, ci, test types and common noise patterns.
func DefaultNoiseFilterConfig() NoiseFilterConfig {
	return NoiseFilterConfig{
		ExcludedTypes: []CommitType{
			CommitTypeChore,
			CommitTypeStyle,
			CommitTypeCI,
			CommitTypeTest,
		},
		NoisePatterns: []string{
			`(?i)^fix(ing)?\s+whitespace`,
			`(?i)^fix(ing)?\s+typo`,
			`(?i)^fix(ing)?\s+lint`,
			`(?i)^fix(ing)?\s+format`,
			`(?i)^remove\s+trailing`,
			`(?i)^update\s+lock\s+file`,
			`(?i)^merge\s+(branch|pull\s+request)`,
			`(?i)^wip\b`,
			`(?i)^minor\b`,
		},
		PreserveStats: true,
	}
}


// IsNoiseCommit checks if a single commit is noise based on the config.
// A commit is noise if its type is in the excluded types list OR
// if its description matches any of the noise patterns.
func IsNoiseCommit(commit ParsedCommit, config NoiseFilterConfig) bool {
	// Check if type is excluded
	for _, excludedType := range config.ExcludedTypes {
		if commit.Type == excludedType {
			return true
		}
	}

	// Check if description matches any noise pattern
	for _, pattern := range config.NoisePatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue // Skip invalid patterns
		}
		if re.MatchString(commit.Description) {
			return true
		}
	}

	return false
}

// FilterCommits removes noise commits from the list based on the config.
// It filters by excluded types and noise patterns, preserving the original count.
func FilterCommits(commits []ParsedCommit, config NoiseFilterConfig) FilterResult {
	result := FilterResult{
		OriginalCount: len(commits),
		Commits:       make([]ParsedCommit, 0, len(commits)),
		NoiseCommits:  make([]ParsedCommit, 0),
	}

	for _, commit := range commits {
		if IsNoiseCommit(commit, config) {
			result.NoiseCommits = append(result.NoiseCommits, commit)
		} else {
			result.Commits = append(result.Commits, commit)
		}
	}

	result.FilteredCount = len(result.Commits)
	return result
}


// FeatureArea represents a detected feature area for grouping commits.
type FeatureArea string

const (
	FeatureAreaBuildSystem      FeatureArea = "Build System"
	FeatureAreaDeployment       FeatureArea = "Deployment"
	FeatureAreaAuthentication   FeatureArea = "Authentication"
	FeatureAreaAPI              FeatureArea = "API"
	FeatureAreaDatabase         FeatureArea = "Database"
	FeatureAreaUserInterface    FeatureArea = "User Interface"
	FeatureAreaConfiguration    FeatureArea = "Configuration"
	FeatureAreaContainerization FeatureArea = "Containerization"
	FeatureAreaCommunication    FeatureArea = "Communication"
	FeatureAreaScheduler        FeatureArea = "Scheduler"
	FeatureAreaLogging          FeatureArea = "Logging"
	FeatureAreaSecurity         FeatureArea = "Security"
	FeatureAreaTesting          FeatureArea = "Testing"
	FeatureAreaDocumentation    FeatureArea = "Documentation"
	FeatureAreaOther            FeatureArea = "Other"
)

// KeywordMapping maps keywords to feature areas.
// Keywords are matched case-insensitively against commit descriptions.
var KeywordMapping = map[string]FeatureArea{
	// Build System
	"build":   FeatureAreaBuildSystem,
	"nix":     FeatureAreaBuildSystem,
	"flake":   FeatureAreaBuildSystem,
	"compile": FeatureAreaBuildSystem,
	"makefile": FeatureAreaBuildSystem,

	// Deployment
	"deploy":     FeatureAreaDeployment,
	"deployment": FeatureAreaDeployment,
	"release":    FeatureAreaDeployment,
	"rollout":    FeatureAreaDeployment,

	// Authentication
	"auth":           FeatureAreaAuthentication,
	"authentication": FeatureAreaAuthentication,
	"login":          FeatureAreaAuthentication,
	"logout":         FeatureAreaAuthentication,
	"session":        FeatureAreaAuthentication,
	"token":          FeatureAreaAuthentication,
	"oauth":          FeatureAreaAuthentication,
	"rbac":           FeatureAreaAuthentication,

	// API
	"api":      FeatureAreaAPI,
	"endpoint": FeatureAreaAPI,
	"rest":     FeatureAreaAPI,
	"openapi":  FeatureAreaAPI,
	"handler":  FeatureAreaAPI,

	// Database
	"database":  FeatureAreaDatabase,
	"db":        FeatureAreaDatabase,
	"migration": FeatureAreaDatabase,
	"postgres":  FeatureAreaDatabase,
	"sql":       FeatureAreaDatabase,
	"query":     FeatureAreaDatabase,
	"store":     FeatureAreaDatabase,

	// User Interface
	"ui":        FeatureAreaUserInterface,
	"web":       FeatureAreaUserInterface,
	"dashboard": FeatureAreaUserInterface,
	"frontend":  FeatureAreaUserInterface,
	"page":      FeatureAreaUserInterface,
	"template":  FeatureAreaUserInterface,
	"templ":     FeatureAreaUserInterface,

	// Configuration
	"config":   FeatureAreaConfiguration,
	"settings": FeatureAreaConfiguration,
	"env":      FeatureAreaConfiguration,
	"secret":   FeatureAreaConfiguration,
	"secrets":  FeatureAreaConfiguration,

	// Containerization
	"docker":     FeatureAreaContainerization,
	"container":  FeatureAreaContainerization,
	"podman":     FeatureAreaContainerization,
	"oci":        FeatureAreaContainerization,
	"image":      FeatureAreaContainerization,
	"dockerfile": FeatureAreaContainerization,

	// Communication
	"grpc":      FeatureAreaCommunication,
	"websocket": FeatureAreaCommunication,
	"socket":    FeatureAreaCommunication,
	"stream":    FeatureAreaCommunication,
	"proto":     FeatureAreaCommunication,
	"protobuf":  FeatureAreaCommunication,

	// Scheduler
	"scheduler": FeatureAreaScheduler,
	"schedule":  FeatureAreaScheduler,
	"queue":     FeatureAreaScheduler,
	"job":       FeatureAreaScheduler,
	"worker":    FeatureAreaScheduler,

	// Logging
	"log":     FeatureAreaLogging,
	"logs":    FeatureAreaLogging,
	"logging": FeatureAreaLogging,
	"logger":  FeatureAreaLogging,

	// Security
	"security":   FeatureAreaSecurity,
	"encrypt":    FeatureAreaSecurity,
	"encryption": FeatureAreaSecurity,
	"sops":       FeatureAreaSecurity,
	"tls":        FeatureAreaSecurity,
	"ssl":        FeatureAreaSecurity,

	// Testing
	"test":  FeatureAreaTesting,
	"tests": FeatureAreaTesting,

	// Documentation
	"doc":           FeatureAreaDocumentation,
	"docs":          FeatureAreaDocumentation,
	"documentation": FeatureAreaDocumentation,
	"readme":        FeatureAreaDocumentation,
}

// DetectFeatureArea analyzes a commit description to detect feature area.
// It matches keywords case-insensitively and returns the first matching feature area.
// If no keywords match or multiple conflicting keywords are found, it returns FeatureAreaOther.
func DetectFeatureArea(commit ParsedCommit) FeatureArea {
	desc := strings.ToLower(commit.Description)
	
	// Track matched feature areas to detect ambiguity
	matchedAreas := make(map[FeatureArea]bool)
	
	// Check each keyword against the description
	for keyword, area := range KeywordMapping {
		// Use word boundary matching to avoid partial matches
		// Check if keyword appears as a word in the description
		if containsWord(desc, keyword) {
			matchedAreas[area] = true
		}
	}
	
	// If no matches found, return Other
	if len(matchedAreas) == 0 {
		return FeatureAreaOther
	}
	
	// If multiple different areas matched, it's ambiguous - return Other
	if len(matchedAreas) > 1 {
		return FeatureAreaOther
	}
	
	// Return the single matched area
	for area := range matchedAreas {
		return area
	}
	
	return FeatureAreaOther
}

// containsWord checks if a word appears in the text as a complete word.
// It handles word boundaries to avoid partial matches (e.g., "api" shouldn't match "capital").
func containsWord(text, word string) bool {
	// Simple word boundary check using spaces and punctuation
	text = " " + text + " "
	word = strings.ToLower(word)
	
	// Check for the word with various boundary characters
	boundaries := []string{" ", ".", ",", ":", ";", "-", "_", "/", "(", ")", "[", "]", "'", "\""}
	
	for _, leftBound := range boundaries {
		for _, rightBound := range boundaries {
			if strings.Contains(text, leftBound+word+rightBound) {
				return true
			}
		}
	}
	
	return false
}

// GetEffectiveScope returns the scope to use for grouping a commit.
// If the commit has an explicit scope, it returns that scope.
// Otherwise, it falls back to the detected feature area from keywords.
// This ensures explicit scopes take priority over keyword matching.
func GetEffectiveScope(commit ParsedCommit) string {
	// If commit has an explicit scope, use it
	if commit.Scope != "" {
		return commit.Scope
	}
	
	// Fall back to detected feature area
	area := DetectFeatureArea(commit)
	return string(area)
}

// CommitGroup represents a group of related commits.
// Commits are grouped by their effective scope (explicit scope or detected feature area).
type CommitGroup struct {
	Name      string         // Group name (scope or detected feature area)
	Commits   []ParsedCommit // Commits in this group
	Summary   string         // Merged summary of changes (for groups > 3 commits)
	IsSummary bool           // Whether this group uses a summary vs individual list
}

// GroupCommits groups commits by their effective scope.
// Commits with the same effective scope are placed in the same group.
// Groups track whether they should be summarized (>3 commits).
func GroupCommits(commits []ParsedCommit) []CommitGroup {
	if len(commits) == 0 {
		return []CommitGroup{}
	}

	// Map to collect commits by effective scope
	groupMap := make(map[string][]ParsedCommit)
	// Track order of first appearance for consistent output
	order := make([]string, 0)

	for _, commit := range commits {
		scope := GetEffectiveScope(commit)
		if _, exists := groupMap[scope]; !exists {
			order = append(order, scope)
		}
		groupMap[scope] = append(groupMap[scope], commit)
	}

	// Build groups in order of first appearance
	groups := make([]CommitGroup, 0, len(order))
	for _, scope := range order {
		commits := groupMap[scope]
		group := CommitGroup{
			Name:      scope,
			Commits:   commits,
			IsSummary: ShouldSummarize(CommitGroup{Commits: commits}),
		}
		groups = append(groups, group)
	}

	return groups
}

// ShouldSummarize returns true if a group should be summarized.
// Groups with more than 3 commits should produce a summary instead of listing each commit.
func ShouldSummarize(group CommitGroup) bool {
	return len(group.Commits) > 3
}

// ExtractKeyFeatures identifies the main features from a group of commits.
// It extracts unique, meaningful descriptions from the commits, removing duplicates
// and preserving important technical details.
func ExtractKeyFeatures(commits []ParsedCommit) []string {
	if len(commits) == 0 {
		return []string{}
	}

	// Use a map to track unique features (case-insensitive deduplication)
	seen := make(map[string]bool)
	features := make([]string, 0, len(commits))

	for _, commit := range commits {
		// Clean the description for display
		cleaned := CleanDescription(commit.Description)
		if cleaned == "" {
			continue
		}

		// Remove trailing period for comparison
		normalized := strings.TrimSuffix(strings.ToLower(cleaned), ".")

		// Skip if we've seen a similar feature
		if seen[normalized] {
			continue
		}
		seen[normalized] = true

		features = append(features, cleaned)
	}

	return features
}

// SummarizeGroup creates a high-level summary for a commit group.
// For groups with more than 3 commits, it creates a concise summary paragraph
// that extracts key functionality rather than listing every implementation detail.
// It preserves important technical details that affect users.
func SummarizeGroup(group CommitGroup) string {
	if len(group.Commits) == 0 {
		return ""
	}

	// For small groups (<=3), don't summarize - return empty to indicate individual listing
	if !ShouldSummarize(group) {
		return ""
	}

	// Extract key features from the commits
	features := ExtractKeyFeatures(group.Commits)
	if len(features) == 0 {
		return ""
	}

	// Determine the primary action based on commit types
	primaryAction := determinePrimaryAction(group.Commits)

	// Build the summary
	var sb strings.Builder

	// Start with the group name and primary action
	groupName := group.Name
	if groupName == "" || groupName == string(FeatureAreaOther) {
		groupName = "Various"
	}

	// Create a summary based on the number of features
	if len(features) == 1 {
		// Single unique feature - just use it directly
		sb.WriteString(features[0])
	} else if len(features) <= 3 {
		// 2-3 unique features - list them concisely
		sb.WriteString(primaryAction)
		sb.WriteString(" ")
		sb.WriteString(strings.ToLower(groupName))
		sb.WriteString(": ")
		for i, feature := range features {
			// Remove trailing period for inline listing
			feature = strings.TrimSuffix(feature, ".")
			// Lowercase the first letter for inline listing
			if len(feature) > 0 {
				feature = strings.ToLower(feature[:1]) + feature[1:]
			}
			if i > 0 {
				if i == len(features)-1 {
					sb.WriteString(", and ")
				} else {
					sb.WriteString(", ")
				}
			}
			sb.WriteString(feature)
		}
		sb.WriteString(".")
	} else {
		// Many features - create a high-level summary
		sb.WriteString(primaryAction)
		sb.WriteString(" multiple ")
		sb.WriteString(strings.ToLower(groupName))
		sb.WriteString(" improvements including ")

		// Take the first 3 most important features
		topFeatures := features
		if len(topFeatures) > 3 {
			topFeatures = topFeatures[:3]
		}

		for i, feature := range topFeatures {
			// Remove trailing period for inline listing
			feature = strings.TrimSuffix(feature, ".")
			// Lowercase the first letter for inline listing
			if len(feature) > 0 {
				feature = strings.ToLower(feature[:1]) + feature[1:]
			}
			if i > 0 {
				if i == len(topFeatures)-1 {
					sb.WriteString(", and ")
				} else {
					sb.WriteString(", ")
				}
			}
			sb.WriteString(feature)
		}

		// Add indication of more changes
		remaining := len(features) - 3
		if remaining > 0 {
			sb.WriteString(fmt.Sprintf(", plus %d more enhancement", remaining))
			if remaining > 1 {
				sb.WriteString("s")
			}
		}
		sb.WriteString(".")
	}

	return sb.String()
}

// determinePrimaryAction determines the primary action verb based on commit types.
// It prioritizes feat > fix > perf > refactor > other types.
func determinePrimaryAction(commits []ParsedCommit) string {
	typeCounts := make(map[CommitType]int)
	for _, c := range commits {
		typeCounts[c.Type]++
	}

	// Priority order for determining primary action
	if typeCounts[CommitTypeFeat] > 0 {
		return "Enhanced"
	}
	if typeCounts[CommitTypeFix] > 0 {
		return "Fixed"
	}
	if typeCounts[CommitTypePerf] > 0 {
		return "Optimized"
	}
	if typeCounts[CommitTypeRefactor] > 0 {
		return "Improved"
	}
	if typeCounts[CommitTypeDocs] > 0 {
		return "Updated"
	}
	if typeCounts[CommitTypeBuild] > 0 {
		return "Updated"
	}

	return "Updated"
}

// ReleaseSection represents a section in the release notes.
// Each section groups commits by their semantic meaning (features, fixes, etc.).
type ReleaseSection string

const (
	SectionFeatures        ReleaseSection = "features"
	SectionImprovements    ReleaseSection = "improvements"
	SectionBugFixes        ReleaseSection = "bugfixes"
	SectionBreakingChanges ReleaseSection = "breaking"
	SectionOther           ReleaseSection = "other"
)

// SectionHeaders maps each section to its emoji-enhanced header.
// These headers are used when generating the release notes markdown.
var SectionHeaders = map[ReleaseSection]string{
	SectionFeatures:        "🍿 New Features & Enhancements",
	SectionImprovements:    "⚡ Performance & Improvements",
	SectionBugFixes:        "🐞 Bug Fixes",
	SectionBreakingChanges: "⚠️ Breaking Changes",
	SectionOther:           "🔄 Other Changes",
}

// SectionHeader returns the emoji-enhanced header for a section.
// Returns the appropriate header with emoji prefix based on the section type.
func SectionHeader(section ReleaseSection) string {
	if header, ok := SectionHeaders[section]; ok {
		return header
	}
	return "🔄 Other Changes"
}

// CategorizedContent holds commits organized by section.
// This is the result of categorizing commit groups into release note sections.
type CategorizedContent struct {
	Features        []CommitGroup // New features (feat commits)
	Improvements    []CommitGroup // Performance improvements (perf commits)
	BugFixes        []CommitGroup // Bug fixes (fix commits)
	BreakingChanges []CommitGroup // Breaking changes (commits with BreakingChange=true)
	Other           []CommitGroup // Other changes (remaining commits)
}

// HasContent returns true if any section has commits.
// Used to determine if there's any content to render in the release notes.
func (c CategorizedContent) HasContent() bool {
	return len(c.Features) > 0 ||
		len(c.Improvements) > 0 ||
		len(c.BugFixes) > 0 ||
		len(c.BreakingChanges) > 0 ||
		len(c.Other) > 0
}

// NonEmptySections returns a list of sections that have content.
// Useful for iterating only over sections that should be rendered.
func (c CategorizedContent) NonEmptySections() []ReleaseSection {
	var sections []ReleaseSection
	if len(c.BreakingChanges) > 0 {
		sections = append(sections, SectionBreakingChanges)
	}
	if len(c.Features) > 0 {
		sections = append(sections, SectionFeatures)
	}
	if len(c.Improvements) > 0 {
		sections = append(sections, SectionImprovements)
	}
	if len(c.BugFixes) > 0 {
		sections = append(sections, SectionBugFixes)
	}
	if len(c.Other) > 0 {
		sections = append(sections, SectionOther)
	}
	return sections
}

// GetSectionGroups returns the commit groups for a given section.
func (c CategorizedContent) GetSectionGroups(section ReleaseSection) []CommitGroup {
	switch section {
	case SectionFeatures:
		return c.Features
	case SectionImprovements:
		return c.Improvements
	case SectionBugFixes:
		return c.BugFixes
	case SectionBreakingChanges:
		return c.BreakingChanges
	case SectionOther:
		return c.Other
	default:
		return nil
	}
}

// CategorizeCommits organizes commit groups into sections based on commit types.
// - feat commits go to Features
// - fix commits go to BugFixes
// - perf commits go to Improvements
// - Commits with BreakingChange=true go to BreakingChanges (regardless of type)
// - Remaining commits go to Other
//
// Note: A commit with BreakingChange=true will appear in BreakingChanges AND
// its type-appropriate section (e.g., a breaking feat will be in both Features and BreakingChanges).
func CategorizeCommits(groups []CommitGroup) CategorizedContent {
	result := CategorizedContent{
		Features:        make([]CommitGroup, 0),
		Improvements:    make([]CommitGroup, 0),
		BugFixes:        make([]CommitGroup, 0),
		BreakingChanges: make([]CommitGroup, 0),
		Other:           make([]CommitGroup, 0),
	}

	for _, group := range groups {
		// Separate commits by their target section
		featCommits := make([]ParsedCommit, 0)
		fixCommits := make([]ParsedCommit, 0)
		perfCommits := make([]ParsedCommit, 0)
		breakingCommits := make([]ParsedCommit, 0)
		otherCommits := make([]ParsedCommit, 0)

		for _, commit := range group.Commits {
			// Breaking changes go to their own section
			if commit.BreakingChange {
				breakingCommits = append(breakingCommits, commit)
			}

			// Categorize by type
			switch commit.Type {
			case CommitTypeFeat:
				featCommits = append(featCommits, commit)
			case CommitTypeFix:
				fixCommits = append(fixCommits, commit)
			case CommitTypePerf:
				perfCommits = append(perfCommits, commit)
			default:
				// Only add to Other if not a breaking change (to avoid duplication)
				if !commit.BreakingChange {
					otherCommits = append(otherCommits, commit)
				}
			}
		}

		// Create groups for each section if there are commits
		if len(featCommits) > 0 {
			result.Features = append(result.Features, CommitGroup{
				Name:      group.Name,
				Commits:   featCommits,
				IsSummary: len(featCommits) > 3,
			})
		}

		if len(fixCommits) > 0 {
			result.BugFixes = append(result.BugFixes, CommitGroup{
				Name:      group.Name,
				Commits:   fixCommits,
				IsSummary: len(fixCommits) > 3,
			})
		}

		if len(perfCommits) > 0 {
			result.Improvements = append(result.Improvements, CommitGroup{
				Name:      group.Name,
				Commits:   perfCommits,
				IsSummary: len(perfCommits) > 3,
			})
		}

		if len(breakingCommits) > 0 {
			result.BreakingChanges = append(result.BreakingChanges, CommitGroup{
				Name:      group.Name,
				Commits:   breakingCommits,
				IsSummary: len(breakingCommits) > 3,
			})
		}

		if len(otherCommits) > 0 {
			result.Other = append(result.Other, CommitGroup{
				Name:      group.Name,
				Commits:   otherCommits,
				IsSummary: len(otherCommits) > 3,
			})
		}
	}

	return result
}

// CategorizeCommitsByType is a simpler categorization that works directly on commits
// without pre-grouping. It groups commits by type first, then by scope within each section.
func CategorizeCommitsByType(commits []ParsedCommit) CategorizedContent {
	// First group all commits by scope
	groups := GroupCommits(commits)
	// Then categorize the groups
	return CategorizeCommits(groups)
}

// FormatSectionItem formats a single commit or group for display in release notes.
// It uses consistent indentation (bullet point with space) for all items.
const SectionItemIndent = "- "

// FormatSectionItems formats all items in a section with consistent indentation.
// Each item is prefixed with "- " for bullet point formatting.
func FormatSectionItems(groups []CommitGroup) []string {
	var items []string

	for _, group := range groups {
		if group.IsSummary && len(group.Commits) > 3 {
			// Use summary for large groups
			summary := SummarizeGroup(group)
			if summary != "" {
				// Format as bold group name followed by summary
				item := fmt.Sprintf("%s**%s**: %s", SectionItemIndent, group.Name, summary)
				items = append(items, item)
			}
		} else {
			// List individual commits
			for _, commit := range group.Commits {
				desc := CleanDescription(commit.Description)
				if desc == "" {
					continue
				}
				if group.Name != "" && group.Name != string(FeatureAreaOther) {
					// Include scope/group name for context
					item := fmt.Sprintf("%s**%s**: %s", SectionItemIndent, group.Name, desc)
					items = append(items, item)
				} else {
					// No scope, just the description
					item := fmt.Sprintf("%s%s", SectionItemIndent, desc)
					items = append(items, item)
				}
			}
		}
	}

	return items
}

// FormatSection formats a complete section with header and items.
// Returns empty string if the section has no items.
func FormatSection(section ReleaseSection, groups []CommitGroup) string {
	if len(groups) == 0 {
		return ""
	}

	items := FormatSectionItems(groups)
	if len(items) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## ")
	sb.WriteString(SectionHeader(section))
	sb.WriteString("\n\n")

	for _, item := range items {
		sb.WriteString(item)
		sb.WriteString("\n")
	}

	return sb.String()
}

// titleSemverRegex matches semantic version strings like "1.0.0", "v1.0.0", "2.3.4"
// Named differently from version.go's semverRegex to avoid redeclaration.
var titleSemverRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

// IsMajorRelease checks if a version string represents a major release (X.0.0).
// A major release is defined as a version where both minor and patch are zero.
// Accepts versions with or without "v" prefix (e.g., "1.0.0" or "v1.0.0").
func IsMajorRelease(version string) bool {
	matches := titleSemverRegex.FindStringSubmatch(version)
	if matches == nil {
		return false
	}
	// matches[1] = major, matches[2] = minor, matches[3] = patch
	return matches[2] == "0" && matches[3] == "0"
}

// ExtractMajorVersion extracts the major version number from a version string.
// Returns -1 if the version string is invalid.
func ExtractMajorVersion(version string) int {
	matches := titleSemverRegex.FindStringSubmatch(version)
	if matches == nil {
		return -1
	}
	major := 0
	fmt.Sscanf(matches[1], "%d", &major)
	return major
}

// MajorReleaseTitles provides creative title templates for major releases.
// The key is the major version number, and the value is the creative title.
var MajorReleaseTitles = map[int]string{
	1: "The Beginning",
	2: "A New World",
	3: "The Next Chapter",
	4: "Rising Higher",
	5: "Breaking Boundaries",
}

// GetMajorVersionTitle returns a creative title for a major release.
// For known major versions, it returns a predefined creative title.
// For unknown major versions, it generates a generic creative title.
func GetMajorVersionTitle(majorVersion int) string {
	if title, ok := MajorReleaseTitles[majorVersion]; ok {
		return fmt.Sprintf("%s with %d.0", title, majorVersion)
	}
	// Generic creative title for unknown major versions
	return fmt.Sprintf("A New Era with %d.0", majorVersion)
}

// DefaultProjectName is the default project name used in titles and closings.
const DefaultProjectName = "Narvana"

// GenerateTitle creates a title based on version and optional override.
// If an override is provided (non-empty), it is used regardless of version.
// For major versions (X.0.0), a creative title is generated.
// For minor/patch versions, the standard "Narvana vX.Y.Z" format is used.
func GenerateTitle(version string, override string) string {
	// If override is provided, use it
	if override != "" {
		return override
	}

	// Clean version string (remove leading "v" if present for display)
	cleanVersion := strings.TrimPrefix(version, "v")
	if cleanVersion == "" {
		cleanVersion = version
	}

	// Check if this is a major release
	if IsMajorRelease(version) {
		majorVersion := ExtractMajorVersion(version)
		if majorVersion > 0 {
			return GetMajorVersionTitle(majorVersion)
		}
	}

	// Standard title for minor/patch releases
	return fmt.Sprintf("%s v%s", DefaultProjectName, cleanVersion)
}

// DefaultIntroduction is the default introduction message for release notes.
// It provides a friendly greeting to users about the new release.
const DefaultIntroduction = "Hey there, Narvana users! We're back with some exciting updates that will turbocharge your Narvana experience. Here's the lowdown:"

// GenerateIntroduction creates the introduction paragraph for release notes.
// If an override is provided (non-empty), it is used instead of the default.
// The introduction appears at the beginning of the release notes, before any sections.
func GenerateIntroduction(override string) string {
	if override != "" {
		return strings.TrimSpace(override)
	}
	return DefaultIntroduction
}

// DefaultClosingTemplate is the default closing message template for release notes.
// It includes a placeholder for the project name that will be dynamically replaced.
const DefaultClosingTemplate = "Thank you for making %s your tech partner. We thrive on your feedback, so if you have ideas or run into bumps, don't hesitate to drop a line to our support wizards. Together, we're taking %s to the next level!"

// GenerateClosing creates the closing paragraph for release notes.
// If an override is provided (non-empty), it is used instead of the default.
// The closing appears at the end of the release notes, after all sections.
// The project name is dynamically inserted into the default closing message.
func GenerateClosing(projectName string, override string) string {
	if override != "" {
		return strings.TrimSpace(override)
	}
	
	// Use default project name if not provided
	if projectName == "" {
		projectName = DefaultProjectName
	}
	
	return fmt.Sprintf(DefaultClosingTemplate, projectName, projectName)
}


// ReleaseNotesConfig holds configuration for generating release notes.
// It contains all the information needed to produce a complete release entry.
type ReleaseNotesConfig struct {
	Version      string // The version number (e.g., "1.0.0" or "v1.0.0")
	Date         string // The release date in YYYY-MM-DD format
	Title        string // Optional custom title (from override file)
	Introduction string // Optional custom introduction (from override file)
	Closing      string // Optional custom closing (from override file)
	BannerPath   string // Path to banner image (relative to content directory)
	ProjectName  string // Project name for templates (default: "Narvana")
}

// ReleaseNotesFrontmatter represents the YAML frontmatter for a release entry.
type ReleaseNotesFrontmatter struct {
	Title         string                      `yaml:"title"`
	Date          string                      `yaml:"date"`
	VersionNumber string                      `yaml:"versionNumber"`
	Description   string                      `yaml:"description"`
	Image         ReleaseNotesFrontmatterImage `yaml:"image"`
}

// ReleaseNotesFrontmatterImage represents the image field in frontmatter.
type ReleaseNotesFrontmatterImage struct {
	Src string `yaml:"src"`
	Alt string `yaml:"alt"`
}

// GenerateReleaseNotes produces the final markdown release notes.
// It accepts CategorizedContent and config, generating complete markdown with frontmatter,
// introduction, sections, and closing.
func GenerateReleaseNotes(content CategorizedContent, config ReleaseNotesConfig) (string, error) {
	// Validate required fields
	if config.Version == "" {
		return "", fmt.Errorf("version is required")
	}
	if config.Date == "" {
		return "", fmt.Errorf("date is required")
	}

	// Set default project name if not provided
	projectName := config.ProjectName
	if projectName == "" {
		projectName = DefaultProjectName
	}

	// Generate title
	title := GenerateTitle(config.Version, config.Title)

	// Clean version for display (remove leading "v" if present)
	cleanVersion := strings.TrimPrefix(config.Version, "v")

	// Generate banner path if not provided
	bannerPath := config.BannerPath
	if bannerPath == "" {
		// Default banner path format: ../../assets/release-X_Y_Z.svg
		bannerPath = generateDefaultBannerPath(cleanVersion)
	}

	// Build the markdown output
	var sb strings.Builder

	// Write frontmatter
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("title: %q\n", title))
	sb.WriteString(fmt.Sprintf("date: %q\n", config.Date))
	sb.WriteString(fmt.Sprintf("versionNumber: %q\n", cleanVersion))
	sb.WriteString(fmt.Sprintf("description: \"Release notes for %s v%s\"\n", projectName, cleanVersion))
	sb.WriteString("image:\n")
	sb.WriteString(fmt.Sprintf("  src: %q\n", bannerPath))
	sb.WriteString(fmt.Sprintf("  alt: \"%s v%s Release\"\n", projectName, cleanVersion))
	sb.WriteString("---\n\n")

	// Write introduction
	intro := GenerateIntroduction(config.Introduction)
	sb.WriteString(intro)
	sb.WriteString("\n\n")

	// Write sections (only non-empty ones)
	sectionsWritten := false
	for _, section := range content.NonEmptySections() {
		groups := content.GetSectionGroups(section)
		formatted := FormatSection(section, groups)
		if formatted != "" {
			sb.WriteString(formatted)
			sb.WriteString("\n")
			sectionsWritten = true
		}
	}

	// If no sections were written, add a placeholder message
	if !sectionsWritten {
		sb.WriteString("No significant changes in this release.\n\n")
	}

	// Write closing
	closing := GenerateClosing(projectName, config.Closing)
	sb.WriteString(closing)
	sb.WriteString("\n")

	return sb.String(), nil
}

// generateDefaultBannerPath generates the default banner path for a version.
// The path is relative to the content directory and uses underscores instead of dots.
func generateDefaultBannerPath(version string) string {
	// Replace dots with underscores for the filename
	safeVersion := strings.ReplaceAll(version, ".", "_")
	return fmt.Sprintf("../../assets/release-%s.svg", safeVersion)
}

// ParsedReleaseNotes represents the parsed structure of release notes.
// This is used for round-trip testing to verify the structure can be reconstructed.
type ParsedReleaseNotes struct {
	Frontmatter ReleaseNotesFrontmatter
	Introduction string
	Sections     map[ReleaseSection][]string // Section -> list of items
	Closing      string
}

// ParseReleaseNotesFrontmatter extracts frontmatter from release notes markdown.
// It returns the frontmatter struct and the remaining content after frontmatter.
func ParseReleaseNotesFrontmatter(markdown string) (*ReleaseNotesFrontmatter, string, error) {
	// Check for frontmatter delimiters
	if !strings.HasPrefix(markdown, "---\n") {
		return nil, markdown, fmt.Errorf("no frontmatter found: missing opening delimiter")
	}

	// Find the closing delimiter
	rest := markdown[4:] // Skip opening "---\n"
	endIdx := strings.Index(rest, "\n---\n")
	if endIdx == -1 {
		// Try with just "---" at end of line
		endIdx = strings.Index(rest, "\n---")
		if endIdx == -1 {
			return nil, markdown, fmt.Errorf("no frontmatter found: missing closing delimiter")
		}
	}

	frontmatterYAML := rest[:endIdx]
	content := strings.TrimPrefix(rest[endIdx+4:], "\n")

	// Parse the YAML frontmatter manually (simple key-value parsing)
	fm := &ReleaseNotesFrontmatter{}
	lines := strings.Split(frontmatterYAML, "\n")
	
	inImage := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "image:") {
			inImage = true
			continue
		}

		if inImage {
			if strings.HasPrefix(line, "src:") {
				fm.Image.Src = extractYAMLValue(line[4:])
			} else if strings.HasPrefix(line, "alt:") {
				fm.Image.Alt = extractYAMLValue(line[4:])
			} else if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				inImage = false
			}
		}

		if !inImage {
			if strings.HasPrefix(line, "title:") {
				fm.Title = extractYAMLValue(line[6:])
			} else if strings.HasPrefix(line, "date:") {
				fm.Date = extractYAMLValue(line[5:])
			} else if strings.HasPrefix(line, "versionNumber:") {
				fm.VersionNumber = extractYAMLValue(line[14:])
			} else if strings.HasPrefix(line, "description:") {
				fm.Description = extractYAMLValue(line[12:])
			}
		}
	}

	return fm, content, nil
}

// extractYAMLValue extracts a value from a YAML line, handling quoted strings.
func extractYAMLValue(s string) string {
	s = strings.TrimSpace(s)
	// Remove surrounding quotes if present
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}


// OverrideContent holds parsed override file content.
// Override files allow customization of release notes with custom titles,
// introductions, and closing messages.
type OverrideContent struct {
	Title        string // Custom title for the release
	Introduction string // Custom introduction paragraph
	Closing      string // Custom closing paragraph
}

// ParseOverrideFile reads and parses an override file.
// Override files use YAML frontmatter format with optional title, introduction, and closing fields.
// If the file doesn't exist or is malformed, it returns nil without error (graceful fallback).
// Returns an error only for unexpected I/O errors.
func ParseOverrideFile(path string) (*OverrideContent, error) {
	// Read the file
	data, err := readFileFunc(path)
	if err != nil {
		// File doesn't exist or can't be read - this is not an error, just return nil
		return nil, nil
	}

	return ParseOverrideContent(string(data))
}

// readFileFunc is a variable to allow testing with mock file reads.
// In production, this is set to os.ReadFile.
var readFileFunc = defaultReadFile

// defaultReadFile reads a file using os.ReadFile.
func defaultReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// ParseOverrideContent parses override content from a string.
// This is separated from ParseOverrideFile to allow testing without file I/O.
func ParseOverrideContent(content string) (*OverrideContent, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, nil
	}

	// Check for frontmatter delimiters
	if !strings.HasPrefix(content, "---") {
		// No frontmatter, treat entire content as introduction
		return &OverrideContent{
			Introduction: content,
		}, nil
	}

	// Find the closing delimiter
	rest := content[3:] // Skip opening "---"
	rest = strings.TrimPrefix(rest, "\n")
	
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		// Malformed frontmatter - no closing delimiter
		// Treat as no override (graceful fallback)
		return nil, nil
	}

	frontmatterYAML := rest[:endIdx]
	bodyContent := strings.TrimSpace(rest[endIdx+4:])

	// Parse the YAML frontmatter
	override := &OverrideContent{}
	
	// Parse line by line, handling multi-line values with | or >
	lines := strings.Split(frontmatterYAML, "\n")
	var currentKey string
	var multiLineValue strings.Builder
	inMultiLine := false
	multiLineIndent := 0

	for i, line := range lines {
		// Check if this is a continuation of a multi-line value
		if inMultiLine {
			// Check if line is indented (part of multi-line value)
			trimmed := strings.TrimLeft(line, " \t")
			indent := len(line) - len(trimmed)
			
			if indent >= multiLineIndent && trimmed != "" {
				// This is a continuation line
				if multiLineValue.Len() > 0 {
					multiLineValue.WriteString("\n")
				}
				multiLineValue.WriteString(trimmed)
				continue
			} else if trimmed == "" && i < len(lines)-1 {
				// Empty line in multi-line value
				multiLineValue.WriteString("\n")
				continue
			} else {
				// End of multi-line value
				setOverrideField(override, currentKey, strings.TrimSpace(multiLineValue.String()))
				inMultiLine = false
				multiLineValue.Reset()
			}
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse key: value
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])

		// Check for multi-line indicator
		if value == "|" || value == ">" {
			currentKey = key
			inMultiLine = true
			multiLineIndent = 2 // Standard YAML indent
			multiLineValue.Reset()
			continue
		}

		// Single-line value
		value = extractYAMLValue(value)
		setOverrideField(override, key, value)
	}

	// Handle any remaining multi-line value
	if inMultiLine && multiLineValue.Len() > 0 {
		setOverrideField(override, currentKey, strings.TrimSpace(multiLineValue.String()))
	}

	// If there's body content after frontmatter and no introduction was set,
	// use the body as introduction
	if override.Introduction == "" && bodyContent != "" {
		override.Introduction = bodyContent
	}

	return override, nil
}

// setOverrideField sets a field on OverrideContent based on the key name.
func setOverrideField(override *OverrideContent, key, value string) {
	switch strings.ToLower(key) {
	case "title":
		override.Title = value
	case "introduction", "intro":
		override.Introduction = value
	case "closing", "close":
		override.Closing = value
	}
}

// ParseCommitWithFallback parses a raw commit message with fallback handling.
// If parsing fails or produces an "other" type commit, it ensures the raw message
// is preserved for display in the "Other Changes" section.
// This function wraps ParseCommit and ensures graceful degradation.
func ParseCommitWithFallback(raw string) ParsedCommit {
	commit := ParseCommit(raw)
	
	// If the commit type is "other", ensure the raw message is preserved
	// and the description is set to something displayable
	if commit.Type == CommitTypeOther {
		// If description is empty but we have a raw message, use the raw message
		if commit.Description == "" && commit.Raw != "" {
			// Use the first line of the raw message as description
			lines := strings.SplitN(commit.Raw, "\n", 2)
			commit.Description = strings.TrimSpace(lines[0])
		}
		// Ensure raw is always set for fallback commits
		if commit.Raw == "" {
			commit.Raw = raw
		}
	}
	
	return commit
}

// ParseCommitsWithFallback parses multiple raw commit messages with fallback handling.
// Any commits that fail to parse are categorized as "other" with their raw message preserved.
// This ensures the release notes generation never fails due to unparseable commits.
func ParseCommitsWithFallback(rawCommits []string) []ParsedCommit {
	commits := make([]ParsedCommit, 0, len(rawCommits))
	for _, raw := range rawCommits {
		commit := ParseCommitWithFallback(raw)
		commits = append(commits, commit)
	}
	return commits
}

// GroupCommitsWithFallback groups commits by their effective scope with fallback handling.
// If grouping fails for any reason, it falls back to listing commits by type without grouping.
// This ensures the release notes generation never fails due to grouping errors.
func GroupCommitsWithFallback(commits []ParsedCommit) []CommitGroup {
	// Attempt normal grouping
	groups := GroupCommits(commits)
	
	// If grouping produced no results but we have commits, fall back to type-based listing
	if len(groups) == 0 && len(commits) > 0 {
		return groupCommitsByTypeOnly(commits)
	}
	
	return groups
}

// groupCommitsByTypeOnly creates groups based solely on commit type.
// This is a fallback when scope-based grouping fails or produces no results.
// Each commit type becomes its own group, ensuring all commits are included.
func groupCommitsByTypeOnly(commits []ParsedCommit) []CommitGroup {
	if len(commits) == 0 {
		return []CommitGroup{}
	}

	// Map to collect commits by type
	typeMap := make(map[CommitType][]ParsedCommit)
	// Track order of first appearance for consistent output
	order := make([]CommitType, 0)

	for _, commit := range commits {
		if _, exists := typeMap[commit.Type]; !exists {
			order = append(order, commit.Type)
		}
		typeMap[commit.Type] = append(typeMap[commit.Type], commit)
	}

	// Build groups in order of first appearance
	groups := make([]CommitGroup, 0, len(order))
	for _, commitType := range order {
		typeCommits := typeMap[commitType]
		group := CommitGroup{
			Name:      getTypeDisplayName(commitType),
			Commits:   typeCommits,
			IsSummary: len(typeCommits) > 3,
		}
		groups = append(groups, group)
	}

	return groups
}

// getTypeDisplayName returns a human-readable display name for a commit type.
// Used when falling back to type-based grouping.
func getTypeDisplayName(t CommitType) string {
	switch t {
	case CommitTypeFeat:
		return "Features"
	case CommitTypeFix:
		return "Bug Fixes"
	case CommitTypeDocs:
		return "Documentation"
	case CommitTypeStyle:
		return "Style"
	case CommitTypeRefactor:
		return "Refactoring"
	case CommitTypePerf:
		return "Performance"
	case CommitTypeTest:
		return "Testing"
	case CommitTypeBuild:
		return "Build"
	case CommitTypeCI:
		return "CI/CD"
	case CommitTypeChore:
		return "Chores"
	case CommitTypeOther:
		return "Other Changes"
	default:
		return "Other Changes"
	}
}

// CategorizeCommitsWithFallback categorizes commit groups into sections with fallback handling.
// If normal categorization fails, it falls back to placing all commits in the "Other" section.
// This ensures the release notes generation never fails due to categorization errors.
func CategorizeCommitsWithFallback(groups []CommitGroup) CategorizedContent {
	// Attempt normal categorization
	result := CategorizeCommits(groups)
	
	// If categorization produced no content but we have groups, fall back to Other section
	if !result.HasContent() && len(groups) > 0 {
		// Place all commits in Other section
		result.Other = groups
	}
	
	return result
}

// ProcessCommitsWithFallback is a high-level function that processes raw commit messages
// through the entire pipeline with fallback handling at each stage.
// It parses, filters, groups, and categorizes commits, ensuring graceful degradation
// at each step if errors occur.
func ProcessCommitsWithFallback(rawCommits []string, filterConfig NoiseFilterConfig) CategorizedContent {
	// Step 1: Parse commits with fallback
	commits := ParseCommitsWithFallback(rawCommits)
	
	// Step 2: Filter noise commits (this is safe and won't fail)
	filterResult := FilterCommits(commits, filterConfig)
	
	// Step 3: Group commits with fallback
	groups := GroupCommitsWithFallback(filterResult.Commits)
	
	// Step 4: Categorize with fallback
	return CategorizeCommitsWithFallback(groups)
}

// EnsureOtherCommitsPreserved ensures that commits with type "other" are properly
// preserved in the categorized content. This is a validation function that can be
// used to verify the fallback behavior is working correctly.
func EnsureOtherCommitsPreserved(commits []ParsedCommit, content CategorizedContent) bool {
	// Count "other" type commits in input
	otherCount := 0
	for _, c := range commits {
		if c.Type == CommitTypeOther && !c.BreakingChange {
			otherCount++
		}
	}
	
	// Count commits in Other section
	otherInContent := 0
	for _, group := range content.Other {
		otherInContent += len(group.Commits)
	}
	
	// All "other" type commits should be in the Other section
	return otherInContent >= otherCount
}
