package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/adapter/kms"
	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
	"github.com/m-mizutani/ariel/pkg/repository/memory"
	"github.com/m-mizutani/ariel/pkg/usecase"
)

var (
	googleScopes = []string{
		"openid",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/calendar.readonly",
		"https://www.googleapis.com/auth/drive.readonly",
		"https://www.googleapis.com/auth/gmail.readonly",
	}
	aliceIdentity = &model.GoogleIdentity{Subject: "1234567890", Email: "alice@example.com"}
)

func TestGoogleWorkspaceAccess_StoreAndToken(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	cipher := &fakeCipher{}
	access := usecase.NewGoogleWorkspaceAccess(repo, cipher)

	now := time.Now().UTC()
	gt.NoError(t, access.Store(ctx, testKey, "refresh-secret", googleScopes, aliceIdentity, now)).Required()

	cred, err := repo.GoogleWorkspaceCredential().Get(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.String(t, cred.RefreshToken.KeyName).Equal(testKeyName)
	gt.Bool(t, bytes.HasSuffix(cred.RefreshToken.Ciphertext, []byte("refresh-secret"))).True()
	gt.Value(t, cred.Scopes).Equal(googleScopes)
	gt.String(t, cred.Subject).Equal("1234567890")
	gt.String(t, cred.Email).Equal("alice@example.com")
	gt.Bool(t, cred.CreatedAt.Equal(now)).True()
	gt.Bool(t, cred.UpdatedAt.Equal(now)).True()
	gt.Array(t, cipher.encryptions).Length(1).Required()
	gt.Value(t, cipher.encryptions[0].AAD).Equal(usecase.GoogleTokenAADForTest(testKey))
	gt.String(t, string(cipher.encryptions[0].AAD)).Equal("ariel:google-refresh-token:v1:T0123ABCD:U0123ABCD")

	token, err := access.Token(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.Value(t, token.RefreshToken).Equal(model.GoogleRefreshToken("refresh-secret"))
	gt.Array(t, cipher.decryptions).Length(1).Required()
	gt.Value(t, cipher.decryptions[0].AAD).Equal(usecase.GoogleTokenAADForTest(testKey))
}

func TestGoogleWorkspaceAccess_StoreKeepsCreatedAt(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	access := usecase.NewGoogleWorkspaceAccess(repo, &fakeCipher{})

	first := time.Now().Add(-time.Hour).UTC()
	second := time.Now().UTC()
	gt.NoError(t, access.Store(ctx, testKey, "refresh-first", googleScopes, aliceIdentity, first)).Required()
	bob := &model.GoogleIdentity{Subject: "999", Email: "bob@example.com"}
	gt.NoError(t, access.Store(ctx, testKey, "refresh-second", googleScopes, bob, second)).Required()

	cred, err := repo.GoogleWorkspaceCredential().Get(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.Bool(t, cred.CreatedAt.Equal(first)).True()
	gt.Bool(t, cred.UpdatedAt.Equal(second)).True()
	gt.Bool(t, bytes.HasSuffix(cred.RefreshToken.Ciphertext, []byte("refresh-second"))).True()
	gt.String(t, cred.Email).Equal("bob@example.com")
	gt.String(t, cred.Subject).Equal("999")
}

func TestGoogleWorkspaceAccess_StoreEncryptError(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	access := usecase.NewGoogleWorkspaceAccess(repo, &fakeCipher{encryptErr: errors.New("kms unavailable")})

	gt.Value(t, access.Store(ctx, testKey, "refresh-secret", googleScopes, aliceIdentity, time.Now())).NotNil()
	_, err := repo.GoogleWorkspaceCredential().Get(ctx, testKey)
	gt.Error(t, err).Is(interfaces.ErrNotFound)
}

func TestGoogleWorkspaceAccess_TokenNotConnected(t *testing.T) {
	cipher := &fakeCipher{}
	access := usecase.NewGoogleWorkspaceAccess(memory.New(), cipher)

	_, err := access.Token(context.Background(), testKey)
	gt.Error(t, err).Is(usecase.ErrGoogleWorkspaceNotConnected)
	gt.Number(t, cipher.decryptCount()).Equal(0)
}

func TestGoogleWorkspaceAccess_TokenDecryptError(t *testing.T) {
	ctx := context.Background()
	cipher := &fakeCipher{}
	access := usecase.NewGoogleWorkspaceAccess(memory.New(), cipher)
	gt.NoError(t, access.Store(ctx, testKey, "refresh-secret", googleScopes, aliceIdentity, time.Now())).Required()

	cipher.decryptErr = errors.New("permission denied")
	_, err := access.Token(ctx, testKey)
	gt.Value(t, err).NotNil().Required()
	gt.Bool(t, errors.Is(err, usecase.ErrGoogleWorkspaceNotConnected)).False()
}

func TestGoogleWorkspaceAccess_CiphertextOfAnotherUserCannotBeDecrypted(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	access := usecase.NewGoogleWorkspaceAccess(repo, &fakeCipher{})
	gt.NoError(t, access.Store(ctx, testKey, "refresh-owner", googleScopes, aliceIdentity, time.Now())).Required()

	ownerCred, err := repo.GoogleWorkspaceCredential().Get(ctx, testKey)
	gt.NoError(t, err).Required()
	other := model.UserKey{TeamID: testKey.TeamID, UserID: "U9999ZZZZ"}
	copied := *ownerCred
	copied.UserID = other.UserID
	gt.NoError(t, repo.GoogleWorkspaceCredential().Put(ctx, other, &copied)).Required()

	_, err = access.Token(ctx, other)
	gt.Value(t, err).NotNil()
}

// A disconnect that read the old token must not delete the credential a
// connection stored after that read.
func TestGoogleWorkspaceAccess_DeleteKeepsNewerCredential(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	access := usecase.NewGoogleWorkspaceAccess(repo, &fakeCipher{})

	gt.NoError(t, access.Store(ctx, testKey, "refresh-old", googleScopes, aliceIdentity, time.Now())).Required()
	oldToken, err := access.Token(ctx, testKey)
	gt.NoError(t, err).Required()

	gt.NoError(t, access.Store(ctx, testKey, "refresh-new", googleScopes, aliceIdentity, time.Now())).Required()
	gt.NoError(t, access.Delete(ctx, oldToken)).Required()

	cred, err := repo.GoogleWorkspaceCredential().Get(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.Bool(t, bytes.HasSuffix(cred.RefreshToken.Ciphertext, []byte("refresh-new"))).True()
}

func TestGoogleWorkspaceAccess_DeleteAndConnection(t *testing.T) {
	ctx := context.Background()
	access := usecase.NewGoogleWorkspaceAccess(memory.New(), &fakeCipher{})

	status, err := access.Connection(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.Value(t, status).Equal(&usecase.GoogleWorkspaceStatus{})

	gt.NoError(t, access.Store(ctx, testKey, "refresh-secret", googleScopes, aliceIdentity, time.Now())).Required()
	status, err = access.Connection(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.Value(t, status).Equal(&usecase.GoogleWorkspaceStatus{Connected: true, Email: "alice@example.com"})

	token, err := access.Token(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.NoError(t, access.Delete(ctx, token)).Required()
	status, err = access.Connection(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.Value(t, status).Equal(&usecase.GoogleWorkspaceStatus{})
}

// TestGoogleWorkspaceAccess_WithCloudKMS stores and reads a refresh token
// through a real Cloud KMS key named by TEST_GCP_KMS.
func TestGoogleWorkspaceAccess_WithCloudKMS(t *testing.T) {
	keyName := os.Getenv("TEST_GCP_KMS")
	if keyName == "" {
		t.Skip("TEST_GCP_KMS is not set")
	}
	ctx := context.Background()
	cipher, err := kms.New(ctx, keyName)
	gt.NoError(t, err).Required()
	t.Cleanup(func() { gt.NoError(t, cipher.Close()) })

	repo := memory.New()
	access := usecase.NewGoogleWorkspaceAccess(repo, cipher)
	gt.NoError(t, access.Store(ctx, testKey, "refresh-kms-token", googleScopes, aliceIdentity, time.Now())).Required()

	cred, err := repo.GoogleWorkspaceCredential().Get(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.String(t, cred.RefreshToken.KeyName).Equal(keyName)
	gt.Bool(t, bytes.Contains(cred.RefreshToken.Ciphertext, []byte("refresh-kms-token"))).False()

	token, err := access.Token(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.Value(t, token.RefreshToken).Equal(model.GoogleRefreshToken("refresh-kms-token"))

	other := model.UserKey{TeamID: testKey.TeamID, UserID: "U9999ZZZZ"}
	copied := *cred
	copied.UserID = other.UserID
	gt.NoError(t, repo.GoogleWorkspaceCredential().Put(ctx, other, &copied)).Required()
	_, err = access.Token(ctx, other)
	gt.Value(t, err).NotNil()
}
