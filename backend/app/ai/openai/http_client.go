package openai

import (
	"net/http"
	"time"
)

const TIMEOUT = 60 * time.Second

var HTTP_CLIENT = &http.Client{
	Timeout: TIMEOUT,
}
