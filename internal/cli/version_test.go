package cli

import (
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		v1   string
		v2   string
		want int
	}{
		// Equal versions
		{
			name: "equal versions - same major.minor.patch",
			v1:   "1.2.3",
			v2:   "1.2.3",
			want: 0,
		},
		{
			name: "equal versions - with v prefix",
			v1:   "v1.2.3",
			v2:   "v1.2.3",
			want: 0,
		},
		{
			name: "equal versions - mixed v prefix",
			v1:   "v1.2.3",
			v2:   "1.2.3",
			want: 0,
		},
		{
			name: "equal versions - major.minor only",
			v1:   "1.2",
			v2:   "1.2.0",
			want: 0,
		},

		// v1 < v2 (older)
		{
			name: "older - major version lower",
			v1:   "1.2.3",
			v2:   "2.0.0",
			want: -1,
		},
		{
			name: "older - minor version lower",
			v1:   "1.2.3",
			v2:   "1.3.0",
			want: -1,
		},
		{
			name: "older - patch version lower",
			v1:   "1.2.3",
			v2:   "1.2.4",
			want: -1,
		},
		{
			name: "older - multiple version differences",
			v1:   "1.21.2",
			v2:   "1.22.0",
			want: -1,
		},

		// v1 > v2 (newer)
		{
			name: "newer - major version higher",
			v1:   "2.0.0",
			v2:   "1.9.9",
			want: 1,
		},
		{
			name: "newer - minor version higher",
			v1:   "1.3.0",
			v2:   "1.2.9",
			want: 1,
		},
		{
			name: "newer - patch version higher",
			v1:   "1.2.4",
			v2:   "1.2.3",
			want: 1,
		},
		{
			name: "newer - case from bug (1.22.0 > 1.21.2)",
			v1:   "1.22.0",
			v2:   "1.21.2",
			want: 1,
		},

		// Edge cases
		{
			name: "major.minor vs major.minor.patch",
			v1:   "1.2",
			v2:   "1.2.1",
			want: -1,
		},
		{
			name: "version with pre-release tag",
			v1:   "1.2.3-beta",
			v2:   "1.2.3",
			want: 0, // We only compare numeric parts
		},
		{
			name: "zero patch version explicit",
			v1:   "1.2.0",
			v2:   "1.2",
			want: 0,
		},
		{
			name: "large version numbers",
			v1:   "99.99.99",
			v2:   "100.0.0",
			want: -1,
		},

		// Invalid versions
		{
			name: "invalid - v1 not a semantic version",
			v1:   "invalid",
			v2:   "1.2.3",
			want: -2,
		},
		{
			name: "invalid - v2 not a semantic version",
			v1:   "1.2.3",
			v2:   "invalid",
			want: -2,
		},
		{
			name: "invalid - both invalid",
			v1:   "invalid1",
			v2:   "invalid2",
			want: -2,
		},
		{
			name: "invalid - empty string v1",
			v1:   "",
			v2:   "1.2.3",
			want: -2,
		},
		{
			name: "invalid - too many parts",
			v1:   "1.2.3.4",
			v2:   "1.2.3",
			want: -2,
		},
		{
			name: "invalid - single number",
			v1:   "1",
			v2:   "1.2.3",
			want: -2,
		},
		{
			name: "invalid - non-numeric parts",
			v1:   "1.a.3",
			v2:   "1.2.3",
			want: -2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareVersions(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    []int
		wantNil bool
	}{
		// Valid versions
		{
			name:    "standard semantic version",
			version: "1.2.3",
			want:    []int{1, 2, 3},
			wantNil: false,
		},
		{
			name:    "version with v prefix",
			version: "v1.2.3",
			want:    []int{1, 2, 3},
			wantNil: false,
		},
		{
			name:    "major.minor only",
			version: "1.2",
			want:    []int{1, 2, 0},
			wantNil: false,
		},
		{
			name:    "version with pre-release tag",
			version: "1.2.3-beta",
			want:    []int{1, 2, 3},
			wantNil: false,
		},
		{
			name:    "version with pre-release and build metadata",
			version: "1.2.3-alpha.1+build.123",
			want:    []int{1, 2, 3},
			wantNil: false,
		},
		{
			name:    "zero versions",
			version: "0.0.0",
			want:    []int{0, 0, 0},
			wantNil: false,
		},
		{
			name:    "large version numbers",
			version: "99.88.77",
			want:    []int{99, 88, 77},
			wantNil: false,
		},

		// Invalid versions
		{
			name:    "empty string",
			version: "",
			want:    nil,
			wantNil: true,
		},
		{
			name:    "single number",
			version: "1",
			want:    nil,
			wantNil: true,
		},
		{
			name:    "too many parts",
			version: "1.2.3.4",
			want:    nil,
			wantNil: true,
		},
		{
			name:    "non-numeric major",
			version: "a.2.3",
			want:    nil,
			wantNil: true,
		},
		{
			name:    "non-numeric minor",
			version: "1.b.3",
			want:    nil,
			wantNil: true,
		},
		{
			name:    "non-numeric patch",
			version: "1.2.c",
			want:    nil,
			wantNil: true,
		},
		{
			name:    "invalid characters",
			version: "1.2.3!",
			want:    nil,
			wantNil: true,
		},
		{
			name:    "just text",
			version: "invalid",
			want:    nil,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVersion(tt.version)
			if tt.wantNil {
				if got != nil {
					t.Errorf("parseVersion(%q) = %v, want nil", tt.version, got)
				}
			} else {
				if got == nil {
					t.Errorf("parseVersion(%q) = nil, want %v", tt.version, tt.want)
					return
				}
				if len(got) != len(tt.want) {
					t.Errorf("parseVersion(%q) returned %v (len=%d), want %v (len=%d)",
						tt.version, got, len(got), tt.want, len(tt.want))
					return
				}
				for i := range got {
					if got[i] != tt.want[i] {
						t.Errorf("parseVersion(%q)[%d] = %d, want %d", tt.version, i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

// TestCompareVersionsSymmetry tests that version comparison is consistent
func TestCompareVersionsSymmetry(t *testing.T) {
	tests := []struct {
		v1 string
		v2 string
	}{
		{"1.2.3", "1.2.4"},
		{"1.2.3", "2.0.0"},
		{"1.22.0", "1.21.2"},
		{"v1.0.0", "v2.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			cmp1 := compareVersions(tt.v1, tt.v2)
			cmp2 := compareVersions(tt.v2, tt.v1)

			// If cmp1 says v1 < v2 (-1), then cmp2 should say v2 > v1 (1)
			// If cmp1 says v1 > v2 (1), then cmp2 should say v2 < v1 (-1)
			// If cmp1 says equal (0), then cmp2 should also say equal (0)
			// If cmp1 says invalid (-2), then cmp2 should also say invalid (-2)

			if cmp1 == 0 && cmp2 != 0 {
				t.Errorf("compareVersions not symmetric: (%q, %q)=%d but (%q, %q)=%d",
					tt.v1, tt.v2, cmp1, tt.v2, tt.v1, cmp2)
			}
			if cmp1 == -1 && cmp2 != 1 {
				t.Errorf("compareVersions not symmetric: (%q, %q)=%d but (%q, %q)=%d",
					tt.v1, tt.v2, cmp1, tt.v2, tt.v1, cmp2)
			}
			if cmp1 == 1 && cmp2 != -1 {
				t.Errorf("compareVersions not symmetric: (%q, %q)=%d but (%q, %q)=%d",
					tt.v1, tt.v2, cmp1, tt.v2, tt.v1, cmp2)
			}
			if cmp1 == -2 && cmp2 != -2 {
				t.Errorf("compareVersions not consistent for invalid: (%q, %q)=%d but (%q, %q)=%d",
					tt.v1, tt.v2, cmp1, tt.v2, tt.v1, cmp2)
			}
		})
	}
}

// BenchmarkCompareVersions benchmarks the version comparison function
func BenchmarkCompareVersions(b *testing.B) {
	for i := 0; i < b.N; i++ {
		compareVersions("1.22.0", "1.21.2")
	}
}

// BenchmarkParseVersion benchmarks the version parsing function
func BenchmarkParseVersion(b *testing.B) {
	for i := 0; i < b.N; i++ {
		parseVersion("1.22.0")
	}
}

func TestLatestReleaseWithAssets(t *testing.T) {
	assetsWithBinary := []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{{Name: "binary"}}
	releaseWithAssets := githubRelease{TagName: "v1.2.0", Assets: assetsWithBinary}
	releaseNoAssets := githubRelease{TagName: "v1.1.0", Assets: nil}
	releaseDraft := githubRelease{TagName: "v1.3.0", Draft: true, Assets: assetsWithBinary}
	releasePrerelease := githubRelease{TagName: "v1.4.0", Prerelease: true, Assets: assetsWithBinary}

	tests := []struct {
		name     string
		releases []githubRelease
		want     *githubRelease
	}{
		{
			name:     "first release has assets",
			releases: []githubRelease{releaseWithAssets, releaseNoAssets},
			want:     &releaseWithAssets,
		},
		{
			name:     "skips draft with assets",
			releases: []githubRelease{releaseDraft, releaseWithAssets},
			want:     &releaseWithAssets,
		},
		{
			name:     "skips prerelease with assets",
			releases: []githubRelease{releasePrerelease, releaseWithAssets},
			want:     &releaseWithAssets,
		},
		{
			name:     "skips release without assets",
			releases: []githubRelease{releaseNoAssets, releaseWithAssets},
			want:     &releaseWithAssets,
		},
		{
			name:     "returns nil when no release has assets",
			releases: []githubRelease{releaseNoAssets},
			want:     nil,
		},
		{
			name:     "returns nil for empty list",
			releases: []githubRelease{},
			want:     nil,
		},
		{
			name:     "returns nil when all are draft or prerelease",
			releases: []githubRelease{releaseDraft, releasePrerelease},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := latestReleaseWithAssets(tt.releases)
			if tt.want == nil {
				if got != nil {
					t.Errorf("latestReleaseWithAssets() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("latestReleaseWithAssets() = nil, want %s", tt.want.TagName)
				return
			}
			if got.TagName != tt.want.TagName {
				t.Errorf("latestReleaseWithAssets() = %s, want %s", got.TagName, tt.want.TagName)
			}
		})
	}
}

