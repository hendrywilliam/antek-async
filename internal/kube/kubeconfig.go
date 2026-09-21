// Package kube resolves kubeconfig files and streams pod snapshots from a cluster.
package kube

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Source identifies where the active kubeconfig was resolved from.
type Source string

const (
	SourceManual  Source = "manual" // picked by the user through the file dialog
	SourceEnv     Source = "KUBECONFIG"
	SourceHome    Source = "home"    // ~/.kube/config
	SourceProject Source = "project" // <cwd>/.kube/config
	SourceNone    Source = "none"
)

const (
	kubeDirName        = ".kube"
	kubeconfigFileName = "config"
)

// Candidate is one location inspected while resolving the kubeconfig.
type Candidate struct {
	Source Source `json:"source"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

// Status describes the resolved kubeconfig and the cluster it points at.
type Status struct {
	Source     Source      `json:"source"`
	Path       string      `json:"path"`
	Context    string      `json:"context"`
	Cluster    string      `json:"cluster"`
	Server     string      `json:"server"`
	Candidates []Candidate `json:"candidates"`
	Error      string      `json:"error"`
}

// Resolve returns the kubeconfig to use and the cluster it points at. Precedence is
// manual pick, then $KUBECONFIG, then ~/.kube/config, then <cwd>/.kube/config. Error
// strings are written for direct display in the UI.
func Resolve(manual string) Status {
	homeDir, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()

	path, source, candidates := resolveKubeconfig(
		manual,
		os.Getenv(clientcmd.RecommendedConfigPathEnvVar),
		homeDir,
		cwd,
	)

	status := Status{Source: source, Path: path, Candidates: candidates}
	switch {
	case manual != "" && source != SourceManual:
		status.Error = fmt.Sprintf("Selected kubeconfig file not found: %s", manual)
	case source == SourceNone:
		status.Error = "Kubeconfig not found"
	}

	if path == "" {
		return status
	}

	contextName, cluster, server, err := describeKubeconfig(path)
	if err != nil {
		status.Error = fmt.Sprintf("Cannot read kubeconfig: %v", err)
	}
	status.Context, status.Cluster, status.Server = contextName, cluster, server

	return status
}

// resolveKubeconfig picks a single kubeconfig file, first match wins:
//
//	manual > $KUBECONFIG > <homeDir>/.kube/config > <cwd>/.kube/config
//
// Only one file is used, without merging, so a project kubeconfig never mixes with
// the home one. Inputs are arguments rather than environment reads so the lookup
// stays testable.
func resolveKubeconfig(manual, envValue, homeDir, cwd string) (string, Source, []Candidate) {
	candidates := make([]Candidate, 0, 4)

	if manual != "" {
		candidates = append(candidates, newCandidate(SourceManual, manual))
		if fileExists(manual) {
			return manual, SourceManual, candidates
		}
	}

	for _, entry := range filepath.SplitList(envValue) {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		candidates = append(candidates, newCandidate(SourceEnv, entry))
		if fileExists(entry) {
			return entry, SourceEnv, candidates
		}
	}

	if homeDir != "" {
		home := filepath.Join(homeDir, kubeDirName, kubeconfigFileName)
		candidates = append(candidates, newCandidate(SourceHome, home))
		if fileExists(home) {
			return home, SourceHome, candidates
		}
	}

	if cwd != "" {
		project := filepath.Join(cwd, kubeDirName, kubeconfigFileName)
		candidates = append(candidates, newCandidate(SourceProject, project))
		if fileExists(project) {
			return project, SourceProject, candidates
		}
	}

	return "", SourceNone, candidates
}

func newCandidate(source Source, path string) Candidate {
	return Candidate{Source: source, Path: path, Exists: fileExists(path)}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// DefaultConfigDir is the starting directory for the file dialog. It returns "" when
// there is no ~/.kube directory, because Wails rejects a missing DefaultDirectory.
func DefaultConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	dir := filepath.Join(homeDir, kubeDirName)
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}

	return ""
}

// RestConfigFor builds a rest.Config for a single kubeconfig file. The typed, dynamic and
// discovery clients are all built from it, so the loading rules live in one place.
func RestConfigFor(path string) (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	// ExplicitPath makes Load() read this file only and fail loudly when missing.
	rules.ExplicitPath = path

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	// rest.Config.Timeout is deliberately left unset: it also applies to watches
	// and would tear down the pod watch on every interval.
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot load kubeconfig %s: %w", path, err)
	}

	return restConfig, nil
}

// ClientFor builds a Kubernetes clientset for a single kubeconfig file. One client is shared
// by whichever resource watcher is active.
func ClientFor(path string) (kubernetes.Interface, error) {
	restConfig, err := RestConfigFor(path)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(restConfig)
}

// describeKubeconfig reports the current context, cluster name and API server of a
// kubeconfig file so the UI can show which cluster is being watched.
func describeKubeconfig(path string) (contextName, cluster, server string, err error) {
	raw, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return "", "", "", err
	}

	contextName = raw.CurrentContext
	kubeContext, ok := raw.Contexts[contextName]
	if !ok {
		return contextName, "", "", nil
	}

	clusterEntry, ok := raw.Clusters[kubeContext.Cluster]
	if !ok {
		return contextName, kubeContext.Cluster, "", nil
	}

	return contextName, kubeContext.Cluster, clusterEntry.Server, nil
}
