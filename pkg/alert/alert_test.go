package alert

import (
	"context"
	"testing"

	"github.com/korrel8r/korrel8r/internal/pkg/test"
	"github.com/korrel8r/korrel8r/pkg/korrel8r"
	"github.com/korrel8r/korrel8r/pkg/uri"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	// Dubious test, assumes there is an alert on the cluster.
	test.SkipIfNoCluster(t)
	store, err := NewOpenshiftAlertManagerStore(context.Background(), test.RESTConfig)
	require.NoError(t, err)
	result := korrel8r.NewListResult()
	require.NoError(t, store.Get(context.Background(), uri.Reference{}, result))
	require.NotEmpty(t, result.List())
}
