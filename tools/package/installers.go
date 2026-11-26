package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func buildInstallers(stage, name, targetOS, arch, version string) ([]string, error) {
	stage, err := filepath.Abs(stage)
	if err != nil {
		return nil, err
	}
	if targetOS == "linux" {
		tool := os.Getenv("NFPM")
		if tool == "" {
			tool = "nfpm"
		}
		env := []string{"PACKAGE_STAGE=" + stage, "PACKAGE_ARCH=" + arch, "PACKAGE_VERSION=" + version}
		var outputs []string
		for _, kind := range []string{"deb", "rpm"} {
			output := filepath.Join("dist", name+"."+kind)
			if err = command(env, tool, "package", "--config", "packaging/nfpm.yaml", "--packager", kind, "--target", output); err != nil {
				return nil, err
			}
			outputs = append(outputs, output)
		}
		return outputs, nil
	}
	if err = copyFile("packaging/windows/NSIS-LICENSE.txt", filepath.Join(stage, "THIRD_PARTY_LICENSES", "NSIS", "COPYING"), 0644); err != nil {
		return nil, err
	}
	tool := os.Getenv("MAKENSIS")
	if tool == "" {
		tool = "makensis"
	}
	var files, dirs []string
	err = filepath.WalkDir(stage, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if path == filepath.Join(stage, "foliochest.exe") {
			return nil
		}
		if path == stage {
			return nil
		}
		relative, e := filepath.Rel(stage, path)
		if e != nil {
			return e
		}
		relative = strings.ReplaceAll(filepath.ToSlash(relative), "/", "\\")
		relative = strings.ReplaceAll(relative, "$", "$$")
		relative = strings.ReplaceAll(relative, "\"", "$\\\"")
		if d.IsDir() {
			dirs = append(dirs, relative)
		} else {
			files = append(files, relative)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var manifest strings.Builder
	for _, path := range files {
		fmt.Fprintf(&manifest, "Delete \"$INSTDIR\\%s\"\n", path)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, path := range dirs {
		fmt.Fprintf(&manifest, "RMDir \"$INSTDIR\\%s\"\n", path)
	}
	manifestPath := filepath.Join(filepath.Dir(stage), "uninstall-"+arch+".nsh")
	if err = os.WriteFile(manifestPath, []byte(manifest.String()), 0644); err != nil {
		return nil, err
	}
	output, err := filepath.Abs(filepath.Join("dist", name+"-setup.exe"))
	if err != nil {
		return nil, err
	}
	icon, err := filepath.Abs("packaging/assets/foliochest.ico")
	if err != nil {
		return nil, err
	}
	// -NOCD and absolute paths make the config independent of NSIS's script directory.
	if err = command(nil, tool, "-V2", "-NOCD", "-DSTAGE="+stage, "-DOUTPUT="+output, "-DICON="+icon, "-DVERSION="+version, "-DARCH="+arch, "-DUNINSTALL_MANIFEST="+manifestPath, "packaging/windows/installer.nsi"); err != nil {
		return nil, err
	}
	return []string{output}, nil
}
