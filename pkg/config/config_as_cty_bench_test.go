package config_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/pkg/config"
)

func BenchmarkTerragruntConfigAsCtyInputs(b *testing.B) {
	benchmarks := []struct {
		name    string
		ownerID string
	}{
		{name: "ascii", ownerID: "platform-team"},
		{name: "non-ascii value", ownerID: "équipe-plateforme"},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			cfg := &config.TerragruntConfig{Inputs: benchmarkInputs(bm.ownerID)}

			for b.Loop() {
				_, err := config.TerragruntConfigAsCty(cfg)
				require.NoError(b, err)
			}
		})
	}
}

// benchmarkInputs returns the inputs of a typical unit that deploys a cluster, with ownerID as the value of one tag.
func benchmarkInputs(ownerID string) map[string]any {
	tags := map[string]any{
		"Owner":       ownerID,
		"Environment": "production",
		"CostCenter":  "1234",
		"ManagedBy":   "terragrunt",
		"Repository":  "https://example.com/org/infrastructure-live?ref=main&path=prod",
	}

	subnets := make([]any, 0, 6)
	for i := range 6 {
		subnets = append(subnets, fmt.Sprintf("10.0.%d.0/24", i))
	}

	nodeGroups := make([]any, 0, 4)
	for i := range 4 {
		nodeGroups = append(nodeGroups, map[string]any{
			"name":           fmt.Sprintf("workers-%d", i),
			"instance_types": []any{"m6i.large", "m6i.xlarge"},
			"min_size":       float64(1),
			"max_size":       float64(10),
			"desired_size":   float64(3),
			"labels":         map[string]any{"role": "worker", "pool": fmt.Sprintf("pool-%d", i)},
			"taints":         []any{},
			"spot":           i%2 == 0,
		})
	}

	inputs := map[string]any{
		"name":               "prod-cluster",
		"region":             "us-east-1",
		"cluster_version":    "1.31",
		"vpc_cidr":           "10.0.0.0/16",
		"private_subnets":    subnets,
		"enable_nat_gateway": true,
		"single_nat_gateway": false,
		"node_groups":        nodeGroups,
		"tags":               tags,
		"policy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow",` +
			`"Action":["s3:GetObject"],"Resource":"arn:aws:s3:::bucket/*"}]}`,
	}

	for i := range 30 {
		inputs[fmt.Sprintf("setting_%02d", i)] = fmt.Sprintf("value-%02d", i)
	}

	return inputs
}
