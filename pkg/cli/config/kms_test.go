package config_test

import (
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/cli/config"
)

func TestKMS_Validate(t *testing.T) {
	unsetEnv(t, "ARIEL_KMS_KEY_NAME")

	t.Run("valid", func(t *testing.T) {
		var k config.KMS
		parse(t, k.Flags(), "--kms-key-name", "projects/p/locations/l/keyRings/r/cryptoKeys/k")
		gt.NoError(t, k.Validate())
	})

	t.Run("missing names the flag", func(t *testing.T) {
		var k config.KMS
		parse(t, k.Flags())
		err := k.Validate()
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("--kms-key-name")
	})

	for _, name := range []string{
		"projects/p/locations/l/keyRings/r/cryptoKeys/k/cryptoKeyVersions/1",
		"projects/p/locations/l/keyRings/r",
	} {
		t.Run(name, func(t *testing.T) {
			var k config.KMS
			parse(t, k.Flags(), "--kms-key-name", name)
			gt.Error(t, k.Validate())
		})
	}
}
