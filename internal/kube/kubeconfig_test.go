package kube

import (
	"os"
	"path/filepath"
	"testing"
)

func createKubeconfig(t *testing.T, path string) string {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	return path
}

func TestResolveKubeconfig(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	cwd := filepath.Join(base, "project")
	emptyHome := filepath.Join(base, "empty-home")
	emptyCwd := filepath.Join(base, "empty-project")

	manualPath := createKubeconfig(t, filepath.Join(base, "manual.yaml"))
	envFirst := createKubeconfig(t, filepath.Join(base, "env-first.yaml"))
	envSecond := createKubeconfig(t, filepath.Join(base, "env-second.yaml"))
	homeConfig := createKubeconfig(t, filepath.Join(home, kubeDirName, kubeconfigFileName))
	projectConfig := createKubeconfig(t, filepath.Join(cwd, kubeDirName, kubeconfigFileName))
	missing := filepath.Join(base, "missing.yaml")

	// This directory only has a project kubeconfig, to exercise the last fallback.
	lonelyCwd := filepath.Join(base, "lonely")
	lonelyConfig := createKubeconfig(t, filepath.Join(lonelyCwd, kubeDirName, kubeconfigFileName))

	// Every existing kubeconfig is listed so each case also proves which files were
	// deliberately not chosen.
	tests := []struct {
		name        string
		manual      string
		envValue    string
		homeDir     string
		cwd         string
		wantPath    string
		wantSource  Source
		wantSources []Source // inspected locations, in order
		notResolved []string // existing files that must not be chosen
	}{
		{
			name:        "manual pick wins over everything",
			manual:      manualPath,
			envValue:    envFirst,
			homeDir:     home,
			cwd:         cwd,
			wantPath:    manualPath,
			wantSource:  SourceManual,
			wantSources: []Source{SourceManual},
			notResolved: []string{envFirst, homeConfig, projectConfig},
		},
		{
			name:        "KUBECONFIG beats home and project",
			envValue:    envFirst,
			homeDir:     home,
			cwd:         cwd,
			wantPath:    envFirst,
			wantSource:  SourceEnv,
			wantSources: []Source{SourceEnv},
			notResolved: []string{homeConfig, projectConfig},
		},
		{
			name:        "first existing KUBECONFIG entry is used",
			envValue:    missing + string(os.PathListSeparator) + envSecond,
			homeDir:     home,
			cwd:         cwd,
			wantPath:    envSecond,
			wantSource:  SourceEnv,
			wantSources: []Source{SourceEnv, SourceEnv},
			notResolved: []string{envFirst, homeConfig, projectConfig},
		},
		{
			name:        "home beats an existing project kubeconfig",
			homeDir:     home,
			cwd:         cwd,
			wantPath:    homeConfig,
			wantSource:  SourceHome,
			wantSources: []Source{SourceHome},
			notResolved: []string{projectConfig},
		},
		{
			name:        "project is the last fallback",
			homeDir:     emptyHome,
			cwd:         lonelyCwd,
			wantPath:    lonelyConfig,
			wantSource:  SourceProject,
			wantSources: []Source{SourceHome, SourceProject},
			notResolved: []string{homeConfig, manualPath},
		},
		{
			name:        "nothing found",
			homeDir:     emptyHome,
			cwd:         emptyCwd,
			wantPath:    "",
			wantSource:  SourceNone,
			wantSources: []Source{SourceHome, SourceProject},
			notResolved: []string{homeConfig, projectConfig, manualPath},
		},
		{
			name:        "manual path that disappeared falls through to discovery",
			manual:      missing,
			homeDir:     home,
			cwd:         cwd,
			wantPath:    homeConfig,
			wantSource:  SourceHome,
			wantSources: []Source{SourceManual, SourceHome},
			notResolved: []string{projectConfig},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotPath, gotSource, candidates := resolveKubeconfig(test.manual, test.envValue, test.homeDir, test.cwd)

			if gotPath != test.wantPath {
				t.Errorf("path = %q, want %q", gotPath, test.wantPath)
			}
			if gotSource != test.wantSource {
				t.Errorf("source = %q, want %q", gotSource, test.wantSource)
			}

			if len(candidates) != len(test.wantSources) {
				t.Fatalf("inspected %d locations, want %d", len(candidates), len(test.wantSources))
			}
			for i, want := range test.wantSources {
				if candidates[i].Source != want {
					t.Errorf("candidate %d source = %q, want %q", i, candidates[i].Source, want)
				}
			}
			for _, path := range test.notResolved {
				if gotPath == path {
					t.Errorf("resolved %q, but it must lose to a higher priority source", path)
				}
			}

			// When a file was resolved it must be the last inspected location and be
			// reported as existing; when nothing was found none may exist.
			last := candidates[len(candidates)-1]
			if test.wantSource == SourceNone {
				for i, candidate := range candidates {
					if candidate.Exists {
						t.Errorf("candidate %d %q reported as existing, want all missing", i, candidate.Path)
					}
				}
				return
			}
			if !last.Exists {
				t.Errorf("resolved candidate %q reported as missing", last.Path)
			}
			if last.Path != test.wantPath {
				t.Errorf("resolved candidate path = %q, want %q", last.Path, test.wantPath)
			}
		})
	}
}
