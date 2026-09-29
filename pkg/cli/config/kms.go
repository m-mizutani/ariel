package config

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"

	"github.com/m-mizutani/ariel/pkg/adapter/kms"
)

type KMS struct {
	keyName string
}

func (x *KMS) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "kms-key-name",
			Category:    "KMS",
			Usage:       "Cloud KMS key that encrypts user tokens of Slack, Google Workspace, Notion, and GitHub (projects/*/locations/*/keyRings/*/cryptoKeys/*)",
			Sources:     cli.EnvVars("ARIEL_KMS_KEY_NAME"),
			Destination: &x.keyName,
		},
	}
}

func (x *KMS) IsSet() bool {
	return x.keyName != ""
}

func (x *KMS) Validate() error {
	if x.keyName == "" {
		return goerr.New("--kms-key-name is required")
	}
	if err := kms.ValidateKeyName(x.keyName); err != nil {
		return goerr.Wrap(err, "invalid --kms-key-name")
	}
	return nil
}

func (x *KMS) Configure(ctx context.Context) (*kms.Client, error) {
	if err := x.Validate(); err != nil {
		return nil, err
	}
	return kms.New(ctx, x.keyName)
}
