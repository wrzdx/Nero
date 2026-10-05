package web_views

// AssetURL changes when the built file changes, so a previous release's cache
// cannot supply CSS or JavaScript for the new HTML.
func AssetURL(path string) string {
	if version := assetVersions[path]; version != "" {
		return path + "?v=" + version
	}
	return path
}
