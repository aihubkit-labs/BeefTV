package protocol

import "infinite-canvas/backend/internal/buildinfo"

// PresentMetadata resolves display-only tokens without changing protocol identity.
func PresentMetadata(metadata Metadata) Metadata {
	metadata.Name = buildinfo.DisplayText(metadata.Name)
	metadata.Description = buildinfo.DisplayText(metadata.Description)
	metadata.Documentation = buildinfo.DisplayText(metadata.Documentation)
	return metadata
}
