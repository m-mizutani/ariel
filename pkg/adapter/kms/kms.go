package kms

import (
	"context"
	"hash/crc32"
	"regexp"

	kmsapi "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/m-mizutani/goerr/v2"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
)

var keyNamePattern = regexp.MustCompile(`^projects/[^/]+/locations/[^/]+/keyRings/[^/]+/cryptoKeys/[^/]+$`)

var crc32cTable = crc32.MakeTable(crc32.Castagnoli)

func crc32c(data []byte) int64 {
	return int64(crc32.Checksum(data, crc32cTable))
}

// Client encrypts and decrypts with one Cloud KMS symmetric key.
type Client struct {
	client  *kmsapi.KeyManagementClient
	keyName string
}

var _ interfaces.Cipher = &Client{}

// ValidateKeyName checks that keyName names a crypto key, not a key ring or a
// key version.
func ValidateKeyName(keyName string) error {
	if !keyNamePattern.MatchString(keyName) {
		return goerr.New("invalid KMS key name; expected projects/*/locations/*/keyRings/*/cryptoKeys/*",
			goerr.V("key_name", keyName))
	}
	return nil
}

func New(ctx context.Context, keyName string) (*Client, error) {
	if err := ValidateKeyName(keyName); err != nil {
		return nil, err
	}
	client, err := kmsapi.NewKeyManagementClient(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create KMS client")
	}
	return &Client{client: client, keyName: keyName}, nil
}

func (c *Client) Encrypt(ctx context.Context, plaintext, aad []byte) (*model.EncryptedData, error) {
	resp, err := c.client.Encrypt(ctx, &kmspb.EncryptRequest{
		Name:                              c.keyName,
		Plaintext:                         plaintext,
		AdditionalAuthenticatedData:       aad,
		PlaintextCrc32C:                   wrapperspb.Int64(crc32c(plaintext)),
		AdditionalAuthenticatedDataCrc32C: wrapperspb.Int64(crc32c(aad)),
	})
	if err != nil {
		return nil, goerr.Wrap(err, "KMS encrypt failed", goerr.V("key_name", c.keyName))
	}
	if !resp.GetVerifiedPlaintextCrc32C() || !resp.GetVerifiedAdditionalAuthenticatedDataCrc32C() {
		return nil, goerr.New("KMS did not verify the request checksums", goerr.V("key_name", c.keyName))
	}
	if resp.GetCiphertextCrc32C().GetValue() != crc32c(resp.GetCiphertext()) {
		return nil, goerr.New("KMS encrypt response is corrupted", goerr.V("key_name", c.keyName))
	}

	return &model.EncryptedData{KeyName: c.keyName, Ciphertext: resp.GetCiphertext()}, nil
}

func (c *Client) Decrypt(ctx context.Context, data *model.EncryptedData, aad []byte) ([]byte, error) {
	resp, err := c.client.Decrypt(ctx, &kmspb.DecryptRequest{
		Name:                              data.KeyName,
		Ciphertext:                        data.Ciphertext,
		AdditionalAuthenticatedData:       aad,
		CiphertextCrc32C:                  wrapperspb.Int64(crc32c(data.Ciphertext)),
		AdditionalAuthenticatedDataCrc32C: wrapperspb.Int64(crc32c(aad)),
	})
	if err != nil {
		return nil, goerr.Wrap(err, "KMS decrypt failed", goerr.V("key_name", data.KeyName))
	}
	if resp.GetPlaintextCrc32C().GetValue() != crc32c(resp.GetPlaintext()) {
		return nil, goerr.New("KMS decrypt response is corrupted", goerr.V("key_name", data.KeyName))
	}
	return resp.GetPlaintext(), nil
}

func (c *Client) Close() error {
	return c.client.Close()
}
