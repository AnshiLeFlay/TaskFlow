package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeycloakVerifierRejectsTokenForAnotherAudience(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	keyID := "test-key"
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA",
			"kid": keyID,
			"use": "sig",
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	defer jwks.Close()

	const issuer = "https://identity.example.test/realms/taskflow"
	verifier, err := NewKeycloakVerifier(context.Background(), issuer, jwks.URL, "taskflow-web", false)
	require.NoError(t, err)

	wrongAudience := signAccessToken(t, key, keyID, issuer, "another-client")
	_, err = verifier.Verify(context.Background(), wrongAudience)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnauthorized))

	valid := signAccessToken(t, key, keyID, issuer, "taskflow-web")
	user, err := verifier.Verify(context.Background(), valid)
	require.NoError(t, err)
	assert.Equal(t, "user-subject", user.ID)

	explicitlySkipped, err := NewKeycloakVerifier(context.Background(), issuer, jwks.URL, "taskflow-web", true)
	require.NoError(t, err)
	_, err = explicitlySkipped.Verify(context.Background(), wrongAudience)
	require.NoError(t, err)
}

func signAccessToken(t *testing.T, key *rsa.PrivateKey, keyID, issuer, audience string) string {
	t.Helper()
	header, err := json.Marshal(map[string]any{"alg": "RS256", "kid": keyID, "typ": "JWT"})
	require.NoError(t, err)
	now := time.Now()
	claims, err := json.Marshal(map[string]any{
		"iss": issuer,
		"sub": "user-subject",
		"aud": audience,
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	})
	require.NoError(t, err)
	encoded := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(encoded))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	require.NoError(t, err)
	return encoded + "." + base64.RawURLEncoding.EncodeToString(signature)
}
