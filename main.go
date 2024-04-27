package main

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var extractRoots = map[string]string{
	"RobloxApp.zip": "",
	"shaders.zip":   "shaders/",
	"ssl.zip":       "ssl/",

	"WebView2.zip":                 "",
	"WebView2RuntimeInstaller.zip": "WebView2RuntimeInstaller/",

	"content-avatar.zip":    "content/avatar/",
	"content-configs.zip":   "content/configs/",
	"content-fonts.zip":     "content/fonts/",
	"content-sky.zip":       "content/sky/",
	"content-sounds.zip":    "content/sounds/",
	"content-textures2.zip": "content/textures/",
	"content-models.zip":    "content/models/",

	"content-textures3.zip":      "PlatformContent/pc/textures/",
	"content-terrain.zip":        "PlatformContent/pc/terrain/",
	"content-platform-fonts.zip": "PlatformContent/pc/fonts/",

	"extracontent-luapackages.zip":  "ExtraContent/LuaPackages/",
	"extracontent-translations.zip": "ExtraContent/translations/",
	"extracontent-models.zip":       "ExtraContent/models/",
	"extracontent-textures.zip":     "ExtraContent/textures/",
	"extracontent-places.zip":       "ExtraContent/places/",
}

type Deployment struct {
	Version     string
	FileVersion string
}

func main() {
	deployments, err := fetchDeployments("https://setup.rbxcdn.com/DeployHistory.txt")
	if err != nil {
		fmt.Println("Error getting deployment history:", err)
		return
	}

	oldDeployment, latestDeployment := deployments[len(deployments)-2], deployments[len(deployments)-1]

	fmt.Println("Old deployment for WindowsPlayer:", oldDeployment)
	fmt.Println("Latest deployment for WindowsPlayer:", latestDeployment)

	if err := downloadAndExtractPackages(oldDeployment.Version); err != nil {
		fmt.Println("Error downloading and extracting packages:", err)
		return
	}

	file, err := os.Create("out/AppSettings.xml")
	if err != nil {
		fmt.Println("Error creating AppSettings.xml:", err)
		return
	}

	_, err = file.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<Settings>\n\t<ContentFolder>content</ContentFolder>\n\t<BaseUrl>http://www.roblox.com</BaseUrl>\n</Settings>")
	if err != nil {
		fmt.Println("Error writing to AppSettings.xml:", err)
		return
	}

	filepath.Walk("out", func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".zip") {
			if err := os.Remove(path); err != nil {
				fmt.Println("Error removing zip file:", err)
			}
		}
		return nil
	})

	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error getting user home directory:", err)
		return
	}

	latestDir := filepath.Join(userHomeDir, "AppData", "Local", "Roblox", "Versions", latestDeployment.Version)
	fmt.Println("[+] replacing files in Roblox directory:", latestDir)
	if err := replaceFiles("out", latestDir); err != nil {
		println("Make sure Roblox is closed and updated before running again. (Ignore this if you're using Bloxstrap)")
	}
	println("[+] Files replaced in Roblox directory!")

	bloxstrapDir := filepath.Join(userHomeDir, "AppData", "Local", "Bloxstrap", "Versions", latestDeployment.Version)
	fmt.Println("[+] replacing files to Bloxstrap directory:", bloxstrapDir)
	if err := replaceFiles("out", bloxstrapDir); err != nil {
		println("Make sure Roblox & Bloxstrap are closed and updated before running again. (Ignore this if you're not using Bloxstrap)")
	}
	println("[+] Files replaced in Bloxstrap directory!")

	fmt.Println("[+] Done!")
}

func replaceFiles(source, destination string) error {
	if err := removeAllFiles(destination); err != nil {
		return err
	}

	fileList := make(chan string)
	var wg sync.WaitGroup

	walkFunc := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			fileList <- path
		}
		return nil
	}

	go func() {
		err := filepath.Walk(source, walkFunc)
		if err != nil {
			close(fileList)
			return
		}
		close(fileList)
	}()

	for path := range fileList {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			destPath := strings.Replace(path, source, destination, 1)
			if err := os.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
				fmt.Println("Error creating directory:", err)
				return
			}
			if err := copyFile(path, destPath); err != nil {
				fmt.Println("Error copying file:", err)
				return
			}
		}(path)
	}

	wg.Wait()
	return nil
}

func removeAllFiles(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()

	entries, err := d.Readdir(-1)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		fullPath := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			if err := os.RemoveAll(fullPath); err != nil {
				return err
			}
		} else {
			if err := os.Remove(fullPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}
	return nil
}

func downloadAndExtractPackages(oldVersion string) error {
	os.MkdirAll("out", os.ModePerm)
	var wg sync.WaitGroup

	for packageName := range extractRoots {
		wg.Add(1)
		go func(pkg string) {
			defer wg.Done()
			if err := downloadAndExtractPackage(pkg, oldVersion); err != nil {
				fmt.Printf("Error downloading and extracting package %s: %s\n", pkg, err)
			}
		}(packageName)
	}

	wg.Wait()
	fmt.Println("[+] Replacing files...")
	return nil
}

func downloadAndExtractPackage(packageName, version string) error {
	url := fmt.Sprintf("https://roblox-setup.cachefly.net/%s-%s", version, packageName)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("error getting %s: %s", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	filePath := filepath.Join("out", packageName)
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating file: %s", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("error copying package data: %s", err)
	}

	if rootFolder, ok := extractRoots[packageName]; ok {
		if err := extractZip(filePath, rootFolder); err != nil {
			return fmt.Errorf("error extracting package: %s", err)
		}
		fmt.Printf("[+] Extracted \"%s\"!\n", packageName)
	} else {
		fmt.Printf("[*] Package name \"%s\" not defined in extraction roots, skipping extraction!\n", packageName)
	}

	return nil
}

func extractZip(zipFile, extractRootFolder string) error {
	r, err := zip.OpenReader(zipFile)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		extractPath := filepath.Join("out", extractRootFolder, f.Name)

		if f.FileInfo().IsDir() {
			if _, err := os.Stat(extractPath); os.IsNotExist(err) {
				if err := os.MkdirAll(extractPath, os.ModePerm); err != nil {
					return err
				}
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(extractPath), os.ModePerm); err != nil {
			return err
		}

		file, err := os.Create(extractPath)
		if err != nil {
			return err
		}
		defer file.Close()

		if _, err := io.Copy(file, rc); err != nil {
			return err
		}
	}

	return nil
}

func fetchDeployments(url string) ([]Deployment, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("error getting deployment history: %s", err)
	}
	defer resp.Body.Close()

	var deployments []Deployment
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) != 17 || parts[1] != "WindowsPlayer" {
			continue
		}

		deployments = append(deployments, Deployment{
			Version:     parts[2],
			FileVersion: strings.ReplaceAll(strings.Join(parts[9:13], ""), ",", ""),
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading deployment history: %s", err)
	}

	return deployments, nil
}
