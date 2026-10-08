package instanceapp

import (
	"strings"

	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/sshtunnel"
)

// SSHTunnelInput describes the SSH bastion an external instance is reached
// through. Password, PrivateKey and Passphrase are write-only.
type SSHTunnelInput struct {
	Host       string
	Port       int // 0 = 22
	Username   string
	AuthMethod string // password | key
	Password   string
	PrivateKey string
	Passphrase string
}

// buildTunnel validates in against the instance's current tunnel (prev, nil
// when there is none). It returns the new tunnel and, when new credentials
// were supplied, those credentials; nil credentials mean "keep the stored
// ones", which is only allowed while the bastion, user and auth method stay
// the same.
func buildTunnel(in *SSHTunnelInput, prev *instancedom.SSHTunnel) (*instancedom.SSHTunnel, *sshtunnel.Credentials, error) {
	host := strings.TrimSpace(in.Host)
	if host == "" {
		return nil, nil, apperr.Invalid("ssh_tunnel.host", "SSH host is required")
	}
	port := in.Port
	if port == 0 {
		port = 22
	}
	if port < 1 || port > 65535 {
		return nil, nil, apperr.Invalid("ssh_tunnel.port", "SSH port must be between 1 and 65535")
	}
	user := strings.TrimSpace(in.Username)
	if user == "" {
		return nil, nil, apperr.Invalid("ssh_tunnel.username", "SSH username is required")
	}
	auth := instancedom.SSHAuth(in.AuthMethod)
	if !auth.Valid() {
		return nil, nil, apperr.Invalid("ssh_tunnel.auth_method", "SSH auth method must be password or key")
	}

	t := &instancedom.SSHTunnel{Host: host, Port: port, User: user, Auth: auth}
	if prev != nil && prev.Host == host && prev.Port == port {
		// Same bastion: keep its pinned key.
		t.HostKey = prev.HostKey
	}

	var creds sshtunnel.Credentials
	secretField := "ssh_tunnel.password"
	if auth == instancedom.SSHAuthKey {
		creds = sshtunnel.Credentials{PrivateKey: in.PrivateKey, Passphrase: in.Passphrase}
		secretField = "ssh_tunnel.private_key"
	} else {
		creds = sshtunnel.Credentials{Password: in.Password}
	}

	if creds.Password == "" && strings.TrimSpace(creds.PrivateKey) == "" {
		// Keeping the stored secret is fine while it would be presented to the
		// same bastion as the same user; otherwise the caller must prove they
		// know it, just as with the admin password on a host change.
		if prev != nil && prev.SecretRef != nil && prev.Host == host && prev.User == user && prev.Auth == auth {
			t.SecretRef = prev.SecretRef
			return t, nil, nil
		}
		if auth == instancedom.SSHAuthKey {
			return nil, nil, apperr.Invalid(secretField, "SSH private key is required")
		}
		return nil, nil, apperr.Invalid(secretField, "SSH password is required")
	}
	if err := sshtunnel.ValidateAuth(string(auth), creds); err != nil {
		field := secretField
		if strings.Contains(err.Error(), "passphrase") {
			field = "ssh_tunnel.passphrase"
		}
		return nil, nil, apperr.Invalid(field, err.Error())
	}
	return t, &creds, nil
}

// sshPlan is the post-update SSH tunnel plus the secret-store side effects
// needed to get there, ordered around the row update by Update.
type sshPlan struct {
	tunnel   *instancedom.SSHTunnel // nil = tunnel untouched (or removed)
	remove   bool
	resetPin bool
	// rerouted: the path to the database changes (tunnel added, moved to
	// another bastion, or removed).
	rerouted    bool
	ref         string
	secret      []byte
	putBefore   bool // new secret: write before the row references it
	putAfter    bool // rotation of an existing secret: overwrite after
	deleteAfter bool // tunnel removed: delete after
}

// planSSH works out the instance's post-update SSH tunnel without touching
// the secret store.
func planSSH(inst *instancedom.Instance, in UpdateInput) (sshPlan, error) {
	if in.SSHTunnel == nil && !in.RemoveSSHTunnel && !in.ResetSSHHostKey {
		return sshPlan{}, nil
	}
	if inst.Kind != instancedom.KindExternal {
		return sshPlan{}, apperr.Invalid("ssh_tunnel", "SSH tunnels are only supported for external instances")
	}
	if in.RemoveSSHTunnel {
		if in.SSHTunnel != nil {
			return sshPlan{}, apperr.Invalid("ssh_tunnel", "cannot set and remove the SSH tunnel in the same request")
		}
		if inst.SSH == nil {
			return sshPlan{}, nil
		}
		p := sshPlan{remove: true, rerouted: true}
		if inst.SSH.SecretRef != nil {
			p.ref, p.deleteAfter = *inst.SSH.SecretRef, true
		}
		return p, nil
	}
	if in.SSHTunnel == nil {
		if inst.SSH == nil {
			return sshPlan{}, apperr.Invalid("reset_ssh_host_key", "this instance has no SSH tunnel")
		}
		return sshPlan{resetPin: true}, nil
	}

	t, creds, err := buildTunnel(in.SSHTunnel, inst.SSH)
	if err != nil {
		return sshPlan{}, err
	}
	if in.ResetSSHHostKey {
		t.HostKey = ""
	}
	p := sshPlan{
		tunnel:   t,
		rerouted: inst.SSH == nil || inst.SSH.Host != t.Host || inst.SSH.Port != t.Port,
	}
	if creds != nil {
		raw, err := creds.Marshal()
		if err != nil {
			return sshPlan{}, apperr.Internal(err)
		}
		ref := sshSecretRef(inst.ID)
		p.ref, p.secret = ref, raw
		if inst.SSH != nil && inst.SSH.SecretRef != nil && *inst.SSH.SecretRef == ref {
			p.putAfter = true
		} else {
			p.putBefore = true
		}
		t.SecretRef = &ref
	}
	return p, nil
}
