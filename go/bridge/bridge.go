package bridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Config struct {
	Theme   *string  `json:"theme"`
	Apps    []string `json:"apps"`
	Modules []string `json:"modules"`
}

type Settings struct {
	DevPort            int                    `json:"devPort"`
	GithubUser         *string                `json:"githubUser"`
	GithubOrgs         []string               `json:"githubOrgs"`
	InstallMode        string                 `json:"installMode"`
	CatalogSort        string                 `json:"catalogSort"`
	CatalogNewDays     int                    `json:"catalogNewDays"`
	LastInstallChoices map[string]interface{} `json:"lastInstallChoices"`
}

type WorkspacePaths struct {
	Desktop      string `json:"desktop"`
	Config       string `json:"config"`
	ConfigWrite  string `json:"configWrite"`
	ConfigLegacy bool   `json:"configLegacy"`
	PackageJson  string `json:"packageJson"`
	IsPlayground bool   `json:"isPlayground"`
	PackageName  string `json:"packageName,omitempty"`
	PackageDir   string `json:"packageDir,omitempty"`
	MetaDir      string `json:"metaDir"`
}

type WorkspaceContext struct {
	WorkspaceRoot string         `json:"workspaceRoot"`
	Settings      Settings       `json:"settings"`
	Config        Config         `json:"config"`
	Deps          []string       `json:"deps"`
	Paths         WorkspacePaths `json:"paths"`
}

type GithubForkInfo struct {
	Owner  string `json:"owner"`
	Exists bool   `json:"exists"`
	IsFork bool   `json:"isFork"`
}

type GithubSources struct {
	Official struct {
		Owner   string `json:"owner"`
		HtmlUrl string `json:"htmlUrl"`
	} `json:"official"`
	Fork *GithubForkInfo `json:"fork"`
}

type NpmSourceInfo struct {
	Version string `json:"version"`
}

type SourcesMetadata struct {
	Npm    *NpmSourceInfo `json:"npm"`
	Github GithubSources  `json:"github"`
}

type CatalogEntry struct {
	ShortName      string           `json:"shortName"`
	Description    string           `json:"description"`
	Stars          int              `json:"stars"`
	HtmlUrl        *string          `json:"htmlUrl"`
	Org            string           `json:"org"`
	Kind           string           `json:"kind"`
	LocalPath      *string          `json:"localPath"`
	Remote         *string          `json:"remote"`
	Version        *string          `json:"version"`
	Name           string           `json:"name"`
	Trusted        bool             `json:"trusted"`
	Installed      bool             `json:"installed"`
	LocalSource    bool             `json:"localSource"`
	InPackageJson  bool             `json:"inPackageJson"`
	ActiveTheme    bool             `json:"activeTheme"`
	SourcesMeta    *SourcesMetadata `json:"sourcesMeta"`
	UpdatedAt      *string          `json:"updatedAt"`
	PushedAt       *string          `json:"pushedAt"`
}

type CatalogResponse struct {
	Entries  []CatalogEntry `json:"entries"`
	CacheAge string         `json:"cacheAge"`
}

type WritePayload struct {
	Config       *Config           `json:"config,omitempty"`
	DepsToAdd    map[string]string `json:"depsToAdd,omitempty"`
	DepsToRemove []string          `json:"depsToRemove,omitempty"`
	Settings     *Settings         `json:"settings,omitempty"`
}

// FindWorkspaceRoot searches upwards from the current directory for pnpm-workspace.yaml
func FindWorkspaceRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "pnpm-workspace.yaml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("not inside an OWD workspace")
}

func getBridgePath(root string) string {
	return filepath.Join(root, "packages", "cli", "bin", "bridge.js")
}

// ReadWorkspaceContext runs the node bridge with --read
func ReadWorkspaceContext(root string) (*WorkspaceContext, error) {
	cmd := exec.Command("node", getBridgePath(root), "--read")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("bridge --read failed: %w (stderr: %s)", err, stderr.String())
	}

	var ctx WorkspaceContext
	if err := json.Unmarshal(stdout.Bytes(), &ctx); err != nil {
		return nil, fmt.Errorf("failed to unmarshal context JSON: %w", err)
	}

	return &ctx, nil
}

// ReadCatalog runs the node bridge with --catalog
func ReadCatalog(root string, force bool) (*CatalogResponse, error) {
	args := []string{getBridgePath(root), "--catalog"}
	if force {
		args = append(args, "--force")
	}
	cmd := exec.Command("node", args...)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("bridge --catalog failed: %w (stderr: %s)", err, stderr.String())
	}

	var cat CatalogResponse
	if err := json.Unmarshal(stdout.Bytes(), &cat); err != nil {
		return nil, fmt.Errorf("failed to unmarshal catalog JSON: %w", err)
	}

	return &cat, nil
}

// WriteChanges feeds the write payload to the node bridge via stdin
func WriteChanges(root string, payload *WritePayload) error {
	cmd := exec.Command("node", getBridgePath(root), "--write")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	if _, err := stdinPipe.Write(payloadBytes); err != nil {
		return err
	}
	stdinPipe.Close()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("bridge --write failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}
