package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/shenjiecode/keyfolio/internal/server"
	"github.com/shenjiecode/keyfolio/internal/vault"
	"github.com/shenjiecode/keyfolio/web"
)

func main() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	device := flag.String("device-dir", filepath.Join(configDir, "keyfolio"), "本机配置目录，不参与同步")
	root := flag.String("vault", "", "资料库目录；也可以在网页选择")
	listen := flag.String("listen", "127.0.0.1:7463", "仅限回环地址")
	flag.Parse()
	host, _, err := net.SplitHostPort(*listen)
	if err != nil {
		log.Fatal(err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		log.Fatal("只允许显式回环 IP，例如 127.0.0.1:7463")
	}
	if *root == "" {
		if b, e := os.ReadFile(filepath.Join(*device, "location.json")); e == nil {
			var location struct{ Directory string }
			if json.Unmarshal(b, &location) == nil {
				*root = location.Directory
			}
		}
		if *root == "" {
			*root = filepath.Join(*device, "vault")
		}
	}
	*root, err = filepath.Abs(*root)
	if err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	baseURL := "http://" + listener.Addr().String()
	v := vault.New(*root)
	app, err := server.New(v, *device, baseURL, web.Files())
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Handler: app, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				app.ExpireIdle()
				app.Access.Sweep()
				if !v.Locked() {
					_ = v.Scan()
				}
			}
		}
	}()
	go func() {
		<-ctx.Done()
		app.Access.Lock()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	fmt.Printf("Keyfolio 0.2.0-dev\n网页: %s\n资料库: %s\n当前为开发预览，请仅使用合成测试数据。\n", baseURL, *root)
	if err = srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
