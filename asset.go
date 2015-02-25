package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func copyRegularFile(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(destFile, srcFile); err != nil {
		return err
	}
	return nil
}

func makeAbsoluteTarget(base, target string) string {
	dirPart := filepath.Dir(base)
	testPath := filepath.Join(dirPart, target)
	return filepath.Clean(testPath)
}

func absoluteSrcTargetToDest(srcBase, srcTarget, destBase string) (string, error) {
	relTarget, err := filepath.Rel(srcBase, srcTarget)
	if err != nil {
		return "", err
	}
	destTarget := filepath.Join(destBase, relTarget)
	return filepath.Clean(destTarget), nil
}

func copySymlink(src, dest, imageAssetDir, root string) error {
	symTarget, err := os.Readlink(src)
	if err != nil {
		return err
	}
	absolute := filepath.IsAbs(symTarget)
	if !absolute {
		symTarget = makeAbsoluteTarget(src, symTarget)
	}
	if strings.HasPrefix(symTarget, root) {
		var err error
		linkTarget := ""
		if absolute {
			linkTarget, err = absoluteSrcTargetToDest(root, symTarget, imageAssetDir)
		} else {
			linkTarget, err = filepath.Rel(filepath.Dir(src), symTarget)
		}
		if err != nil {
			return err
		}
		if err := os.Symlink(linkTarget, dest); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("Symlink %s points to %s, which is outside asset %s", src, symTarget, root)
	}
	return nil
}

func copyTree(src, dest, imageAssetDir string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rootLess := path[len(src):]
		target := filepath.Join(dest, rootLess)
		mode := info.Mode()
		switch {
		case mode.IsDir():
			err := os.Mkdir(target, mode.Perm())
			if err != nil {
				return err
			}
		case mode.IsRegular():
			if err := copyRegularFile(path, target); err != nil {
				return err
			}
		case mode&os.ModeSymlink == os.ModeSymlink:
			if err := copySymlink(path, target, imageAssetDir, src); err != nil {
				return err
			}
		default:
			return fmt.Errorf("Unsupported node (%s) in assets, only regular files, directories and symlinks pointing to node inside asset are supported.", path, mode.String())
		}
		return nil
	})
}

func replacePlaceholders(path string, paths map[string]string) string {
	fmt.Printf("Processing path: %s\n", path)
	newPath := path
	for placeholder, replacement := range paths {
		newPath = strings.Replace(newPath, placeholder, replacement, -1)
	}
	fmt.Printf("Processed path: %s\n", newPath)
	return newPath
}

func validateAsset(ACIAsset, localAsset string) error {
	if !filepath.IsAbs(ACIAsset) {
		return fmt.Errorf("Wrong ACI asset: '%v' - ACI asset has to be absolute path", ACIAsset)
	}
	if !filepath.IsAbs(localAsset) {
		return fmt.Errorf("Wrong local asset: '%v' - local asset has to be absolute path", localAsset)
	}
	fi, err := os.Stat(localAsset)
	if err != nil {
		return fmt.Errorf("Error stating %v: %v", localAsset, err)
	}
	if fi.Mode().IsDir() || fi.Mode().IsRegular() {
		return nil
	}
	return fmt.Errorf("Can't handle local asset %v - not a file, not a dir", fi.Name())
}

func PrepareAssets(assets []string, rootfs string, paths map[string]string) error {
	for _, asset := range assets {
		splitAsset := filepath.SplitList(asset)
		if len(splitAsset) != 2 {
			return fmt.Errorf("Malformed asset option: '%v' - expected two absolute paths separated with %v", asset, listSeparator())
		}
		ACIAsset := replacePlaceholders(splitAsset[0], paths)
		localAsset := replacePlaceholders(splitAsset[1], paths)
		if err := validateAsset(ACIAsset, localAsset); err != nil {
			return err
		}
		ACIAssetSubPath := filepath.Join(rootfs, filepath.Dir(ACIAsset))
		err := os.MkdirAll(ACIAssetSubPath, 0755)
		if err != nil {
			return fmt.Errorf("Failed to create directory tree for asset '%v': %v", asset, err)
		}
		err = copyTree(localAsset, filepath.Join(rootfs, ACIAsset), ACIAsset)
		if err != nil {
			return fmt.Errorf("Failed to copy assets for '%v': %v", asset, err)
		}
	}
	return nil
}