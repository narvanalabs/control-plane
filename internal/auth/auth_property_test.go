package auth

import (
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: control-plane, Property 18: JWT token round-trip**
// For any valid user credentials, generating a JWT token and then validating it
// should extract the same user identity.
// **Validates: Requirements 7.1, 7.2**

// genUserID generates a valid user ID (non-empty alphanumeric string).
func genUserID() gopter.Gen {
	return gen.Identifier().SuchThat(func(s string) bool {
		return len(s) > 0 && len(s) <= 255
	})
}

// genEmail generates a valid email-like string.
func genEmail() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	).Map(func(vals []interface{}) string {
		return vals[0].(string) + "@" + vals[1].(string) + ".com"
	})
}

// genJWTSecret generates a valid JWT secret (at least 32 bytes).
func genJWTSecret() gopter.Gen {
	return gen.SliceOfN(32, gen.UInt8()).Map(func(bytes []uint8) []byte {
		result := make([]byte, len(bytes))
		for i, b := range bytes {
			result[i] = byte(b)
		}
		return result
	})
}

func TestJWTTokenRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("JWT token round-trip preserves user identity", prop.ForAll(
		func(userID, email string, secret []byte) bool {
			// Create auth service with the generated secret
			cfg := &Config{
				JWTSecret:   secret,
				TokenExpiry: 1 * time.Hour,
			}
			svc := NewService(cfg, nil, nil)

			// Generate a token
			token, err := svc.GenerateToken(userID, email)
			if err != nil {
				return false
			}

			// Validate the token
			claims, err := svc.ValidateToken(token)
			if err != nil {
				return false
			}

			// Verify the user identity is preserved
			return claims.UserID == userID && claims.Email == email
		},
		genUserID(),
		genEmail(),
		genJWTSecret(),
	))

	properties.TestingRun(t)
}


// **Feature: control-plane, Property 19: Invalid token rejection**
// For any malformed or expired token, validation should fail with an unauthorized error.
// **Validates: Requirements 7.3**

// genMalformedToken generates various types of malformed tokens.
func genMalformedToken() gopter.Gen {
	return gen.OneGenOf(
		// Empty string
		gen.Const(""),
		// Random string (not a valid JWT)
		gen.AlphaString().SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) < 100
		}),
		// String with dots but not valid JWT structure
		gopter.CombineGens(
			gen.AlphaString(),
			gen.AlphaString(),
			gen.AlphaString(),
		).Map(func(vals []interface{}) string {
			return vals[0].(string) + "." + vals[1].(string) + "." + vals[2].(string)
		}),
		// Valid-looking but tampered JWT (modified payload)
		gen.Const("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.tampered_signature"),
	)
}

func TestInvalidTokenRejection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Malformed tokens are rejected", prop.ForAll(
		func(malformedToken string, secret []byte) bool {
			cfg := &Config{
				JWTSecret:   secret,
				TokenExpiry: 1 * time.Hour,
			}
			svc := NewService(cfg, nil, nil)

			// Attempt to validate the malformed token
			claims, err := svc.ValidateToken(malformedToken)

			// Should return an error and nil claims
			return err != nil && claims == nil
		},
		genMalformedToken(),
		genJWTSecret(),
	))

	properties.TestingRun(t)
}

func TestExpiredTokenRejection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Expired tokens are rejected", prop.ForAll(
		func(userID, email string, secret []byte) bool {
			// Create auth service with very short expiry
			cfg := &Config{
				JWTSecret:   secret,
				TokenExpiry: -1 * time.Hour, // Already expired
			}
			svc := NewService(cfg, nil, nil)

			// Generate a token (will be expired immediately)
			token, err := svc.GenerateToken(userID, email)
			if err != nil {
				return false
			}

			// Attempt to validate the expired token
			claims, err := svc.ValidateToken(token)

			// Should return an error (specifically ErrExpiredToken) and nil claims
			return err != nil && claims == nil
		},
		genUserID(),
		genEmail(),
		genJWTSecret(),
	))

	properties.TestingRun(t)
}

func TestWrongSecretRejection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Tokens signed with different secret are rejected", prop.ForAll(
		func(userID, email string, secret1, secret2 []byte) bool {
			// Ensure secrets are different
			if string(secret1) == string(secret2) {
				return true // Skip this case
			}

			// Create token with first secret
			cfg1 := &Config{
				JWTSecret:   secret1,
				TokenExpiry: 1 * time.Hour,
			}
			svc1 := NewService(cfg1, nil, nil)

			token, err := svc1.GenerateToken(userID, email)
			if err != nil {
				return false
			}

			// Try to validate with different secret
			cfg2 := &Config{
				JWTSecret:   secret2,
				TokenExpiry: 1 * time.Hour,
			}
			svc2 := NewService(cfg2, nil, nil)

			claims, err := svc2.ValidateToken(token)

			// Should return an error and nil claims
			return err != nil && claims == nil
		},
		genUserID(),
		genEmail(),
		genJWTSecret(),
		genJWTSecret(),
	))

	properties.TestingRun(t)
}
