package converters

import (
	"fmt"
	"os/exec"
	"strings"
	"talk2robots/m/v2/app/util"
	"time"

	"github.com/sirupsen/logrus"
)

func ConvertWithFFMPEG(inputFile string, outputFile string) (duration time.Duration, err error) {
	var cmd *exec.Cmd
	// if input file is .ogg, rename it first to avoid overwriting the original file
	if strings.HasSuffix(inputFile, ".ogg") {
		renamedInputFile := inputFile + ".tmp"
		err := exec.Command("mv", inputFile, renamedInputFile).Run()
		if err != nil {
			return 0, fmt.Errorf("failed to rename %s to %s: %v", inputFile, renamedInputFile, err)
		}
		defer util.SafeOsDelete(renamedInputFile)
		inputFile = renamedInputFile
	}

	if strings.HasSuffix(inputFile, ".oga") {
		// don't change the bitrate for .oga files, i.e. voice messages from Telegram
		cmd = exec.Command("ffmpeg", "-i", inputFile, "-af", "silencedetect=n=-50dB:d=1", "-map", "a", "-q:a", "0", "-ac", "1", "-c:a", "libopus", outputFile)
	} else {
		cmd = exec.Command("ffmpeg", "-i", inputFile, "-af", "silencedetect=n=-50dB:d=1", "-map", "a", "-q:a", "0", "-ac", "1", "-c:a", "libopus", "-b:a", "12k", outputFile)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to convert %s to %s: %v\n%s", inputFile, outputFile, err, output)
	}
	outputStr := string(output)
	if outputStr == "" && err != nil {
		logrus.Errorf("failed to get duration of %s, output: %s, error: %v", outputFile, outputStr, err)
		return 0, nil
	}

	duration, err = ParseDuration(outputStr)
	if err != nil {
		logrus.Errorf("failed to parse duration %s: %v", outputStr, err)
		return 0, nil
	}
	return duration, nil
}

func ParseDuration(outputStr string) (duration time.Duration, err error) {
	// ffmpeg version 4.2.7-0ubuntu0.1 Copyright (c) 2000-2022 the FFmpeg developers
	// ...
	// size=      13kB time=00:00:01.63 bitrate=  67.5kbits/s speed=6.97x
	// video:0kB audio:12kB subtitle:0kB other streams:0kB global headers:0kB muxing overhead: 9.588932%
	arrayOfTimes := strings.Split(outputStr, "time=")
	durationStr := arrayOfTimes[len(arrayOfTimes)-1]
	durationStr = strings.Split(durationStr, " ")[0]
	if durationStr == "" {
		return 0, fmt.Errorf("duration is empty, full output: %s", outputStr)
	}
	parsedTime, err := time.Parse("15:04:05.99", durationStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse time %s: %v", durationStr, err)
	}
	dayOnly := time.Date(parsedTime.Year(), parsedTime.Month(), parsedTime.Day(), 0, 0, 0, 0, parsedTime.Location())
	return parsedTime.Sub(dayOnly), nil
}
