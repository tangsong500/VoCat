package vowifisettings

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"vocat/internal/store"
)

const (
	MTUCompatibilityKey = "vowifi.mtu_compatibility"
	IMSAPNKey           = "vowifi.ims_apn"

	// DefaultIMSAPN is the APN a 3GPP ePDG expects for IMS when the carrier
	// does not provision a dedicated one.
	DefaultIMSAPN = "ims"
)

// ErrInvalidIMSAPN reports a malformed IMS APN before anything is persisted.
var ErrInvalidIMSAPN = errors.New("IMS APN must contain only letters, digits, dots, underscores, or hyphens")

// imsAPNPattern matches the modem/card APN charset so the value can be used
// as an IKE IDr FQDN without escaping.
var imsAPNPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,98}[A-Za-z0-9])?$`)

// MTUCompatibility is opt-in, including for existing installations.
func MTUCompatibility(ctx context.Context, database *store.Store) bool {
	if database == nil {
		return false
	}
	setting, err := database.AppSetting(ctx, MTUCompatibilityKey)
	if err != nil {
		return false
	}
	var value struct {
		Enabled bool `json:"enabled"`
	}
	return json.Unmarshal(setting.Value, &value) == nil && value.Enabled
}

func SetMTUCompatibility(ctx context.Context, database *store.Store, enabled bool) error {
	return ApplyUpdate(ctx, database, Update{MTUCompatibility: &enabled})
}

// IMSAPN returns the dedicated APN used by the VoWiFi (ePDG/IKE) tunnel.
// VoWiFi is an IMS service, so the tunnel must request the IMS APN and must not
// reuse the cellular data APN persisted on the device or card policy. The
// setting is optional and falls back to DefaultIMSAPN.
func IMSAPN(ctx context.Context, database *store.Store) string {
	if database == nil {
		return DefaultIMSAPN
	}
	setting, err := database.AppSetting(ctx, IMSAPNKey)
	if err != nil {
		return DefaultIMSAPN
	}
	var value struct {
		APN string `json:"apn"`
	}
	if json.Unmarshal(setting.Value, &value) != nil {
		return DefaultIMSAPN
	}
	apn, err := NormalizeIMSAPN(value.APN)
	if err != nil {
		return DefaultIMSAPN
	}
	return apn
}

// NormalizeIMSAPN trims and validates a configured IMS APN. An empty value
// restores DefaultIMSAPN; invalid values return ErrInvalidIMSAPN.
func NormalizeIMSAPN(apn string) (string, error) {
	apn = strings.TrimSpace(apn)
	if apn == "" {
		return DefaultIMSAPN, nil
	}
	if !imsAPNPattern.MatchString(apn) {
		return "", ErrInvalidIMSAPN
	}
	return apn, nil
}

// SetIMSAPN persists the dedicated IMS APN used by the VoWiFi tunnel. An empty
// value restores DefaultIMSAPN.
func SetIMSAPN(ctx context.Context, database *store.Store, apn string) error {
	return ApplyUpdate(ctx, database, Update{IMSAPN: &apn})
}

// Update carries the VoWiFi settings supplied by one request. Nil fields are
// left unchanged.
type Update struct {
	MTUCompatibility *bool
	IMSAPN           *string
}

// ApplyUpdate validates every supplied setting first, then persists them in a
// single transaction. A rejected request therefore never writes a valid field
// from the same request, and a persistence failure never leaves a partial
// update behind.
func ApplyUpdate(ctx context.Context, database *store.Store, update Update) error {
	values := make([]store.AppSetting, 0, 2)
	if update.MTUCompatibility != nil {
		value, err := json.Marshal(map[string]bool{"enabled": *update.MTUCompatibility})
		if err != nil {
			return err
		}
		values = append(values, store.AppSetting{Key: MTUCompatibilityKey, Value: value})
	}
	if update.IMSAPN != nil {
		apn, err := NormalizeIMSAPN(*update.IMSAPN)
		if err != nil {
			return err
		}
		value, err := json.Marshal(map[string]string{"apn": apn})
		if err != nil {
			return err
		}
		values = append(values, store.AppSetting{Key: IMSAPNKey, Value: value})
	}
	if len(values) == 0 {
		return nil
	}
	return database.UpsertAppSettings(ctx, values)
}
