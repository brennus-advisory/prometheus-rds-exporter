package ec2_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	aws_ec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	aws_ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/qonto/prometheus-rds-exporter/internal/app/ec2"
	mock "github.com/qonto/prometheus-rds-exporter/internal/app/ec2/mock"
	converter "github.com/qonto/prometheus-rds-exporter/internal/app/unit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDBInstanceTypeInformation(t *testing.T) {
	context := context.TODO()
	client := mock.EC2Client{}

	testCases := []struct {
		instanceType       string
		vCPU               int32
		memory             int64
		baselineIops       int32
		maximumIops        int32
		baselineThroughput float64
		maximumThroughput  float64
	}{
		{
			baselineIops:       mock.InstanceT3Large.BaselineIOPS,
			baselineThroughput: converter.MegaBytesToBytes(mock.InstanceT3Large.BaselineThroughput),
			instanceType:       "t3.large",
			vCPU:               mock.InstanceT3Large.Vcpu,
			memory:             converter.MegaBytesToBytes(mock.InstanceT3Large.Memory),
			maximumIops:        mock.InstanceT3Large.MaximumIops,
			maximumThroughput:  converter.MegaBytesToBytes(mock.InstanceT3Large.MaximumThroughput),
		},
		{
			baselineIops:       mock.InstanceT3Small.BaselineIOPS,
			baselineThroughput: converter.MegaBytesToBytes(mock.InstanceT3Small.BaselineThroughput),
			instanceType:       "t3.small",
			vCPU:               mock.InstanceT3Small.Vcpu,
			memory:             converter.MegaBytesToBytes(mock.InstanceT3Small.Memory),
			maximumIops:        mock.InstanceT3Small.MaximumIops,
			maximumThroughput:  converter.MegaBytesToBytes(mock.InstanceT3Small.MaximumThroughput),
		},
		{
			baselineIops:       0, // Don't have Maximum IOPS for non EBS optimized instances
			baselineThroughput: 0, // Don't have Maximum IOPS for non EBS optimized instances
			instanceType:       "t2.small",
			vCPU:               mock.InstanceT2Small.Vcpu,
			memory:             converter.MegaBytesToBytes(mock.InstanceT2Small.Memory),
			maximumIops:        0, // Don't have Maximum IOPS for non EBS optimized instances
			maximumThroughput:  0, // Don't have Maximum throughput for non EBS optimized instances
		},
	}
	expectedAPICalls := float64(1)

	instanceTypes := make([]string, len(testCases))
	for i, instance := range testCases {
		instanceTypes[i] = instance.instanceType
	}

	fetcher := ec2.NewFetcher(context, client)
	result, err := fetcher.GetDBInstanceTypeInformation(instanceTypes)

	require.NoError(t, err, "GetDBInstanceTypeInformation must succeed")
	assert.Equal(t, expectedAPICalls, fetcher.GetStatistics().EC2ApiCall, "EC2 API calls don't match")

	for _, tc := range testCases {
		testName := "Test " + tc.instanceType
		t.Run(testName, func(t *testing.T) {
			instance := result.Instances["db."+tc.instanceType]

			assert.Equal(t, tc.vCPU, instance.Vcpu, "vCPU don't match")
			assert.Equal(t, tc.memory, instance.Memory, "Memory don't match")
			assert.Equal(t, tc.baselineIops, instance.BaselineIOPS, "Baseline IOPS don't match")
			assert.Equal(t, tc.baselineThroughput, instance.BaselineThroughput, "Baseline throughput don't match")
			assert.Equal(t, tc.maximumIops, instance.MaximumIops, "Maximum IOPS don't match")
			assert.Equal(t, tc.maximumThroughput, instance.MaximumThroughput, "Maximum throughput don't match")
		})
	}
}

func TestGetDBInstanceTypeInformationNetworkBandwidth(t *testing.T) {
	testCases := []struct {
		name        string
		networkInfo *aws_ec2_types.NetworkInfo
		wantBytes   float64
	}{
		{
			name: "eight gigabits",
			networkInfo: &aws_ec2_types.NetworkInfo{NetworkCards: []aws_ec2_types.NetworkCardInfo{
				{BaselineBandwidthInGbps: aws.Float64(8)},
			}},
			wantBytes: 1_000_000_000,
		},
		{
			name: "one gigabit",
			networkInfo: &aws_ec2_types.NetworkInfo{NetworkCards: []aws_ec2_types.NetworkCardInfo{
				{BaselineBandwidthInGbps: aws.Float64(1)},
			}},
			wantBytes: 125_000_000,
		},
		{
			name: "fractional gigabit",
			networkInfo: &aws_ec2_types.NetworkInfo{NetworkCards: []aws_ec2_types.NetworkCardInfo{
				{BaselineBandwidthInGbps: aws.Float64(0.5)},
			}},
			wantBytes: 62_500_000,
		},
		{
			name: "zero bandwidth",
			networkInfo: &aws_ec2_types.NetworkInfo{NetworkCards: []aws_ec2_types.NetworkCardInfo{
				{BaselineBandwidthInGbps: aws.Float64(0)},
			}},
		},
		{name: "missing network info"},
		{name: "nil network cards", networkInfo: &aws_ec2_types.NetworkInfo{}},
		{
			name:        "empty network cards",
			networkInfo: &aws_ec2_types.NetworkInfo{NetworkCards: []aws_ec2_types.NetworkCardInfo{}},
		},
		{
			name: "missing bandwidth",
			networkInfo: &aws_ec2_types.NetworkInfo{NetworkCards: []aws_ec2_types.NetworkCardInfo{
				{},
			}},
		},
		{
			name: "primary network card only",
			networkInfo: &aws_ec2_types.NetworkInfo{NetworkCards: []aws_ec2_types.NetworkCardInfo{
				{BaselineBandwidthInGbps: aws.Float64(8)},
				{BaselineBandwidthInGbps: aws.Float64(16)},
			}},
			wantBytes: 1_000_000_000,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fetcher := ec2.NewFetcher(context.Background(), networkBandwidthEC2Client{networkInfo: tc.networkInfo})
			result, err := fetcher.GetDBInstanceTypeInformation([]string{"db.t3.large"})

			require.NoError(t, err)
			instance, ok := result.Instances["db.t3.large"]
			require.True(t, ok, "requested instance must be returned")
			assert.Equal(t, tc.wantBytes, instance.BaselineNetworkBandwidth, "bandwidth must be in bytes per second")
		})
	}
}

type networkBandwidthEC2Client struct {
	networkInfo *aws_ec2_types.NetworkInfo
}

func (c networkBandwidthEC2Client) DescribeInstanceTypes(_ context.Context, _ *aws_ec2.DescribeInstanceTypesInput, _ ...func(*aws_ec2.Options)) (*aws_ec2.DescribeInstanceTypesOutput, error) {
	return &aws_ec2.DescribeInstanceTypesOutput{
		InstanceTypes: []aws_ec2_types.InstanceTypeInfo{
			{InstanceType: aws_ec2_types.InstanceTypeT3Large, NetworkInfo: c.networkInfo},
		},
	}, nil
}
