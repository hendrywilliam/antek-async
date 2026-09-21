package kube

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
)

// metricsPath is the metrics-server endpoint the CPU and memory numbers come from. The metrics
// API is not a normal typed resource in client-go, so the app calls it through the discovery
// REST client and decodes the small part of it that the tables show. That keeps this package off
// the k8s.io/metrics module, which would otherwise have to be pinned to the client-go version.
const metricsPath = "/apis/metrics.k8s.io/v1beta1/"

// PodUsage is what metrics-server reports one pod is using, already formatted the way kubectl
// prints it. The table stacks the two values in one column, so they travel together.
type PodUsage struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	CPU       string `json:"cpu"`
	Memory    string `json:"memory"`
}

// NodeUsage is the same for a node. Nodes are cluster scoped, so there is no namespace.
type NodeUsage struct {
	Name   string `json:"name"`
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

// The rest of this file mirrors metrics.k8s.io/v1beta1 by hand.
type metricsObjectMeta struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type metricsUsage struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type containerMetrics struct {
	Usage metricsUsage `json:"usage"`
}

type podMetrics struct {
	Metadata   metricsObjectMeta  `json:"metadata"`
	Containers []containerMetrics `json:"containers"`
}

type podMetricsList struct {
	Items []podMetrics `json:"items"`
}

type nodeMetrics struct {
	Metadata metricsObjectMeta `json:"metadata"`
	Usage    metricsUsage      `json:"usage"`
}

type nodeMetricsList struct {
	Items []nodeMetrics `json:"items"`
}

// PodUsages reads the CPU and memory of every pod in one request, with each pod's containers
// already summed. The metrics API has no watch, so whoever shows these numbers decides how
// often to ask again: that interval belongs to the page, not to this package.
func PodUsages(ctx context.Context, clientset kubernetes.Interface) ([]PodUsage, error) {
	raw, err := metricsRaw(ctx, clientset, "pods")
	if err != nil {
		return nil, err
	}

	var list podMetricsList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("cannot read Pod metrics: %w", err)
	}

	usages := make([]PodUsage, 0, len(list.Items))
	for _, item := range list.Items {
		cpu, memory := sumUsage(item.Containers)
		usages = append(usages, PodUsage{
			Namespace: item.Metadata.Namespace,
			Name:      item.Metadata.Name,
			CPU:       cpu,
			Memory:    memory,
		})
	}

	sortByNamespaceAndName(
		usages,
		func(usage PodUsage) string { return usage.Namespace },
		func(usage PodUsage) string { return usage.Name },
	)

	return usages, nil
}

func NodeUsages(ctx context.Context, clientset kubernetes.Interface) ([]NodeUsage, error) {
	raw, err := metricsRaw(ctx, clientset, "nodes")
	if err != nil {
		return nil, err
	}

	var list nodeMetricsList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("cannot read Node metrics: %w", err)
	}

	usages := make([]NodeUsage, 0, len(list.Items))
	for _, item := range list.Items {
		usages = append(usages, NodeUsage{
			Name:   item.Metadata.Name,
			CPU:    formatCPU(parseQuantity(item.Usage.CPU)),
			Memory: formatMemory(parseQuantity(item.Usage.Memory)),
		})
	}

	sortByName(usages, func(usage NodeUsage) string { return usage.Name })

	return usages, nil
}

// metricsRaw reads one metrics kind. The API server's own message is wrapped rather than
// replaced, so an uninstalled metrics-server reads the same way it does in kubectl, while the
// prefix says which column went blank.
func metricsRaw(ctx context.Context, clientset kubernetes.Interface, kind string) ([]byte, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	raw, err := clientset.Discovery().RESTClient().
		Get().
		AbsPath(metricsPath + kind).
		Do(requestCtx).
		Raw()
	if err != nil {
		return nil, fmt.Errorf("cannot read %s metrics: %w", kindNoun(kind), err)
	}

	return raw, nil
}

// sumUsage adds up a pod's containers the way `kubectl top pod` does and formats the totals.
func sumUsage(containers []containerMetrics) (cpu, memory string) {
	totalCPU := resource.Quantity{}
	totalMemory := resource.Quantity{}

	for _, container := range containers {
		totalCPU.Add(parseQuantity(container.Usage.CPU))
		totalMemory.Add(parseQuantity(container.Usage.Memory))
	}

	return formatCPU(totalCPU), formatMemory(totalMemory)
}

// formatCPU and formatMemory are the two helpers behind kubectl's CPU(cores) and MEMORY(bytes)
// columns, so a value here reads exactly as it does in `kubectl top`.
func formatCPU(quantity resource.Quantity) string {
	return fmt.Sprintf("%dm", quantity.MilliValue())
}

func formatMemory(quantity resource.Quantity) string {
	return fmt.Sprintf("%dMi", quantity.Value()/(1024*1024))
}

// parseQuantity treats an unparseable value as no usage, the same way kubectl does, because a
// container that reports nothing should not blank the whole row.
func parseQuantity(value string) resource.Quantity {
	quantity, err := resource.ParseQuantity(value)
	if err != nil {
		return resource.Quantity{}
	}

	return quantity
}

// kindNoun spells the metrics kind the way the error messages and the UI need it.
func kindNoun(kind string) string {
	if kind == "nodes" {
		return "Node"
	}
	return "Pod"
}
