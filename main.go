package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/bitrise/tools/timeoutcmd"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-tools/go-steputils/stepconf"
	"github.com/hashicorp/go-version"
	"github.com/kballard/go-shellquote"
	"gopkg.in/yaml.v2"
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
	tmpPth, err := pathutil.NormalizedOSTempDirPath("credential")
	if err != nil {
		return err
	}
	pth := filepath.Join(tmpPth, "cred.json")
	if err := fileutil.WriteStringToFile(pth, cred); err != nil {
		return err
	}
	return os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", pth)
}

func parseGitTags(gitOutput string) (versions []string) {
	for _, line := range strings.Split(gitOutput, "\n") {
		if ref := strings.Split(line, "\t"); len(ref) == 2 {
			versions = append(versions, strings.TrimPrefix(ref[1], "refs/tags/"))
		}
	}
	return
}

func findLatestVersion(versions []string) string {
	lastVersion, err := version.NewVersion("0.0.0")
	if err != nil {
		return ""
	}

	for _, v := range versions {
		if cVersion, err := version.NewVersion(v); err == nil {
			if cVersion.GreaterThan(lastVersion) {
				lastVersion = cVersion
			}
		}
	}

	return lastVersion.String()
}

// gets the tags list, splits the lines per tab and finds the prefix-truncated version strings
// if the version string is a valid semver version then this function returns the latest one
func getLatestVersion(repoURL string) (string, error) {
	cmd := command.New("git", "ls-remote", "--tags", "--quiet", repoURL)
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run git command, error: %s, output: %s", err, out)
	}

	lastVersion := findLatestVersion(parseGitTags(out))

	if lastVersion == "0.0.0" {
		return "", fmt.Errorf("unable to find latest version")
	}

	return lastVersion, nil
}

// if input version is latest then it returns the fetched latest release version download url otherwise
// returns the release version download url for the given version
func getDownloadURLbyVersion(repoURL, version string) (string, error) {
	if version == "latest" {
		var err error
		if version, err = getLatestVersion(repoURL); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%s/releases/download/v%s/flank.jar", baseURL, version), nil
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
		if !fInf.ModTime().Before(latestModtime) {
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
		if err := ioutil.WriteFile(destinationFile, data, 0644); err != nil {
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
	downloadURL, err := getDownloadURLbyVersion(baseURL, cfg.Version)
	if err != nil {
		failf("Failed to get download URL, error: %s", err)
	}

	binaryPath, err := download(downloadURL)
	if err != nil {
		failf("Failed to download binary, error: %s", err)
	}

	log.Donef("- Done")
	fmt.Println()

	// string credentials
	if err := storeCredentials(string(cfg.ServiceAccountJSON)); err != nil {
		failf("Failed to store credential file, error: %s", err)
	}

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
