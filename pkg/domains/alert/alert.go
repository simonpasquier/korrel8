// package alert implements korrel8r interfaces on prometheus alerts.
package alert

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	openapiclient "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/korrel8r/korrel8r/pkg/korrel8r"
	"github.com/korrel8r/korrel8r/pkg/korrel8r/impl"
	"github.com/prometheus/alertmanager/api/v2/client"
	"github.com/prometheus/alertmanager/api/v2/client/alert"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

var (
	_ korrel8r.Domain = Domain
	_ korrel8r.Class  = Class{}
	_ korrel8r.Query  = &Query{}
	_ korrel8r.Store  = &Store{}
)

var Domain = domain{}

type domain struct{}

func (domain) String() string              { return "alert" }
func (domain) Class(string) korrel8r.Class { return Class{} }
func (domain) Classes() []korrel8r.Class   { return []korrel8r.Class{Class{}} }
func (domain) UnmarshalQuery(r []byte) (korrel8r.Query, error) {
	return impl.UnmarshalQuery(r, &Query{})
}

type Class struct{} // Only one class - "alert"

func (c Class) Domain() korrel8r.Domain { return Domain }
func (c Class) String() string          { return "alert" }
func (c Class) New() korrel8r.Object    { return &Object{} }
func (c Class) ID(o korrel8r.Object) any {
	if o, _ := o.(*Object); o != nil {
		// The identity of an alert is defined by its labels.
		return o.Fingerprint
	}

	return nil
}

type Object struct {
	// Common fields.
	Labels      map[string]string
	Annotations map[string]string
	Fingerprint string `json:"fingerprint"`
	Status      string // inactive|pending|firing|suppressed

	// Prometheus fields.
	Value    string
	ActiveAt time.Time `json:"activeAt"`

	// Alertmanager fields.
	StartsAt     time.Time  `json:"startsAt"`
	EndsAt       time.Time  `json:"endsAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	Receivers    []Receiver `json:"receivers"`
	InhibitedBy  []string   `json:"inhibitedBy"`
	SilencedBy   []string   `json:"silencedBy"`
	GeneratorURL string     `json:"generatorURL"`
}

type Receiver struct {
	Name string `json:"name"`
}

type Query struct {
	Labels map[string]string
}

func (q *Query) Class() korrel8r.Class { return Class{} }

func (domain) ConsoleURLToQuery(u *url.URL) (korrel8r.Query, error) {
	m := map[string]string{}
	uq := u.Query()
	for k := range uq {
		m[k] = uq.Get(k)
	}
	return &Query{Labels: m}, nil
}

func (domain) QueryToConsoleURL(query korrel8r.Query) (*url.URL, error) {
	q, err := impl.TypeAssert[*Query](query)
	if err != nil {
		return nil, err
	}
	uq := url.Values{
		"rowFilter-alert-state": []string{""}, // do not filter by alert state.
	}
	alertFilter := make([]string, 0, len(q.Labels))
	for k, v := range q.Labels {
		alertFilter = append(alertFilter, fmt.Sprintf("%s=%s", k, v))
	}
	uq.Add("alerts", strings.Join(alertFilter, ","))

	return &url.URL{
		Path:     "/monitoring/alerts",
		RawQuery: uq.Encode(),
	}, nil
}

type Store struct {
	alertmanagerAPI *client.AlertmanagerAPI
	prometheusAPI   v1.API
}

func NewStore(alertmanagerURL *url.URL, prometheusURL *url.URL, hc *http.Client) (*Store, error) {
	alertmanagerAPI, err := newAlertmanagerClient(alertmanagerURL, hc)
	if err != nil {
		return nil, err
	}

	prometheusAPI, err := newPrometheusClient(prometheusURL, hc)
	if err != nil {
		return nil, err
	}

	return &Store{
		alertmanagerAPI: alertmanagerAPI,
		prometheusAPI:   prometheusAPI,
	}, nil
}

func newAlertmanagerClient(u *url.URL, hc *http.Client) (*client.AlertmanagerAPI, error) {
	transport := openapiclient.NewWithClient(u.Host, client.DefaultBasePath, []string{u.Scheme}, hc)

	// Append the "/api/v2" path if not already present.
	path, err := url.JoinPath(strings.TrimSuffix(u.Path, client.DefaultBasePath), client.DefaultBasePath)
	if err != nil {
		return nil, err
	}
	u.Path = path

	return client.New(transport, strfmt.Default), nil
}

func newPrometheusClient(u *url.URL, hc *http.Client) (v1.API, error) {
	client, err := api.NewClient(api.Config{
		Address: u.String(),
		Client:  hc,
	})
	if err != nil {
		return nil, err
	}

	return v1.NewAPI(client), nil
}

func (Store) Domain() korrel8r.Domain { return Domain }

func convertLabelSetToMap(m model.LabelSet) map[string]string {
	res := make(map[string]string, len(m))
	for k, v := range m {
		res[string(k)] = string(v)
	}

	return res
}

// matches returns true if the alert matches the korrel8r query.
func (q *Query) matches(a v1.Alert) bool {
	for k, v := range q.Labels {
		v2 := string(a.Labels[model.LabelName(k)])
		if v != v2 {
			return false
		}
	}

	return true
}

func (s Store) Get(ctx context.Context, query korrel8r.Query, result korrel8r.Appender) error {
	q, err := impl.TypeAssert[*Query](query)
	if err != nil {
		return err
	}

	// Gather matching alerts from the Prometheus Alerts API.
	// TODO(simonpasquier): use the Rules API to get the PromQL query.
	alertsResult, err := s.prometheusAPI.Alerts(ctx)
	if err != nil {
		return fmt.Errorf("failed to query alerts from Prometheus API: %w", err)
	}

	var (
		alerts       = []*Object{}
		fingerprints = map[string]int{}
	)
	for i, a := range alertsResult.Alerts {
		if !q.matches(a) {
			continue
		}

		alerts = append(alerts, &Object{
			Labels:      convertLabelSetToMap(a.Labels),
			Annotations: convertLabelSetToMap(a.Annotations),
			Status:      string(a.State),
			Value:       a.Value,
			ActiveAt:    a.ActiveAt,
		})

		fingerprints[a.Labels.Fingerprint().String()] = i
	}

	// Gather matching alerts from the Alertmanager API and merge with the existing alerts.
	var filters []string
	for k, v := range q.Labels {
		filters = append(filters, fmt.Sprintf("%v=%v", k, v))
	}
	resp, err := s.alertmanagerAPI.Alert.GetAlerts(alert.NewGetAlertsParamsWithContext(ctx).WithFilter(filters))
	if err != nil {
		return fmt.Errorf("failed to query alerts from Alertmanager API: %w", err)
	}

	for _, a := range resp.Payload {
		i, found := fingerprints[*a.Fingerprint]
		if !found {
			o := &Object{
				Labels:      a.Alert.Labels,
				Annotations: a.Annotations,
			}

			i = len(alerts)
			alerts = append(alerts, o)
		}

		o := alerts[i]
		o.StartsAt = time.Time(*a.StartsAt)
		o.EndsAt = time.Time(*a.EndsAt)
		o.GeneratorURL = a.Alert.GeneratorURL.String()
		for _, r := range a.Receivers {
			o.Receivers = append(o.Receivers, Receiver{Name: *r.Name})
		}
		o.SilencedBy = a.Status.SilencedBy
		o.InhibitedBy = a.Status.InhibitedBy

		if o.Status == "" {
			o.Status = *a.Status.State
			if o.Status != "suppressed" {
				o.Status = "firing"
			}
		} else if *a.Status.State == "suppressed" {
			o.Status = *a.Status.State
		}
	}

	for _, a := range alerts {
		result.Append(a)
	}

	return nil
}
