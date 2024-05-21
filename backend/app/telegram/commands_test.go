package telegram

import (
	"testing"
)

// test func IsCreateImageCommand(prompt string) bool {
// prompts := []string{
// 	"Can you create an image of a sunset?",
// 	"Draw, please, an image of a cat",
// 	"I'd like a picture! Of a mountain",
// 	"Imagine this image: a futuristic city",
// 	"Cam you creete an imege of a sunset?",
// }

func TestIsCreateImageCommandTrue(t *testing.T) {
	prompts := []string{
		"Can you create a drawing of a sunset?",
		"Can you create an image of a sunset?",
		"Draw, please, an image of a cat",
		"I'd like a picture! Of a mountain",
		"Imagine this image: a futuristic city",
		"Cam you creete an image of a sunset?",
		"Sketch a picture of a cat",
	}

	for _, prompt := range prompts {
		if !IsCreateImageCommand(prompt) {
			t.Errorf("IsCreateImageCommand(%s) = false; want true", prompt)
		}
	}
}

func TestIsCreateImageCommandFalse(t *testing.T) {
	prompts := []string{
		"Imagene, please, an article about a cat",
		"I'd like a video! Of a mountain",
		"Don't you think this image is beautiful?",
		"Can we talk about what you think about this image?",
	}

	for _, prompt := range prompts {
		if IsCreateImageCommand(prompt) {
			t.Errorf("IsCreateImageCommand(%s) = true; want false", prompt)
		}
	}
}
