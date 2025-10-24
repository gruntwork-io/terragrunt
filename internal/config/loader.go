// Package config provides the main orchestration for loading Terragrunt configuration
// from .terragruntrc.json files and environment variables, then applying them to CLI flags.
package config

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/urfave/cli/v2"
)

// LoadTerragruntRC is the main orchestrator function that coordinates all config loading components.
// It implements the complete configuration loading flow:
//
//  1. Find config file using FindConfigFile (returns nil if not found - not an error)
//  2. Parse config file using ParseConfigFile
//  3. Build flag registry from app.Flags using BuildFlagRegistry
//  4. Load env var overrides using LoadEnvVarOverrides
//  5. Merge configurations using MergeConfigurations (env > config file precedence)
//  6. For each merged value:
//     a. Find corresponding flag in app.Flags
//     b. Convert value to correct type using ConvertValue
//     c. Set flag.Value field via Value().Set()
//  7. Log debug info about loaded config
//  8. Return nil on success, ConfigError on failures
//
// The function follows these principles:
// - Early return on errors with context-rich error messages
// - No config file found is NOT an error (backward compatibility)
// - All errors are wrapped in ConfigError with file path and flag context
// - Structured logging at each major step for observability
// - Performance target: <10ms for typical configurations
//
// Parameters:
//   - ctx: context for cancellation and caching
//   - app: cli.App with flags to configure
//
// Returns:
//   - error: ConfigError with context on failure, nil on success
//
// Example usage:
//
//	func main() {
//	    app := cli.NewApp()
//	    app.Flags = []cli.Flag{ /* ... */ }
//
//	    ctx := context.Background()
//	    if err := config.LoadTerragruntRC(ctx, app); err != nil {
//	        log.Fatalf("Failed to load config: %v", err)
//	    }
//
//	    app.Run(os.Args)
//	}
func LoadTerragruntRC(ctx context.Context, app *cli.App) error {
	log.Debugf("Starting LoadTerragruntRC orchestration")

	// Step 1: Find config file
	// Returns empty string if not found - this is NOT an error
	configPath, err := FindConfigFile(ctx)
	if err != nil {
		return NewConfigError("", "", "failed to find config file", err)
	}

	// No config file found - return early (not an error, backward compatibility)
	if configPath == "" {
		log.Debugf("No .terragruntrc.json file found, skipping config loading")
		return nil
	}

	log.Infof("Found config file at: %s", configPath)

	// Step 2: Parse config file
	config, err := ParseConfigFile(configPath)
	if err != nil {
		// Error is already wrapped in ConfigError by ParseConfigFile
		return err
	}

	log.Infof("Parsed config file with %d values", len(config.Values))

	// Step 3: Build flag registry from app.Flags
	flagRegistry := BuildFlagRegistry(app.Flags)
	log.Debugf("Built flag registry with %d flags", len(flagRegistry))

	// Step 4: Load env var overrides
	// This function never returns an error (best-effort processing)
	envOverrides, _ := LoadEnvVarOverrides(flagRegistry)
	log.Debugf("Loaded %d environment variable overrides", len(envOverrides))

	// Step 5: Merge configurations (env > config file precedence)
	merged := MergeConfigurations(config, envOverrides)
	log.Infof("Merged configuration contains %d values", len(merged))

	// Step 6: Apply merged config to app.Flags
	// We'll set the default values directly in the flag structs
	// This works because LoadTerragruntRC is called before app.Run()
	appliedCount := 0
	for flagName, value := range merged {
		mapping, ok := flagRegistry[flagName]
		if !ok {
			// Unknown flag - skip with warning for forward compatibility
			log.Warnf("Unknown flag '%s' in configuration (will be ignored)", flagName)
			continue
		}

		// Convert value to correct type
		converted, err := ConvertValue(value, mapping.Type)
		if err != nil {
			return NewConfigError(
				config.SourceFile,
				flagName,
				fmt.Sprintf("type conversion failed for type %s", mapping.Type),
				err,
			)
		}

		// Apply to flag by setting its destination/default value
		// This requires type assertions to access the concrete flag types
		if err := setFlagDefaultValue(mapping.OriginalFlag, converted, mapping.Type); err != nil {
			return NewConfigError(
				config.SourceFile,
				flagName,
				"failed to set flag default value",
				err,
			)
		}

		log.Debugf("Applied config: %s=%v (type: %s)", flagName, converted, mapping.Type)
		appliedCount++
	}

	// Step 7: Log summary
	log.Infof("Successfully applied %d configuration values from %s", appliedCount, configPath)

	return nil
}

// setFlagDefaultValue sets the default value for a flag based on its type.
// This function performs type assertions to access the concrete flag types
// and sets their Destination or Value fields appropriately.
//
// This approach works because LoadTerragruntRC is called before app.Run(),
// so we're setting initial values that will be used unless overridden by CLI args.
func setFlagDefaultValue(flag cli.Flag, value interface{}, flagType FlagType) error {
	switch flagType {
	case FlagTypeBool:
		if boolFlag, ok := flag.(*cli.BoolFlag); ok {
			if b, ok := value.(bool); ok {
				if boolFlag.Destination == nil {
					boolFlag.Destination = new(bool)
				}
				*boolFlag.Destination = b
				// Also set the Value field for display purposes
				boolFlag.Value = b
				return nil
			}
			return fmt.Errorf("value is not a bool: %T", value)
		}

	case FlagTypeInt:
		if intFlag, ok := flag.(*cli.IntFlag); ok {
			if i, ok := value.(int); ok {
				if intFlag.Destination == nil {
					intFlag.Destination = new(int)
				}
				*intFlag.Destination = i
				intFlag.Value = i
				return nil
			}
			return fmt.Errorf("value is not an int: %T", value)
		}
		// Also try Int64Flag
		if int64Flag, ok := flag.(*cli.Int64Flag); ok {
			if i, ok := value.(int); ok {
				if int64Flag.Destination == nil {
					int64Flag.Destination = new(int64)
				}
				*int64Flag.Destination = int64(i)
				int64Flag.Value = int64(i)
				return nil
			}
			return fmt.Errorf("value is not an int: %T", value)
		}

	case FlagTypeString:
		if stringFlag, ok := flag.(*cli.StringFlag); ok {
			if s, ok := value.(string); ok {
				if stringFlag.Destination == nil {
					stringFlag.Destination = new(string)
				}
				*stringFlag.Destination = s
				stringFlag.Value = s
				return nil
			}
			return fmt.Errorf("value is not a string: %T", value)
		}

	case FlagTypeStringSlice:
		if sliceFlag, ok := flag.(*cli.StringSliceFlag); ok {
			if slice, ok := value.([]string); ok {
				if sliceFlag.Destination == nil {
					sliceFlag.Destination = &cli.StringSlice{}
				}
				// Replace the entire slice
				*sliceFlag.Destination = *cli.NewStringSlice(slice...)
				sliceFlag.Value = cli.NewStringSlice(slice...)
				return nil
			}
			return fmt.Errorf("value is not a []string: %T", value)
		}
	}

	// If we get here, the flag type doesn't match our expectations
	// This could be a Terragrunt-specific flag type or an unknown type
	// Log a warning and skip this flag
	log.Warnf("Unsupported flag type for %v (type: %T), skipping", flag, flag)
	return nil
}

