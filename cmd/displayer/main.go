// displayer 是 arch-lens-dep-displayer 的唯一命令入口（命令名 arch-lens-dep-displayer）。
// 启动即托管前端 + REST API；数据源（源码目录）由用户在页面上指定并触发分析。
// 分析步骤链（scan→analyze→export→ingest→preprocess→activate）由 internal/pipeline 抽象，
// 多数据源管理由 internal/registry 负责，API 只面向当前激活数据源。
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/CodMac/arch-lens-dep-displayer/internal/api"
	"github.com/CodMac/arch-lens-dep-displayer/internal/registry"
)

const (
	defaultRootDir  = "./.arch_lens/sources" // 多数据源根目录
	defaultFrontDir = "./front"
)

type config struct {
	rootDir     string
	listen      string
	openBrowser bool
}

func main() {
	cfg := parseFlags()

	// 1. 多数据源管理器（扫描已有源）。
	mgr, err := registry.New(cfg.rootDir)
	if err != nil {
		log.Fatalf("init registry: %v", err)
	}
	defer mgr.Close()

	// 2. 前端托管：确保 dist 存在（首次 build）。
	if err := ensureFront(); err != nil {
		log.Printf("警告: 前端构建失败（%v），仅提供 API", err)
	}

	// 3. 启动 HTTP（API + 静态 + SPA fallback）。
	mux := http.NewServeMux()
	mux.Handle("/api/", api.New(mgr).Routes())
	if err := mountFront(mux); err == nil {
		log.Printf("前端托管 %s", defaultFrontDir)
	} else {
		log.Printf("警告: 前端未托管（%v）", err)
	}

	log.Printf("arch-lens-dep-displayer 监听 %s", cfg.listen)
	if cfg.openBrowser {
		go openBrowser("http://localhost" + cfg.listen)
	}
	if err := http.ListenAndServe(cfg.listen, mux); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func parseFlags() config {
	var c config
	flag.StringVar(&c.rootDir, "out", defaultRootDir, "数据源根目录（多源 workspace 父目录）")
	flag.StringVar(&c.listen, "listen", ":8080", "HTTP 监听地址")
	flag.BoolVar(&c.openBrowser, "browser", true, "启动后自动打开浏览器")
	flag.Parse()
	return c
}

// ensureFront 确保 front/dist 存在；缺失则先 npm install 再 npm run build。
func ensureFront() error {
	distDir := filepath.Join(defaultFrontDir, "dist")
	if fileExists(filepath.Join(distDir, "index.html")) {
		return nil
	}
	// 依赖缺失时（node_modules 不存在）先安装，否则 vue-tsc 等命令不可用。
	if !fileExists(filepath.Join(defaultFrontDir, "node_modules", ".package-lock.json")) {
		log.Printf("前端依赖缺失，运行 npm install ...")
		cmd := exec.Command("npm", "install")
		cmd.Dir = defaultFrontDir
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	log.Printf("前端未构建，运行 npm run build ...")
	cmd := exec.Command("npm", "run", "build")
	cmd.Dir = defaultFrontDir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// mountFront 挂载前端静态资源 + SPA fallback（非 /api 路径回退 index.html）。
func mountFront(mux *http.ServeMux) error {
	distDir := filepath.Join(defaultFrontDir, "dist")
	if !fileExists(filepath.Join(distDir, "index.html")) {
		return os.ErrNotExist
	}
	fs := http.FileServer(http.Dir(distDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
			http.NotFound(w, r)
			return
		}
		p := filepath.Join(distDir, filepath.Clean(r.URL.Path))
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		// SPA fallback。
		http.ServeFile(w, r, filepath.Join(distDir, "index.html"))
	})
	return nil
}

func openBrowser(url string) {
	time.Sleep(500 * time.Millisecond)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
