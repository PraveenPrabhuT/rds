package createdb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type secretEntry struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// StoreCredentials saves all user credentials as a single JSON secret in
// AWS Secrets Manager at <dbName>/<instanceID>/psql.
func StoreCredentials(ctx context.Context, cfg aws.Config, homeRegion, dbName, instanceID string, users []UserCredentials) error {
	payload := make(map[string]secretEntry)
	for _, u := range users {
		key := u.Role
		if key == "read-only" || key == "read-write" {
			// Derive key from suffix: e.g. "Pricing_ro_v1" -> "ro_v1"
			if len(u.Username) > len(dbName)+1 {
				key = u.Username[len(dbName)+1:]
			}
		}
		payload[key] = secretEntry{Username: u.Username, Password: u.Password}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	sm := secretsmanager.NewFromConfig(cfg, func(o *secretsmanager.Options) {
		o.Region = homeRegion
	})

	secretID := fmt.Sprintf("%s/%s/psql", dbName, instanceID)
	_, err = sm.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         &secretID,
		SecretString: aws.String(string(data)),
	})
	if err != nil {
		return fmt.Errorf("create secret %q: %w", secretID, err)
	}
	return nil
}
