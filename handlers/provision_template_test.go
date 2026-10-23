package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProvisionTemplatesRenderCallbacks(t *testing.T) {
	chdirRepoRoot(t)
	h := NewBootMenuHandlers(nil)

	const (
		token  = "c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00"
		mac    = "de:ad:be:ef:00:01"
		server = "10.9.8.7"
	)
	data := map[string]string{
		"tftpip":          server,
		"mac":             mac,
		"hostname":        "cb-host",
		"ip":              "10.2.3.4",
		"subnet":          "255.255.255.0",
		"gateway":         "10.2.3.1",
		"dns":             "1.1.1.1",
		"provision_token": token,
	}
	stateURL := "http://" + server + "/dhcp/lease/" + mac + "/state"
	beatURL := "http://" + server + "/dhcp/lease/" + mac + "/heartbeat"

	cases := []struct {
		name   string
		path   string
		states []string
	}{
		{
			name:   "cloud-init",
			path:   "public/provision/templates/cloud-init/default.templ",
			states: []string{"ignite_state configuring", "ignite_state complete", "ignite_state failed"},
		},
		{
			name: "kickstart",
			path: "public/provision/templates/kickstart/default.templ",
			states: []string{
				"state=imaging",
				"ignite_state configuring",
				"ignite_state complete",
				"ignite_state failed",
				"state=failed",
			},
		},
		{
			name: "preseed",
			path: "public/provision/templates/preseed/default.templ",
			states: []string{
				"state=imaging",
				"state=configuring",
				"state=complete",
				"state=failed",
			},
		},
		{
			name: "autoyast",
			path: "public/provision/templates/autoyast/default.templ",
			states: []string{
				"state=imaging",
				"ignite_state configuring",
				"ignite_state complete",
				"ignite_state failed",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, err := os.ReadFile(tc.path)
			require.NoError(t, err)
			text := string(src)
			require.Contains(t, text, ".tftpip")
			require.Contains(t, text, ".mac")
			require.Contains(t, text, ".provision_token")
			require.Contains(t, text, "shellQuote")
			require.NotContains(t, text, server)
			require.NotContains(t, text, mac)
			require.NotContains(t, text, token)
			require.NotContains(t, text, ":8080")

			out := filepath.Join(t.TempDir(), "rendered")
			require.NoError(t, h.writeTemplateToDisk(tc.path, out, data))
			rendered, err := os.ReadFile(out)
			require.NoError(t, err)
			body := string(rendered)

			require.Contains(t, body, "Authorization: Bearer "+token)
			require.Contains(t, body, stateURL)
			require.Contains(t, body, beatURL)
			require.Contains(t, body, "-m 10")
			require.Contains(t, body, "|| true")
			require.Contains(t, body, "cb-host")
			require.NotContains(t, body, "{{")
			require.NotContains(t, body, "&lt;")
			require.NotContains(t, body, "&#")
			for _, state := range tc.states {
				require.Contains(t, body, state)
			}
			for _, line := range strings.Split(body, "\n") {
				if (strings.Contains(line, "curl ") || strings.Contains(line, "wget ")) && strings.Contains(line, "/dhcp/lease/") {
					require.Contains(t, line, "|| true", line)
				}
			}
			if tc.name == "preseed" {
				earlyAt := strings.Index(body, "preseed/early_command")
				lateAt := strings.Index(body, "preseed/late_command")
				require.Greater(t, earlyAt, 0)
				require.Greater(t, lateAt, earlyAt)
				early := body[earlyAt:lateAt]
				require.Contains(t, early, "wget")
				require.Contains(t, early, "-T 10")
				require.NotContains(t, early, "curl")
			}
		})
	}
}

// iPXE has no POST, and the pxelinux menu is not an installed-host script.
// Both still have to render after the writer switched to text/template.
func TestBootloaderTemplatesStillRender(t *testing.T) {
	chdirRepoRoot(t)
	h := NewBootMenuHandlers(nil)

	ipxeOut := filepath.Join(t.TempDir(), "ipxe")
	require.NoError(t, h.writeTemplateToDisk(
		"public/provision/templates/ipxe/default.templ",
		ipxeOut,
		map[string]string{
			"hostname":        "cb-host",
			"ip":              "10.2.3.4",
			"subnet":          "255.255.255.0",
			"gateway":         "10.2.3.1",
			"dns":             "1.1.1.1",
			"tftpip":          "10.9.8.7",
			"mac":             "de:ad:be:ef:00:01",
			"provision_token": "should-not-appear",
		},
	))
	ipxe, err := os.ReadFile(ipxeOut)
	require.NoError(t, err)
	ipxeText := string(ipxe)
	require.Contains(t, ipxeText, "cb-host")
	require.NotContains(t, ipxeText, "should-not-appear")
	require.NotContains(t, ipxeText, "/dhcp/lease/")
	require.NotContains(t, ipxeText, "{{")

	menuOut := filepath.Join(t.TempDir(), "menu")
	require.NoError(t, h.writeTemplateToDisk(
		"public/provision/templates/bootmenu/default.templ",
		menuOut,
		BootMenuData{Name: "Ubuntu", Kernel: "vmlinuz", Initrd: "initrd.img", Options: "quiet"},
	))
	menu, err := os.ReadFile(menuOut)
	require.NoError(t, err)
	menuText := string(menu)
	require.Contains(t, menuText, "KERNEL vmlinuz")
	require.Contains(t, menuText, "APPEND initrd=initrd.img quiet")
	require.NotContains(t, menuText, "/dhcp/lease/")
	require.NotContains(t, menuText, "{{")
}
