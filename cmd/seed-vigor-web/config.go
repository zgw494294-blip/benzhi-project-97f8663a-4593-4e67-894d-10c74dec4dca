package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const defaultAddr = "127.0.0.1:19081"

type config struct {
	addr      string
	database  string
	staticDir string
	selfcheck bool
}

func parseConfig(args []string, getenv func(string) string) (config, error) {
	set := flag.NewFlagSet("seed-vigor-web", flag.ContinueOnError)
	addr := set.String("addr", "", "HTTP 监听地址，例如 127.0.0.1:19081")
	database := set.String("db", "seed-vigor.db", "SQLite 数据库文件")
	staticDir := set.String("static-dir", "web/static", "前端静态文件目录")
	selfcheck := set.Bool("selfcheck", false, "运行真实 HTTP 全流程自检后退出")
	if err := set.Parse(args); err != nil {
		return config{}, err
	}
	if set.NArg() != 0 {
		return config{}, fmt.Errorf("存在无法识别的位置参数: %s", strings.Join(set.Args(), " "))
	}
	resolved, err := resolveAddress(*addr, getenv("PORT"))
	if err != nil {
		return config{}, err
	}
	if strings.TrimSpace(*database) == "" {
		return config{}, fmt.Errorf("-db 不得为空")
	}
	if strings.TrimSpace(*staticDir) == "" {
		return config{}, fmt.Errorf("-static-dir 不得为空")
	}
	return config{addr: resolved, database: *database, staticDir: *staticDir, selfcheck: *selfcheck}, nil
}

func resolveAddress(explicit, portValue string) (string, error) {
	if explicit != "" {
		if _, _, err := net.SplitHostPort(explicit); err != nil {
			return "", fmt.Errorf("-addr 无效: %w", err)
		}
		return explicit, nil
	}
	if portValue == "" {
		return defaultAddr, nil
	}
	port, err := strconv.Atoi(portValue)
	if err != nil || port < 1 || port > 65535 || strconv.Itoa(port) != portValue {
		return "", fmt.Errorf("PORT 必须是 1 到 65535 的十进制端口号")
	}
	return net.JoinHostPort("127.0.0.1", portValue), nil
}

func environment(name string) string { return os.Getenv(name) }
