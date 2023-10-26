package converters

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

func ConvertWithFFMPEG(inputFile string, outputFile string) (duration time.Duration, err error) {
	cmd := exec.Command("ffmpeg", "-i", inputFile, "-af", "silencedetect=n=-50dB:d=1", outputFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to convert %s to %s: %s\n%s", inputFile, outputFile, err, output)
	}
	outputStr := string(output)
	if outputStr == "" && err != nil {
		logrus.Errorf("failed to get duration of %s, output: %s, error: %s", outputFile, outputStr, err)
		return 0, nil
	}

	duration, err = ParseDuration(outputStr)
	if err != nil {
		logrus.Errorf("failed to parse duration %s: %s", outputStr, err)
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
		return 0, fmt.Errorf("failed to parse time %s: %s", durationStr, err)
	}
	dayOnly := time.Date(parsedTime.Year(), parsedTime.Month(), parsedTime.Day(), 0, 0, 0, 0, parsedTime.Location())
	return parsedTime.Sub(dayOnly), nil
}
