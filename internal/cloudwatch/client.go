package cloudwatch

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// NewLogsClient creates a new CloudWatch Logs client with the specified profile and region.
func NewLogsClient(profile, region string) (*cloudwatchlogs.Client, error) {
	cfg, err := loadAWSConfig(profile, region)
	if err != nil {
		return nil, err
	}
	return cloudwatchlogs.NewFromConfig(cfg), nil
}

// NewMetricsClient creates a new CloudWatch (metrics) client with the specified profile and region.
func NewMetricsClient(profile, region string) (*cloudwatch.Client, error) {
	cfg, err := loadAWSConfig(profile, region)
	if err != nil {
		return nil, err
	}
	return cloudwatch.NewFromConfig(cfg), nil
}

// loadAWSConfig loads the AWS configuration with optional profile and region.
func loadAWSConfig(profile, region string) (aws.Config, error) {
	var opts []func(*config.LoadOptions) error

	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return cfg, nil
}

// GetResolvedRegion returns the AWS region that would be used for the given profile.
// If region is explicitly provided, it returns that. Otherwise it returns the region
// from the profile configuration or the default region.
func GetResolvedRegion(profile, region string) (string, error) {
	cfg, err := loadAWSConfig(profile, region)
	if err != nil {
		return "", err
	}
	return cfg.Region, nil
}

// GetAccountID returns the AWS account ID for the given profile using STS GetCallerIdentity.
func GetAccountID(profile, region string) (string, error) {
	cfg, err := loadAWSConfig(profile, region)
	if err != nil {
		return "", err
	}

	stsClient := sts.NewFromConfig(cfg)
	result, err := stsClient.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %w", err)
	}

	if result.Account == nil {
		return "", fmt.Errorf("account ID not returned")
	}

	return *result.Account, nil
}
