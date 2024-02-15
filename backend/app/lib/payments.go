package lib

func UserTotalCostKey(user string) string {
	return user + ":total_cost"
}

func UserTotalTokensKey(user string) string {
	return user + ":total_tokens"
}

func UserTotalAudioMinutesKey(user string) string {
	return user + ":total_audio_minutes"
}

func UserCurrentThreadPromptKey(user string) string {
	return user + ":current-thread-prompt-tokens"
}

func UserCurrentThreadKey(user string) string {
	return user + ":current-thread"
}
