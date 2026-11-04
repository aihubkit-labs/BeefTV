package desktopstorage

import (
	"os"
	"path/filepath"
	"testing"

	"infinite-canvas/backend/internal/buildinfo"
)

func TestProductRootIsStableAcrossDisplayNameChanges(t *testing.T) {
	oldName, oldDirectory := buildinfo.ProductName, buildinfo.DataDirectory
	t.Cleanup(func() { buildinfo.ProductName, buildinfo.DataDirectory = oldName, oldDirectory })
	buildinfo.DataDirectory, buildinfo.ProductName = "fixtureTV", "客户平台TV"
	root, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	config, _ := os.UserConfigDir()
	if root != filepath.Join(config, "fixtureTV") {
		t.Fatal(root)
	}
	buildinfo.ProductName = "另一个展示名TV"
	after, err := DefaultRoot()
	if err != nil || after != root {
		t.Fatal("display name moved the workspace", after, err)
	}
	buildinfo.DataDirectory = "secondTV"
	other, err := DefaultRoot()
	if err != nil || other == root {
		t.Fatal("brands share a workspace", other, err)
	}
}
