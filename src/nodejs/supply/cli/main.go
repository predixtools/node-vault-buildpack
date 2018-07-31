package main

import (
	"apt/apt"
	"io"
	"io/ioutil"
	_ "nodejs/hooks"
	"nodejs/npm"
	"nodejs/supply"
	"nodejs/yarn"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/cloudfoundry/libbuildpack"
)

func main() {
	logfile, err := ioutil.TempFile("", "cloudfoundry.nodejs-buildpack.supply")
	defer logfile.Close()
	if err != nil {
		logger := libbuildpack.NewLogger(os.Stdout)
		logger.Error("Unable to create log file: %s", err.Error())
		os.Exit(8)
	}

	stdout := io.MultiWriter(os.Stdout, logfile)
	logger := libbuildpack.NewLogger(stdout)

	cmd := exec.Command("find", ".")
	cmd.Dir = "/tmp/cache"
	cmd.Stdout = os.Stdout
	cmd.Run()

	buildpackDir, err := libbuildpack.GetBuildpackDir()
	if err != nil {
		logger.Error("Unable to determine buildpack directory: %s", err.Error())
		os.Exit(9)
	}

	manifest, err := libbuildpack.NewManifest(buildpackDir, logger, time.Now())
	if err != nil {
		logger.Error("Unable to load buildpack manifest: %s", err.Error())
		os.Exit(10)
	}
	installer := libbuildpack.NewInstaller(manifest)

	stager := libbuildpack.NewStager(os.Args[1:], logger, manifest)
	if err := stager.CheckBuildpackValid(); err != nil {
		os.Exit(11)
	}

	if err = installer.SetAppCacheDir(stager.CacheDir()); err != nil {
		logger.Error("Unable to setup appcache: %s", err)
		os.Exit(18)
	}
	if err = manifest.ApplyOverride(stager.DepsDir()); err != nil {
		logger.Error("Unable to apply override.yml files: %s", err)
		os.Exit(17)
	}

	err = libbuildpack.RunBeforeCompile(stager)
	if err != nil {
		logger.Error("Before Compile: %s", err.Error())
		os.Exit(12)
	}

	err = stager.SetStagingEnvironment()
	if err != nil {
		logger.Error("Unable to setup environment variables: %s", err.Error())
		os.Exit(13)
	}

	if exists, err := libbuildpack.FileExists(filepath.Join(stager.BuildDir(), "apt.yml")); err != nil {
		logger.Error("Unable to test existence of apt.yml: %s", err.Error())
		os.Exit(16)
	} else if !exists {
		logger.Error("Apt buildpack requires apt.yml\n(https://github.com/cloudfoundry/apt-buildpack/blob/master/fixtures/simple/apt.yml)")
		if exists, err := libbuildpack.FileExists(filepath.Join(stager.BuildDir(), "Aptfile")); err != nil || exists {
			logger.Error("Aptfile is deprecated. Please convert to apt.yml")
		}
		os.Exit(17)
	}

	command := &libbuildpack.Command{}
	a := apt.New(command, filepath.Join(stager.BuildDir(), "apt.yml"), stager.CacheDir(), filepath.Join(stager.DepDir(), "apt"))
	if err := a.Setup(); err != nil {
		logger.Error("Unable to initialize apt package: %s", err.Error())
		os.Exit(13)
	}

	s := supply.Supplier{
		Logfile: logfile,
		Stager:  stager,
		Aa: a,
		Yarn: &yarn.Yarn{
			Command: &libbuildpack.Command{},
			Log:     logger,
		},
		NPM: &npm.NPM{
			Command: &libbuildpack.Command{},
			Log:     logger,
		},
		Manifest:  manifest,
		Installer: installer,
		Log:       logger,
		Command:   &libbuildpack.Command{},
	}

	err = supply.Run(&s)
	if err != nil {
		os.Exit(14)
	}

	if err := stager.WriteConfigYml(nil); err != nil {
		logger.Error("Error writing config.yml: %s", err.Error())
		os.Exit(15)
	}
	if err = installer.CleanupAppCache(); err != nil {
		logger.Error("Unable to clean up app cache: %s", err)
		os.Exit(19)
	}
}
