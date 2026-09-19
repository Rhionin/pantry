package cart

import (
	"context"
)

// Provisioner is the seam that replaces CartExporter.
// It performs provisioning against a specific provider and returns a detailed report.
type Provisioner interface {
	Provision(ctx context.Context, userID string, provider ProviderID) (ProvisionReport, error)
}

// NoOpProvisioner is the no-op Provisioner used when no provider is configured.
type NoOpProvisioner struct{}

// Provision returns a no-op report with no confirmed entries.
func (n *NoOpProvisioner) Provision(_ context.Context, _ string, _ ProviderID) (ProvisionReport, error) {
	return ProvisionReport{
		Confirmed: 0,
		Entries:   nil,
	}, nil
}
