package interfaces

import (
	"context"

	"github.com/m-mizutani/ariel/pkg/domain/model"
)

// Cipher encrypts secrets before they are persisted. aad (additional
// authenticated data) must be supplied again, unchanged, to decrypt.
type Cipher interface {
	Encrypt(ctx context.Context, plaintext, aad []byte) (*model.EncryptedData, error)
	Decrypt(ctx context.Context, data *model.EncryptedData, aad []byte) ([]byte, error)
}
