package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
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

// GetRDSCredentials fetches the superuser credentials from AWS Secrets Manager
// for the given instance. For DR replicas the primary instance's secret is used.
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
		return RDSCreds{}, fmt.Errorf("failed to fetch secret '%s' in %s: %w", secretID, homeRegion, err)
	}

	var creds RDSCreds
	if err := json.Unmarshal([]byte(*out.SecretString), &creds); err != nil {
		return RDSCreds{}, err
	}
	return creds, nil
}
