// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package image

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

type TarImage struct {
	files        []string
	excludePaths []string
	infoLog      io.Writer
}

func NewTarImage(files []string, excludePaths []string, infoLog io.Writer) *TarImage {
	return &TarImage{files, excludePaths, infoLog}
}

func (i *TarImage) AsFileBundle() (*FileImage, error) {
	return i.asFileImage(true)
}

func (i *TarImage) AsFileImage() (*FileImage, error) {
	return i.asFileImage(false)
}

func (i *TarImage) asFileImage(bundle bool) (*FileImage, error) {
	tmpFile, err := ioutil.TempFile("", "imgpkg-tar-image")
	if err != nil {
		return nil, err
	}

	defer tmpFile.Close()

	err = i.createTarball(tmpFile, i.files)
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return nil, err
	}

	fileImg, err := NewFileImage(tmpFile.Name(), bundle)
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return nil, err
	}

	return fileImg, nil
}

func (i *TarImage) createTarball(file *os.File, filePaths []string) error {
	tarWriter := tar.NewWriter(file)
	defer tarWriter.Close()

	for _, path := range filePaths {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Walk is deterministic according to https://golang.org/pkg/path/filepath/#Walk
			err := filepath.Walk(path, func(walkedPath string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				relPath, err := filepath.Rel(path, walkedPath)
				if err != nil {
					return err
				}
				if info.IsDir() {
					if i.isExcluded(relPath) {
						return filepath.SkipDir
					}
					return i.addDirToTar(relPath, info, tarWriter)
				}
				if (info.Mode() & os.ModeType) != 0 {
					return fmt.Errorf("Expected file '%s' to be a regular file", walkedPath)
				}
				return i.addFileToTar(walkedPath, relPath, info, tarWriter)
			})
			if err != nil {
				return fmt.Errorf("Adding file '%s' to tar: %s", path, err)
			}
		} else {
			err := i.addFileToTar(path, filepath.Base(path), info, tarWriter)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (i *TarImage) addDirToTar(relPath string, info os.FileInfo, tarWriter *tar.Writer) error {
	if i.isExcluded(relPath) {
		panic("Unreachable") // directories excluded above
	}

	i.infoLog.Write([]byte(fmt.Sprintf("dir: %s\n", relPath)))

	header := &tar.Header{
		Name:     relPath,
		Size:     info.Size(),
		Mode:     0700,        // static
		ModTime:  time.Time{}, // static
		Typeflag: tar.TypeDir,
	}

	return tarWriter.WriteHeader(header)
}

func (i *TarImage) addFileToTar(fullPath, relPath string, info os.FileInfo, tarWriter *tar.Writer) error {
	if i.isExcluded(relPath) {
		return nil
	}

	i.infoLog.Write([]byte(fmt.Sprintf("file: %s\n", relPath)))

	file, err := os.Open(fullPath)
	if err != nil {
		return err
	}

	defer file.Close()

	header := &tar.Header{
		Name:     relPath,
		Size:     info.Size(),
		Mode:     0600,        // static
		ModTime:  time.Time{}, // static
		Typeflag: tar.TypeReg,
	}

	err = tarWriter.WriteHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(tarWriter, file)
	return err
}

func (i *TarImage) isExcluded(relPath string) bool {
	for _, path := range i.excludePaths {
		if path == relPath {
			return true
		}
	}
	return false
}
