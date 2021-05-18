package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"
)

const (
	AlertNotification = 0
	InfoNotification  = 1
)

type SlackRequestBody struct {
	Text string `json:"text"`
}

// SendSlackNotification will post to an 'Incoming Webook' url setup in Slack Apps. It accepts
// some text and the slack channel is saved within Slack.
func SendSlackNotification(msg string, notiType int) error {
	var webhookURL string
	if notiType == AlertNotification {
		webhookURL = os.Getenv("ALERT_WEBHOOK_URL")
	} else if notiType == InfoNotification {
		webhookURL = os.Getenv("INFO_WEBHOOK_URL")
	} else {
		return errors.New("Notification type is not supported")
	}

	slackBody, _ := json.Marshal(SlackRequestBody{Text: msg})
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewBuffer(slackBody))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	if buf.String() != "ok" {
		return errors.New("Non-ok response returned from Slack")
	}
	return nil
}
