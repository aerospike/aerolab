package jfrog

import (
	"fmt"
	"time"
)

// File is a single artifact resolved from a JFrog build.
type File struct {
	Repo        string
	Path        string
	Name        string
	Size        int64
	SHA1        string
	Created     time.Time
	DownloadURL string
	Parts       *NameParts // nil for signatures, source archives, etc.
}

// Files is a list of File with picker helpers.
type Files []File

// MatchCriteria describes the install target we want a package for.
//
// OSName  : "amazon" | "centos" | "debian" | "ubuntu" (post-translation)
// OSVersion: e.g. "2023", "9", "12", "24.04"
// Arch    : "x86_64" | "aarch64" (after debArch normalisation)
// Edition : "community" | "enterprise" | "federal"
type MatchCriteria struct {
	OSName    string
	OSVersion string
	Arch      string
	Edition   string
}

// Match returns the single File matching the criteria. The choice of
// package format (rpm vs deb) is implied by OSName: amazon/centos use
// rpm, debian/ubuntu use deb.
func (fs Files) Match(c MatchCriteria) (*File, error) {
	wantFormat := formatForOS(c.OSName)
	if wantFormat == "" {
		return nil, fmt.Errorf("jfrog: unsupported OS %q (only amazon/centos/debian/ubuntu have JFrog packages)", c.OSName)
	}

	var seen []string
	for i := range fs {
		f := &fs[i]
		if f.Parts == nil {
			continue
		}
		if f.Parts.Format != wantFormat {
			continue
		}
		if f.Parts.Edition != c.Edition {
			continue
		}
		if f.Parts.OSName != c.OSName {
			continue
		}
		if f.Parts.OSVersion != c.OSVersion {
			continue
		}
		if f.Parts.Arch != c.Arch {
			continue
		}
		return f, nil
	}

	// build a helpful "what we did see" message for the user
	for _, f := range fs {
		if f.Parts != nil && f.Parts.Edition == c.Edition && f.Parts.Format == wantFormat {
			seen = append(seen, fmt.Sprintf("%s/%s/%s",
				f.Parts.OSName+f.Parts.OSVersion, f.Parts.Arch, f.Name))
		}
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("jfrog: no %s %s package found for %s/%s/%s",
			c.Edition, wantFormat, c.OSName, c.OSVersion, c.Arch)
	}
	return nil, fmt.Errorf(
		"jfrog: no %s %s package matches %s %s %s; available %s candidates: %v",
		c.Edition, wantFormat, c.OSName, c.OSVersion, c.Arch, c.Edition, seen)
}

// formatForOS returns the package format JFrog publishes for a given OS.
func formatForOS(osName string) string {
	switch osName {
	case "amazon", "centos", "rocky":
		return "rpm"
	case "debian", "ubuntu":
		return "deb"
	}
	return ""
}
