// Package instanceapp holds the application use cases for instances —
// managed (on registered servers) and external (existing databases anywhere,
// e.g. databases you already run elsewhere).
package instanceapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/app/dbtarget"
	operationapp "github.com/Fleetdock/fleetdock/backend/internal/app/operation"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	jobdom "github.com/Fleetdock/fleetdock/backend/internal/domain/job"
	secretdom "github.com/Fleetdock/fleetdock/backend/internal/domain/secret"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/sshtunnel"
)

// Secrets is the secret store surface this service needs.
type Secrets interface {
	Put(ctx context.Context, ref string, kind secretdom.Kind, plaintext []byte) error
	Get(ctx context.Context, ref string) ([]byte, error)
	Delete(ctx context.Context, ref string) error
}

// RegisterInput is the command to register an instance.
type RegisterInput struct {
	// Kind: "managed" (default) requires ServerID; "external" requires Host.
	Kind          string
	ServerID      string
	Host          string
	Name          string
	Engine        string
	EngineVersion string
	Port          int
	Username      string
	Password      string // write-only; stored encrypted
	TLSMode       string // disable | prefer (default) | require | verify-full
	Labels        map[string]string
	Tags          []string
	// SSHTunnel, when set, reaches an external instance through an SSH
	// bastion; Host is then resolved on the bastion.
	SSHTunnel *SSHTunnelInput
	// DataAccess selects the login for the table browser and console
	// (default: the admin login).
	DataAccess DataAccessInput
}

// ListParams are filter + pagination inputs for listing instances.
type ListParams struct {
	ServerID string
	Kind     string
	Limit    int
	Offset   int
	Scope    *authz.ReadSet
}

// ListResult is a page of instances with pagination metadata.
type ListResult struct {
	Items  []*instancedom.Instance
	Total  int
	Limit  int
	Offset int
}

const (
	defaultLimit = 20
	maxLimit     = 100
)

// Service implements instance use cases.
type Service struct {
	repo      instancedom.Repository
	databases databasedom.Repository
	secrets   Secrets
	ops       *operationapp.Service
}

// NewService wires the service.
func NewService(repo instancedom.Repository, databases databasedom.Repository, secrets Secrets, ops *operationapp.Service) *Service {
	return &Service{repo: repo, databases: databases, secrets: secrets, ops: ops}
}

// Register validates input and persists a new instance (managed or external).
func (s *Service) Register(ctx context.Context, in RegisterInput) (*instancedom.Instance, error) {
	var username *string
	if in.Username != "" {
		username = &in.Username
	}
	if in.Password != "" && in.Username == "" {
		return nil, apperr.Invalid("username", "username is required when a password is provided")
	}

	var (
		inst *instancedom.Instance
		err  error
	)
	switch in.Kind {
	case "", string(instancedom.KindManaged):
		serverID, perr := uuid.Parse(in.ServerID)
		if perr != nil {
			return nil, apperr.Invalid("server_id", "server_id must be a valid UUID")
		}
		inst, err = instancedom.NewManaged(serverID, in.Name, instancedom.Engine(in.Engine), in.EngineVersion, in.Port, username, in.Labels, in.Tags)
	case string(instancedom.KindExternal):
		inst, err = instancedom.NewExternal(in.Name, instancedom.Engine(in.Engine), in.EngineVersion, in.Host, in.Port, username, in.Labels, in.Tags)
	default:
		return nil, apperr.Invalid("kind", "kind must be managed or external")
	}
	if err != nil {
		return nil, err
	}
	if !engine.ValidTLSMode(in.TLSMode) {
		return nil, apperr.Invalid("tls_mode", "tls_mode must be disable, prefer, require or verify-full")
	}
	inst.TLSMode = in.TLSMode

	var sshCreds *sshtunnel.Credentials
	if in.SSHTunnel != nil {
		if inst.Kind != instancedom.KindExternal {
			return nil, apperr.Invalid("ssh_tunnel", "SSH tunnels are only supported for external instances")
		}
		inst.SSH, sshCreds, err = buildTunnel(in.SSHTunnel, nil)
		if err != nil {
			return nil, err
		}
		if err := dbtarget.CheckHost(ctx, inst.SSH.Host, inst.SSH.Port, "ssh_tunnel.host"); err != nil {
			return nil, err
		}
	}
	// Behind a tunnel the database host is resolved on the bastion (often
	// 127.0.0.1 there), so the policy applies to the bastion only.
	if inst.Kind == instancedom.KindExternal && inst.Host != nil && inst.SSH == nil {
		if err := dbtarget.CheckHost(ctx, *inst.Host, inst.Port, "host"); err != nil {
			return nil, err
		}
	}

	data, err := planDataAccess(inst, in.DataAccess, false)
	if err != nil {
		return nil, err
	}
	if data.mode != nil {
		inst.DataAccess = *data.mode
	}

	if in.Password != "" {
		ref := rootSecretRef(inst.ID)
		if err := s.secrets.Put(ctx, ref, secretdom.KindMariaDBRoot, []byte(in.Password)); err != nil {
			return nil, err
		}
		inst.RootSecretRef = &ref
	}
	if data.login != nil {
		if err := s.secrets.Put(ctx, data.ref, dataSecretKind(inst), data.password); err != nil {
			if inst.RootSecretRef != nil {
				_ = s.secrets.Delete(ctx, *inst.RootSecretRef)
			}
			return nil, err
		}
		inst.DataUsername, inst.DataSecretRef = data.login.Username, data.login.RootSecretRef
	}
	if sshCreds != nil {
		ref := sshSecretRef(inst.ID)
		raw, err := sshCreds.Marshal()
		if err != nil {
			return nil, apperr.Internal(err)
		}
		if err := s.secrets.Put(ctx, ref, secretdom.KindSSHKey, raw); err != nil {
			if inst.RootSecretRef != nil {
				_ = s.secrets.Delete(ctx, *inst.RootSecretRef)
			}
			if inst.DataSecretRef != nil {
				_ = s.secrets.Delete(ctx, *inst.DataSecretRef)
			}
			return nil, err
		}
		inst.SSH.SecretRef = &ref
	}

	if err := s.repo.Create(ctx, inst); err != nil {
		if inst.RootSecretRef != nil {
			_ = s.secrets.Delete(ctx, *inst.RootSecretRef)
		}
		if inst.DataSecretRef != nil {
			_ = s.secrets.Delete(ctx, *inst.DataSecretRef)
		}
		if inst.SSH != nil && inst.SSH.SecretRef != nil {
			_ = s.secrets.Delete(ctx, *inst.SSH.SecretRef)
		}
		return nil, err
	}
	if inst.RootSecretRef != nil {
		if err := s.repo.SetRootSecretRef(ctx, inst.ID, *inst.RootSecretRef); err != nil {
			return nil, err
		}
	}
	return inst, nil
}

// ProvisionInput is the command to provision a new managed instance as a
// Docker container on a registered server.
type ProvisionInput struct {
	ServerID      string
	Name          string
	Engine        string
	EngineVersion string
	Port          int
	CreatedBy     *uuid.UUID
}

// Provision creates a managed instance and enqueues a job for the server's
// agent to launch it as a Docker container with a generated root password.
func (s *Service) Provision(ctx context.Context, in ProvisionInput) (*instancedom.Instance, *jobdom.Job, error) {
	serverID, err := uuid.Parse(in.ServerID)
	if err != nil {
		return nil, nil, apperr.Invalid("server_id", "server_id must be a valid UUID")
	}
	eng := instancedom.Engine(in.Engine)
	if eng == "" {
		eng = instancedom.EngineMariaDB
	}
	inst, err := instancedom.NewProvisioned(serverID, in.Name, eng, in.EngineVersion, in.Port)
	if err != nil {
		return nil, nil, err
	}

	password, err := genPassword()
	if err != nil {
		return nil, nil, apperr.Internal(err)
	}
	ref := rootSecretRef(inst.ID)
	if err := s.secrets.Put(ctx, ref, secretdom.KindMariaDBRoot, []byte(password)); err != nil {
		return nil, nil, err
	}
	inst.RootSecretRef = &ref

	if err := s.repo.Create(ctx, inst); err != nil {
		_ = s.secrets.Delete(ctx, ref)
		return nil, nil, err
	}
	if err := s.repo.SetRootSecretRef(ctx, inst.ID, ref); err != nil {
		return nil, nil, err
	}

	job, err := s.ops.Create(ctx, jobdom.TypeProvisionInstance, "instance", &inst.ID, &serverID,
		operationapp.Params{InstanceID: inst.ID.String(), Image: string(eng), Version: in.EngineVersion}, in.CreatedBy)
	if err != nil {
		_ = s.repo.SetStatus(ctx, inst.ID, instancedom.StatusError)
		return nil, nil, err
	}
	return inst, job, nil
}

// Lifecycle enqueues a start/stop/restart operation for a provisioned instance.
func (s *Service) Lifecycle(ctx context.Context, id, action string, createdBy *uuid.UUID) (*jobdom.Job, error) {
	inst, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !inst.Provisioned() {
		return nil, apperr.Invalid("id", "only provisioned instances can be started/stopped from here")
	}
	var typ jobdom.Type
	switch action {
	case "start":
		typ = jobdom.TypeStartInstance
	case "stop":
		typ = jobdom.TypeStopInstance
	case "restart":
		typ = jobdom.TypeRestartInstance
	default:
		return nil, apperr.Invalid("action", "action must be start, stop or restart")
	}
	return s.ops.Create(ctx, typ, "instance", &inst.ID, inst.ServerID,
		operationapp.Params{InstanceID: inst.ID.String()}, createdBy)
}

// Get returns an instance by id.
func (s *Service) Get(ctx context.Context, id string) (*instancedom.Instance, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.GetByID(ctx, uid)
}

// UpdateInput is a partial change to an instance. Nil fields are untouched.
//
// Credential semantics, which are the subtle part:
//   - Username: "" clears the admin credentials entirely (and deletes the
//     stored password). Any other value renames the admin user in place.
//   - Password: "" removes the stored password but keeps the username. Any
//     other value sets or rotates it — the secret ref is deterministic, so a
//     rotation overwrites rather than orphaning the old ciphertext.
type UpdateInput struct {
	Name     *string
	Host     *string
	Port     *int
	TLSMode  *string
	Username *string
	Password *string
	// SSHTunnel sets or changes the SSH tunnel. Its secret fields may be left
	// empty to keep the stored SSH credentials, unless the bastion host, user
	// or auth method changes. RemoveSSHTunnel drops the tunnel;
	// ResetSSHHostKey forgets the pinned host key so the next connection
	// pins whatever the bastion presents.
	SSHTunnel       *SSHTunnelInput
	RemoveSSHTunnel bool
	ResetSSHHostKey bool
	// DataAccess changes the login used for the table browser and console.
	DataAccess DataAccessInput
}

// sshSecretRef is the deterministic secret reference for an instance's SSH
// tunnel credentials.
func sshSecretRef(id uuid.UUID) string { return "instance/" + id.String() + "/ssh" }

// rootSecretRef is the deterministic secret reference for an instance's admin
// password. Deterministic so rotation is an upsert, not a leak.
func rootSecretRef(id uuid.UUID) string { return "instance/" + id.String() + "/root" }

// Update applies a partial change to an instance's mutable metadata and
// credentials, and returns the refreshed record.
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*instancedom.Instance, error) {
	inst, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	var f instancedom.UpdateFields
	hostChanged := false

	if in.Name != nil {
		name, err := instancedom.ValidateName(*in.Name)
		if err != nil {
			return nil, err
		}
		f.Name = &name
	}

	if in.Host != nil {
		if inst.Kind != instancedom.KindExternal {
			return nil, apperr.Invalid("host", "host can only be set on external instances")
		}
		host := strings.TrimSpace(*in.Host)
		if host == "" {
			return nil, apperr.Invalid("host", "host is required for external instances")
		}
		// Moving the instance to another host would otherwise hand the stored
		// admin password to whatever answers there. Make the caller prove they
		// know it (or drop the stored password) instead.
		if inst.Host == nil || host != *inst.Host {
			hostChanged = true
			if inst.RootSecretRef != nil && in.Password == nil &&
				(in.Username == nil || strings.TrimSpace(*in.Username) != "") {
				return nil, apperr.Invalid("password", "re-enter the admin password when changing the host")
			}
		}
		f.Host = &host
	}

	if in.Port != nil {
		if inst.Provisioned() {
			return nil, apperr.Invalid("port", "the port of a provisioned instance is fixed by its container")
		}
		if err := instancedom.ValidatePort(*in.Port); err != nil {
			return nil, err
		}
		f.Port = in.Port
	}

	if in.TLSMode != nil {
		if *in.TLSMode == "" || !engine.ValidTLSMode(*in.TLSMode) {
			return nil, apperr.Invalid("tls_mode", "tls_mode must be disable, prefer, require or verify-full")
		}
		f.TLSMode = in.TLSMode
	}

	ssh, err := planSSH(inst, in)
	if err != nil {
		return nil, err
	}
	f.SSH, f.RemoveSSH, f.ResetSSHHostKey = ssh.tunnel, ssh.remove, ssh.resetPin
	if ssh.tunnel != nil {
		if err := dbtarget.CheckHost(ctx, ssh.tunnel.Host, ssh.tunnel.Port, "ssh_tunnel.host"); err != nil {
			return nil, err
		}
	}
	// Re-routing the connection (adding, moving or removing the bastion)
	// would hand the stored admin password to whatever answers on the new
	// route, exactly like a host change.
	if ssh.rerouted && inst.RootSecretRef != nil && in.Password == nil &&
		(in.Username == nil || strings.TrimSpace(*in.Username) != "") {
		return nil, apperr.Invalid("password", "re-enter the admin password when changing the SSH tunnel")
	}
	tunnelled := (inst.SSH != nil && !ssh.remove) || ssh.tunnel != nil

	if inst.Kind == instancedom.KindExternal && !tunnelled && (f.Host != nil || f.Port != nil || ssh.remove) {
		host, port := "", inst.Port
		if inst.Host != nil {
			host = *inst.Host
		}
		if f.Host != nil {
			host = *f.Host
		}
		if f.Port != nil {
			port = *f.Port
		}
		if err := dbtarget.CheckHost(ctx, host, port, "host"); err != nil {
			return nil, err
		}
	}

	plan, err := s.planCredentials(inst, in)
	if err != nil {
		return nil, err
	}
	f.Credentials = plan.creds

	data, err := planDataAccess(inst, in.DataAccess, hostChanged || ssh.rerouted)
	if err != nil {
		return nil, err
	}
	f.DataAccess, f.DataLogin = data.mode, data.login

	// A brand-new secret must exist before the row can reference it (FK);
	// everything else touching the secret store waits until the row update
	// has succeeded, so a failed update never leaves the row pointing at a
	// deleted or replaced password.
	if plan.putBefore {
		if err := s.secrets.Put(ctx, plan.ref, secretdom.KindMariaDBRoot, plan.password); err != nil {
			return nil, err
		}
	}
	if ssh.putBefore {
		if err := s.secrets.Put(ctx, ssh.ref, secretdom.KindSSHKey, ssh.secret); err != nil {
			if plan.putBefore {
				_ = s.secrets.Delete(ctx, plan.ref)
			}
			return nil, err
		}
	}
	if data.putBefore {
		if err := s.secrets.Put(ctx, data.ref, dataSecretKind(inst), data.password); err != nil {
			if plan.putBefore {
				_ = s.secrets.Delete(ctx, plan.ref)
			}
			if ssh.putBefore {
				_ = s.secrets.Delete(ctx, ssh.ref)
			}
			return nil, err
		}
	}
	if err := s.repo.Update(ctx, inst.ID, f); err != nil {
		if plan.putBefore {
			_ = s.secrets.Delete(ctx, plan.ref)
		}
		if ssh.putBefore {
			_ = s.secrets.Delete(ctx, ssh.ref)
		}
		if data.putBefore {
			_ = s.secrets.Delete(ctx, data.ref)
		}
		return nil, err
	}
	switch {
	case data.putAfter:
		if err := s.secrets.Put(ctx, data.ref, dataSecretKind(inst), data.password); err != nil {
			return nil, err
		}
	case data.deleteAfter:
		if err := s.secrets.Delete(ctx, data.ref); err != nil {
			return nil, err
		}
	}
	switch {
	case ssh.putAfter:
		if err := s.secrets.Put(ctx, ssh.ref, secretdom.KindSSHKey, ssh.secret); err != nil {
			return nil, err
		}
	case ssh.deleteAfter:
		if err := s.secrets.Delete(ctx, ssh.ref); err != nil {
			return nil, err
		}
	}
	switch {
	case plan.putAfter:
		if err := s.secrets.Put(ctx, plan.ref, secretdom.KindMariaDBRoot, plan.password); err != nil {
			return nil, err
		}
	case plan.deleteAfter:
		if err := s.secrets.Delete(ctx, plan.ref); err != nil {
			return nil, err
		}
	}
	if plan.staleRef != "" {
		_ = s.secrets.Delete(ctx, plan.staleRef)
	}
	return s.repo.GetByID(ctx, inst.ID)
}

// credentialPlan is the post-update admin login plus the secret-store side
// effects needed to get there, ordered around the row update by Update.
type credentialPlan struct {
	creds       *instancedom.Credentials // nil = credentials untouched
	ref         string
	password    []byte
	putBefore   bool   // new secret: write before the row references it
	putAfter    bool   // rotation of an existing secret: overwrite after
	deleteAfter bool   // credentials removed: delete after
	staleRef    string // previous secret under a different ref, deleted after
}

// planCredentials works out the instance's post-update admin login without
// touching the secret store.
func (s *Service) planCredentials(inst *instancedom.Instance, in UpdateInput) (credentialPlan, error) {
	if in.Username == nil && in.Password == nil {
		return credentialPlan{}, nil
	}

	// Clearing the username drops the whole credential pair.
	if in.Username != nil && strings.TrimSpace(*in.Username) == "" {
		if in.Password != nil && *in.Password != "" {
			return credentialPlan{}, apperr.Invalid("username", "username is required when a password is provided")
		}
		p := credentialPlan{creds: &instancedom.Credentials{}}
		if inst.RootSecretRef != nil {
			p.ref, p.deleteAfter = *inst.RootSecretRef, true
		}
		return p, nil
	}

	username := inst.Username
	if in.Username != nil {
		u := strings.TrimSpace(*in.Username)
		username = &u
	}

	p := credentialPlan{}
	secretRef := inst.RootSecretRef
	switch {
	case in.Password == nil:
		// Username-only change; leave the stored password alone.
	case *in.Password == "":
		if inst.RootSecretRef != nil {
			p.ref, p.deleteAfter = *inst.RootSecretRef, true
		}
		secretRef = nil
	default:
		if username == nil || *username == "" {
			return credentialPlan{}, apperr.Invalid("username", "username is required when a password is provided")
		}
		ref := rootSecretRef(inst.ID)
		p.ref, p.password = ref, []byte(*in.Password)
		if inst.RootSecretRef != nil && *inst.RootSecretRef == ref {
			p.putAfter = true
		} else {
			p.putBefore = true
			if inst.RootSecretRef != nil {
				p.staleRef = *inst.RootSecretRef
			}
		}
		secretRef = &ref
	}

	p.creds = &instancedom.Credentials{Username: username, RootSecretRef: secretRef}
	return p, nil
}

// Delete soft-deletes an instance record. For provisioned instances it also
// enqueues a job for the agent to remove the Docker container (and optionally
// its data volume). For registered/external instances it only drops the record.
func (s *Service) Delete(ctx context.Context, id string, removeVolume bool, createdBy *uuid.UUID) error {
	inst, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if inst.Provisioned() {
		if _, err := s.ops.Create(ctx, jobdom.TypeRemoveInstance, "instance", &inst.ID, inst.ServerID,
			operationapp.Params{InstanceID: inst.ID.String(), RemoveVolume: removeVolume}, createdBy); err != nil {
			return err
		}
	}
	return s.repo.SoftDelete(ctx, inst.ID)
}

func genPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// List returns a filtered, paginated set of instances.
func (s *Service) List(ctx context.Context, p ListParams) (ListResult, error) {
	limit := clampLimit(p.Limit)
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}
	f := instancedom.ListFilter{Limit: limit, Offset: offset, Scope: p.Scope}
	if p.ServerID != "" {
		sid, err := uuid.Parse(p.ServerID)
		if err != nil {
			return ListResult{}, apperr.Invalid("server_id", "server_id must be a valid UUID")
		}
		f.ServerID = &sid
	}
	if p.Kind != "" {
		k := instancedom.Kind(p.Kind)
		f.Kind = &k
	}
	page, err := s.repo.List(ctx, f)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: page.Items, Total: page.Total, Limit: limit, Offset: offset}, nil
}

// TestConnectionResult reports connectivity for an instance.
type TestConnectionResult struct {
	Mode        string // sync | async
	OK          bool
	Version     string
	Error       string
	OperationID string
}

// TestConnection checks connectivity: synchronously for external instances,
// via an agent operation for managed ones.
func (s *Service) TestConnection(ctx context.Context, id string, createdBy *uuid.UUID) (*TestConnectionResult, error) {
	inst, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !inst.HasCredentials() {
		return nil, apperr.Invalid("id", "instance has no admin credentials configured")
	}

	if inst.Kind == instancedom.KindExternal {
		conn, err := s.connParams(ctx, inst)
		if err != nil {
			return nil, err
		}
		eng, err := engine.For(string(inst.Engine))
		if err != nil {
			return nil, apperr.Invalid("engine", err.Error())
		}
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		version, perr := eng.Ping(cctx, conn)
		res := &TestConnectionResult{Mode: "sync", OK: perr == nil, Version: version}
		if perr != nil {
			res.Error = apperr.EngineMessage(perr)
		}
		return res, nil
	}

	job, err := s.ops.Create(ctx, jobdom.TypeTestConnection, "instance", &inst.ID, inst.ServerID,
		operationapp.Params{InstanceID: inst.ID.String()}, createdBy)
	if err != nil {
		return nil, err
	}
	return &TestConnectionResult{Mode: "async", OperationID: job.ID.String()}, nil
}

// ImportResult reports the outcome of a database import.
type ImportResult struct {
	Mode        string // sync | async
	Imported    int
	OperationID string
}

// ImportDatabases discovers existing databases on the instance and registers
// them: synchronously for external instances, via an agent operation for
// managed ones.
func (s *Service) ImportDatabases(ctx context.Context, id string, createdBy *uuid.UUID) (*ImportResult, error) {
	inst, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !inst.HasCredentials() {
		return nil, apperr.Invalid("id", "instance has no admin credentials configured")
	}

	if inst.Kind == instancedom.KindExternal {
		conn, err := s.connParams(ctx, inst)
		if err != nil {
			return nil, err
		}
		eng, err := engine.For(string(inst.Engine))
		if err != nil {
			return nil, apperr.Invalid("engine", err.Error())
		}
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		dbs, err := eng.ListDatabases(cctx, conn)
		if err != nil {
			return nil, apperr.Invalid("id", "could not list databases: "+apperr.EngineMessage(err))
		}
		imported := 0
		for _, d := range dbs {
			db, derr := databasedom.NewDatabase(inst.ID, d.Name, d.Charset, d.Collation,
				map[string]string{"imported": "true"}, nil)
			if derr != nil {
				continue
			}
			db.System = d.System
			if cerr := s.databases.Create(ctx, db); cerr == nil {
				imported++
			}
		}
		return &ImportResult{Mode: "sync", Imported: imported}, nil
	}

	job, err := s.ops.Create(ctx, jobdom.TypeImportDatabases, "instance", &inst.ID, inst.ServerID,
		operationapp.Params{InstanceID: inst.ID.String()}, createdBy)
	if err != nil {
		return nil, err
	}
	return &ImportResult{Mode: "async", OperationID: job.ID.String()}, nil
}

func (s *Service) connParams(ctx context.Context, inst *instancedom.Instance) (engine.ConnParams, error) {
	host := "127.0.0.1"
	if inst.Host != nil {
		host = *inst.Host
	}
	conn := engine.ConnParams{Host: host, Port: inst.Port, TLSMode: inst.TLSModeOrDefault()}
	if inst.Username != nil {
		conn.User = *inst.Username
	}
	if inst.RootSecretRef != nil {
		pw, err := s.secrets.Get(ctx, *inst.RootSecretRef)
		if err != nil {
			return conn, err
		}
		conn.Password = string(pw)
	}
	tunnel, err := dbtarget.Tunnel(ctx, s.secrets, s.repo, inst, "ssh_tunnel")
	if err != nil {
		return conn, err
	}
	conn.SSH = tunnel
	return conn, nil
}

func clampLimit(l int) int {
	switch {
	case l <= 0:
		return defaultLimit
	case l > maxLimit:
		return maxLimit
	default:
		return l
	}
}
