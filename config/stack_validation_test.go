package config_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/config"

	"github.com/stretchr/testify/assert"
)

func TestValidateStackConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *config.StackConfigFile
		wantErr string
	}{
		{
			name: "valid config",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "unit1",
						Source: "source1",
						Path:   "path1",
					},
					{
						Name:   "unit2",
						Source: "source2",
						Path:   "path2",
					},
				},
			},
			wantErr: "",
		},
		{
			name: "empty config",
			config: &config.StackConfigFile{
				Units: []*config.Unit{},
			},
			wantErr: "stack config must contain at least one unit",
		},
		{
			name: "empty unit name",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "",
						Source: "source1",
						Path:   "path1",
					},
				},
			},
			wantErr: "unit at index 0 has empty name",
		},
		{
			name: "whitespace unit name",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "  ",
						Source: "source1",
						Path:   "path1",
					},
				},
			},
			wantErr: "unit at index 0 has empty name",
		},
		{
			name: "empty unit source",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "unit1",
						Source: "",
						Path:   "path1",
					},
				},
			},
			wantErr: "unit 'unit1' has empty source",
		},
		{
			name: "whitespace unit source",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "unit1",
						Source: "   ",
						Path:   "path1",
					},
				},
			},
			wantErr: "unit 'unit1' has empty source",
		},
		{
			name: "empty unit path",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "unit1",
						Source: "source1",
						Path:   "",
					},
				},
			},
			wantErr: "unit 'unit1' has empty path",
		},
		{
			name: "whitespace unit path",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "unit1",
						Source: "source1",
						Path:   "  ",
					},
				},
			},
			wantErr: "unit 'unit1' has empty path",
		},
		{
			name: "duplicate unit names",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "unit1",
						Source: "source1",
						Path:   "path1",
					},
					{
						Name:   "unit1",
						Source: "source2",
						Path:   "path2",
					},
				},
			},
			wantErr: "duplicate unit name found: 'unit1'",
		},
		{
			name: "duplicate unit paths",
			config: &config.StackConfigFile{
				Units: []*config.Unit{
					{
						Name:   "unit1",
						Source: "source1",
						Path:   "path1",
					},
					{
						Name:   "unit2",
						Source: "source2",
						Path:   "path1",
					},
				},
			},
			wantErr: "duplicate unit path found: 'path1'",
		},

		{
			name: "valid config with stacks",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "stack1",
						Source: "source1",
						Path:   "path1",
					},
					{
						Name:   "stack2",
						Source: "source2",
						Path:   "path2",
					},
				},
			},
			wantErr: "",
		},
		{
			name: "empty stack name",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "",
						Source: "source1",
						Path:   "path1",
					},
				},
			},
			wantErr: "stack at index 0 has empty name",
		},
		{
			name: "whitespace stack name",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "  ",
						Source: "source1",
						Path:   "path1",
					},
				},
			},
			wantErr: "stack at index 0 has empty name",
		},
		{
			name: "empty stack source",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "stack1",
						Source: "",
						Path:   "path1",
					},
				},
			},
			wantErr: "stack 'stack1' has empty source",
		},
		{
			name: "whitespace stack source",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "stack1",
						Source: "   ",
						Path:   "path1",
					},
				},
			},
			wantErr: "stack 'stack1' has empty source",
		},
		{
			name: "empty stack path",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "stack1",
						Source: "source1",
						Path:   "",
					},
				},
			},
			wantErr: "stack 'stack1' has empty path",
		},
		{
			name: "whitespace stack path",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "stack1",
						Source: "source1",
						Path:   "  ",
					},
				},
			},
			wantErr: "stack 'stack1' has empty path",
		},
		{
			name: "duplicate stack names",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "stack1",
						Source: "source1",
						Path:   "path1",
					},
					{
						Name:   "stack1",
						Source: "source2",
						Path:   "path2",
					},
				},
			},
			wantErr: "duplicate stack name found: 'stack1'",
		},
		{
			name: "duplicate stack paths",
			config: &config.StackConfigFile{
				Stacks: []*config.Stack{
					{
						Name:   "stack1",
						Source: "source1",
						Path:   "path1",
					},
					{
						Name:   "stack2",
						Source: "source2",
						Path:   "path1",
					},
				},
			},
			wantErr: "duplicate stack path found: 'path1'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := config.ValidateStackConfig(tt.config)
			if tt.wantErr != "" {
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
