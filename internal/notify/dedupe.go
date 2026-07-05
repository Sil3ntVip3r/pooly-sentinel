package notify

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

func deliveryKey(incident storage.IncidentRecord, receiverID string, event Event) string {
	transition := transitionTime(incident).Format(time.RFC3339Nano)
	raw := strings.Join([]string{
		incident.ID,
		receiverID,
		string(event),
		incident.Severity,
		incident.Status,
		transition,
	}, "\x00")
	sum := sha256.Sum256([]byte(raw))
	return "del_" + hex.EncodeToString(sum[:12])
}

func deliveryID(key string, attempt int) string {
	if attempt < 1 {
		attempt = 1
	}
	return fmt.Sprintf("%s_a%03d", key, attempt)
}

func nextAttempt(deliveries []storage.NotificationDeliveryRecord, key string) (int, bool) {
	prefix := key + "_a"
	attempt := 1
	delivered := false
	for _, delivery := range deliveries {
		if !strings.HasPrefix(delivery.ID, prefix) {
			continue
		}
		if delivery.Status == string(StatusDelivered) {
			delivered = true
		}
		if delivery.Attempt >= attempt {
			attempt = delivery.Attempt + 1
		}
	}
	return attempt, delivered
}

func transitionTime(incident storage.IncidentRecord) time.Time {
	if incident.LastTransition != nil && !incident.LastTransition.IsZero() {
		return incident.LastTransition.UTC()
	}
	if incident.ResolvedAt != nil && !incident.ResolvedAt.IsZero() {
		return incident.ResolvedAt.UTC()
	}
	if !incident.LastSeen.IsZero() {
		return incident.LastSeen.UTC()
	}
	return incident.FirstSeen.UTC()
}
