package grafanafix

import (
	"context"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func InstallImageRenderer() {
	if _, err := os.Stat("/opt/grafana-image-renderer.txt"); err == nil {
		return
	}
	log.Println("Installing image renderer")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "grafana-cli", "plugins", "install", "grafana-image-renderer").CombinedOutput()
	if err != nil {
		log.Printf("ERROR: Failed to install image renderer: %s\n%s", err, string(out))
		return
	}
	log.Println("Image renderer installed, launching apt for dependencies in the background")
	go func() {
		out, err := exec.Command("apt", "-y", "install", "libx11-6", "libx11-xcb1", "libxcomposite1", "libxcursor1", "libxdamage1", "libxext6", "libxfixes3", "libxi6", "libxrender1", "libxtst6", "libglib2.0-0", "libnss3", "libcups2", "", "libdbus-1-3", "libxss1", "libxrandr2", "libgtk-3-0", "libasound2", "libxcb-dri3-0", "libgbm1", "libxshmfence1").CombinedOutput()
		if err != nil {
			log.Printf("ERROR: Failed to install deps for the image renderer, it will not work: %s\n%s", err, string(out))
			return
		}
		os.WriteFile("/opt/grafana-image-renderer.txt", []byte(strconv.Itoa(int(time.Now().Unix()))), 0644)
		log.Println("Image renderer dependencies installed")
	}()
}
