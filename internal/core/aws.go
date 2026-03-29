package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// LoadAWSConfig returns both the working config (with the specified region)
// and the home region (ap-south-1) used for Secrets Manager lookups.
func LoadAWSConfig(ctx context.Context, profile, region string) (aws.Config, string, error) {
	homeCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithSharedConfigProfile(profile),
		awsconfig.WithRegion("ap-south-1"),
	)
	if err != nil {
		return aws.Config{}, "", fmt.Errorf("load AWS config (home): %w", err)
	}
	homeRegion := homeCfg.Region

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithSharedConfigProfile(profile),
	}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, "", fmt.Errorf("load AWS config: %w", err)
	}
	return cfg, homeRegion, nil
}

// GetInstancesWithCache returns RDS PostgreSQL instances (cached or from AWS).
// Profile is used as part of the cache key.
func GetInstancesWithCache(ctx context.Context, cfg aws.Config, profile string) ([]InstanceInfo, error) {
	cacheDir := GetCacheDir()
	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("%s_%s_instances.json", profile, cfg.Region))

	if data, err := os.ReadFile(cacheFile); err == nil {
		var envelope CacheEnvelope
		if err := json.Unmarshal(data, &envelope); err == nil {
			info, _ := os.Stat(cacheFile)
			if envelope.Version == CacheVersion && time.Since(info.ModTime()) < time.Hour {
				return envelope.Instances, nil
			}
		}
	}

	fmt.Printf("🔍 Fetching RDS instances [%s:%s]...\n", profile, cfg.Region)

	rdsClient := rds.NewFromConfig(cfg)
	out, err := rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{})
	if err != nil {
		return nil, err
	}

	var instances []InstanceInfo
	for _, db := range out.DBInstances {
		if aws.ToString(db.Engine) == "postgres" {
			instances = append(instances, InstanceInfo{
				ID:       aws.ToString(db.DBInstanceIdentifier),
				Host:     aws.ToString(db.Endpoint.Address),
				Size:     aws.ToString(db.DBInstanceClass),
				Port:     *db.Endpoint.Port,
				Version:  aws.ToString(db.EngineVersion),
				SourceID: aws.ToString(db.ReadReplicaSourceDBInstanceIdentifier),
			})
		}
	}

	os.MkdirAll(cacheDir, 0755)
	newCacheData, _ := json.Marshal(CacheEnvelope{
		Version:   CacheVersion,
		Instances: instances,
	})
	os.WriteFile(cacheFile, newCacheData, 0644)

	return instances, nil
}

// InstanceSecretTargetID resolves the instance ID to use for Secrets Manager
// lookups. For read replicas it returns the primary instance ID.
func InstanceSecretTargetID(selected InstanceInfo) string {
	if selected.SourceID == "" {
		return selected.ID
	}
	if strings.HasPrefix(selected.SourceID, "arn:aws:rds:") {
		parts := strings.Split(selected.SourceID, ":")
		if len(parts) >= 7 {
			return parts[6]
		}
		return selected.ID
	}
	return selected.SourceID
}

// regionFromRDSARN parses region from an RDS DB instance ARN (e.g. arn:aws:rds:ap-south-1:...).
// Returns empty string if the ARN format is not recognized.
func regionFromRDSARN(arn string) string {
	if !strings.HasPrefix(arn, "arn:aws:rds:") {
		return ""
	}
	parts := strings.Split(arn, ":")
	if len(parts) < 4 {
		return ""
	}
	return parts[3]
}

// regionFromSecretARN parses region from a Secrets Manager secret ARN.
// Returns empty string if the ARN format is not recognized.
func regionFromSecretARN(arn string) string {
	if !strings.HasPrefix(arn, "arn:aws:secretsmanager:") {
		return ""
	}
	parts := strings.Split(arn, ":")
	if len(parts) < 4 {
		return ""
	}
	return parts[3]
}

// GetRDSCredentials fetches the superuser credentials from AWS Secrets Manager
// for the given instance. For DR replicas the primary instance's secret is used.
// If the custom secret (root/{id}/psql) is not found, falls back to the AWS-managed
// RDS master secret (MasterUserSecret) when the instance has it enabled.
func GetRDSCredentials(ctx context.Context, cfg aws.Config, selected InstanceInfo, homeRegion string) (RDSCreds, error) {
	secretTargetID := InstanceSecretTargetID(selected)
	if selected.SourceID != "" && strings.HasPrefix(selected.SourceID, "arn:aws:rds:") && secretTargetID != selected.ID {
		fmt.Printf("🌐 DR Replica detected. Fetching master secret '%s' from primary region: %s\n",
			secretTargetID, homeRegion)
	}

	sm := secretsmanager.NewFromConfig(cfg, func(o *secretsmanager.Options) {
		o.Region = homeRegion
	})

	secretID := fmt.Sprintf("root/%s/psql", secretTargetID)
	out, err := sm.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: &secretID})
	if err != nil {
		var notFound *smtypes.ResourceNotFoundException
		if !errors.As(err, &notFound) {
			return RDSCreds{}, fmt.Errorf("failed to fetch secret '%s' in %s: %w", secretID, homeRegion, err)
		}
		// Fallback: AWS-managed RDS master secret
		creds, fallbackErr := getRDSCredentialsFromManagedSecret(ctx, cfg, selected, secretTargetID, homeRegion)
		if fallbackErr != nil {
			return RDSCreds{}, fmt.Errorf("custom secret not found and fallback failed: %w", fallbackErr)
		}
		return creds, nil
	}

	var creds RDSCreds
	if err := json.Unmarshal([]byte(*out.SecretString), &creds); err != nil {
		return RDSCreds{}, err
	}
	return creds, nil
}

// getRDSCredentialsFromManagedSecret fetches credentials from the AWS-managed RDS
// master secret (MasterUserSecret) for the given instance.
func getRDSCredentialsFromManagedSecret(ctx context.Context, cfg aws.Config, selected InstanceInfo, secretTargetID, homeRegion string) (RDSCreds, error) {
	fallbackRegion := cfg.Region
	if secretTargetID != selected.ID && selected.SourceID != "" && strings.HasPrefix(selected.SourceID, "arn:aws:rds:") {
		if r := regionFromRDSARN(selected.SourceID); r != "" {
			fallbackRegion = r
		} else {
			fallbackRegion = homeRegion
		}
	}

	rdsClient := rds.NewFromConfig(cfg, func(o *rds.Options) {
		o.Region = fallbackRegion
	})
	descOut, err := rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: &secretTargetID,
	})
	if err != nil {
		return RDSCreds{}, fmt.Errorf("describe DB instance %s: %w", secretTargetID, err)
	}
	if len(descOut.DBInstances) == 0 {
		return RDSCreds{}, fmt.Errorf("no DB instance found for %s", secretTargetID)
	}
	db := descOut.DBInstances[0]
	if db.MasterUserSecret == nil || db.MasterUserSecret.SecretArn == nil || aws.ToString(db.MasterUserSecret.SecretArn) == "" {
		return RDSCreds{}, fmt.Errorf("no AWS-managed secret for instance %s (enable Manage master user password in Secrets Manager)", secretTargetID)
	}
	secretArn := aws.ToString(db.MasterUserSecret.SecretArn)
	secretRegion := fallbackRegion
	if r := regionFromSecretARN(secretArn); r != "" {
		secretRegion = r
	}
	sm := secretsmanager.NewFromConfig(cfg, func(o *secretsmanager.Options) {
		o.Region = secretRegion
	})
	secretOut, err := sm.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &secretArn,
	})
	if err != nil {
		return RDSCreds{}, fmt.Errorf("fetch managed secret for %s: %w", secretTargetID, err)
	}
	var creds RDSCreds
	if err := json.Unmarshal([]byte(*secretOut.SecretString), &creds); err != nil {
		return RDSCreds{}, fmt.Errorf("parse managed secret for %s: %w", secretTargetID, err)
	}
	return creds, nil
}
