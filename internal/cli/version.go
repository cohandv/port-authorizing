package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var (
	// These are set via ldflags during build
	Version   = "1.0.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information and check for updates",
	Long:  "Display current version and check GitHub for newer releases",
	RunE:  runVersion,
}

func init() {
	versionCmd.Flags().Bool("check", false, "Check for updates from GitHub")
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func runVersion(cmd *cobra.Command, args []string) error {
	// Show current version
	fmt.Printf("Port Authorizing CLI\n")
	fmt.Printf("Version:    %s\n", Version)
	fmt.Printf("Build Time: %s\n", BuildTime)
	fmt.Printf("Git Commit: %s\n", GitCommit)
	fmt.Printf("Go Version: %s\n", runtime.Version())
	fmt.Printf("OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)

	// Check for updates if requested
	checkFlag, _ := cmd.Flags().GetBool("check")
	if checkFlag {
		fmt.Println()
		return checkForUpdates()
	}

	return nil
}

// compareVersions compares two semantic versions (e.g., "1.2.3" and "1.3.0")
// Returns:
//
//	-1 if v1 < v2
//	 0 if v1 == v2
//	 1 if v1 > v2
//	-2 if versions cannot be compared (invalid format)
func compareVersions(v1, v2 string) int {
	// Parse version strings
	parts1 := parseVersion(v1)
	parts2 := parseVersion(v2)

	// If either version is invalid, return -2
	if parts1 == nil || parts2 == nil {
		return -2
	}

	// Compare major, minor, patch
	for i := 0; i < 3; i++ {
		if parts1[i] < parts2[i] {
			return -1
		}
		if parts1[i] > parts2[i] {
			return 1
		}
	}

	return 0 // versions are equal
}

// parseVersion parses a semantic version string (e.g., "1.2.3") into [major, minor, patch]
// Returns nil if the version string is invalid
func parseVersion(version string) []int {
	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")

	// First, strip pre-release and build metadata
	// Pre-release: "-alpha.1"
	// Build metadata: "+build.123"
	// Example: "1.2.3-alpha.1+build.123" → "1.2.3"
	if idx := strings.Index(version, "-"); idx != -1 {
		version = version[:idx]
	}
	if idx := strings.Index(version, "+"); idx != -1 {
		version = version[:idx]
	}

	// Split by '.'
	parts := strings.Split(version, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return nil
	}

	// If only major.minor, add patch=0
	if len(parts) == 2 {
		parts = append(parts, "0")
	}

	result := make([]int, 3)
	for i := 0; i < 3; i++ {
		val, err := strconv.Atoi(parts[i])
		if err != nil {
			return nil
		}
		result[i] = val
	}

	return result
}

// latestReleaseWithAssets returns the first non-draft, non-prerelease release
// that has at least one asset. Releases are assumed to be in newest-first order.
func latestReleaseWithAssets(releases []githubRelease) *githubRelease {
	for i := range releases {
		r := &releases[i]
		if r.Draft || r.Prerelease {
			continue
		}
		if len(r.Assets) > 0 {
			return r
		}
	}
	return nil
}

func checkForUpdates() error {
	// Get GITHUB_TOKEN (optional for public repos, but recommended)
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fmt.Println("⚠️  GITHUB_TOKEN not set (rate limit: 60 requests/hour)")
		fmt.Println("   For higher rate limit (5000/hour), set:")
		fmt.Println("   export GITHUB_TOKEN=your_github_token")
		fmt.Println()
	}

	// Get repository URL
	repoURL := os.Getenv("GITHUB_REPO")
	if repoURL == "" {
		repoURL = "https://github.com/cohandv/port-authorizing"
	} else {
		fmt.Printf("📦 Using custom repository: %s\n", repoURL)
	}

	// Parse owner and repo from URL
	// URL format: https://github.com/owner/repo
	parts := strings.Split(strings.TrimPrefix(repoURL, "https://github.com/"), "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid GITHUB_REPO format: %s (expected: https://github.com/owner/repo)", repoURL)
	}
	owner, repo := parts[0], parts[1]

	fmt.Printf("🔍 Checking for updates from %s/%s...\n", owner, repo)

	// Get releases from GitHub API (newest first)
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=100", owner, repo)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", token))
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return fmt.Errorf("authentication failed: invalid GITHUB_TOKEN")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to fetch releases (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return fmt.Errorf("failed to parse release info: %w", err)
	}

	release := latestReleaseWithAssets(releases)
	if release == nil {
		return fmt.Errorf("no releases with assets found")
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(Version, "v")

	fmt.Printf("Latest version: %s\n", latestVersion)
	fmt.Printf("Current version: %s\n", currentVersion)
	fmt.Println()

	// Compare versions
	// If current is "dev" or "unknown", always show update available
	if currentVersion == "dev" || currentVersion == "unknown" {
		fmt.Printf("🎉 Latest release available: %s\n", latestVersion)
		fmt.Println("(You're running a development build)")
		fmt.Printf("Release page: %s\n\n", release.HTMLURL)
	} else {
		cmp := compareVersions(currentVersion, latestVersion)
		switch cmp {
		case -1:
			// Current version is older than latest
			fmt.Printf("🎉 A new version is available: %s → %s\n", currentVersion, latestVersion)
			fmt.Printf("Release page: %s\n\n", release.HTMLURL)
		case 0:
			// Versions are equal
			fmt.Println("✓ You're up to date!")
			return nil
		case 1:
			// Current version is newer than latest (probably a dev build or pre-release)
			fmt.Printf("✓ You're running a newer version than the latest release (%s > %s)\n", currentVersion, latestVersion)
			fmt.Println("(This is expected for development or pre-release builds)")
			return nil
		case -2:
			// Cannot compare versions (invalid format)
			fmt.Printf("⚠️  Cannot compare versions: '%s' and '%s'\n", currentVersion, latestVersion)
			fmt.Printf("Latest release page: %s\n", release.HTMLURL)
			return nil
		}
	}

	// Find the appropriate binary for this platform
	binaryName := fmt.Sprintf("port-authorizing-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == binaryName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		fmt.Printf("⚠️  No binary found for %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("Please download manually from: %s\n", release.HTMLURL)
		return nil
	}

	// Show download command
	fmt.Println("📥 To update, run:")
	fmt.Println()

	installDir := "$HOME/.local/bin"
	if runtime.GOOS == "windows" {
		installDir = "%USERPROFILE%\\.local\\bin"
	}

	fmt.Printf("  # Download and install to %s\n", installDir)
	if runtime.GOOS == "windows" {
		fmt.Printf("  curl -L -H \"Authorization: token $env:GITHUB_TOKEN\" -H \"Accept: application/octet-stream\" \"%s\" -o \"%s\\port-authorizing.exe\"\n",
			downloadURL, installDir)
	} else {
		fmt.Printf("  curl -fsSL -H \"Authorization: token $GITHUB_TOKEN\" -H \"Accept: application/octet-stream\" \"%s\" -o %s/port-authorizing && chmod +x %s/port-authorizing\n",
			downloadURL, installDir, installDir)
	}

	fmt.Println()
	fmt.Println("Or use the install script:")
	fmt.Printf("  curl -fsSL https://raw.githubusercontent.com/%s/%s/main/scripts/install.sh | bash\n", owner, repo)

	return nil
}

// SetVersion allows the main package to set version info
func SetVersion(version, buildTime, gitCommit string) {
	Version = version
	BuildTime = buildTime
	GitCommit = gitCommit
}
