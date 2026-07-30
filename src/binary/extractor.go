package binary

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Extract 根据归档类型解压文件到指定目录
func Extract(srcFile, destDir string, archiveType ArchiveType) error {
	switch archiveType {
	case ArchiveTypeTarGZ:
		return extractTarGZ(srcFile, destDir)
	case ArchiveTypeTarXZ:
		return extractTarXZ(srcFile, destDir)
	case ArchiveTypeZip:
		return extractZip(srcFile, destDir)
	default:
		return fmt.Errorf("unsupported archive type: %s", archiveType)
	}
}

func extractTarGZ(srcFile, destDir string) error {
	file, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzr.Close()

	return extractTar(gzr, destDir)
}

// extractTarXZ 解压 tar.xz 文件
// Go 标准库无 xz 支持，优先使用系统 tar 命令（macOS/Linux 自带）
func extractTarXZ(srcFile, destDir string) error {
	// 优先使用系统 tar 命令
	if runtime.GOOS != "windows" {
		if _, err := exec.LookPath("tar"); err == nil {
			cmd := exec.Command("tar", "-xJf", srcFile, "-C", destDir)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("tar -xJf failed: %w\n%s", err, string(out))
			}
			return nil
		}
	}
	return fmt.Errorf("tar.xz extraction requires the 'tar' command with xz support")
}

// extractTar 从 tar.Reader 解压到目标目录（供 tar.gz/tar.xz 共用）
func extractTar(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		target := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid tar entry path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create dir %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create parent dir: %w", err)
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("create file %s: %w", target, err)
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return fmt.Errorf("write file %s: %w", target, err)
			}
			outFile.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create parent dir for symlink: %w", err)
			}
			_ = os.Symlink(header.Linkname, target)
		}
	}
	return nil
}

func extractZip(srcFile, destDir string) error {
	r, err := zip.OpenReader(srcFile)
	if err != nil {
		return fmt.Errorf("open zip file: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid zip entry path: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create dir %s: %w", target, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create parent dir: %w", err)
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %s: %w", f.Name, err)
		}
		outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return fmt.Errorf("create file %s: %w", target, err)
		}
		if _, err := io.Copy(outFile, rc); err != nil {
			outFile.Close()
			rc.Close()
			return fmt.Errorf("write file %s: %w", target, err)
		}
		outFile.Close()
		rc.Close()
	}
	return nil
}
