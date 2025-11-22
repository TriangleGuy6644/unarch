package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: unarch <archive_file> [destination].")
		os.Exit(1)
	}

	archivePath := os.Args[1]
	destDir := "."
	if len(os.Args) >= 3 {
		destDir = os.Args[2]
	}

	archiveType, err := detectArchiveType(archivePath)
	if err != nil {
		fmt.Println("error detecting archive type:", err)
		os.Exit(1)
	}

	switch archiveType {
	case "zip":
		err = unzip(archivePath, destDir)
	case "tar", "tar.gz", "tar.bz2", "tar.xz", "tar.lz", "tar.lzma", "tar.zst":
		err = untar(archivePath, destDir)
	case "gz", "bz2", "xz", "lz", "lzma", "zst":
		err = extractSingleFile(archivePath, destDir, archiveType)
	case "7z", "rar":
		err = extractWith7z(archivePath, destDir)
	default:
		fmt.Println("unsupported archive type.")
		os.Exit(1)
	}

	if err != nil {
		fmt.Println("extraction error:", err)
		os.Exit(1)
	}
	fmt.Println("extraction complete.")
}

func detectArchiveType(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := make([]byte, 16)
	_, err = f.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}

	switch {
	case bytes.HasPrefix(buf, []byte("PK")):
		return "zip", nil
	case bytes.HasPrefix(buf, []byte{0x1F, 0x8B}):
		return "tar.gz", nil
	case bytes.HasPrefix(buf, []byte{0x42, 0x5A, 0x68}):
		return "tar.bz2", nil
	case bytes.HasPrefix(buf, []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}):
		return "tar.xz", nil
	case bytes.HasPrefix(buf, []byte{0x52, 0x61, 0x72, 0x21}):
		return "rar", nil
	case bytes.HasPrefix(buf, []byte{0x37, 0x7A, 0xBC, 0xAF, 0x27, 0x1C}):
		return "7z", nil
	case bytes.HasPrefix(buf, []byte{0x28, 0xB5, 0x2F, 0xFD}):
		return "tar.zst", nil
	default:
		ext := filepath.Ext(filePath)
		switch ext {
		case ".gz":
			return "gz", nil
		case ".bz2":
			return "bz2", nil
		case ".xz":
			return "xz", nil
		case ".lz":
			return "lz", nil
		case ".lzma":
			return "lzma", nil
		case ".zst":
			return "zst", nil
		case ".tar":
			return "tar", nil
		case ".tar.lz":
			return "tar.lz", nil
		case ".tar.lzma":
			return "tar.lzma", nil
		default:
			return "", fmt.Errorf("unknown archive type")
		}
	}
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func untar(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 6)
	_, _ = f.Read(buf)
	f.Seek(0, io.SeekStart)
	var fileReader io.Reader = f

	switch {
	case bytes.HasPrefix(buf, []byte{0x1F, 0x8B}):
		gr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gr.Close()
		fileReader = gr
	case bytes.HasPrefix(buf, []byte{0x42, 0x5A, 0x68}):
		fileReader = bzip2.NewReader(f)
	case bytes.HasPrefix(buf, []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}):
		xzr, err := xz.NewReader(f)
		if err != nil {
			return err
		}
		fileReader = xzr
	case bytes.HasPrefix(buf, []byte{0x28, 0xB5, 0x2F, 0xFD}):
		zr, err := zstd.NewReader(f)
		if err != nil {
			return err
		}
		defer zr.Close()
		fileReader = zr
	}

	tr := tar.NewReader(fileReader)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, os.ModePerm)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), os.ModePerm)
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			_, err = io.Copy(outFile, tr)
			outFile.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func extractSingleFile(src, dest, typ string) error {
	var cmd *exec.Cmd
	switch typ {
	case "gz":
		cmd = exec.Command("gunzip", "-k", "-c", src)
	case "bz2":
		cmd = exec.Command("bunzip2", "-k", "-c", src)
	case "xz":
		cmd = exec.Command("unxz", "-k", "-c", src)
	case "zst":
		cmd = exec.Command("unzstd", "-k", "-c", src)
	default:
		return fmt.Errorf("unsupported single file compression type")
	}
	outPath := filepath.Join(dest, filepath.Base(src))
	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()
	cmd.Stdout = outFile
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func extractWith7z(src, dest string) error {
	cmd := exec.Command("7z", "x", src, "-o"+dest, "-y")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

