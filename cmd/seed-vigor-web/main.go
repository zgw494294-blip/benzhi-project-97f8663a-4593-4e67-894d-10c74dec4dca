package main

import (
	"fmt"
	"os"
)

func main() {
	cfg, err := parseConfig(os.Args[1:], environment)
	if err == nil {
		if cfg.selfcheck {
			err = runSelfcheck(cfg)
		} else {
			err = runService(cfg)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}
}
