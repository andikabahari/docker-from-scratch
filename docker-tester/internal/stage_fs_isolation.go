package internal

import (
	"io/ioutil"

	tester_utils "github.com/codecrafters-io/tester-utils"
)

func testFSIsolation(stageHarness *tester_utils.StageHarness) error {
	logger := stageHarness.Logger
	executable := stageHarness.Executable

	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		return err
	}

	logger.Debugf("Created temp dir on host: %v", tempDir)
	logger.Debugf("Executing: ./your_docker.sh run <some_image> /usr/local/bin/docker-explorer ls %v", tempDir)
	logger.Debugf("(expecting that the directory won't be accessible)")

	result, err := executable.Run(
		"run", DOCKER_IMAGE_STAGE_1,
		"/usr/local/bin/docker-explorer", "ls", tempDir,
	)
	if err != nil {
		return err
	}

	if err = assertStdoutContains(result, "no such file or directory"); err != nil {
		return err
	}

	if err = assertExitCode(result, 2); err != nil {
		return err
	}

	return nil
}
