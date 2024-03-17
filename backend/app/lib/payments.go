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

func UserCurrentThreadPromptKey(user string, topic string) string {
	if topic != "" && topic != "0" {
		return user + ":" + topic + ":current-thread-prompt-tokens"
	}
	return user + ":current-thread-prompt-tokens"
}

func UserCurrentThreadKey(user string, topic string) string {
	if topic != "" && topic != "0" {
		return user + ":" + topic + ":current-thread"
	}
	return user + ":current-thread"
}
