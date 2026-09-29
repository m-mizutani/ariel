package localcipher_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/adapter/localcipher"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

func newCipher(t *testing.T) *localcipher.Cipher {
	t.Helper()
	c, err := localcipher.New()
	gt.NoError(t, err).Required()
	return c
}

func TestCipher_RoundTrip(t *testing.T) {
	ctx := context.Background()
	c := newCipher(t)

	data, err := c.Encrypt(ctx, []byte("secret"), []byte("aad-1"))
	gt.NoError(t, err).Required()
	gt.NoError(t, data.Validate())
	gt.Bool(t, bytes.Contains(data.Ciphertext, []byte("secret"))).False()

	plaintext, err := c.Decrypt(ctx, data, []byte("aad-1"))
	gt.NoError(t, err).Required()
	gt.String(t, string(plaintext)).Equal("secret")
}

func TestCipher_WrongAAD(t *testing.T) {
	ctx := context.Background()
	c := newCipher(t)
	data, err := c.Encrypt(ctx, []byte("secret"), []byte("aad-1"))
	gt.NoError(t, err).Required()

	_, err = c.Decrypt(ctx, data, []byte("aad-2"))
	gt.Value(t, err).NotNil()
}

func TestCipher_AnotherInstance(t *testing.T) {
	ctx := context.Background()
	data, err := newCipher(t).Encrypt(ctx, []byte("secret"), []byte("aad"))
	gt.NoError(t, err).Required()

	_, err = newCipher(t).Decrypt(ctx, data, []byte("aad"))
	gt.Value(t, err).NotNil()
}

func TestCipher_FreshNonce(t *testing.T) {
	ctx := context.Background()
	c := newCipher(t)
	a, err := c.Encrypt(ctx, []byte("secret"), []byte("aad"))
	gt.NoError(t, err).Required()
	b, err := c.Encrypt(ctx, []byte("secret"), []byte("aad"))
	gt.NoError(t, err).Required()
	gt.Bool(t, bytes.Equal(a.Ciphertext, b.Ciphertext)).False()
}

func TestCipher_RejectsForeignCiphertext(t *testing.T) {
	ctx := context.Background()
	c := newCipher(t)
	_, err := c.Decrypt(ctx, &model.EncryptedData{KeyName: "projects/p/locations/l/keyRings/r/cryptoKeys/k", Ciphertext: []byte("0123456789abcdef")}, nil)
	gt.Value(t, err).NotNil()

	_, err = c.Decrypt(ctx, &model.EncryptedData{KeyName: "local-ephemeral", Ciphertext: []byte("short")}, nil)
	gt.Value(t, err).NotNil()
}
