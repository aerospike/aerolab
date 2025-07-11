# shellcheck disable=SC2148

ENABLE_VSCODE="{{.EnableVSCode}}"
START_VSCODE="{{.StartVSCode}}"
PATCH_EXTENSIONS="{{.PatchExtensions}}"

set -e
curl -fsSL https://code-server.dev/install.sh | sh

cat <<'EOF' > "/etc/systemd/system/vscode.service"
[Unit]
Description=code-server
After=network.target

[Service]
Type=exec
Environment=PASSWORD='{{ .Password }}'
ExecStart=/usr/bin/code-server --bind-addr={{ .BindAddr }} --auth={{ .AuthType }} --disable-workspace-trust --disable-telemetry --disable-getting-started-override
Restart=always
User={{ .Username }}
Group={{ .Username }}
WorkingDirectory={{ .UserHome }}

[Install]
WantedBy=default.target
EOF
set +e

# patch extensions library paths
if [ "$PATCH_EXTENSIONS" == "true" ]; then
FILE="/usr/lib/code-server/lib/vscode/product.json"
jq '. + {
  extensionsGallery: {
    serviceUrl: "https://marketplace.visualstudio.com/_apis/public/gallery",
    itemUrl: "https://marketplace.visualstudio.com/items",
    cacheUrl: "https://marketplace.visualstudio.com",
    controlUrl: "",
    recommendationsUrl: ""
  }
}' "$FILE" > "${FILE}.tmp" && mv "${FILE}.tmp" "$FILE"
fi

mkdir -p "{{.UserHome}}/.local/share/code-server/User"
chown -R "{{.Username}}" "{{.UserHome}}/.local/share/code-server"

#{{if .OverrideDefaultFolder}}
mkdir -p "{{.OverrideDefaultFolder}}"
chown -R "{{.Username}}" "{{.OverrideDefaultFolder}}"
cat <<'EOF' > "{{.UserHome}}/.local/share/code-server/coder.json"
{
  "query": {
    "folder": "{{.OverrideDefaultFolder}}"
  }
}
EOF
#{{end}}

cat <<'EOF' > "{{.UserHome}}/.local/share/code-server/User/settings.json"
{
    "window.menuBarVisibility": "classic",
    "workbench.colorTheme": "Visual Studio Dark",
    "workbench.startupEditor": "none",
}
EOF


#{{range .RequiredExtensions}}
code-server --force --install-extension "{{.}}" || exit 1
#{{end}}

#{{range .OptionalExtensions}}
code-server --force --install-extension "{{.}}" || true
#{{end}}

systemctl daemon-reload || true
if [ "$ENABLE_VSCODE" == "true" ]; then
    systemctl enable vscode || exit 1
fi
if [ "$START_VSCODE" == "true" ]; then
    systemctl start vscode || exit 1
fi
