package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	yaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
)

// fieldManager identifies this app when the API server tracks ownership of applied fields, so a
// later apply can report a conflict against whatever else touched the same fields.
const fieldManager = "antek-async"

// defaultNamespace is where a namespaced manifest without metadata.namespace lands, matching
// kubectl's behaviour when no namespace is given.
const defaultNamespace = "default"

// ApplyResult names the object a manifest was sent as, so the editor can confirm what happened.
type ApplyResult struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
}

// ApplyYAML sends one manifest to the cluster with server-side apply, which creates the object
// when it is absent and updates it otherwise. Any kind the cluster knows is supported because
// the resource is resolved through discovery rather than a compiled-in mapping.
func ApplyYAML(ctx context.Context, path, document string, force bool) (ApplyResult, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	object, err := decodeObject(document)
	if err != nil {
		return ApplyResult{}, err
	}

	restConfig, err := RestConfigFor(path)
	if err != nil {
		return ApplyResult{}, err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("cannot build discovery client: %w", err)
	}

	mapping, err := resolveMapping(discoveryClient, object.GroupVersionKind())
	if err != nil {
		return ApplyResult{}, err
	}

	// A cluster-scoped kind must not carry a namespace, or the API server rejects it.
	namespace, namespaced := placement(mapping, object.GetNamespace())
	object.SetNamespace(namespace)

	body, err := object.MarshalJSON()
	if err != nil {
		return ApplyResult{}, fmt.Errorf("cannot encode %s %s: %w", object.GetKind(), object.GetName(), err)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("cannot build dynamic client: %w", err)
	}

	resource := dynamicClient.Resource(mapping.Resource)
	var target dynamic.ResourceInterface = resource
	if namespaced {
		target = resource.Namespace(namespace)
	}

	options := metav1.PatchOptions{FieldManager: fieldManager}
	if force {
		// Force only when asked: it silently steals fields that another manager owns.
		options.Force = &force
	}

	applied, err := target.Patch(requestCtx, object.GetName(), types.ApplyPatchType, body, options)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("cannot apply %s %s: %w", object.GetKind(), object.GetName(), err)
	}

	return ApplyResult{
		APIVersion: applied.GetAPIVersion(),
		Kind:       applied.GetKind(),
		Namespace:  applied.GetNamespace(),
		Name:       applied.GetName(),
	}, nil
}

// decodeObject turns one YAML or JSON document into an unstructured object and rejects the
// shapes the API server cannot act on, so parse errors stay readable. Multiple documents are
// refused rather than silently applying only the first.
func decodeObject(document string) (*unstructured.Unstructured, error) {
	if strings.TrimSpace(document) == "" {
		return nil, errors.New("Manifest is empty")
	}

	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(document), 4096)

	var raw map[string]any
	if err := decoder.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("Manifest is empty")
		}
		return nil, fmt.Errorf("cannot parse manifest: %w", err)
	}
	if len(raw) == 0 {
		return nil, errors.New("Manifest is empty")
	}

	var extra map[string]any
	switch err := decoder.Decode(&extra); {
	case err == nil && len(extra) > 0:
		return nil, errors.New("Apply one document at a time; found more than one YAML document")
	case err != nil && !errors.Is(err, io.EOF):
		return nil, fmt.Errorf("cannot parse manifest: %w", err)
	}

	object := &unstructured.Unstructured{Object: raw}
	switch {
	case object.GetAPIVersion() == "":
		return nil, errors.New("Manifest must set apiVersion")
	case object.GetKind() == "":
		return nil, errors.New("Manifest must set kind")
	case object.GetName() == "":
		return nil, errors.New("Manifest must set metadata.name")
	}

	return object, nil
}

// placement decides where an object belongs: the namespace to send it to and whether its kind
// is namespaced at all. A namespaced object without metadata.namespace lands in "default", the
// same way kubectl does, while a cluster-scoped kind is stripped of any namespace.
func placement(mapping *meta.RESTMapping, namespace string) (string, bool) {
	switch {
	case mapping.Scope.Name() != meta.RESTScopeNameNamespace:
		return "", false
	case namespace == "":
		return defaultNamespace, true
	default:
		return namespace, true
	}
}

// resolveMapping asks the cluster which resource a kind maps to, so the editor works for any
// kind without a compiled-in table. Discovery runs per request, which keeps it in step with a
// cluster whose API surface changed since startup.
func resolveMapping(discoveryClient discovery.DiscoveryInterface, gvk schema.GroupVersionKind) (*meta.RESTMapping, error) {
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, fmt.Errorf("cannot discover API resources: %w", err)
	}

	mapping, err := restmapper.NewDiscoveryRESTMapper(groupResources).RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("unknown kind %s: %w", gvk.Kind, err)
	}

	return mapping, nil
}
