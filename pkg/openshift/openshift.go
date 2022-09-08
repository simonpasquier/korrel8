// package openshift has openshift-specific helpers
// The ../k8s package provides the main Kubernetes support.
package openshift

import (
	"context"

	"github.com/alanconway/korrel8/pkg/korrel8"
	"github.com/alanconway/korrel8/pkg/prometheus"
	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	MonitoringNS     = "openshift-monitoring"
	AlertmanagerMain = "alertmanager-main"
)

func init() {
	runtime.Must(routev1.AddToScheme(scheme.Scheme))
}

// AlertManagerHost finds the main alert-manager host in an openshift cluster.
func AlertManagerHost(c client.Client) (string, error) {
	r := routev1.Route{}
	nsName := client.ObjectKey{Name: AlertmanagerMain, Namespace: MonitoringNS}
	if err := c.Get(context.Background(), nsName, &r); err != nil {
		return "", err
	}
	return r.Spec.Host, nil
}

// AlertManagerStore creates a store client for alert manager.
func AlertManagerStore(cfg *rest.Config, host string) (korrel8.Store, error) {
	c, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, err
	}
	return prometheus.NewAlertStore(host, c), nil
}