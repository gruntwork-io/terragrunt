package runall

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMissingRunAllArguments(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.TerraformCommand = ""

	err = Run(context.Background(), tgOptions)
	require.Error(t, err)

	var missingCommand MissingCommand
	ok := errors.As(err, &missingCommand)
	fmt.Println(err, errors.Unwrap(err))
	assert.True(t, ok)
}
