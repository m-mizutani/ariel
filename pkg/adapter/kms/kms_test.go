package kms_test

import (
	"context"
	"os"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/adapter/kms"
)

func TestValidateKeyName(t *testing.T) {
	gt.NoError(t, kms.ValidateKeyName("projects/p/locations/l/keyRings/r/cryptoKeys/k"))

	invalid := []string{
		"",
		"projects/p/locations/l/keyRings/r",
		"projects/p/locations/l/keyRings/r/cryptoKeys/k/cryptoKeyVersions/1",
		"projects/p/locations/l/cryptoKeys/k",
		"projects//locations/l/keyRings/r/cryptoKeys/k",
	}
	for _, name := range invalid {
		t.Run(name, func(t *testing.T) {
			gt.Error(t, kms.ValidateKeyName(name))
		})
	}
}

func TestNew_RejectsInvalidKeyName(t *testing.T) {
	_, err := kms.New(context.Background(), "projects/p/locations/l/keyRings/r")
	gt.Value(t, err).NotNil()
}

// TestClient_RoundTrip runs against a real Cloud KMS key named by
// TEST_GCP_KMS (projects/*/locations/*/keyRings/*/cryptoKeys/*), using
// Application Default Credentials.
func TestClient_RoundTrip(t *testing.T) {
	keyName := os.Getenv("TEST_GCP_KMS")
	if keyName == "" {
		t.Skip("TEST_GCP_KMS is not set")
	}
	ctx := context.Background()
	client, err := kms.New(ctx, keyName)
	gt.NoError(t, err).Required()
	t.Cleanup(func() { gt.NoError(t, client.Close()) })

	aad := []byte("ariel:slack-user-token:v1:T0123ABCD:U0123ABCD")
	encrypted, err := client.Encrypt(ctx, []byte("xoxp-test-token"), aad)
	gt.NoError(t, err).Required()
	gt.String(t, encrypted.KeyName).Equal(keyName)
	gt.Value(t, encrypted.Ciphertext).NotEqual([]byte("xoxp-test-token"))

	plaintext, err := client.Decrypt(ctx, encrypted, aad)
	gt.NoError(t, err).Required()
	gt.Value(t, plaintext).Equal([]byte("xoxp-test-token"))

	_, err = client.Decrypt(ctx, encrypted, []byte("ariel:slack-user-token:v1:T0123ABCD:U9999ZZZZ"))
	gt.Value(t, err).NotNil()
}
