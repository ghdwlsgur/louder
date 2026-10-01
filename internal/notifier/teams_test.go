package notifier

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestTeamsWebhookSendsAdaptiveCard(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", request.Method)
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", request.Header.Get("Content-Type"))
		}
		var envelope struct {
			Type        string `json:"type"`
			Attachments []struct {
				ContentType string `json:"contentType"`
				Content     struct {
					Type    string `json:"type"`
					Version string `json:"version"`
					Body    []struct {
						Type  string `json:"type"`
						Text  string `json:"text"`
						Facts []struct {
							Title string `json:"title"`
							Value string `json:"value"`
						} `json:"facts"`
					} `json:"body"`
				} `json:"content"`
			} `json:"attachments"`
		}
		if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil {
			t.Errorf("decode request body: %v", err)
			return nil, err
		}
		if envelope.Type != "message" || len(envelope.Attachments) != 1 || envelope.Attachments[0].ContentType != "application/vnd.microsoft.card.adaptive" {
			t.Errorf("envelope = %#v, want one Teams Adaptive Card attachment", envelope)
			return response(request, http.StatusAccepted, ""), nil
		}
		card := envelope.Attachments[0].Content
		if card.Type != "AdaptiveCard" || card.Version != "1.2" || len(card.Body) < 2 || card.Body[0].Text != "Budget threshold reached" || card.Body[1].Text != "SRE has reached 80% of its monthly budget." {
			t.Errorf("card = %#v, want notification title and summary", card)
		}
		if len(card.Body) < 3 || len(card.Body[2].Facts) != 4 || card.Body[2].Facts[0].Title != "Type" || card.Body[2].Facts[1].Title != "Severity" || card.Body[2].Facts[2].Title != "Currency" || card.Body[2].Facts[3].Title != "Spend" {
			t.Errorf("card facts = %#v, want fixed fields followed by sorted details", card.Body)
		}
		return response(request, http.StatusAccepted, ""), nil
	})}

	teams, err := NewTeamsWebhookNotifier("https://tenant.example.test/workflow/secret", client)
	if err != nil {
		t.Fatalf("NewTeamsWebhookNotifier() error = %v", err)
	}
	notification := Notification{
		Type: "BudgetThreshold", Severity: "Warning", Title: "Budget threshold reached",
		Summary: "SRE has reached 80% of its monthly budget.",
		Details: map[string]string{"Currency": "USD", "Spend": "800.00"},
	}
	if err := teams.Send(t.Context(), notification); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestTeamsWebhookSanitizesHTTPFailures(t *testing.T) {
	const secretURL = "https://example.invalid/webhook/secret-token"
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return response(request, http.StatusInternalServerError, "upstream echoed "+secretURL), nil
	})}
	teams, err := NewTeamsWebhookNotifier(secretURL, client)
	if err != nil {
		t.Fatal(err)
	}
	err = teams.Send(t.Context(), Notification{Title: "test"})
	if !errors.Is(err, ErrDeliveryFailed) {
		t.Fatalf("Send() error = %v, want ErrDeliveryFailed", err)
	}
	if strings.Contains(err.Error(), secretURL) || strings.Contains(err.Error(), "upstream echoed") {
		t.Errorf("Send() error leaked sensitive data: %v", err)
	}
}

func TestTeamsWebhookSanitizesTransportErrors(t *testing.T) {
	const secretURL = "https://example.invalid/webhook/secret-token"
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection failed for " + secretURL)
	})}
	teams, err := NewTeamsWebhookNotifier(secretURL, client)
	if err != nil {
		t.Fatal(err)
	}
	err = teams.Send(t.Context(), Notification{Title: "test"})
	if !errors.Is(err, ErrDeliveryFailed) {
		t.Fatalf("Send() error = %v, want ErrDeliveryFailed", err)
	}
	if strings.Contains(err.Error(), secretURL) {
		t.Errorf("Send() error leaked webhook URL: %v", err)
	}
}

func TestTeamsWebhookDoesNotFollowRedirects(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusTemporaryRedirect,
			Header:     http.Header{"Location": []string{"https://attacker.example.test/collect"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})}
	teams, err := NewTeamsWebhookNotifier("https://tenant.example.test/workflow/secret", client)
	if err != nil {
		t.Fatal(err)
	}
	if err := teams.Send(t.Context(), Notification{Title: "test"}); !errors.Is(err, ErrDeliveryFailed) {
		t.Fatalf("Send() error = %v, want ErrDeliveryFailed", err)
	}
	if calls != 1 {
		t.Errorf("HTTP calls = %d, want one call without following redirect", calls)
	}
}

func TestTeamsWebhookRejectsInvalidURLs(t *testing.T) {
	for _, endpoint := range []string{"", "http://example.test/hook", "https://user:pass@example.test/hook", "https://example.test/hook#fragment", "://invalid"} {
		if _, err := NewTeamsWebhookNotifier(endpoint, nil); !errors.Is(err, ErrInvalidWebhookURL) {
			t.Errorf("NewTeamsWebhookNotifier(%q) error = %v, want ErrInvalidWebhookURL", endpoint, err)
		}
	}
}

func TestFakeNotifierRecordsNotification(t *testing.T) {
	fake := &FakeNotifier{}
	want := Notification{Type: "BudgetThreshold", Severity: "Warning", Title: "Budget threshold reached"}
	if err := fake.Send(t.Context(), want); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if len(fake.Notifications) != 1 || !reflect.DeepEqual(fake.Notifications[0], want) {
		t.Errorf("notifications = %#v, want %#v", fake.Notifications, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func response(request *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}
}
