package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type TeamsWebhookNotifier struct {
	endpoint string
	client   *http.Client
}

func NewTeamsWebhookNotifier(endpoint string, client *http.Client) (*TeamsWebhookNotifier, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, ErrInvalidWebhookURL
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &TeamsWebhookNotifier{endpoint: parsed.String(), client: &clientCopy}, nil
}

func (n *TeamsWebhookNotifier) Send(ctx context.Context, notification Notification) error {
	payload, err := json.Marshal(adaptiveMessage(notification))
	if err != nil {
		return ErrDeliveryFailed
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, n.endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return ErrDeliveryFailed
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := n.client.Do(request)
	if err != nil {
		return ErrDeliveryFailed
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return ErrDeliveryFailed
	}
	return nil
}

type teamsMessage struct {
	Type        string            `json:"type"`
	Attachments []teamsAttachment `json:"attachments"`
}

type teamsAttachment struct {
	ContentType string       `json:"contentType"`
	ContentURL  *string      `json:"contentUrl"`
	Content     adaptiveCard `json:"content"`
}

type adaptiveCard struct {
	Schema  string            `json:"$schema"`
	Type    string            `json:"type"`
	Version string            `json:"version"`
	Body    []adaptiveElement `json:"body"`
}

type adaptiveElement struct {
	Type   string         `json:"type"`
	Text   string         `json:"text,omitempty"`
	Weight string         `json:"weight,omitempty"`
	Size   string         `json:"size,omitempty"`
	Wrap   bool           `json:"wrap,omitempty"`
	Facts  []adaptiveFact `json:"facts,omitempty"`
}

type adaptiveFact struct {
	Title string `json:"title"`
	Value string `json:"value"`
}

func adaptiveMessage(notification Notification) teamsMessage {
	body := []adaptiveElement{{Type: "TextBlock", Text: notification.Title, Weight: "Bolder", Size: "Medium", Wrap: true}}
	if notification.Summary != "" {
		body = append(body, adaptiveElement{Type: "TextBlock", Text: notification.Summary, Wrap: true})
	}
	facts := []adaptiveFact{{Title: "Type", Value: notification.Type}, {Title: "Severity", Value: notification.Severity}}
	keys := make([]string, 0, len(notification.Details))
	for key := range notification.Details {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		facts = append(facts, adaptiveFact{Title: key, Value: notification.Details[key]})
	}
	body = append(body, adaptiveElement{Type: "FactSet", Facts: facts})
	return teamsMessage{
		Type: "message",
		Attachments: []teamsAttachment{{
			ContentType: "application/vnd.microsoft.card.adaptive",
			ContentURL:  nil,
			Content: adaptiveCard{
				Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
				Type:    "AdaptiveCard",
				Version: "1.2",
				Body:    body,
			},
		}},
	}
}
