package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alanconway/korrel8/internal/pkg/test"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

func TestGet_Alert(t *testing.T) {
	// Dubious test, assumes there is an alert on the cluster.
	test.SkipIfNoCluster(t)
	var exitCode int
	stdout, stderr := test.FakeMain([]string{"", "get", "alert", "{}", "-o=json"}, func() { exitCode = Execute() })
	require.Equal(t, 0, exitCode, "exitCode=%v: %v", exitCode, stderr)
	var result []models.GettableAlert
	require.NoError(t, json.Unmarshal([]byte(stdout), &result), "expect valid alerts, got: %v", stdout)
	require.NotEmpty(t, result, "expect at least one alert")
	t.Logf("%v", test.AsJSON(result[0]))
}

func TestCorrelate_Pods(t *testing.T) {
	test.SkipIfNoCluster(t)
	c := test.K8sClient
	ns := test.CreateUniqueNamespace(t, c)

	// Pod for test deployment
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.Name(),
			Namespace: ns,
			Labels:    map[string]string{"test": "testme"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "testme",
					Image: "quay.io/quay/busybox",
					Command: []string{
						"sh", "-c",
						"for i in range 6 do; echo $(date) here we go $i; sleep 10; echo $(date) Oh dear, oh dear; exit 1",
					},
				},
			}}}
	// Watch for pod creation
	w, err := c.Watch(context.Background(), &corev1.PodList{Items: []corev1.Pod{pod}})
	require.NoError(t, err)
	defer w.Stop()

	// Deployment
	d := appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "testme", Namespace: ns},
		Spec: appv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: pod.Labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: pod.ObjectMeta,
				Spec:       pod.Spec,
			},
		},
	}
	require.NoError(t, c.Create(context.Background(), &d))
	require.NoError(t, err)
	select {
	case e := <-w.ResultChan():
		assert.Equal(t, e.Type, watch.Added)
	case <-time.After(time.Second * 10):
		t.Fatal("timeout waiting for Pod")
	}
	var exitCode int
	stdout, stderr := test.FakeMain([]string{"", "correlate", "alert/alert", "loki/log", "testdata/kubeDeploymentAlert.json"}, func() {
		exitCode = Execute()
	})
	require.Equal(t, 0, exitCode, "exitCode=%v: %v", exitCode, stderr)
	require.Regexp(t, `resulting queries: \[{kubernetes_namespace_name="default",kubernetes_pod_name="demo-.*"}]`, strings.TrimSpace(stdout))
}