package facts

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
)

// imdsClient is satisfied by *imds.Client; declared here so tests can inject
// a fake instead of talking to 169.254.169.254.
type imdsClient interface {
	GetInstanceIdentityDocument(ctx context.Context, params *imds.GetInstanceIdentityDocumentInput, optFns ...func(*imds.Options)) (*imds.GetInstanceIdentityDocumentOutput, error)
}

func fetchIdentityDocument(ctx context.Context, client imdsClient) (imds.InstanceIdentityDocument, error) {
	out, err := client.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
	if err != nil {
		return imds.InstanceIdentityDocument{}, fmt.Errorf("IMDS GetInstanceIdentityDocument: %w", err)
	}
	return out.InstanceIdentityDocument, nil
}

// gatherIdentity fetches identity Facts from the real IMDSv2 endpoint. It
// requires only network reachability to 169.254.169.254, not IAM
// credentials.
func gatherIdentity(ctx context.Context) (Facts, error) {
	doc, err := fetchIdentityDocument(ctx, imds.New(imds.Options{}))
	if err != nil {
		return Facts{}, err
	}
	return Facts{
		InstanceID:       doc.InstanceID,
		InstanceType:     doc.InstanceType,
		Region:           doc.Region,
		AvailabilityZone: doc.AvailabilityZone,
		Arch:             doc.Architecture,
	}, nil
}
