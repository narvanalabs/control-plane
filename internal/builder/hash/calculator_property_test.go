package hash

import (
	"os"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: flexible-build-strategies, Property 10: Vendor Hash Retry**
// For any Go build where initial vendor hash calculation fails, the Build_System
// SHALL retry with a fake hash and extract the correct hash from the Nix error output.
// **Validates: Requirements 3.7**

// genBase64Char generates valid base64 characters.
func genBase64Char() gopter.Gen {
	return gen.OneConstOf(
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
		'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
		'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
		'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '+', '/',
	)
}

// genBase64String generates a valid base64 string of specified length.
func genBase64String(length int) gopter.Gen {
	return gen.SliceOfN(length, genBase64Char()).Map(func(chars []rune) string {
		result := make([]byte, len(chars))
		for i, c := range chars {
			result[i] = byte(c)
		}
		return string(result)
	})
}

// genValidSRIHash generates valid SRI hashes.
func genValidSRIHash() gopter.Gen {
	// SHA256 base64 is 44 characters (43 + padding)
	return genBase64String(43).Map(func(base64Part string) string {
		return "sha256-" + base64Part + "="
	})
}

// genNixErrorOutput generates simulated Nix error output containing hash mismatches.
func genNixErrorOutput() gopter.Gen {
	return genValidSRIHash().Map(func(hash string) string {
		return "error: hash mismatch in fixed-output derivation '/nix/store/abc123-source':\n" +
			"  specified: sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n" +
			"  got:       " + hash
	})
}

func TestVendorHashRetryExtraction(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("extractHashFromNixOutput extracts hash from valid error output", prop.ForAll(
		func(nixOutput string) bool {
			hash := extractHashFromNixOutput(nixOutput)
			// Should extract a hash that starts with sha256-
			if hash == "" {
				return false
			}
			return len(hash) > 7 && hash[:7] == "sha256-"
		},
		genNixErrorOutput(),
	))

	properties.TestingRun(t)
}

func TestValidSRIHashRecognition(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("IsValidSRIHash returns true for valid SRI hashes", prop.ForAll(
		func(hash string) bool {
			return IsValidSRIHash(hash)
		},
		genValidSRIHash(),
	))

	properties.Property("IsValidSRIHash returns false for hashes without sha256 prefix", prop.ForAll(
		func(randomStr string) bool {
			// Strings without sha256- prefix should be invalid
			return !IsValidSRIHash(randomStr)
		},
		gen.OneConstOf(
			"md5-abc123",
			"sha512-xyz789",
			"invalid",
			"",
			"sha256",
			"sha256-",
		),
	))

	properties.TestingRun(t)
}

func TestFakeHashDetection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	calc := NewCalculator()

	properties.Property("IsFakeHash correctly identifies the fake hash", prop.ForAll(
		func(isFakeInput bool) bool {
			var hash string
			if isFakeInput {
				hash = calc.FakeHash
			} else {
				hash = "sha256-RealHashValueThatIsNotTheFakeOne123456789AB="
			}
			isFake := calc.IsFakeHash(hash)
			return isFake == isFakeInput
		},
		gen.Bool(),
	))

	properties.TestingRun(t)
}

func TestCalculatorConfiguration(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("WithMaxRetries sets correct retry count", prop.ForAll(
		func(retries int) bool {
			if retries < 0 {
				retries = 0
			}
			calc := NewCalculatorWithOptions(WithMaxRetries(retries))
			return calc.MaxRetries == retries
		},
		gen.IntRange(0, 10),
	))

	properties.Property("WithFakeHash sets correct fake hash", prop.ForAll(
		func(hash string) bool {
			calc := NewCalculatorWithOptions(WithFakeHash(hash))
			return calc.FakeHash == hash
		},
		genValidSRIHash(),
	))

	properties.TestingRun(t)
}

// TestHashExtractionPatterns tests various Nix error output formats.
func TestHashExtractionPatterns(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generate different Nix error output formats
	genNixErrorFormats := func() gopter.Gen {
		return genValidSRIHash().Map(func(hash string) []string {
			return []string{
				// Format 1: Simple got:
				"got: " + hash,
				// Format 2: With specified:
				"specified: sha256-AAAA got: " + hash,
				// Format 3: Hash mismatch format
				"hash mismatch in derivation got: " + hash,
			}
		})
	}

	properties.Property("extractHashFromNixOutput handles all known formats", prop.ForAll(
		func(formats []string) bool {
			for _, format := range formats {
				hash := extractHashFromNixOutput(format)
				if hash == "" || !IsValidSRIHash(hash) {
					return false
				}
			}
			return true
		},
		genNixErrorFormats(),
	))

	properties.TestingRun(t)
}

// TestHashMismatchErrorDetection tests the isHashMismatchError function.
func TestHashMismatchErrorDetection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generate error messages that should be detected as hash mismatches
	genHashMismatchError := func() gopter.Gen {
		return gen.OneConstOf(
			"hash mismatch in fixed-output derivation",
			"specified: sha256-AAA got: sha256-BBB",
			"got: sha256-CCCC",
		).Map(func(msg string) error {
			return &testError{msg: msg}
		})
	}

	// Generate error messages that should NOT be detected as hash mismatches
	genOtherError := func() gopter.Gen {
		return gen.OneConstOf(
			"network error",
			"file not found",
			"permission denied",
			"timeout exceeded",
		).Map(func(msg string) error {
			return &testError{msg: msg}
		})
	}

	properties.Property("isHashMismatchError returns true for hash mismatch errors", prop.ForAll(
		func(err error) bool {
			return isHashMismatchError(err)
		},
		genHashMismatchError(),
	))

	properties.Property("isHashMismatchError returns false for other errors", prop.ForAll(
		func(err error) bool {
			return !isHashMismatchError(err)
		},
		genOtherError(),
	))

	properties.Property("isHashMismatchError returns false for nil", prop.ForAll(
		func(_ int) bool {
			return !isHashMismatchError(nil)
		},
		gen.Int(),
	))

	properties.TestingRun(t)
}

// testError is a simple error type for testing.
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}


// TestNpmHashCalculation tests npm hash calculation with different lock files.
func TestNpmHashCalculation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generate package-lock.json content
	genPackageLockContent := func() gopter.Gen {
		return gen.OneConstOf(
			`{"name": "test", "lockfileVersion": 2}`,
			`{"name": "app", "lockfileVersion": 3, "packages": {}}`,
			`{"name": "project", "version": "1.0.0"}`,
		)
	}

	properties.Property("CalculateNpmHash produces valid SRI hash for package-lock.json", prop.ForAll(
		func(content string) bool {
			// Create a temporary directory
			dir := t.TempDir()

			// Write package-lock.json
			lockPath := dir + "/package-lock.json"
			if err := writeFile(lockPath, content); err != nil {
				return false
			}

			// Calculate hash
			calc := NewCalculator()
			hash, err := calc.CalculateNpmHash(t.Context(), dir)
			if err != nil {
				return false
			}

			// Verify it's a valid SRI hash
			return IsValidSRIHash(hash)
		},
		genPackageLockContent(),
	))

	properties.TestingRun(t)
}

// TestNpmHashDeterminism tests that npm hash calculation is deterministic.
func TestNpmHashDeterminism(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("same lock file content produces same hash", prop.ForAll(
		func(content string) bool {
			// Create two temporary directories with same content
			dir1 := t.TempDir()
			dir2 := t.TempDir()

			// Write same content to both
			if err := writeFile(dir1+"/package-lock.json", content); err != nil {
				return false
			}
			if err := writeFile(dir2+"/package-lock.json", content); err != nil {
				return false
			}

			// Calculate hashes
			calc := NewCalculator()
			hash1, err1 := calc.CalculateNpmHash(t.Context(), dir1)
			hash2, err2 := calc.CalculateNpmHash(t.Context(), dir2)

			if err1 != nil || err2 != nil {
				return false
			}

			// Hashes should be identical
			return hash1 == hash2
		},
		gen.OneConstOf(
			`{"name": "test", "lockfileVersion": 2}`,
			`{"name": "app", "version": "1.0.0"}`,
		),
	))

	properties.TestingRun(t)
}

// writeFile is a helper to write content to a file.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}


// TestCargoHashCalculation tests Cargo hash calculation.
func TestCargoHashCalculation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Generate Cargo.lock content
	genCargoLockContent := func() gopter.Gen {
		return gen.OneConstOf(
			`# This file is automatically @generated by Cargo.
[[package]]
name = "test"
version = "0.1.0"
`,
			`[[package]]
name = "app"
version = "1.0.0"
dependencies = []
`,
			`# Cargo.lock
[[package]]
name = "project"
version = "2.0.0"
source = "registry+https://github.com/rust-lang/crates.io-index"
`,
		)
	}

	properties.Property("CalculateCargoHash produces valid SRI hash for Cargo.lock", prop.ForAll(
		func(content string) bool {
			// Create a temporary directory
			dir := t.TempDir()

			// Write Cargo.lock
			lockPath := dir + "/Cargo.lock"
			if err := writeFile(lockPath, content); err != nil {
				return false
			}

			// Calculate hash
			calc := NewCalculator()
			hash, err := calc.CalculateCargoHash(t.Context(), dir)
			if err != nil {
				return false
			}

			// Verify it's a valid SRI hash
			return IsValidSRIHash(hash)
		},
		genCargoLockContent(),
	))

	properties.TestingRun(t)
}

// TestCargoHashDeterminism tests that Cargo hash calculation is deterministic.
func TestCargoHashDeterminism(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("same Cargo.lock content produces same hash", prop.ForAll(
		func(content string) bool {
			// Create two temporary directories with same content
			dir1 := t.TempDir()
			dir2 := t.TempDir()

			// Write same content to both
			if err := writeFile(dir1+"/Cargo.lock", content); err != nil {
				return false
			}
			if err := writeFile(dir2+"/Cargo.lock", content); err != nil {
				return false
			}

			// Calculate hashes
			calc := NewCalculator()
			hash1, err1 := calc.CalculateCargoHash(t.Context(), dir1)
			hash2, err2 := calc.CalculateCargoHash(t.Context(), dir2)

			if err1 != nil || err2 != nil {
				return false
			}

			// Hashes should be identical
			return hash1 == hash2
		},
		gen.OneConstOf(
			`[[package]]
name = "test"
version = "0.1.0"
`,
			`[[package]]
name = "app"
version = "1.0.0"
`,
		),
	))

	properties.TestingRun(t)
}

// TestHashResultMethods tests the *WithResult methods.
func TestHashResultMethods(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("CalculateNpmHashWithResult returns correct source file", prop.ForAll(
		func(lockType int) bool {
			dir := t.TempDir()
			calc := NewCalculator()

			var expectedSource string
			switch lockType {
			case 0:
				writeFile(dir+"/package-lock.json", `{"name": "test"}`)
				expectedSource = "package-lock.json"
			case 1:
				writeFile(dir+"/yarn.lock", "# yarn lockfile v1")
				expectedSource = "yarn.lock"
			case 2:
				writeFile(dir+"/pnpm-lock.yaml", "lockfileVersion: 5.4")
				expectedSource = "pnpm-lock.yaml"
			}

			result, err := calc.CalculateNpmHashWithResult(t.Context(), dir)
			if err != nil {
				return false
			}

			return result.SourceFile == expectedSource && result.Algorithm == "sha256"
		},
		gen.IntRange(0, 2),
	))

	properties.Property("CalculateCargoHashWithResult returns correct metadata", prop.ForAll(
		func(_ int) bool {
			dir := t.TempDir()
			writeFile(dir+"/Cargo.lock", `[[package]]
name = "test"
version = "0.1.0"
`)

			calc := NewCalculator()
			result, err := calc.CalculateCargoHashWithResult(t.Context(), dir)
			if err != nil {
				return false
			}

			return result.SourceFile == "Cargo.lock" && result.Algorithm == "sha256" && IsValidSRIHash(result.Hash)
		},
		gen.Int(),
	))

	properties.TestingRun(t)
}
