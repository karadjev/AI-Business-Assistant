package auth

// CommercialAllowance reports a safe scope label without echoing database detail.
func CommercialAllowance(scope string) error {
	messages := map[string]string{
		"paused":   "Подписка приостановлена",
		"staff":    "Лимит сотрудников исчерпан",
		"channels": "Лимит активных каналов исчерпан",
		"tasks":    "Месячный лимит задач исчерпан",
		"agents":   "Месячный лимит используемых агентов исчерпан",
		"ai":       "Месячный лимит расходов AI исчерпан",
	}
	message := messages[scope]
	if message == "" {
		message = "Лимит подписки исчерпан"
	}
	return Failure(429, "COMMERCIAL_LIMIT", message)
}
