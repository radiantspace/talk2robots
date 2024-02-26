/*
	translation from TypeScript of https://github.com/chengsokdara/use-whisper
	powered by GPT-4 and Copilot
	there is some stuff that is not used, so maybe need to clean it up
*/

package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/payments"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

const (
	WHISPER_PRICE_PER_MINUTE = 0.006
)

type WhisperConfig struct {
	APIKey             string
	WhisperAPIEndpoint string
	Mode               string
	StopTimeout        time.Duration
	OnTranscribe       func()
	Language           string
	Prompt             string
	ResponseFormat     string
	Temperature        float64
}

type WhisperTimeout struct {
	Stop *time.Timer
}

type WhisperTranscript struct {
	Blob []byte
	Text string
}

type WhisperHook interface {
	Whisper(ctx context.Context, config WhisperConfig, read io.ReadCloser, filename string)
	Transcript() WhisperTranscript
}

type whisper struct {
	WhisperConfig WhisperConfig
	chunks        [][]byte
	filename      string
	listener      *websocket.Conn
	mode          string
	read          io.ReadCloser
	recorder      interface{} // *RecordRTCPromisesHandler
	recording     bool
	speaking      bool
	stream        interface{} //*MediaStream
	timeout       WhisperTimeout
	transcribing  bool
	transcript    WhisperTranscript
	ctx           context.Context
	usage         models.CostAndUsage
}

func NewWhisper() WhisperHook {
	return &whisper{}
}

func (uw *whisper) Transcript() WhisperTranscript {
	return uw.transcript
}

func (uw *whisper) Whisper(ctx context.Context, config WhisperConfig, read io.ReadCloser, filename string) {
	// Merge default config with provided config
	uw.mergeConfig(config)

	// Check for required apiKey
	if config.APIKey == "" && config.OnTranscribe == nil {
		panic(errors.New("apiKey is required if onTranscribe is not provided"))
	}

	// Initialize values
	uw.ctx = ctx
	uw.read = read
	uw.filename = filename
	uw.chunks = [][]byte{}
	uw.listener = nil
	uw.recorder = nil
	uw.stream = nil
	uw.timeout = WhisperTimeout{}
	uw.recording = false
	uw.speaking = false
	uw.transcribing = false
	uw.transcript = WhisperTranscript{}
	uw.usage = models.CostAndUsage{
		Engine:            models.Engine(models.Whisper),
		PricePerInputUnit: WHISPER_PRICE_PER_MINUTE,
		Cost:              0,
		Usage:             models.Usage{},
	}

	uw.onTranscribing()
}

func (uw *whisper) mergeConfig(config WhisperConfig) {
	uw.WhisperConfig = config
	uw.mode = config.Mode
	if uw.mode == "" {
		logrus.Debug("mode is not provided, defaulting to 'transcriptions'")
		uw.mode = "transcriptions"
	}
}

func (uw *whisper) onTranscribing() {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println(err)
			uw.transcribing = false
		}
	}()

	text, err := uw.onWhispered(uw.read, uw.filename)
	if err != nil {
		logrus.Debugf("onWhispered error: %s", err)
		panic(err)
	}

	uw.transcript.Text = text
}

func (uw *whisper) onWhispered(reader io.Reader, fileName string) (string, error) {
	timeNow := time.Now()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		logrus.Debug("Whisper: could not create form file: ", err)
		return "", err
	}
	io.Copy(part, reader)

	writer.WriteField("model", "whisper-1")

	// if uw.mode == "transcriptions" {
	// 	language := uw.WhisperConfig.Language
	// 	if language == "" {
	// 		language = "en"
	// 	}
	// 	writer.WriteField("language", language)
	// }
	params := uw.ctx.Value(models.ParamsContext{}).(string)
	if params != "" {
		writer.WriteField("language", params)
	}

	if uw.WhisperConfig.Prompt != "" {
		writer.WriteField("prompt", uw.WhisperConfig.Prompt)
	}

	if uw.WhisperConfig.ResponseFormat != "" {
		writer.WriteField("response_format", uw.WhisperConfig.ResponseFormat)
	}

	if uw.WhisperConfig.Temperature != 0 {
		writer.WriteField("temperature", fmt.Sprintf("%f", uw.WhisperConfig.Temperature))
	}

	err = writer.Close()
	if err != nil {
		logrus.Debug("Whisper: could not close writer: ", err)
		return "", err
	}

	req, err := http.NewRequest("POST", uw.WhisperConfig.WhisperAPIEndpoint+uw.mode, body)
	if err != nil {
		logrus.Debug("Whisper: could not create request: ", err)
		return "", err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	if uw.WhisperConfig.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+uw.WhisperConfig.APIKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logrus.Debugf("Whisper: could not send request: %s, response: %+v", err, resp)
		return "", err
	}
	defer resp.Body.Close()

	responseData, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.Debugf("Whisper: could not read response: %s", responseData)
		return "", err
	}

	var jsonResponse map[string]interface{}
	err = json.Unmarshal(responseData, &jsonResponse)
	if err != nil {
		logrus.Debugf("Whisper: could not parse response: %s", responseData)
		return "", err
	}

	duration := uw.ctx.Value(models.WhisperDurationContext{}).(time.Duration)
	uw.usage.Usage = models.Usage{
		AudioDuration: duration.Minutes(),
	}
	go payments.Bill(uw.ctx, uw.usage)

	config.CONFIG.DataDogClient.Timing("openai.whisper.latency", time.Since(timeNow), []string{"model:" + string(uw.usage.Engine)}, 1)
	logrus.Debugf("Whisper response: %+v", jsonResponse)
	textValue, ok := jsonResponse["text"].(string)
	if !ok {
		logrus.Debug("Could not find 'text' key or it's not a string in the jsonResponse")
		return "", fmt.Errorf("Whisper: unexpected JSON response: %v", jsonResponse)
	}
	return textValue, nil
}
