package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/Mellanox/spectrum-x-operator/internal/railcni"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"
)

func main() {
	logfile, err := os.OpenFile("/var/log/railcni.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}
	defer logfile.Close()

	logger := slog.New(slog.NewJSONHandler(logfile, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	railcni := &railcni.RailCNI{
		Log: logger,
	}

	skel.PluginMainFuncs(skel.CNIFuncs{
		Add:   railcni.Add,
		Check: railcni.Check,
		Del:   railcni.Del,
	}, version.All, "rail-cni")
}
