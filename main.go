package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/bitrise-io/go-utils/pathutil"

	"github.com/hashicorp/go-version"

	"github.com/bitrise-io/bitrise/tools/timeoutcmd"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-tools/go-steputils/stepconf"
	"github.com/kballard/go-shellquote"
)

const (
	platformAndroid   = "android"
	platformIos       = "ios"
	credentialPathFmt = "%s/.config/gcloud/application_default_credentials.json"
	baseURL           = "https://github.com/TestArmada/flank"
	resultsDir        = "./results"
)

type config struct {
	ServiceAccountJSON stepconf.Secret `env:"google_service_account_json,required"`
	ConfigPath         string          `env:"config_path,file"`
	Version            string          `env:"version,required"`
	CommandFlags       string          `env:"command_flags"`
}

// returns android if there is an app field under gcloud in the config yml
func detectPlatform(configYMLPath string) (string, error) {
	var androidApp struct {
		Gcloud struct {
			App string `yaml:"app"`
		} `yaml:"gcloud"`
	}

	ymlBytes, err := fileutil.ReadBytesFromFile(configYMLPath)
	if err != nil {
		return "", err
	}

	if err := yaml.Unmarshal(ymlBytes, &androidApp); err != nil {
		return "", err
	}

	if len(androidApp.Gcloud.App) > 0 {
		return platformAndroid, nil
	}
	return platformIos, nil
}

// stores string under the default gcloud credential's local path
func storeCredentials(cred string) error {
	pth := fmt.Sprintf(credentialPathFmt, os.Getenv("HOME"))
	if err := os.MkdirAll(filepath.Dir(pth), 0777); err != nil {
		return err
	}
	return fileutil.WriteStringToFile(pth, cred)
}

// gets the tags list, splits the lines per tab and finds the prefix-truncated version strings
// if the version string is a valid semver version then this function returns the latest one
func getLatestVersion() (string, error) {
	cmd := command.New("git", "ls-remote", "--tags", "--quiet", "https://github.com/TestArmada/flank")
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return "", err
	}

	// hashes and version refs are separated by tab, output will be something like:
	// a85cdf850b7562d38559b84d405c97f7d194603s refs/tags/flank_snapshot
	// f0d902fdd947e7f2db43213929bdc6069a681624	refs/tags/v1.4.2
	// 4f00292267eaa2b230c4a5705a50c5302d7f9dc1	refs/tags/v1.5.0
	// 44ed2076a684a740c41660ab80052b4218633538	refs/tags/v1.6.0
	// 65762d7888a8a6e7490cbe7433a725201cbd5304	refs/tags/v1.7.0
	// eae4b1e50e7eb05c02fc678994c6425bf14e1701	refs/tags/v2.0.0

	var versions []string
	for _, line := range strings.Split(out, "\n") {
		if ref := strings.Split(line, "\t"); len(ref) == 2 {
			versions = append(versions, strings.TrimPrefix(ref[1], "refs/tags/"))
		}
	}

	lastVersion, err := version.NewVersion("0.0.0")
	if err != nil {
		return "", err
	}

	for _, v := range versions {
		if cVersion, err := version.NewVersion(v); err == nil {
			if cVersion.GreaterThan(lastVersion) {
				lastVersion = cVersion
			}
		}
	}

	return lastVersion.String(), nil
}

// if input version is latest then it returns the fetched latest release version download url otherwise
// returns the release version download url for the given version
func getDownloadURLbyVersion(version string) (string, error) {
	if version != "latest" {
		return fmt.Sprintf("%s/releases/download/%s/flank.jar", baseURL, version), nil
	}
	latest, err := getLatestVersion()
	if err != nil {
		return "", nil
	}
	return fmt.Sprintf("%s/releases/download/v%s/flank.jar", baseURL, latest), nil
}

// downloads file from an url to a temp location and returns the full path for the file
func download(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Warnf("Failed to close response body, error: %s", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unsuccessful status code: %d", resp.StatusCode)
	}

	tmpPath, err := pathutil.NormalizedOSTempDirPath("flank-bin")
	if err != nil {
		return "", err
	}

	binPath := filepath.Join(tmpPath, "flank.jar")

	bodyData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return binPath, fileutil.WriteBytesToFile(binPath, bodyData)
}

func failf(format string, args ...interface{}) {
	log.Errorf(format, args...)
	os.Exit(1)
}

// lists all dirs inside of ./results dir and selects the latest(by modtime)
// then all the files in the root will be copied from this dir to the root of the dir under BITRISE_DEPLOY_DIR
func exportArtifacts(copied func(src, dest string)) error {
	fInfs, err := ioutil.ReadDir(resultsDir)
	if err != nil {
		return err
	}

	var latestDir string
	var latestModtime time.Time

	for _, fInf := range fInfs {
		if !fInf.IsDir() {
			continue
		}
		if fInf.ModTime().After(latestModtime) || fInf.ModTime().Equal(latestModtime) {
			latestModtime = fInf.ModTime()
			latestDir = filepath.Join(resultsDir, fInf.Name())
		}
	}

	fInfs, err = ioutil.ReadDir(latestDir)
	if err != nil {
		return err
	}

	for _, fInf := range fInfs {
		if fInf.IsDir() {
			continue
		}

		srcFile := filepath.Join(latestDir, fInf.Name())
		destinationFile := filepath.Join(os.Getenv("BITRISE_DEPLOY_DIR"), fInf.Name())

		data, err := ioutil.ReadFile(srcFile)
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(destinationFile, data, 0644)
		if err != nil {
			return err
		}

		if copied != nil {
			copied(srcFile, destinationFile)
		}
	}

	return nil
}

func main() {
	//
	// configuration
	var cfg config
	if err := stepconf.Parse(&cfg); err != nil {
		failf("Issue with input: %s", err)
	}
	stepconf.Print(cfg)
	fmt.Println()

	//
	// tool setup
	log.Infof("Downloading binary")
	downloadURL, err := getDownloadURLbyVersion(cfg.Version)
	if err != nil {
		failf("Failed to get download URL, error: %s", err)
	}

	binaryPath, err := download(downloadURL)
	if err != nil {
		failf("Failed to download binary, error: %s", err)
	}

	log.Donef("- Done")
	fmt.Println()

	//
	// running the tool
	log.Infof("Running test")
	platform, err := detectPlatform(cfg.ConfigPath)
	if err != nil {
		failf("Failed to detect platform, error: %s", err)
	}
	log.Printf("- Detected platform: %s", platform)

	commandFlags, err := shellquote.Split(cfg.CommandFlags)
	if err != nil {
		failf("Failed to split command flags, error: %s", err)
	}

	fmt.Println()
	command := command.New("java", append([]string{"-jar", binaryPath, platform, "run", "-c", cfg.ConfigPath}, commandFlags...)...).
		SetStdin(os.Stdin).
		SetStdout(os.Stdout).
		SetStderr(os.Stderr)

	log.Donef("$ %s", command.PrintableCommandArgs())
	fmt.Println()

	cmdErr := command.Run()

	fmt.Println()

	//
	// exporting generated artifacts

	log.Infof("Exporting artifacts")
	if err := exportArtifacts(
		func(src, dest string) {
			log.Printf("- copied: %s -> %s", src, dest)
		},
	); err != nil {
		failf("Failed to export artifacts, error: %s", err)
	}
	log.Donef("- Done")

	os.Exit(timeoutcmd.ExitStatus(cmdErr))
}
