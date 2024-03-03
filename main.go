package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/apepenkov/yalog"
	"os"
	"procsman_backend/api"
	"procsman_backend/config"
	"procsman_backend/procsmanager"
)

func main() {
	f, err := os.Open("config.json")
	if err != nil {
		panic(err)
	}
	logger := yalog.NewLogger("procsman", yalog.WithPrintTime("2006-01-02 15:04:05"), yalog.WithPrintCaller(20), yalog.WithPrintLevel(), yalog.WithColorEnabled(), yalog.WithPrintTreeName(1, true), yalog.WithVerboseLevel(yalog.VerboseLevelDebug), yalog.WithAnotherColor(yalog.VerboseLevelDebug, yalog.ColorCyan))
	logger.Debugln("Starting procsman")
	defer f.Close()

	var serveAddr string
	var allowOrigin string
	flag.StringVar(&serveAddr, "serve", "127.0.0.1:54580", "Address to serve the HTTP API on")
	flag.StringVar(&allowOrigin, "allow-origin", "*", "Allow origin for CORS")
	flag.Parse()

	var cfg config.Config
	err = json.NewDecoder(f).Decode(&cfg)
	if err != nil {
		logger.Errorln(fmt.Sprintf("Error decoding config: %s", err.Error()))
		panic(err)
	}
	if err = cfg.Validate(); err != nil {
		logger.Errorln(fmt.Sprintf("Error validating config: %s", err.Error()))
		panic(err)
	}

	serv, err := procsmanager.NewProcessManager(cfg, logger)
	if err != nil {
		logger.Errorln(fmt.Sprintf("Error creating process manager: %s", err.Error()))
		panic(err)
	}

	defer serv.Close()
	httpServ := api.NewHttpServer(serv, serveAddr, allowOrigin)
	httpServ.Logger.SetVerboseLevel(yalog.VerboseLevelInfo)

	if err = httpServ.ListenAndServe(); err != nil {
		logger.Errorln(fmt.Sprintf("Error in HTTP server: %s", err.Error()))
		panic(err)
	}

	defer httpServ.Close()

}
