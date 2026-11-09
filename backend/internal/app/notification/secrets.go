package notificationapp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	notifdom "github.com/Fleetdock/fleetdock/backend/internal/domain/notification"
	secretdom "github.com/Fleetdock/fleetdock/backend/internal/domain/secret"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

func isNotFound(err error) bool { return apperr.KindOf(err) == apperr.KindNotFound }

// SecretStore keeps channel secrets encrypted (secretsapp.Service).
type SecretStore interface {
	Put(ctx context.Context, ref string, kind secretdom.Kind, plaintext []byte) error
	Get(ctx context.Context, ref string) ([]byte, error)
	Delete(ctx context.Context, ref string) error
}

// sealedPrefix marks a config value that lives in the secret store; the rest
// of the value is the secret's ref.
const sealedPrefix = "secret:"

// channelSecretRef is where a channel's secret config value is stored. Only
// this exact ref is ever read for that channel and key, so a config value
// that names another secret is treated as plain text, never resolved.
func channelSecretRef(channelID uuid.UUID, key string) string {
	return "notification-channel/" + channelID.String() + "/" + key
}

func sealedValue(channelID uuid.UUID, key string) string {
	return sealedPrefix + channelSecretRef(channelID, key)
}

// seal moves the channel's secret config values (webhook URLs, the SMTP
// password and PagerDuty routing keys) into the secret store, leaving a reference in the config.
// Values already sealed for this channel are left as they are.
func (s *Service) seal(ctx context.Context, c *notifdom.Channel) error {
	if s.secrets == nil {
		return nil
	}
	for k, v := range c.Config {
		if !secretConfigKeys[k] || v == "" || v == sealedValue(c.ID, k) {
			continue
		}
		if err := s.secrets.Put(ctx, channelSecretRef(c.ID, k), secretdom.KindOther, []byte(v)); err != nil {
			return fmt.Errorf("store channel secret: %w", err)
		}
		c.Config[k] = sealedValue(c.ID, k)
	}
	return nil
}

// open returns the channel's config with its secret values decrypted, for
// delivery. The channel itself is not changed.
func (s *Service) open(ctx context.Context, c *notifdom.Channel) (map[string]string, error) {
	out := make(map[string]string, len(c.Config))
	for k, v := range c.Config {
		if s.secrets != nil && secretConfigKeys[k] && v == sealedValue(c.ID, k) {
			plain, err := s.secrets.Get(ctx, channelSecretRef(c.ID, k))
			if err != nil {
				return nil, fmt.Errorf("read channel secret %s: %w", k, err)
			}
			v = string(plain)
		}
		out[k] = v
	}
	return out, nil
}

// dropSecrets removes a deleted channel's secrets.
func (s *Service) dropSecrets(ctx context.Context, id uuid.UUID) {
	if s.secrets == nil {
		return
	}
	for k := range secretConfigKeys {
		if err := s.secrets.Delete(ctx, channelSecretRef(id, k)); err != nil && !isNotFound(err) {
			slog.Warn("notification: delete channel secret", "channel", id.String(), "key", k, "error", err.Error())
		}
	}
}

// SealExisting encrypts the secret values of channels saved before they were
// kept in the secret store. It runs at start-up and is a no-op once done.
func (s *Service) SealExisting(ctx context.Context) (int, error) {
	if s.secrets == nil {
		return 0, nil
	}
	channels, err := s.repo.ListChannels(ctx)
	if err != nil {
		return 0, err
	}
	sealed := 0
	for _, c := range channels {
		plain := false
		for k, v := range c.Config {
			if secretConfigKeys[k] && v != "" && v != sealedValue(c.ID, k) {
				plain = true
			}
		}
		if !plain {
			continue
		}
		if err := s.seal(ctx, c); err != nil {
			return sealed, err
		}
		if err := s.repo.UpdateChannel(ctx, c); err != nil {
			return sealed, err
		}
		sealed++
	}
	return sealed, nil
}
