package lib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSeedDataAndPrimer(t *testing.T) {
	seedData, primer := GetSeedDataAndPrimer(Grammar)
	assert.Equal(t, grammarSeed, seedData)
	assert.Equal(t, "Text to correct:\n", primer)

	seedData, primer = GetSeedDataAndPrimer(Teacher)
	assert.Equal(t, teacherSeed, seedData)
	assert.Equal(t, "Text to correct:\n", primer)

	seedData, primer = GetSeedDataAndPrimer(Emili)
	assert.Equal(t, emiliSeed, seedData)
	assert.Equal(t, "טקסט לתיקון:\n", primer)

	seedData, primer = GetSeedDataAndPrimer(Vasilisa)
	assert.Equal(t, vasilisaSeed, seedData)
	assert.Equal(t, "Текст для исправления:\n", primer)

	seedData, primer = GetSeedDataAndPrimer(ChatGPT)
	assert.Equal(t, chatGPTSeed, seedData)
	assert.Equal(t, "", primer)

	seedData, primer = GetSeedDataAndPrimer("unknown")
	assert.Equal(t, chatGPTSeed, seedData)
	assert.Equal(t, "", primer)
}
