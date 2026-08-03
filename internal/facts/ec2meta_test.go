package facts

import (
	"context"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type fakeEC2Client struct {
	info types.InstanceTypeInfo
	err  error
}

func (f *fakeEC2Client) DescribeInstanceTypes(ctx context.Context, params *ec2.DescribeInstanceTypesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &ec2.DescribeInstanceTypesOutput{InstanceTypes: []types.InstanceTypeInfo{f.info}}, nil
}

func TestParseNetworkPerformanceGbps(t *testing.T) {
	cases := map[string]float64{
		"Up to 12.5 Gigabit": 12.5,
		"25 Gigabit":         25,
		"10 Gigabit":         10,
		"Low to Moderate":    0,
		"":                   0,
	}
	for input, want := range cases {
		if got := parseNetworkPerformanceGbps(input); got != want {
			t.Errorf("parseNetworkPerformanceGbps(%q) = %v, want %v", input, got, want)
		}
	}
}

// TestApplyInstanceTypeInfo covers representative instance types spanning
// burstable, general-purpose, Graviton/ARM, network-optimized, and
// storage-optimized families to exercise the AWS response mapping.
func TestApplyInstanceTypeInfo(t *testing.T) {
	tests := []struct {
		name string
		info types.InstanceTypeInfo
		want Facts
	}{
		{
			name: "t3.micro burstable, qualitative network performance",
			info: types.InstanceTypeInfo{
				VCpuInfo:   &types.VCpuInfo{DefaultVCpus: aws.Int32(2)},
				MemoryInfo: &types.MemoryInfo{SizeInMiB: aws.Int64(1024)},
				Hypervisor: types.InstanceTypeHypervisorNitro,
				NetworkInfo: &types.NetworkInfo{
					EnaSupport:                types.EnaSupportSupported,
					MaximumNetworkInterfaces:  aws.Int32(2),
					Ipv4AddressesPerInterface: aws.Int32(2),
					NetworkPerformance:        aws.String("Low to Moderate"),
				},
			},
			want: Facts{VCPUs: 2, MemMiB: 1024, Hypervisor: "nitro", EnaSupport: true, MaxENIs: 2, Ipv4AddressesPerENI: 2, NetworkBandwidthGbps: 0},
		},
		{
			name: "m6i.xlarge general purpose",
			info: types.InstanceTypeInfo{
				VCpuInfo:   &types.VCpuInfo{DefaultVCpus: aws.Int32(4)},
				MemoryInfo: &types.MemoryInfo{SizeInMiB: aws.Int64(16384)},
				Hypervisor: types.InstanceTypeHypervisorNitro,
				NetworkInfo: &types.NetworkInfo{
					EnaSupport:                types.EnaSupportRequired,
					MaximumNetworkInterfaces:  aws.Int32(4),
					Ipv4AddressesPerInterface: aws.Int32(15),
					NetworkCards:              []types.NetworkCardInfo{{BaselineBandwidthInGbps: aws.Float64(6.25)}},
				},
			},
			want: Facts{VCPUs: 4, MemMiB: 16384, Hypervisor: "nitro", EnaSupport: true, MaxENIs: 4, Ipv4AddressesPerENI: 15, NetworkBandwidthGbps: 6.25},
		},
		{
			name: "c7g.4xlarge Graviton/ARM compute-optimized",
			info: types.InstanceTypeInfo{
				VCpuInfo:   &types.VCpuInfo{DefaultVCpus: aws.Int32(16)},
				MemoryInfo: &types.MemoryInfo{SizeInMiB: aws.Int64(32768)},
				Hypervisor: types.InstanceTypeHypervisorNitro,
				NetworkInfo: &types.NetworkInfo{
					EnaSupport:                types.EnaSupportRequired,
					MaximumNetworkInterfaces:  aws.Int32(8),
					Ipv4AddressesPerInterface: aws.Int32(30),
					NetworkCards:              []types.NetworkCardInfo{{BaselineBandwidthInGbps: aws.Float64(10)}},
				},
			},
			want: Facts{VCPUs: 16, MemMiB: 32768, Hypervisor: "nitro", EnaSupport: true, MaxENIs: 8, Ipv4AddressesPerENI: 30, NetworkBandwidthGbps: 10},
		},
		{
			name: "r5dn.24xlarge memory-optimized, network-optimized, NVMe instance store",
			info: types.InstanceTypeInfo{
				VCpuInfo:            &types.VCpuInfo{DefaultVCpus: aws.Int32(96)},
				MemoryInfo:          &types.MemoryInfo{SizeInMiB: aws.Int64(805306368 / 1024)},
				Hypervisor:          types.InstanceTypeHypervisorNitro,
				InstanceStorageInfo: &types.InstanceStorageInfo{NvmeSupport: types.EphemeralNvmeSupportRequired},
				NetworkInfo: &types.NetworkInfo{
					EnaSupport:                types.EnaSupportRequired,
					MaximumNetworkInterfaces:  aws.Int32(15),
					Ipv4AddressesPerInterface: aws.Int32(50),
					NetworkCards:              []types.NetworkCardInfo{{BaselineBandwidthInGbps: aws.Float64(75)}},
				},
			},
			want: Facts{VCPUs: 96, MemMiB: 786432, Hypervisor: "nitro", EnaSupport: true, MaxENIs: 15, Ipv4AddressesPerENI: 50, NetworkBandwidthGbps: 75, NvmeInstanceStore: true},
		},
		{
			name: "i3en.metal bare-metal storage-optimized",
			info: types.InstanceTypeInfo{
				VCpuInfo:            &types.VCpuInfo{DefaultVCpus: aws.Int32(96)},
				MemoryInfo:          &types.MemoryInfo{SizeInMiB: aws.Int64(786432)},
				BareMetal:           aws.Bool(true),
				Hypervisor:          types.InstanceTypeHypervisorNitro,
				InstanceStorageInfo: &types.InstanceStorageInfo{NvmeSupport: types.EphemeralNvmeSupportRequired},
				NetworkInfo: &types.NetworkInfo{
					EnaSupport:                types.EnaSupportRequired,
					MaximumNetworkInterfaces:  aws.Int32(15),
					Ipv4AddressesPerInterface: aws.Int32(50),
					NetworkCards:              []types.NetworkCardInfo{{BaselineBandwidthInGbps: aws.Float64(100)}},
				},
			},
			want: Facts{VCPUs: 96, MemMiB: 786432, BareMetal: true, Hypervisor: "nitro", EnaSupport: true, MaxENIs: 15, Ipv4AddressesPerENI: 50, NetworkBandwidthGbps: 100, NvmeInstanceStore: true},
		},
		{
			name: "trn1.32xlarge accelerated-computing, xen-less nitro",
			info: types.InstanceTypeInfo{
				VCpuInfo:   &types.VCpuInfo{DefaultVCpus: aws.Int32(128)},
				MemoryInfo: &types.MemoryInfo{SizeInMiB: aws.Int64(524288)},
				Hypervisor: types.InstanceTypeHypervisorNitro,
				NetworkInfo: &types.NetworkInfo{
					EnaSupport:                types.EnaSupportRequired,
					MaximumNetworkInterfaces:  aws.Int32(8),
					Ipv4AddressesPerInterface: aws.Int32(30),
					NetworkCards:              []types.NetworkCardInfo{{BaselineBandwidthInGbps: aws.Float64(800)}},
				},
			},
			want: Facts{VCPUs: 128, MemMiB: 524288, Hypervisor: "nitro", EnaSupport: true, MaxENIs: 8, Ipv4AddressesPerENI: 30, NetworkBandwidthGbps: 800},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got Facts
			applyInstanceTypeInfo(&got, tc.info)
			// Zero out fields applyInstanceTypeInfo never touches so the comparison
			// only covers what this function is responsible for.
			got.InstanceID, got.InstanceType, got.Family = "", "", ""
			got.Region, got.AvailabilityZone, got.Arch = "", "", ""
			got.Source, got.Warnings = "", nil

			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("applyInstanceTypeInfo() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestFetchInstanceTypeInfoNoResults(t *testing.T) {
	client := &fakeEC2Client{}
	client.info = types.InstanceTypeInfo{} // present but DescribeInstanceTypes returns it anyway
	_, err := fetchInstanceTypeInfo(context.Background(), client, "m6i.xlarge")
	if err != nil {
		t.Fatalf("expected no error with a populated response, got %v", err)
	}
}
