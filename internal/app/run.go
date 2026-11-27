// Package app runs the FolioChest local service.
package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/shenjiecode/keyfolio/internal/buildinfo"
	"github.com/shenjiecode/keyfolio/internal/platform"
	"github.com/shenjiecode/keyfolio/internal/server"
	"github.com/shenjiecode/keyfolio/internal/vault"
	"github.com/shenjiecode/keyfolio/web"
)

func Run() {
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		if err := runStdio(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		platform.ShowLaunchError(err.Error())
		os.Exit(1)
	}
}
func run() error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	// Keep the legacy default path so upgrading does not orphan device state.
	device := flag.String("device-dir", filepath.Join(configDir, "keyfolio"), "本机配置目录，不参与同步")
	root := flag.String("vault", "", "资料库目录；也可以在网页选择")
	listen := flag.String("listen", "127.0.0.1:7463", "仅限回环地址")
	background := flag.Bool("background", false, "只启动后台服务，不打开浏览器（登录自启动使用）")
	flag.Parse()
	*device, err = platform.Canonical(*device)
	if err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("只允许显式回环 IP，例如 127.0.0.1:7463")
	}
	selectedRoot, err := resolveVaultDirectory(*root, *device)
	if err != nil {
		return err
	}
	if selectedRoot != "" {
		if err = platform.SeparateDirectories(selectedRoot, *device); err != nil {
			return err
		}
	}
	owner, locked, err := acquireInstance(*device)
	if err != nil {
		return err
	}
	if !locked {
		address, err := existingInstance(*device)
		if err != nil {
			return err
		}
		if *root != "" {
			return errors.New("这个本机配置已有资料匣进程；请打开已有网页；如需变更保存位置，请使用设置中的迁移操作")
		}
		if !*background {
			return platform.OpenBrowser(address)
		}
		return nil
	}
	defer owner.close()
	*root = selectedRoot
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return fmt.Errorf("无法监听 %s；地址可能被其他程序或旧版本占用，请先关闭旧进程或指定其他 --listen 地址：%w", *listen, err)
	}
	defer listener.Close()
	baseURL := "http://" + listener.Addr().String()
	v := vault.New(*root)
	app, err := server.New(v, *device, baseURL, web.Files())
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err = owner.publish(baseURL); err != nil {
		return err
	}
	app.LauncherToken = owner.info.Token
	if err = app.PublishStdioBridge(); err != nil {
		return err
	}
	defer app.CloseStdioBridge()
	app.Shutdown = stop
	srv := &http.Server{Handler: app, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 16 << 10}
	app.StartSystemMonitor(ctx)
	app.TryAutoUnlock(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				app.SystemHeartbeat()
				app.ExpireIdle()
				app.Access.Sweep()
				if !v.Locked() {
					_ = v.Scan()
				}
			}
		}
	}()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		app.Access.Lock()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if srv.Shutdown(shutdown) != nil {
			_ = srv.Close()
		}
	}()
	if !*background {
		go func() {
			// Confirm our authenticated readiness endpoint, not merely an occupied port.
			address, e := existingInstance(*device)
			if ctx.Err() != nil {
				return
			}
			if e == nil {
				e = platform.OpenBrowser(address)
			}
			if e != nil {
				platform.ShowLaunchError("资料匣已启动，但无法自动打开浏览器。请手动访问 " + baseURL)
			}
		}()
	}
	displayRoot := *root
	if displayRoot == "" {
		displayRoot = "尚未选择资料库"
	}
	fmt.Printf("资料匣 / FolioChest %s\n网页: %s\n资料库: %s\n关闭网页不会退出后台；请使用网页中的退出按钮。\n", buildinfo.Version, baseURL, displayRoot)
	err = srv.Serve(listener)
	stop()
	<-shutdownDone
	<-workerDone
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
