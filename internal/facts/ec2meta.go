package facts

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// ec2Client is satisfied by *ec2.Client; declared here so tests can inject a
// fake DescribeInstanceTypes response instead of calling AWS.
type ec2Client interface {
	DescribeInstanceTypes(ctx context.Context, params *ec2.DescribeInstanceTypesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error)
}

func fetchInstanceTypeInfo(ctx context.Context, client ec2Client, instanceType string) (types.InstanceTypeInfo, error) {
	out, err := client.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
		InstanceTypes: []types.InstanceType{types.InstanceType(instanceType)},
	})
	if err != nil {
		return types.InstanceTypeInfo{}, fmt.Errorf("EC2 DescribeInstanceTypes(%s): %w", instanceType, err)
	}
	if len(out.InstanceTypes) == 0 {
		return types.InstanceTypeInfo{}, fmt.Errorf("EC2 DescribeInstanceTypes(%s): no results returned", instanceType)
	}
	return out.InstanceTypes[0], nil
}

// gatherInstanceTypeInfo calls the real EC2 API. It requires the instance
// role (or ambient credentials) to have ec2:DescribeInstanceTypes.
func gatherInstanceTypeInfo(ctx context.Context, region, instanceType string) (Facts, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return Facts{}, fmt.Errorf("loading AWS config: %w", err)
	}
	info, err := fetchInstanceTypeInfo(ctx, ec2.NewFromConfig(cfg), instanceType)
	if err != nil {
		return Facts{}, err
	}
	var f Facts
	applyInstanceTypeInfo(&f, info)
	return f, nil
}

// applyInstanceTypeInfo maps an EC2 DescribeInstanceTypes result onto the
// sizing fields of Facts.
func applyInstanceTypeInfo(f *Facts, info types.InstanceTypeInfo) {
	if info.VCpuInfo != nil && info.VCpuInfo.DefaultVCpus != nil {
		f.VCPUs = int(*info.VCpuInfo.DefaultVCpus)
	}
	if info.MemoryInfo != nil && info.MemoryInfo.SizeInMiB != nil {
		f.MemMiB = *info.MemoryInfo.SizeInMiB
	}
	if info.BareMetal != nil {
		f.BareMetal = *info.BareMetal
	}
	f.Hypervisor = string(info.Hypervisor)

	if info.InstanceStorageInfo != nil {
		switch info.InstanceStorageInfo.NvmeSupport {
		case types.EphemeralNvmeSupportSupported, types.EphemeralNvmeSupportRequired:
			f.NvmeInstanceStore = true
		}
	}

	if ni := info.NetworkInfo; ni != nil {
		switch ni.EnaSupport {
		case types.EnaSupportSupported, types.EnaSupportRequired:
			f.EnaSupport = true
		}
		if ni.MaximumNetworkInterfaces != nil {
			f.MaxENIs = int(*ni.MaximumNetworkInterfaces)
		}
		if ni.Ipv4AddressesPerInterface != nil {
			f.Ipv4AddressesPerENI = int(*ni.Ipv4AddressesPerInterface)
		}
		f.NetworkBandwidthGbps = bandwidthFromNetworkInfo(ni)
	}
}

func bandwidthFromNetworkInfo(ni *types.NetworkInfo) float64 {
	if len(ni.NetworkCards) > 0 && ni.NetworkCards[0].BaselineBandwidthInGbps != nil {
		return *ni.NetworkCards[0].BaselineBandwidthInGbps
	}
	if ni.NetworkPerformance != nil {
		return parseNetworkPerformanceGbps(*ni.NetworkPerformance)
	}
	return 0
}

// networkPerformanceNumberRe pulls the first number out of strings like
// "Up to 12.5 Gigabit", "25 Gigabit", or "10 Gigabit". Some very small
// instance types (e.g. t2.micro) report qualitative strings like "Low to
// Moderate" with no number at all; those fall back to a bandwidth of 0,
// which the profile package treats as its lowest tier.
var networkPerformanceNumberRe = regexp.MustCompile(`[\d.]+`)

func parseNetworkPerformanceGbps(s string) float64 {
	match := networkPerformanceNumberRe.FindString(s)
	if match == "" {
		return 0
	}
	v, err := strconv.ParseFloat(match, 64)
	if err != nil {
		return 0
	}
	return v
}
