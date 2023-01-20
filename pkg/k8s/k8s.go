// package k8s is a Kubernetes implementation of the korrel8r interfaces
package k8s

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"reflect"
	"regexp"

	"github.com/korrel8r/korrel8r/pkg/korrel8r"
	"github.com/korrel8r/korrel8r/pkg/uri"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	_ korrel8r.Domain       = Domain
	_ korrel8r.Class        = Class{}
	_ korrel8r.RefConverter = &Store{}
)

type domain struct{}

func (d domain) String() string { return "k8s" }

// Class name in one of the forms: Kind,  Kind.Group,  Kind.Version.Group.
func (d domain) Class(name string) korrel8r.Class {
	tryGVK, tryGK := schema.ParseKindArg(name)
	switch {
	case tryGVK != nil && Scheme.Recognizes(*tryGVK): // Direct hit
		return Class(*tryGVK)
	case tryGK.Group != "": // GroupKind, must find version
		for _, gv := range Scheme.VersionsForGroupKind(tryGK) {
			if gvk := tryGK.WithVersion(gv.Version); Scheme.Recognizes(gvk) {
				return Class(gvk)
			}
		}
	default: // Only have a Kind, search for group and version.
		for _, gv := range Scheme.PreferredVersionAllGroups() {
			if gvk := gv.WithKind(tryGK.Kind); Scheme.Recognizes(gvk) {
				return Class(gvk)
			}
		}
	}
	return nil
}

func (d domain) Classes() (classes []korrel8r.Class) {
	for gvk := range Scheme.AllKnownTypes() {
		classes = append(classes, Class(gvk))
	}
	return classes
}

var Domain korrel8r.Domain = domain{} // Implements interface

// TODO the Class implementation assumes all objects are pointers to the generated API struct.
// We could use scheme & GVK comparisons to generalize to untyped representations as well.

// Class is a k8s GroupVersionKind.
type Class schema.GroupVersionKind

// ClassOf returns the Class of o, which must be a pointer to a typed API resource struct.
func ClassOf(o client.Object) korrel8r.Class {
	if gvks, _, err := Scheme.ObjectKinds(o); err == nil {
		return Class(gvks[0])
	}
	return nil
}

func (c Class) ID(o korrel8r.Object) any {
	if o, _ := o.(client.Object); o != nil {
		return client.ObjectKeyFromObject(o)
	}
	return nil
}

func (c Class) Domain() korrel8r.Domain { return Domain }
func (c Class) New() korrel8r.Object {
	if o, err := Scheme.New(schema.GroupVersionKind(c)); err == nil {
		return o
	}
	return nil
}

func (c Class) String() string { return fmt.Sprintf("%v.%v.%v", c.Kind, c.Version, c.Group) }

type Object client.Object

// Store implements the korrel8r.Store interface as a k8s API client.
type Store struct {
	c    client.Client
	base *url.URL
}

// NewStore creates a new store
func NewStore(c client.Client, cfg *rest.Config) (*Store, error) {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	base, _, err := rest.DefaultServerURL(host, cfg.APIPath, schema.GroupVersion{}, true)
	return &Store{c: c, base: base}, err
}

func (s *Store) Resolve(ref uri.Reference) *url.URL { return ref.Resolve(s.base) }

func (s *Store) Get(ctx context.Context, ref uri.Reference, result korrel8r.Appender) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("get %v: %w", ref, err)
		}
	}()
	gvk, _, nsName, err := s.parsePath(ref.Path)
	if err != nil {
		return err
	}
	if nsName.Name != "" { // Request for single object.
		return s.getObject(ctx, gvk, nsName, result)
	} else {
		return s.getList(ctx, gvk, nsName.Namespace, ref.Query(), result)
	}
}

func (s *Store) RefClass(ref uri.Reference) (korrel8r.Class, error) {
	if gvk, _, _, err := s.parsePath(ref.Path); err == nil {
		return Class(gvk), nil
	}
	return nil, fmt.Errorf("not a valid %d reference: %v", Domain, ref)
}

// Parsing a REST URI into components then using client.Client to recreate the REST URI.
//
// TODO revisit: this is weirdly indirect - parse an API path to make a Client call which re-creates the API path.
// Review tools in package rest to build the request more reliably.
func (s *Store) parsePath(path string) (gvk schema.GroupVersionKind, gvr schema.GroupVersionResource, nsName types.NamespacedName, err error) {
	parts := apiPath.FindStringSubmatch(path)
	if parts == nil {
		return gvk, gvr, nsName, fmt.Errorf("invalid k8s REST path: %v", path)
	}
	nsName.Namespace, nsName.Name = parts[apiNamespace], parts[apiName]
	gvr = schema.GroupVersionResource{Group: parts[apiGroup], Version: parts[apiVersion], Resource: parts[apiResource]}
	gvks, err := s.c.RESTMapper().KindsFor(gvr)
	if len(gvks) == 0 {
		return gvk, gvr, nsName, fmt.Errorf("not a valid %d reference: %v", Domain, path)
	}
	return gvks[0], gvr, nsName, err
}

func (s *Store) ClassFor(resource string) korrel8r.Class {
	gvks, err := s.c.RESTMapper().KindsFor(schema.GroupVersionResource{Resource: resource})
	if err != nil || len(gvks) == 0 {
		return nil
	}
	return Class(gvks[0])
}

func (s *Store) getObject(ctx context.Context, gvk schema.GroupVersionKind, nsName types.NamespacedName, result korrel8r.Appender) error {
	scheme := s.c.Scheme()
	o, err := scheme.New(gvk)
	if err != nil {
		return err
	}
	co, _ := o.(client.Object)
	if co == nil {
		return fmt.Errorf("invalid client.Object: %T", o)
	}
	err = s.c.Get(ctx, nsName, co)
	if err != nil {
		return err
	}
	result.Append(co)
	return nil
}

func (s *Store) getOpts(q url.Values) (opts []client.ListOption, err error) {
	if s := q.Get("labelSelector"); s != "" {
		selector, err := labels.Parse(s)
		if err != nil {
			return nil, err
		}
		opts = append(opts, client.MatchingLabelsSelector{Selector: selector})
	}
	if s := q.Get("fieldSelector"); s != "" {
		selector, err := fields.ParseSelector(s)
		if err != nil {
			return nil, err
		}
		opts = append(opts, client.MatchingFieldsSelector{Selector: selector})
	}
	return opts, nil
}

func (s *Store) getList(ctx context.Context, gvk schema.GroupVersionKind, namespace string, query url.Values, result korrel8r.Appender) error {
	gvk.Kind = gvk.Kind + "List"
	o, err := s.c.Scheme().New(gvk)
	if err != nil {
		return err
	}
	list, _ := o.(client.ObjectList)
	if list == nil {
		return fmt.Errorf("invalid list object %T", o)
	}
	opts, err := s.getOpts(query)
	if err != nil {
		return err
	}
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}
	if err := s.c.List(ctx, list, opts...); err != nil {
		return err
	}
	defer func() { // Handle reflect panics.
		if r := recover(); r != nil && err == nil {
			err = fmt.Errorf("invalid list object: %T", list)
		}
	}()
	items := reflect.ValueOf(list).Elem().FieldByName("Items")
	for i := 0; i < items.Len(); i++ {
		result.Append(items.Index(i).Addr().Interface().(client.Object))
	}
	return nil
}

// Parse a K8s API path into: group, version, namespace, resourcetype, name.
// See: https://kubernetes.io/docs/reference/using-api/api-concepts/
var apiPath = regexp.MustCompile(`(?:^|/)(?:(?:apis/([^/]+)/)|(?:api/))([^/]+)(?:/namespaces/([^/]+))?/([^/]+)(?:/([^/]+))?$`)

// Indices for match results from k8sPathRegex
const (
	apiGroup = iota + 1
	apiVersion
	apiNamespace
	apiResource
	apiName
)

// RefStoreToConsole converts a k8s reference to a console URL
func (s *Store) RefStoreToConsole(_ korrel8r.Class, storeRef uri.Reference) (uri.Reference, error) {
	_, gvr, nsName, err := s.parsePath(storeRef.Path)
	if err != nil {
		return uri.Reference{}, fmt.Errorf("invalid k8s reference: %v", storeRef)
	}
	var consoleRef uri.Reference
	if nsName.Namespace != "" { // Namespaced resource
		consoleRef.Path = path.Join("k8s", "ns", nsName.Namespace, gvr.Resource, nsName.Name)
	} else { // Cluster resource
		consoleRef.Path = path.Join("k8s", "cluster", gvr.Resource, nsName.Name)
	}
	return consoleRef, nil
}

var consolePath = regexp.MustCompile(`(?:^|/)(?:k8s/ns/([^/]+)|cluster)/([^/]+)(?:/([^/]+))?$`)

const (
	consoleNamepace = iota + 1
	consoleResource
	consoleName
)

func (s *Store) RefConsoleToStore(ref uri.Reference) (korrel8r.Class, uri.Reference, error) {
	p := consolePath.FindStringSubmatch(ref.Path)
	if p == nil {
		return nil, uri.Reference{}, fmt.Errorf("invalid k8s console reference: %v", ref)
	}
	if p[consoleResource] == "projects" { // Openshift alias for namespace
		p[consoleResource] = "namespaces"
	}
	gvks, err := s.c.RESTMapper().KindsFor(schema.GroupVersionResource{Resource: p[consoleResource]})
	if err != nil {
		return nil, uri.Reference{}, fmt.Errorf("invalid resrouce in console reference: %v", ref)
	}
	gvk := gvks[0]
	prefix := "apis"
	if gvk.Group == "" {
		prefix = "api"
	}
	r := uri.Reference{Path: path.Join(prefix, gvk.Group, gvk.Version)}
	if p[consoleNamepace] != "" {
		r.Path = path.Join(r.Path, "namespaces", p[consoleNamepace])
	}
	r.Path = path.Join(r.Path, p[consoleResource], p[consoleName])
	return Class(gvk), r, nil
}
