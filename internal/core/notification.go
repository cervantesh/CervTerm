package core

import "unicode/utf8"

const (
	MaxNotificationTitleBytes = 256
	MaxNotificationBodyBytes  = 4096
	MaxNotificationRequests   = 32
)

// NotificationRequest is bounded terminal-originated metadata. It never invokes
// a platform API; frontend policy decides whether a detached request may become
// an OS notification.
type NotificationRequest struct {
	Sequence uint64
	Title    string
	Body     string
}

type notificationStore struct {
	entries []NotificationRequest
	next    uint64
}

func validNotificationText(value string, limit int) bool {
	if len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func (t *Terminal) RequestNotification(title, body string) bool {
	if body == "" || !validNotificationText(title, MaxNotificationTitleBytes) || !validNotificationText(body, MaxNotificationBodyBytes) {
		return false
	}
	if t.notifications.entries == nil {
		t.notifications.entries = make([]NotificationRequest, 0, MaxNotificationRequests)
	}
	t.notifications.next++
	request := NotificationRequest{Sequence: t.notifications.next, Title: title, Body: body}
	if len(t.notifications.entries) == MaxNotificationRequests {
		copy(t.notifications.entries, t.notifications.entries[1:])
		t.notifications.entries[len(t.notifications.entries)-1] = request
	} else {
		t.notifications.entries = append(t.notifications.entries, request)
	}
	return true
}

// NotificationRequestsSince copies the retained suffix after sequence. latest
// always reports the current monotonic sequence so a bounded observer can
// acknowledge overwritten requests explicitly through truncated.
func (t *Terminal) NotificationRequestsSince(sequence uint64, dst []NotificationRequest) (requests []NotificationRequest, latest uint64, truncated bool) {
	latest = t.notifications.next
	if latest == 0 || sequence >= latest {
		return dst[:0], latest, false
	}
	oldest := latest - uint64(len(t.notifications.entries)) + 1
	if sequence+1 < oldest {
		truncated = true
		sequence = oldest - 1
	}
	dst = dst[:0]
	for _, request := range t.notifications.entries {
		if request.Sequence > sequence {
			dst = append(dst, request)
		}
	}
	return dst, latest, truncated
}
