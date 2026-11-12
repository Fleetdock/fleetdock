package httpapi

// auditRoute says how a request to one route is written to the audit log.
type auditRoute struct {
	action       string
	resourceType string
	// idParam is the path value naming the resource; empty when the route
	// creates it (the id then comes from the response when fromResponse).
	idParam      string
	fromResponse bool
}

// auditRoutes lists the audited routes by their exact mux pattern. Routes not
// listed (reads, previews, comparisons, agent traffic) are not logged.
// TestAuditRoutesExist keeps every key a registered route.
//
// Routes whose service writes its own, more detailed event (public access,
// database credentials, fleet runs) are listed too: the service's event then
// replaces the generic one, and a refused request is still logged as denied.
var auditRoutes = map[string]auditRoute{
	// Sessions and profile.
	"POST /v1/auth/login":       {action: "auth.login", resourceType: "user"},
	"POST /v1/auth/logout":      {action: "auth.logout", resourceType: "user"},
	"PATCH /v1/profile":         {action: "profile.update", resourceType: "user"},
	"POST /v1/profile/password": {action: "profile.password_change", resourceType: "user"},

	// Users, roles, grants, tokens.
	"POST /v1/users":                              {action: "user.create", resourceType: "user", fromResponse: true},
	"PATCH /v1/users/{id}":                        {action: "user.update", resourceType: "user", idParam: "id"},
	"POST /v1/users/{id}/password":                {action: "user.password_reset", resourceType: "user", idParam: "id"},
	"DELETE /v1/users/{id}":                       {action: "user.delete", resourceType: "user", idParam: "id"},
	"POST /v1/users/{id}/role-grants":             {action: "user.grant_add", resourceType: "user", idParam: "id"},
	"DELETE /v1/users/{id}/role-grants/{grantId}": {action: "user.grant_remove", resourceType: "user", idParam: "id"},
	"POST /v1/roles":                              {action: "role.create", resourceType: "role", fromResponse: true},
	"PATCH /v1/roles/{id}":                        {action: "role.update", resourceType: "role", idParam: "id"},
	"DELETE /v1/roles/{id}":                       {action: "role.delete", resourceType: "role", idParam: "id"},
	"POST /v1/tokens":                             {action: "api_token.create", resourceType: "api_token", fromResponse: true},
	"DELETE /v1/tokens/{id}":                      {action: "api_token.revoke", resourceType: "api_token", idParam: "id"},
	"DELETE /v1/api-tokens/{id}":                  {action: "api_token.revoke_any", resourceType: "api_token", idParam: "id"},
	"POST /v1/agent-tokens":                       {action: "agent_token.create", resourceType: "agent_token", fromResponse: true},
	"DELETE /v1/agent-tokens/{id}":                {action: "agent_token.revoke", resourceType: "agent_token", idParam: "id"},

	// Servers and database servers.
	"POST /v1/servers":                             {action: "server.register", resourceType: "server", fromResponse: true},
	"PATCH /v1/servers/{id}":                       {action: "server.update", resourceType: "server", idParam: "id"},
	"DELETE /v1/servers/{id}":                      {action: "server.delete", resourceType: "server", idParam: "id"},
	"DELETE /v1/servers/{id}/agent-token":          {action: "server.agent_token_revoke", resourceType: "server", idParam: "id"},
	"POST /v1/servers/{id}/agent-token/rotate":     {action: "server.agent_token_rotate", resourceType: "server", idParam: "id"},
	"POST /v1/instances":                           {action: "instance.connect", resourceType: "instance", fromResponse: true},
	"POST /v1/instances/provision":                 {action: "instance.provision", resourceType: "instance", fromResponse: true},
	"PATCH /v1/instances/{id}":                     {action: "instance.update", resourceType: "instance", idParam: "id"},
	"DELETE /v1/instances/{id}":                    {action: "instance.delete", resourceType: "instance", idParam: "id"},
	"POST /v1/instances/{id}/start":                {action: "instance.start", resourceType: "instance", idParam: "id"},
	"POST /v1/instances/{id}/stop":                 {action: "instance.stop", resourceType: "instance", idParam: "id"},
	"POST /v1/instances/{id}/restart":              {action: "instance.restart", resourceType: "instance", idParam: "id"},
	"POST /v1/instances/{id}/db-users":             {action: "db_user.create", resourceType: "instance", idParam: "id"},
	"POST /v1/instances/{id}/db-users/drop":        {action: "db_user.drop", resourceType: "instance", idParam: "id"},
	"POST /v1/instances/{id}/db-users/password":    {action: "db_user.password", resourceType: "instance", idParam: "id"},
	"POST /v1/instances/{id}/grants":               {action: "db_grant.add", resourceType: "instance", idParam: "id"},
	"POST /v1/instances/{id}/grants/revoke":        {action: "db_grant.revoke", resourceType: "instance", idParam: "id"},
	"POST /v1/instances/{id}/processes/{pid}/kill": {action: "instance.kill_process", resourceType: "instance", idParam: "id"},

	// Databases.
	"POST /v1/databases":                                        {action: "database.create", resourceType: "database", fromResponse: true},
	"POST /v1/databases/{id}/lock":                              {action: "database.lock", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/unlock":                            {action: "database.unlock", resourceType: "database", idParam: "id"},
	"DELETE /v1/databases/{id}":                                 {action: "database.delete", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/public-access":                     {action: "public_access.enable", resourceType: "database", idParam: "id"},
	"PATCH /v1/databases/{id}/public-access":                    {action: "public_access.update_cidrs", resourceType: "database", idParam: "id"},
	"DELETE /v1/databases/{id}/public-access":                   {action: "public_access.disable", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/credentials":                       {action: "credential.create", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/credentials/{credentialId}/rotate": {action: "credential.rotate", resourceType: "database", idParam: "id"},
	"DELETE /v1/databases/{id}/credentials/{credentialId}":      {action: "credential.revoke", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/grants":                            {action: "db_grant.add", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/grants/revoke":                     {action: "db_grant.revoke", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/table-views":                       {action: "table_view.create", resourceType: "database", idParam: "id"},
	"PATCH /v1/databases/{id}/table-views/{viewId}":             {action: "table_view.update", resourceType: "database", idParam: "id"},
	"DELETE /v1/databases/{id}/table-views/{viewId}":            {action: "table_view.delete", resourceType: "database", idParam: "id"},
	"PUT /v1/databases/{id}/schema/watch":                       {action: "schema.watch", resourceType: "database", idParam: "id"},
	"POST /v1/moves":                                            {action: "database.move", resourceType: "operation", fromResponse: true},

	// Data and structure.
	"POST /v1/databases/{id}/query":                                {action: "sql.execute", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/tables/{table}/rows":                  {action: "data.row_insert", resourceType: "database", idParam: "id"},
	"PATCH /v1/databases/{id}/tables/{table}/rows":                 {action: "data.row_update", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/tables/{table}/rows/delete":           {action: "data.row_delete", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/tables/{table}/import":                {action: "data.import_csv", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/import-sql":                           {action: "data.import_sql", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/changes/undo":                         {action: "data.undo", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/changes/{changeId}/undo":              {action: "data.undo", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/backup-copies/{copyId}/restore-rows":  {action: "data.restore_rows", resourceType: "database", idParam: "id"},
	"GET /v1/databases/{id}/tables/{table}/export":                 {action: "data.export", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/tables/{table}/export":                {action: "data.export", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/export":                               {action: "data.export", resourceType: "database", idParam: "id"},
	"GET /v1/databases/{id}/dump":                                  {action: "data.dump", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/tables":                               {action: "table.create", resourceType: "database", idParam: "id"},
	"PATCH /v1/databases/{id}/tables/{table}":                      {action: "table.alter", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/tables/{table}/drop":                  {action: "table.drop", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/tables/{table}/truncate":              {action: "table.truncate", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/tables/{table}/rename":                {action: "table.rename", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/tables/{table}/indexes":               {action: "index.create", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/tables/{table}/indexes/drop":          {action: "index.drop", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/schemas":                              {action: "schema.create", resourceType: "database", idParam: "id"},
	"PATCH /v1/databases/{id}/schemas/{schema}":                    {action: "schema.alter", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/schemas/{schema}/drop":                {action: "schema.drop", resourceType: "database", idParam: "id"},
	"PUT /v1/databases/{id}/schemas/{schema}/privileges":           {action: "schema.grant", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/schemas/{schema}/default-privileges":  {action: "schema.default_privileges", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/sequences/{schema}/{sequence}/setval": {action: "sequence.setval", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/matviews/{schema}/{view}/refresh":     {action: "matview.refresh", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/maintenance":                          {action: "table.maintain", resourceType: "database", idParam: "id"},
	"PUT /v1/databases/{id}/search-path":                           {action: "database.search_path", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/schemas/{schema}/copy":                {action: "schema.copy", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/objects/move":                         {action: "object.move", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/backup-copies":                        {action: "backup_copy.create", resourceType: "database", idParam: "id"},
	"POST /v1/databases/{id}/backup-copies/{copyId}/extend":        {action: "backup_copy.extend", resourceType: "database", idParam: "id"},
	"DELETE /v1/databases/{id}/backup-copies/{copyId}":             {action: "backup_copy.discard", resourceType: "database", idParam: "id"},
	"POST /v1/fleet-runs":                                          {action: "fleet_run.create", resourceType: "fleet_run", fromResponse: true},
	"POST /v1/fleet-runs/{id}/resume":                              {action: "fleet_run.resume", resourceType: "fleet_run", idParam: "id"},
	"POST /v1/fleet-runs/{id}/cancel":                              {action: "fleet_run.cancel", resourceType: "fleet_run", idParam: "id"},
	"POST /v1/fleet-runs/{id}/retry":                               {action: "fleet_run.retry", resourceType: "fleet_run", idParam: "id"},
	"GET /v1/fleet-runs/{id}/results.csv":                          {action: "fleet_run.export", resourceType: "fleet_run", idParam: "id"},

	// Backups.
	"POST /v1/backups":                    {action: "backup.create", resourceType: "backup", fromResponse: true},
	"POST /v1/backups/{id}/restore":       {action: "backup.restore", resourceType: "backup", idParam: "id"},
	"POST /v1/backups/{id}/verify":        {action: "backup.verify", resourceType: "backup", idParam: "id"},
	"DELETE /v1/backups/{id}":             {action: "backup.delete", resourceType: "backup", idParam: "id"},
	"GET /v1/backups/{id}/download":       {action: "backup.download", resourceType: "backup", idParam: "id"},
	"POST /v1/backup-destinations":        {action: "backup_destination.create", resourceType: "backup_destination", fromResponse: true},
	"PATCH /v1/backup-destinations/{id}":  {action: "backup_destination.update", resourceType: "backup_destination", idParam: "id"},
	"DELETE /v1/backup-destinations/{id}": {action: "backup_destination.delete", resourceType: "backup_destination", idParam: "id"},
	"POST /v1/backup-schedules":           {action: "backup_schedule.create", resourceType: "backup_schedule", fromResponse: true},
	"PATCH /v1/backup-schedules/{id}":     {action: "backup_schedule.update", resourceType: "backup_schedule", idParam: "id"},
	"DELETE /v1/backup-schedules/{id}":    {action: "backup_schedule.delete", resourceType: "backup_schedule", idParam: "id"},

	// Notifications.
	"POST /v1/notification-channels":        {action: "notification_channel.create", resourceType: "notification_channel", fromResponse: true},
	"PATCH /v1/notification-channels/{id}":  {action: "notification_channel.update", resourceType: "notification_channel", idParam: "id"},
	"DELETE /v1/notification-channels/{id}": {action: "notification_channel.delete", resourceType: "notification_channel", idParam: "id"},
	"POST /v1/alert-rules":                  {action: "alert_rule.create", resourceType: "alert_rule", fromResponse: true},
	"PATCH /v1/alert-rules/{id}":            {action: "alert_rule.update", resourceType: "alert_rule", idParam: "id"},
	"DELETE /v1/alert-rules/{id}":           {action: "alert_rule.delete", resourceType: "alert_rule", idParam: "id"},
}
