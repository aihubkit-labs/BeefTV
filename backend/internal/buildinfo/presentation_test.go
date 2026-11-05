package buildinfo

import "testing"

func TestDisplayNameRefreshKeepsInstallationIdentity(t *testing.T) {
	t.Cleanup(func() { SetDisplayName("") })
	installed := ProductName
	for _, name := range []string{"ZModelTV", "测试100%TV"} {
		SetDisplayName(name)
		if got := DisplayText("请打开 {{productName}}"); got != "请打开 "+name {
			t.Fatal(got)
		}
		if ProductName != installed {
			t.Fatal("display refresh changed package identity")
		}
		if DisplayText("https://example.com/BeefTV") != "https://example.com/BeefTV" {
			t.Fatal("rewrote ordinary content")
		}
	}
	SetDisplayName("")
	if DisplayName() != installed {
		t.Fatal("local product default was lost")
	}
}
