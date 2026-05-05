package getter

import (
	"net/url"
	"path"
	"strings"

	getter "github.com/hashicorp/go-getter/v2"
	urlhelper "github.com/hashicorp/go-getter/v2/helper/url"
)

// URLParse wraps go-getter/v2/helper/url.Parse, which delegates to net/url.Parse
// but returns Windows-safe URLs on Windows.
func URLParse(rawURL string) (*url.URL, error) {
	return urlhelper.Parse(rawURL)
}

// SourceDirSubdir splits a source URL on "//", returning the base URL and the
// optional subdirectory selector.
func SourceDirSubdir(src string) (string, string) {
	return getter.SourceDirSubdir(src)
}

// SubdirGlob expands a globbed subdirectory path against a directory on disk
// and returns the resolved subdirectory path.
func SubdirGlob(dst, subDir string) (string, error) {
	return getter.SubdirGlob(dst, subDir)
}

// Detect runs the default detector chain (the equivalent of v1's
// getter.Detect(src, pwd, getter.Detectors)) and returns the canonicalized
// source. Sources that already carry a valid URL scheme are returned
// unchanged, matching v1's "Valid URL" early-return.
func Detect(source, pwd string) (string, error) {
	return DetectWith(source, pwd, defaultDetectors())
}

// DetectWith is Detect with a caller-supplied detector chain. internal/tf
// uses this to deliberately exclude FileDetector + S3Detector from the
// http-scheme stripping path in normalizeSourceURL.
//
// The procedure mirrors v1's package-level getter.Detect:
//
//  1. Strip any "<getter>::" forced prefix and the "//subdir" selector.
//  2. If the remaining string already parses as a URL with a non-empty
//     scheme, return the original source unchanged. This avoids the
//     S3/GCS detectors mangling already-resolved https:// URLs.
//  3. Otherwise run the detector chain; the first detector that recognizes
//     the source wins, and any detected forced-getter / subdir selector is
//     reattached.
func DetectWith(source, pwd string, detectors []Detector) (string, error) {
	getForce, getSrc := splitForcedGetter(source)
	getSrc, subDir := SourceDirSubdir(getSrc)

	if u, err := URLParse(getSrc); err == nil && u.Scheme != "" {
		return source, nil
	}

	for _, d := range detectors {
		result, ok, err := d.Detect(getSrc, pwd)
		if err != nil {
			return source, err
		}

		if !ok {
			continue
		}

		detectForce, result := splitForcedGetter(result)
		result, detectSubdir := SourceDirSubdir(result)

		if detectSubdir != "" {
			subDir = path.Join(detectSubdir, subDir)
		}

		if subDir != "" {
			u, err := URLParse(result)
			if err != nil {
				return source, err
			}

			u.Path += "//" + subDir
			result = u.String()
		}

		if getForce == "" {
			getForce = detectForce
		}

		if getForce != "" {
			result = getForce + "::" + result
		}

		return result, nil
	}

	return source, nil
}

// prefixedDetector decorates a Detector so its successful matches are
// wrapped with a "<prefix>::" forced-getter marker, matching v1 detector
// output. Used for the GCS and S3 detectors since v2 dropped their inline
// prefixing in favor of Request.Forced.
type prefixedDetector struct {
	Detector
	prefix string
}

func (d prefixedDetector) Detect(src, pwd string) (string, bool, error) {
	out, ok, err := d.Detector.Detect(src, pwd)
	if !ok || err != nil {
		return out, ok, err
	}

	return d.prefix + "::" + out, true, nil
}

// fileSchemeDetector decorates a FileDetector so its successful matches are
// returned with a "file://" scheme prefix, matching v1 FileDetector output.
// v2 returns the raw filesystem path; downstream parseSourceURL needs the
// scheme so IsLocalSource can recognize it.
type fileSchemeDetector struct {
	Detector
}

func (d fileSchemeDetector) Detect(src, pwd string) (string, bool, error) {
	out, ok, err := d.Detector.Detect(src, pwd)
	if !ok || err != nil {
		return out, ok, err
	}

	if len(out) > 0 && out[0] == '/' {
		return "file://" + out, true, nil
	}

	return "file:///" + out, true, nil
}

// defaultDetectors is the detector chain Detect runs against by default.
// It mirrors the v1 default (GitHub, GitLab, Git, BitBucket, GCS, S3, File)
// so callers preserve the canonicalization they had before the v1 to v2
// migration. The GCS, S3, and File detectors are wrapped so their output
// retains the v1 textual conventions (forced-getter "gcs::"/"s3::" prefix
// and "file://" scheme respectively); v2 dropped those because the
// forced-getter signal moved onto Request.Forced.
func defaultDetectors() []Detector {
	return []Detector{
		new(GitHubDetector),
		new(GitDetector),
		new(BitBucketDetector),
		new(GitLabDetector),
		prefixedDetector{Detector: new(GCSDetector), prefix: "gcs"},
		prefixedDetector{Detector: new(S3Detector), prefix: "s3"},
		fileSchemeDetector{Detector: new(FileDetector)},
	}
}

// splitForcedGetter splits "<getter>::<rest>" into its prefix and remainder.
// It returns ("", src) when src has no forced-getter prefix, matching the
// behavior of go-getter v1's getForcedGetter.
func splitForcedGetter(src string) (string, string) {
	prefix, rest, ok := strings.Cut(src, "::")
	if !ok || prefix == "" || !isASCIIAlnum(prefix) {
		return "", src
	}

	return prefix, rest
}

// isASCIIAlnum reports whether s consists entirely of ASCII letters and digits.
//
// The unicode.IsLetter and unicode.IsDigit helpers are not used here because
// go-getter v1's getForcedGetter accepts only [A-Za-z0-9]. Accepting non-ASCII
// letters would widen the set of strings treated as forced-getter prefixes.
func isASCIIAlnum(s string) bool {
	for i := range len(s) {
		c := s[i]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}

	return true
}
