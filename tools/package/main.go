// Command package builds portable distributions. It never starts the application.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/shenjiecode/keyfolio/internal/buildinfo"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func command(env []string, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Env = append(os.Environ(), env...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
func copyFile(src, dst string, mode fs.FileMode) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, raw, mode)
}
func licenseName(name string) bool {
	n := strings.ToLower(name)
	return n == "license" || n == "copying" || n == "notice" || strings.HasPrefix(n, "license.") || strings.HasPrefix(n, "license-") || strings.HasPrefix(n, "copying.") || strings.HasPrefix(n, "notice.")
}
func licenses(destination string) error {
	if err := copyFile(filepath.Join(runtime.GOROOT(), "LICENSE"), filepath.Join(destination, "Go", "LICENSE"), 0644); err != nil {
		return err
	}
	// Include build-time licenses as well, rather than omitting transitive notices.
	c := exec.Command("go", "list", "-m", "-json", "all")
	raw, err := c.Output()
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	for {
		var m struct {
			Path, Version, Dir string
			Main               bool
		}
		err = decoder.Decode(&m)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if m.Main || m.Dir == "" {
			continue
		}
		entries, e := os.ReadDir(m.Dir)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if !entry.IsDir() && licenseName(entry.Name()) {
				if e = copyFile(filepath.Join(m.Dir, entry.Name()), filepath.Join(destination, "go", filepath.FromSlash(m.Path+"@"+m.Version), entry.Name()), 0644); e != nil {
					return e
				}
			}
		}
	}
	return filepath.WalkDir("web/node_modules", func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() || !licenseName(d.Name()) {
			return nil
		}
		relative, e := filepath.Rel("web/node_modules", path)
		if e != nil {
			return e
		}
		return copyFile(path, filepath.Join(destination, "npm", relative), 0644)
	})
}
func archive(dir, output string, windows bool) error {
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer file.Close()
	if windows {
		writer := zip.NewWriter(file)
		err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			info, e := d.Info()
			if e != nil {
				return e
			}
			header, e := zip.FileInfoHeader(info)
			if e != nil {
				return e
			}
			relative, _ := filepath.Rel(filepath.Dir(dir), path)
			header.Name = filepath.ToSlash(relative)
			header.Method = zip.Deflate
			target, e := writer.CreateHeader(header)
			if e != nil {
				return e
			}
			source, e := os.Open(path)
			if e != nil {
				return e
			}
			defer source.Close()
			_, e = io.Copy(target, source)
			return e
		})
		closeErr := writer.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	gz := gzip.NewWriter(file)
	writer := tar.NewWriter(gz)
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		header, e := tar.FileInfoHeader(info, "")
		if e != nil {
			return e
		}
		relative, _ := filepath.Rel(filepath.Dir(dir), path)
		header.Name = filepath.ToSlash(relative)
		header.Uid = 0
		header.Gid = 0
		if e = writer.WriteHeader(header); e != nil || d.IsDir() {
			return e
		}
		source, e := os.Open(path)
		if e != nil {
			return e
		}
		defer source.Close()
		_, e = io.Copy(writer, source)
		return e
	})
	closeErr := writer.Close()
	gzipErr := gz.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return gzipErr
}
func run() error {
	version := flag.String("version", buildinfo.Version, "package and application version")
	withInstallers := flag.Bool("installers", false, "also build NSIS Windows installers and nFPM DEB/RPM packages")
	windowsX86 := flag.Bool("windows-x86", false, "include experimental Windows 32-bit packages")
	flag.Parse()
	if !regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]*$`).MatchString(*version) {
		return fmt.Errorf("invalid version")
	}
	if _, err := os.Stat("web/dist/index.html"); err != nil {
		return fmt.Errorf("先运行 npm --prefix web ci && npm --prefix web run build")
	}
	if err := os.MkdirAll("dist", 0755); err != nil {
		return err
	}
	work, err := os.MkdirTemp("dist", ".package-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	notices := filepath.Join(work, "licenses")
	if err = licenses(notices); err != nil {
		return err
	}
	checksum := strings.Builder{}
	targets := []struct{ os, arch string }{{"windows", "amd64"}, {"linux", "amd64"}, {"linux", "arm64"}}
	if *windowsX86 {
		targets = append(targets, struct{ os, arch string }{"windows", "386"})
	}
	for _, target := range targets {
		name := "foliochest-" + *version + "-" + target.os + "-" + target.arch
		dir := filepath.Join(work, name)
		if err = os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		binary := "foliochest"
		ldflags := "-s -w -X github.com/shenjiecode/keyfolio/internal/buildinfo.Version=" + *version
		resource := "cmd/foliochest/resource_windows_" + target.arch + ".syso"
		if target.os == "windows" {
			if _, e := os.Lstat(resource); e == nil {
				return fmt.Errorf("refusing to overwrite %s", resource)
			} else if !os.IsNotExist(e) {
				return e
			}
			if err = command(nil, "go", "run", "github.com/akavel/rsrc", "-arch", target.arch, "-ico", "packaging/assets/foliochest.ico", "-o", resource); err != nil {
				_ = os.Remove(resource)
				return err
			}
			defer os.Remove(resource)
			binary += ".exe"
			ldflags += " -H=windowsgui"
		}
		err = command([]string{"CGO_ENABLED=0", "GOOS=" + target.os, "GOARCH=" + target.arch}, "go", "build", "-trimpath", "-ldflags", ldflags, "-o", filepath.Join(dir, binary), "./cmd/foliochest")
		if target.os == "windows" {
			_ = os.Remove(resource)
		}
		if err != nil {
			return err
		}
		for _, pair := range [][2]string{{"LICENSE", "LICENSE"}, {"packaging/README.txt", "README.txt"}, {"packaging/INSTALL.txt", "INSTALL.txt"}, {"web/src/components/ui/NOTICE.md", "SHADCN_NOTICE.md"}, {"packaging/assets/foliochest.png", "foliochest.png"}} {
			if err = copyFile(pair[0], filepath.Join(dir, pair[1]), 0644); err != nil {
				return err
			}
		}
		if target.os == "linux" {
			if err = copyFile("packaging/linux/install-shortcut.sh", filepath.Join(dir, "install-shortcut.sh"), 0755); err != nil {
				return err
			}
		}
		if err = filepath.WalkDir(notices, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			relative, _ := filepath.Rel(notices, path)
			return copyFile(path, filepath.Join(dir, "THIRD_PARTY_LICENSES", relative), 0644)
		}); err != nil {
			return err
		}
		ext := ".tar.gz"
		if target.os == "windows" {
			ext = ".zip"
		}
		output := filepath.Join("dist", name+ext)
		if err = archive(dir, output, target.os == "windows"); err != nil {
			return err
		}
		outputs := []string{output}
		if *withInstallers {
			additional, e := buildInstallers(dir, name, target.os, target.arch, *version)
			if e != nil {
				return e
			}
			outputs = append(outputs, additional...)
		}
		for _, output := range outputs {
			f, e := os.Open(output)
			if e != nil {
				return e
			}
			hash := sha256.New()
			_, e = io.Copy(hash, f)
			f.Close()
			if e != nil {
				return e
			}
			fmt.Fprintf(&checksum, "%s  %s\n", hex.EncodeToString(hash.Sum(nil)), filepath.Base(output))
			fmt.Println("Packaged:", output)
		}
	}
	return os.WriteFile(filepath.Join("dist", "SHA256SUMS-"+*version+".txt"), []byte(checksum.String()), 0644)
}
