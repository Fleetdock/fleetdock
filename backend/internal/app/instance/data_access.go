package instanceapp

import (
	"strings"

	"github.com/google/uuid"

	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	secretdom "github.com/Fleetdock/fleetdock/backend/internal/domain/secret"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// dataSecretRef is the deterministic secret reference for an instance's
// dedicated data-access login.
func dataSecretRef(id uuid.UUID) string { return "instance/" + id.String() + "/data" }

func dataSecretKind(inst *instancedom.Instance) secretdom.Kind {
	if inst.Engine == instancedom.EnginePostgres {
		return secretdom.KindPostgresUser
	}
	return secretdom.KindMariaDBUser
}

// DataAccessInput selects the login used for the table browser, SQL console,
// exports and imports. Nil fields are untouched on update.
type DataAccessInput struct {
	Mode     *string // admin | login | managed
	Username *string // mode "login" only
	Password *string // mode "login" only; write-only
}

// dataAccessPlan is the post-update data access setting plus the secret
// store side effects, ordered around the row update like credentialPlan.
type dataAccessPlan struct {
	mode        *instancedom.DataAccess
	login       *instancedom.Credentials // nil = data login untouched
	ref         string
	password    []byte
	putBefore   bool
	putAfter    bool
	deleteAfter bool
}

// planDataAccess works out an instance's data access setting without
// touching the secret store. rerouted reports that the update sends
// connections somewhere new (host or SSH tunnel), in which case a stored
// data password must be re-entered, as the admin password must.
func planDataAccess(inst *instancedom.Instance, in DataAccessInput, rerouted bool) (dataAccessPlan, error) {
	current := inst.DataAccessOrDefault()
	mode := current
	if in.Mode != nil {
		mode = instancedom.DataAccess(strings.TrimSpace(*in.Mode))
		if !mode.Valid() {
			return dataAccessPlan{}, apperr.Invalid("data_access", "data_access must be admin, login or managed")
		}
	}
	var p dataAccessPlan
	if mode != current {
		p.mode = &mode
	}

	if mode != instancedom.DataAccessLogin {
		if in.Username != nil && strings.TrimSpace(*in.Username) != "" || in.Password != nil && *in.Password != "" {
			return dataAccessPlan{}, apperr.Invalid("data_username", "a data login is only used when data_access is login")
		}
		if inst.DataUsername != nil || inst.DataSecretRef != nil {
			p.login = &instancedom.Credentials{}
			if inst.DataSecretRef != nil {
				p.ref, p.deleteAfter = *inst.DataSecretRef, true
			}
		}
		return p, nil
	}

	username := ""
	if inst.DataUsername != nil {
		username = *inst.DataUsername
	}
	renamed := false
	if in.Username != nil {
		u := strings.TrimSpace(*in.Username)
		renamed = u != username
		username = u
	}
	if username == "" {
		return dataAccessPlan{}, apperr.Invalid("data_username", "a username is required for a data login")
	}
	if in.Password == nil || *in.Password == "" {
		switch {
		case inst.DataSecretRef == nil:
			return dataAccessPlan{}, apperr.Invalid("data_password", "a password is required for a data login")
		case renamed:
			return dataAccessPlan{}, apperr.Invalid("data_password", "re-enter the data login password when changing its username")
		case rerouted:
			return dataAccessPlan{}, apperr.Invalid("data_password", "re-enter the data login password when changing the host or SSH tunnel")
		}
		if in.Username != nil {
			p.login = &instancedom.Credentials{Username: &username, RootSecretRef: inst.DataSecretRef}
		}
		return p, nil
	}

	ref := dataSecretRef(inst.ID)
	p.ref, p.password = ref, []byte(*in.Password)
	if inst.DataSecretRef != nil && *inst.DataSecretRef == ref {
		p.putAfter = true
	} else {
		p.putBefore = true
	}
	p.login = &instancedom.Credentials{Username: &username, RootSecretRef: &ref}
	return p, nil
}
