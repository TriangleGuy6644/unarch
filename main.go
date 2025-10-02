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

	"github.com/ulikunitz/xz"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: unarch <archive_file> [destination]")
		os.Exit(1)
	}

	archivePath := os.Args[1]
	destDir := "."
	if len(os.Args) >= 3 {
		destDir = os.Args[2]
	}

	archiveType, err := detectArchiveType(archivePath)
	if err != nil {
		fmt.Println("Error detecting archive type:", err)
		os.Exit(1)
	}

	switch archiveType {
	case "zip":
		err = unzip(archivePath, destDir)
	case "tar", "tar.gz", "tar.bz2", "tar.xz":
		err = untar(archivePath, destDir)
	case "7z", "rar":
		err = extractWith7z(archivePath, destDir)
	default:
		fmt.Println("Unsupported archive type")
		os.Exit(1)
	}

	if err != nil {
		fmt.Println("Extraction error:", err)
		os.Exit(1)
	}
	fmt.Println("Extraction complete!")
}

// ---------------- Detect Archive Type ----------------
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
	default:
		return "", fmt.Errorf("unknown archive type")
	}
}

// ---------------- Zip ----------------
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

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

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

// ---------------- Tar ----------------
func untar(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	var fileReader io.Reader = f

	// detect compression by magic bytes
	buf := make([]byte, 6)
	_, _ = f.Read(buf)
	f.Seek(0, io.SeekStart)

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

// ---------------- 7z / Rar ----------------
func extractWith7z(src, dest string) error {
	cmd := exec.Command("7z", "x", src, "-o"+dest, "-y")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
